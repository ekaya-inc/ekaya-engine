# PLAN: Text2SQL - Overview

> **Navigation:** [Overview](PLAN-text2sql-overview.md) | [Vector Infrastructure](PLAN-text2sql-vector.md) | [Service](PLAN-text2sql-service.md) | [Enhancements](PLAN-text2sql-enhancements.md) | [Security](PLAN-text2sql-security.md) | [Ontology Linking](PLAN-text2sql-ontology-linking.md) | [Memory System](PLAN-text2sql-memory.md) | [Implementation](PLAN-text2sql-implementation.md)

## Justification

**Audience:** This document is written for Claude Code (you, in a future session) to understand why we're implementing text2SQL this way and how to execute it correctly.

**Problem:** ekaya-engine currently has NO text2SQL functionality. Users cannot ask natural language questions about their data. Generating SQL from natural language is challenging because:

1. **Schema size explosion** - Enterprise databases have hundreds of tables. Sending all schema metadata to LLMs hits token limits and degrades accuracy
2. **Syntax variation** - Different databases (PostgreSQL, ClickHouse, MSSQL) have incompatible SQL dialects
3. **Learning curve** - LLMs improve with examples, but we don't know which examples are relevant to a given question
4. **No query history** - We can't learn from successful past queries to improve future generations

**Solution:** Implement a two-level vector search system:

**Level 1: Schema Filtering (Relevance)**
- User asks: "What were our top selling products last month?"
- Vector search finds the 5 most relevant tables (e.g., `orders`, `order_items`, `products`, `timestamps`)
- LLM only sees 5 tables instead of 500 tables → stays within token limits, better accuracy

**Level 2: Few-Shot Examples (Learning)**
- Vector search finds 3 similar past successful queries
- These examples teach the LLM the specific patterns this database uses
- Example: If past queries used `DATE_TRUNC('month', created_at)` for monthly grouping, LLM learns that pattern

**Enhanced Pattern Matching (from claudontology proof-of-concept):**
- Multi-dimensional similarity scoring (not just embedding cosine distance)
- Confidence learning with success/failure tracking
- Pattern confidence evolution based on execution results
- Decision thresholds: High confidence (>0.85) = use cached SQL directly, bypassing LLM generation

**Why this matters:**
- **Token efficiency:** 5 tables @ ~200 tokens each = 1000 tokens vs. 500 tables = 100k+ tokens
- **Accuracy:** LLM sees only relevant schema → less confusion, better joins
- **Learning:** LLM learns from actual working queries in this database → correct syntax, idioms
- **Adaptability:** System improves over time as query history grows
- **Performance:** Cached high-confidence patterns skip expensive LLM calls (from claudontology)

**Reference architecture:** ekaya-region implements this exact pattern in its text2sql system. We're adapting it for ekaya-engine's multi-tenant architecture with PostgreSQL customer databases.

## Current State Analysis

### What EXISTS
- **LLM client** (`pkg/llm/client.go`) - OpenAI-compatible chat completions (used for ontology chat)
- **Database adapters** (`pkg/adapters/datasource/postgres/`) - PostgreSQL connection handling via pgxpool
- **Schema discovery** (`pkg/services/schema.go`) - Already discovers tables, columns, foreign keys
- **Query execution** (`pkg/services/query.go`) - Can execute SQL and return results
- **Syntax knowledge** - We have `knowledge/postgres.md`, `knowledge/mssql_syntax.md`, `knowledge/clickhouse_syntax.md` from ekaya-region

### What DOES NOT EXIST
- ❌ **Vector embeddings infrastructure** - No `vector_embeddings` table, no pgvector extension configuration
- ❌ **Embedding generation** - No Qwen3-Embedding-0.6B client
- ❌ **Schema indexing** - No mechanism to embed table schemas for vector search
- ❌ **Query history storage** - No vector storage of successful queries for few-shot learning
- ❌ **Text2SQL service** - No component that orchestrates the entire flow
- ❌ **Prompt builder** - No logic to construct the 7-section LLM prompt with filtered schema + examples

### Key Dependencies
- **Embedding API:** Qwen3-Embedding-0.6B at `http://sparktwo:30001/v1` (1024-dimensional vectors)
- **Redis:** Already available for embedding caching (`pkg/database/redis.go`)
- **PostgreSQL:** ekaya_region database for metadata, needs pgvector extension
- **Connection Manager:** From PLAN-connection-manager.md - will provide pooled connections to customer datasources

## Architecture Design

### Component Diagram

```
User Question
    ↓
┌─────────────────────────────────────────────────────────────────┐
│ Text2SQLService (pkg/services/text2sql.go)                      │
│                                                                   │
│  1. Validate question (not empty, not malicious)                │
│  2. Schema linking (vector search → top 5 tables)               │
│  3. Few-shot retrieval (vector search → top 3 similar queries)  │
│  4. Build 7-section prompt                                       │
│  5. LLM generation                                               │
│  6. SQL extraction                                               │
│  7. Async storage (question+SQL for future few-shot)            │
│     ↓                                                             │
└─────┼─────────────────────────────────────────────────────────────┘
      ↓
   Generated SQL
      ↓
Execute via QueryService (existing)
      ↓
   Results
      ↓
Store (question, SQL) async → Vector Store
```

## Integration with Connection Manager

**From PLAN-connection-manager.md:** Connection manager pools customer datasource connections by `(projectID, userID, datasourceID)` tuple.

**Text2SQL integration points:**

1. **Schema indexing** - When indexing schema, use connection manager to get pooled connection to customer database
2. **Metadata queries** - schemaRepo queries use connection manager (already implemented in connection manager plan)
3. **SQL execution** - Generated SQL executes via existing QueryService, which will use connection manager

**No direct text2sql→connection_manager dependency.** Text2SQL generates SQL, QueryService executes it using connection manager.

## Key Design Decisions

### Why Two Vector Collections?
**"schema" collection:** Stores table metadata for schema linking
**"query_history" collection:** Stores (question, SQL) pairs for few-shot learning

**Rationale:** Different search purposes require different content. Schema search needs "find tables about X" while few-shot search needs "find similar questions".

### Why Similarity Threshold = 0.7 for Few-Shot?
High threshold ensures only high-quality examples are used. Bad examples hurt more than no examples.

**Empirical finding:** Score >= 0.7 means semantically similar questions. Score < 0.7 is often irrelevant.

### Why Top 5 Tables Instead of Top 10?
Token budget and accuracy tradeoff. 5 tables fit comfortably in LLM context with examples and syntax knowledge. 10 tables risk exceeding token limits and confusing the LLM.

**Adaptive strategy (future):** Could dynamically adjust based on question complexity.

### Why Async Storage of Query History?
Non-blocking - don't make user wait for embedding generation and storage. They just want their SQL result.

**Trade-off:** Eventual consistency. A query won't immediately become available as few-shot example. Acceptable for this use case.

### Why Embed Question Instead of SQL for Few-Shot?
We want to find similar *questions*, not similar SQL. User asks "top selling products" → we want examples of similar questions, regardless of SQL structure.

**Alternative considered:** Embed both question and SQL, search both. Rejected as overly complex for marginal benefit.

### Why Cosine Distance Instead of Euclidean or Dot Product?
Cosine distance is magnitude-invariant - measures direction, not length. Perfect for semantic similarity where "top products last month" and "best-selling items in April" should be close despite different word counts.

**Technical:** pgvector's `<=>` operator computes cosine distance. Score = 1 - distance for intuitive "higher is better" semantics.

## Success Criteria

**Core Text2SQL (Original Plan):**
- [ ] Vector embeddings table created with pgvector extension
- [ ] Qwen embedding client generates 1024-dim vectors
- [ ] Redis caching reduces duplicate embedding API calls
- [ ] Schema indexing embeds all tables for a datasource
- [ ] Vector search finds relevant tables for natural language questions
- [ ] Query history storage enables few-shot learning
- [ ] Text2SQL generates syntactically correct SQL for PostgreSQL
- [ ] Generated SQL executes without errors on customer databases
- [ ] Confidence scores accurately reflect query quality
- [ ] System learns from successful queries (few-shot improves over time)
- [ ] Multi-tenant isolation (projects can't see each other's vectors)
- [ ] Integration tests pass end-to-end
- [ ] Knowledge files provide correct syntax guidance for PostgreSQL

**Enhanced Pattern Matching (from claudontology):**
- [ ] Multi-dimensional similarity scoring (keyword, entity, operation, structure, parameter) is implemented
- [ ] Pattern confidence increases after successful query execution
- [ ] Pattern confidence decays for unused patterns (daily job runs)
- [ ] Low-confidence old patterns are automatically removed
- [ ] High-confidence patterns (≥0.85 with ≥0.9 similarity) skip LLM generation and return cached SQL
- [ ] Pattern success/failure tracking influences future pattern selection

**Ambiguity Detection (from claudontology):**
- [ ] Ambiguous terms (temporal, quantitative, state) are detected before SQL generation
- [ ] Clarification requests are returned when confidence <0.7
- [ ] System proceeds with stated assumptions when 0.7 ≤ confidence <0.9
- [ ] Resolution hierarchy (session context → project knowledge → ontology → ask user) is implemented
- [ ] Users receive clear clarification questions with suggested options

**Session Context (from claudontology):**
- [ ] Session context is read from X-Session-Context header
- [ ] Session context filters (time focus, user segment, etc.) are injected into prompts
- [ ] Context applies to multiple queries without repetition
- [ ] Generated SQL respects session-level filters

**Ontology Integration (from claudontology):**
- [ ] Schema indexing uses ontology business names instead of raw table names
- [ ] Domain summary is included in LLM prompts
- [ ] Column semantic types (measure, dimension, identifier) guide SQL generation
- [ ] Table descriptions include domain classification and business descriptions
- [ ] Ontology column descriptions are used for ambiguous term resolution

## Future Enhancements (Out of Scope for Initial Implementation)

- **Multi-database support** - Extend to MSSQL, ClickHouse (knowledge files already exist)
- **Query optimization** - Suggest indexes, rewrite inefficient queries
- **Iterative refinement** - If query fails, use error message to regenerate
- **Semantic caching** - Cache SQL for semantically identical questions
- **User feedback loop** - Let users mark good/bad SQL, use as training signal
- **Adaptive schema selection** - Dynamically adjust top-K based on question complexity
- **Cross-datasource queries** - Generate SQL joining multiple datasources (requires federation layer)
- **SQL alternatives generation** - Generate 2-3 different SQL approaches for complex questions
- **Semantic syntax retrieval** - Index syntax patterns as vectors, retrieve relevant patterns per question (from ekaya-query)

## Glossary

**Vector embedding:** Numerical representation of text as 1024-dimensional array. Semantically similar texts have similar embeddings.

**Cosine distance:** Measure of similarity between vectors. Range [0, 2], where 0 = identical, 2 = opposite.

**Cosine similarity:** 1 - cosine_distance. Range [0, 1], where 1 = identical, 0 = orthogonal.

**Schema linking:** Finding relevant database tables for a natural language question using vector search.

**Few-shot learning:** Providing examples to LLM to guide output format and patterns.

**pgvector:** PostgreSQL extension enabling vector similarity search with specialized indexes (ivfflat, hnsw).

**ivfflat:** Inverted file with flat compression - approximate nearest neighbor index. Fast for medium-scale datasets.

**UPSERT:** INSERT or UPDATE - creates new row if not exists, updates if exists. `ON CONFLICT DO UPDATE` in PostgreSQL.

**RLS (Row Level Security):** PostgreSQL feature that filters rows based on user/tenant context. Ensures multi-tenant isolation.

**Chunk:** Unit of text stored in vector database. For text2sql: one chunk per table (schema) or one chunk per query (history).

**Collection:** Logical grouping of chunks. For text2sql: "schema" collection and "query_history" collection.

**Token budget:** Maximum number of tokens (words/subwords) an LLM can process in one request. Exceeding budget causes errors or truncation.

**Temperature:** LLM sampling parameter. 0 = deterministic (always same output), 1 = creative/random. Text2SQL uses 0 for consistency.

**Semantic mismatch:** Security signal where the complexity of generated SQL doesn't match the expected complexity for the user's question. Example: "count of users" producing SQL with 5 JOINs and window functions.

**Complexity score:** Numerical measure of SQL query complexity based on structural patterns (JOINs, subqueries, window functions, nesting depth). Used as a security feature.

**Tiered defense:** Security architecture with multiple layers of increasing cost and sophistication. Fast deterministic rules catch most attacks; expensive LLM verification handles edge cases.

**XGBoost:** Gradient boosted decision tree algorithm. Fast inference, handles feature interactions well, works with small training data. Used for ML classifier layer.

**SQL injection (SQLi):** Attack technique where malicious SQL is injected through user input. Patterns include UNION-based, stacked queries, comment injection, and encoding obfuscation.

**Feature vector:** Numerical representation of a SQL query's characteristics for ML classification. Includes structural features (JOIN count, nesting depth), token features, and anomaly flags.
