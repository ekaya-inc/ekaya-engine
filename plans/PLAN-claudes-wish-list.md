# PLAN: Claude's MCP Tool Wishlist

**Author:** Claude (as MCP Client)
**Date:** 2025-01-05 (Updated: 2025-01-08)
**Status:** Design Proposal

---

## Executive Summary

This document captures my wishlist as an MCP client after hands-on experience with the ekaya-engine MCP server. The core thesis: **the current tools work, but they're designed for humans managing a system rather than AI agents collaborating with it.**

**Key insight from testing:** *"The AI Data Liaison isn't just answering questions—it's building a curated library of business queries tailored to your domain."*

Four major capabilities would transform this from a query tool into a learning system:

1. **Unified Context Tool** - Progressive disclosure with graceful degradation
2. **Ontology Update Tools** - Let me enhance what I learn (`update_*` with upsert semantics)
3. **Ontology Questions Workflow** - Let me answer questions the schema can't
4. **Query Suggestion** - Let me propose reusable queries for approval

---

## Part 1: Unified Context Tool (`get_context`)

### Problem Statement

Currently I call multiple tools to understand a database:
- `get_ontology(depth=domain)` → business context
- `get_schema` → table/column details
- `list_glossary` → business terms

This creates three problems:
1. **Token waste** - Entity descriptions appear in both ontology and schema
2. **Cognitive load** - I must decide which tool to call
3. **Inconsistent experience** - If ontology doesn't exist, I get an error instead of graceful fallback

### Proposed Solution

A single `get_context` tool with progressive depth levels that gracefully degrades when ontology is unavailable.

```
get_context(depth, tables?, include_relationships?)
```

**Depth Levels:**

| Depth | With Ontology | Without Ontology | Tokens |
|-------|---------------|------------------|--------|
| `domain` | Business summary, conventions, domains | "No ontology. This is a PostgreSQL database with N tables." | ~500 |
| `entities` | Entity list with descriptions | Table list with row counts | ~2k |
| `tables` | Table summaries + key columns | Full schema for specified tables | ~4k |
| `columns` | Full column details with semantics | Raw column metadata | ~8k |

**Key Design Principles:**

1. **Always returns something useful** - Never fails, just provides less semantic richness
2. **Schema is always available** - Even without ontology extraction
3. **Ontology enhances, doesn't gate** - Business context is additive
4. **Single call for common cases** - `depth=entities` covers 80% of my needs

### Response Structure

```json
{
  "has_ontology": true,
  "ontology_status": "complete",  // "complete" | "extracting" | "none"
  "depth": "entities",

  "domain": {
    "description": "A peer-to-peer live engagement platform...",
    "primary_domains": ["billing", "engagement", "marketing"],
    "conventions": {
      "soft_delete": { "enabled": true, "column": "deleted_at" },
      "currency": { "format": "cents", "default": "USD" }
    }
  },

  "entities": [
    {
      "name": "User",
      "table": "users",
      "description": "A user profile with availability status...",
      "row_count": 95,
      "key_columns": ["user_id", "username", "is_available"]
    }
  ],

  "relationships": [
    {
      "from": "Account",
      "to": "User",
      "label": "owns",
      "columns": "accounts.default_user_id -> users.user_id"
    }
  ],

  "glossary": [
    {
      "term": "GMV",
      "definition": "Gross Merchandise Value",
      "sql_pattern": "SUM(total_amount) / 100.0"
    }
  ]
}
```

### Implementation Notes

**File:** `pkg/mcp/tools/context.go` (new file)

**Logic:**
```go
func handleGetContext(ctx, depth, tables, includeRelationships) {
    // 1. Always get schema (never fails)
    schema := schemaService.GetSchema(ctx, projectID)

    // 2. Try to get ontology (may be nil)
    ontology := ontologyRepo.GetActive(ctx, projectID)

    // 3. Build response based on depth + ontology availability
    if ontology != nil {
        return buildEnrichedResponse(schema, ontology, depth)
    }
    return buildSchemaOnlyResponse(schema, depth)
}
```

**Migration from existing tools:**
- `get_schema` → `get_context(depth=columns)` with `include_entities=false` equivalent
- `get_ontology` → `get_context(depth=domain|entities)`
- Keep existing tools for backwards compatibility, mark as deprecated

---

## Part 2: Ontology Update Tools

### Design Principles

1. **`update_*` naming with upsert semantics** - No cognitive overhead deciding "is this new or existing?"
2. **Separate `delete_*` tools** - Deletion is rare and destructive; it deserves its own verb
3. **Key identifier is the upsert key** - Entity name, term name, etc.

### 2.1 Entity Management

#### `update_entity`

Create or update entity metadata.

```
update_entity(
  name: string,              // Required - entity name (upsert key)
  description?: string,      // Entity description
  aliases?: string[],        // Alternative names
  key_columns?: string[]     // Important business columns
)
```

**Examples:**

```json
// Add description to existing entity
{
  "name": "User",
  "description": "Platform user who can act as a host (content creator) or visitor (consumer)"
}

// Add aliases discovered during queries
{
  "name": "User",
  "aliases": ["creator", "host", "visitor"]
}
```

**Storage:** `engine_ontology_entities`, `engine_ontology_entity_aliases`, `engine_ontology_entity_key_columns`

#### `delete_entity`

Remove an entity that doesn't belong.

```
delete_entity(name: string)
```

### 2.2 Relationship Management

#### `update_relationship`

Create or update relationship between entities.

```
update_relationship(
  from_entity: string,       // Required - source entity
  to_entity: string,         // Required - target entity (together form upsert key)
  description?: string,      // What the relationship means
  label?: string             // Short label (e.g., "owns", "follows")
)
```

**Example:**

```json
{
  "from_entity": "Billing Engagement",
  "to_entity": "User",
  "label": "charges",
  "description": "The visitor (payer) who initiated the engagement and will be charged"
}
```

**Storage:** `engine_entity_relationships`

#### `delete_relationship`

Remove a relationship that doesn't exist.

```
delete_relationship(
  from_entity: string,
  to_entity: string
)
```

### 2.3 Glossary Management

#### `update_glossary_term`

Create or update a business term definition.

```
update_glossary_term(
  term: string,              // Required - term name (upsert key)
  definition?: string,       // What it means
  sql?: string,              // SQL pattern to calculate it
  aliases?: string[]         // Alternative names (AOV, Average Order Value, etc.)
)
```

**Example:**

```json
{
  "term": "Platform Take Rate",
  "definition": "Percentage of transaction value retained by the platform",
  "sql": "SUM(tikr_share) / NULLIF(SUM(total_amount), 0) * 100",
  "aliases": ["Take Rate", "Platform Commission Rate"]
}
```

**Storage:** `engine_glossary_terms` (or existing glossary table)

#### `delete_glossary_term`

Remove a term that's no longer relevant.

```
delete_glossary_term(term: string)
```

### 2.4 Project Knowledge Management

#### `update_project_knowledge`

Create or update domain facts that persist across sessions.

```
update_project_knowledge(
  fact: string,              // Required - the knowledge (upsert key or use fact_id)
  fact_id?: string,          // Optional - for updating existing fact
  context?: string,          // How this was discovered
  category?: string          // "terminology" | "business_rule" | "enumeration" | "convention"
)
```

**Examples:**

```json
// Business rule
{
  "fact": "Platform fees are approximately 33% of total_amount, stored in tikr_share column",
  "context": "Verified: tikr_share / total_amount ≈ 0.33 across all transactions",
  "category": "business_rule"
}

// Enumeration
{
  "fact": "users.status values: ACTIVE (normal), SUSPENDED (temporary hold), BANNED (permanent)",
  "context": "Found in user.go lines 45-67",
  "category": "enumeration"
}

// Terminology
{
  "fact": "A 'tik' is a billing unit representing approximately 6 seconds of engagement time",
  "context": "Inferred from tiks * 6 ≈ total_duration_s in billing_engagements",
  "category": "terminology"
}
```

**Storage:** `engine_project_knowledge` (already exists)

#### `delete_project_knowledge`

Remove knowledge that's incorrect or outdated.

```
delete_project_knowledge(fact_id: string)
```

### 2.5 Column Metadata Management

#### `update_column`

Add or update semantic information about a column.

```
update_column(
  table: string,             // Required - table name
  column: string,            // Required - column name (together form upsert key)
  description?: string,      // What the column means
  enum_values?: string[]     // Known enumeration values with descriptions
)
```

**Example:**

```json
{
  "table": "billing_transactions",
  "column": "transaction_state",
  "description": "Current state of the billing transaction lifecycle",
  "enum_values": [
    "TRANSACTION_STATE_PENDING - Payment initiated, awaiting processing",
    "TRANSACTION_STATE_WAITING - Payment captured, in hold period",
    "TRANSACTION_STATE_ENDED - Session ended, no charges applied",
    "TRANSACTION_STATE_FAILED - Payment failed"
  ]
}
```

#### `delete_column_metadata`

Clear custom metadata for a column (revert to schema-only).

```
delete_column_metadata(
  table: string,
  column: string
)
```

### Tool Summary

| Tool | Purpose | Upsert Key |
|------|---------|------------|
| `update_entity` | Entity descriptions, aliases, key columns | `name` |
| `delete_entity` | Remove entity | `name` |
| `update_relationship` | Relationship descriptions, labels | `from_entity` + `to_entity` |
| `delete_relationship` | Remove relationship | `from_entity` + `to_entity` |
| `update_glossary_term` | Business term definitions, SQL | `term` |
| `delete_glossary_term` | Remove term | `term` |
| `update_project_knowledge` | Domain facts, rules, conventions | `fact_id` or content match |
| `delete_project_knowledge` | Remove knowledge | `fact_id` |
| `update_column` | Column descriptions, enum values | `table` + `column` |
| `delete_column_metadata` | Clear column metadata | `table` + `column` |

---

## Part 3: Ontology Questions Workflow

### Problem Statement

When the server builds the ontology, it generates questions it cannot answer from the schema or data alone:
- "What does this enumeration in this column mean?"
- "What is the business relationship between these entities?"
- "What does this cryptic column name refer to?"

These questions might be answerable by me (Claude Code) by reviewing source code, documentation, or other assets the user has access to.

### Question States

| State | Meaning | Who Moves It Here |
|-------|---------|-------------------|
| `pending` | Not yet attempted | Initial state |
| `resolved` | Answered, ontology updated | Me, after research |
| `skipped` | Can't answer now, revisit later | Me |
| `escalated` | Needs human domain knowledge | Me |
| `dismissed` | Unanswerable or not worth pursuing | Me or Admin |

### 3.1 List Questions with Filtering

```
list_ontology_questions(
  status?: string,           // Filter: "pending" | "resolved" | "skipped" | "escalated" | "dismissed"
  category?: string,         // Filter: "enumeration" | "relationship" | "business_logic" | "naming" | "calculation"
  entity?: string,           // Filter: related entity name
  priority?: string,         // Filter: "high" | "medium" | "low"
  limit?: number,            // Pagination (default 20)
  offset?: number            // Pagination offset
)
```

**Response:**

```json
{
  "questions": [
    {
      "id": "uuid",
      "question": "What do the status values 'ACTIVE', 'SUSPENDED', 'BANNED' mean in users.status?",
      "category": "enumeration",
      "priority": "high",
      "context": {
        "entity": "User",
        "table": "users",
        "column": "status",
        "observed_values": ["ACTIVE", "SUSPENDED", "BANNED"]
      },
      "created_at": "2025-01-05T10:00:00Z"
    }
  ],
  "total_count": 147,
  "counts_by_status": {
    "pending": 89,
    "resolved": 42,
    "skipped": 8,
    "escalated": 5,
    "dismissed": 3
  }
}
```

### Question Categories

| Category | Example Question | Where I'd Look |
|----------|------------------|----------------|
| `enumeration` | "What does status='PENDING' mean?" | Model definitions, constants |
| `relationship` | "How are Channel and Account related?" | Foreign keys, model associations |
| `business_logic` | "When is a transaction 'available'?" | Service layer, business rules |
| `naming` | "What does 'tiks' mean?" | Domain glossary, product docs |
| `calculation` | "How is earned_amount computed?" | Transaction processing code |

### 3.2 Resolve Question

After researching and updating the ontology with what I learned.

```
resolve_ontology_question(
  question_id: string,       // Required
  resolution_notes?: string  // How I found the answer (e.g., "Found in user.go:45-67")
)
```

### 3.3 Triage Questions

When I can't answer a question right now or ever.

```
skip_ontology_question(
  question_id: string,
  reason: string             // Why I'm skipping (e.g., "Need access to frontend repo")
)

escalate_ontology_question(
  question_id: string,
  reason: string             // Why human needed (e.g., "Business rule not documented in code")
)

dismiss_ontology_question(
  question_id: string,
  reason: string             // Why not worth answering (e.g., "Column appears unused, legacy")
)
```

### My Workflow for Hundreds of Questions

```python
# 1. Get high-priority pending questions, 20 at a time
questions = list_ontology_questions(status="pending", priority="high", limit=20)

# 2. For each question:
for q in questions:
    # Research using my existing tools (Grep, Read, etc.)
    results = grep(q.context.column, path="tikr-backend/")
    code = read("tikr-backend/models/user.go")

    if found_answer:
        # Update ontology with what I learned (may call multiple update tools)
        update_column("users", "status", description="...", enum_values=[...])
        update_project_knowledge(fact="Users are soft-banned via SUSPENDED status...")
        resolve_ontology_question(q.id, notes="Found in user.go:45-67")

    elif needs_human:
        escalate_ontology_question(q.id, reason="Business rule, not in code")

    elif might_find_later:
        skip_ontology_question(q.id, reason="Need access to frontend repo")

    else:
        dismiss_ontology_question(q.id, reason="Column appears unused")

# 3. Repeat until done or context exhausted
```

### Why Separate Update + Resolve (Not One Mega-Tool)

Researching one question may teach me **multiple things**:
- The enum values → `update_column`
- A business rule → `update_project_knowledge`
- A relationship I didn't know about → `update_relationship`
- A glossary term → `update_glossary_term`

Keeping them separate lets me record everything I learn, not just the narrow answer to the question.

---

## Part 4: Query Suggestion Tool

### Problem Statement

When I write a useful query, it's ephemeral. The next session starts from scratch. The admin must manually recreate it as an approved query.

### Understanding Query vs Execute Approved Query

| Tool | Purpose | When to Use |
|------|---------|-------------|
| `query` | Ad-hoc SQL execution | Exploration, one-off questions, testing ideas |
| `execute_approved_query` | Run vetted, parameterized query | Repeatable business questions |

**The value loop:**
1. I explore with `query` to understand the data
2. I discover a useful pattern
3. I call `suggest_approved_query` to codify it
4. Human approves (or auto-approve if enabled)
5. Future sessions use `execute_approved_query`
6. **The system gets smarter over time**

### 4.1 Suggest Approved Query

```
suggest_approved_query(
  name: string,              // Required - human-readable name
  description: string,       // Required - what business question it answers
  sql: string,               // Required - SQL with {{parameter}} placeholders
  parameters?: [             // Optional - inferred from SQL if omitted
    {
      name: string,          // Matches {{placeholder}} in SQL
      type: string,          // "string" | "number" | "date" | "boolean"
      description?: string,  // User-friendly explanation
      required?: boolean,    // Default true
      example: any           // Required for dry-run validation (unless has default)
    }
  ],
  output_column_descriptions?: {  // Optional - types auto-detected, descriptions added
    "column_name": "description"
  }
)
```

**Example:**

```json
{
  "name": "Host Revenue by Month",
  "description": "Total earnings for a specific host broken down by month",
  "sql": "SELECT date_trunc('month', created_at) as month, SUM(earned_amount) / 100.0 as total_earned_usd, COUNT(*) as transaction_count FROM billing_transactions WHERE payee_username = {{host_username}} AND transaction_state = 'TRANSACTION_STATE_WAITING' AND deleted_at IS NULL GROUP BY 1 ORDER BY 1 DESC",
  "parameters": [
    {
      "name": "host_username",
      "type": "string",
      "description": "Host's username",
      "required": true,
      "example": "damon"
    }
  ],
  "output_column_descriptions": {
    "total_earned_usd": "Total earnings in USD (converted from cents)",
    "transaction_count": "Number of completed transactions"
  }
}
```

### Validation Workflow

With the `example` values provided, the system can:

1. **Validate SQL** - Substitute examples and run `EXPLAIN`
2. **Detect output columns** - Run the actual query (with `LIMIT 1`) to get column names/types
3. **Sanity check** - Confirm it returns data (not a guaranteed failure)
4. **Show preview** - Display sample results to the human reviewer

### Approval Workflow

```
Claude calls suggest_approved_query()
        ↓
System validates SQL with example parameters
        ↓
System auto-detects output column types
        ↓
Suggestion stored (status='pending')
        ↓
Admin notified immediately
        ↓
[If auto-approve enabled] → Query immediately available
[If manual review] → Admin reviews in UI, approves/rejects
        ↓
Query appears in list_approved_queries
```

### Return Value

```json
{
  "suggestion_id": "uuid",
  "status": "pending" | "approved",  // approved if auto-approve enabled
  "validation": {
    "sql_valid": true,
    "dry_run_rows": 3,
    "detected_output_columns": [
      {"name": "month", "type": "TIMESTAMP"},
      {"name": "total_earned_usd", "type": "NUMERIC"},
      {"name": "transaction_count", "type": "BIGINT"}
    ]
  },
  "approved_query": { ... }  // Only if auto-approved
}
```

---

## Part 5: Ontology Probe Tools

**See also:** `PLAN-ontology-probe-tools.md` for detailed design.

### Problem Statement

During ontology extraction, the system collects extensive data:
- Column statistics (distinct counts, null rates, cardinality)
- Sample values for enum detection
- Join analysis (match rates, orphan counts)
- Joinability classification

This data is either **persisted but not exposed** or **collected but discarded**. I shouldn't have to query the database to learn what's already been learned.

### 5.1 `get_entity` Tool

Before updating an entity, I need to see its current state.

```
get_entity(name: string)
```

**Returns:**
```json
{
  "name": "User",
  "primary_table": "users",
  "description": "Platform user who can act as host or visitor",
  "aliases": ["creator", "host", "visitor"],
  "key_columns": ["user_id", "username", "is_available"],
  "occurrences": [
    {"table": "billing_transactions", "column": "payee_user_id", "role": "payee"},
    {"table": "billing_transactions", "column": "payer_user_id", "role": "payer"}
  ],
  "relationships": [
    {"to": "Account", "label": "belongs to", "cardinality": "N:1"},
    {"from": "Channel", "label": "owns", "cardinality": "1:N"}
  ]
}
```

### 5.2 `probe_column` Tool

Deep-dive into specific columns without writing SQL.

```
probe_column(table: string, column: string)
```

**Returns:**
```json
{
  "table": "users",
  "column": "status",
  "statistics": {
    "distinct_count": 5,
    "row_count": 1000,
    "null_rate": 0.02,
    "cardinality_ratio": 0.005
  },
  "joinability": {
    "is_joinable": false,
    "reason": "low_cardinality"
  },
  "sample_values": ["ACTIVE", "SUSPENDED", "BANNED", "PENDING", "DELETED"],
  "semantic": {
    "entity": "User",
    "role": "status",
    "enum_labels": {
      "ACTIVE": "Normal active account",
      "SUSPENDED": "Temporarily disabled"
    }
  }
}
```

**Batch variant:** `probe_columns(columns: [{table, column}, ...])`

### 5.3 `probe_relationship` Tool

Deep-dive into relationships with pre-computed metrics.

```
probe_relationship(from_entity?: string, to_entity?: string)
```

**Returns:**
```json
{
  "relationships": [{
    "from_entity": "Account",
    "to_entity": "User",
    "cardinality": "N:1",
    "data_quality": {
      "match_rate": 0.98,
      "orphan_count": 10,
      "source_distinct": 500,
      "target_distinct": 450
    }
  }],
  "rejected_candidates": [{
    "from_column": "accounts.created_by",
    "to_column": "users.user_id",
    "rejection_reason": "low_match_rate",
    "match_rate": 0.12
  }]
}
```

### 5.4 Enhance `get_context` with `include` Parameter

```
get_context(
  depth: "domain" | "entities" | "tables" | "columns",
  include?: ["statistics", "sample_values"]  // NEW
)
```

When `include` contains `"statistics"`, column responses include:
- `distinct_count`, `row_count`, `null_rate`, `cardinality_ratio`
- `is_joinable`, `joinability_reason`

When `include` contains `"sample_values"`, columns with ≤50 distinct values include the actual values.

### 5.5 Extend `update_column` with Entity/Role

```
update_column(
  table: string,
  column: string,
  description?: string,
  enum_values?: string[],
  entity?: string,        // NEW: "User", "Account", etc.
  role?: string           // NEW: "payee", "visitor", "owner"
)
```

---

## Part 6: Additional Improvements

### 6.1 Query History (Lower Priority)

**Problem:** I write the same queries repeatedly across sessions.

**Solution:** New tool `get_query_history`:

```
get_query_history(
  limit?: number,            // Default 20
  hours_back?: number        // Default 24
)
```

**Response:**
```json
{
  "recent_queries": [
    {
      "sql": "SELECT ...",
      "executed_at": "2024-01-05T10:30:00Z",
      "row_count": 42,
      "execution_time_ms": 145
    }
  ]
}
```

### 6.2 Schema Search (Lower Priority)

**Problem:** With 38+ tables, finding relevant ones is hard.

**Solution:** New tool `search_schema`:

```
search_schema(query: string)
```

Returns tables/columns/entities matching the query with relevance ranking. Uses full-text search (trigram/GIN), not embeddings.

### 6.3 Explain Query (Lower Priority)

**Problem:** I write slow queries without knowing.

**Solution:** New tool or enhancement to `validate`:

```
explain_query(sql: string)
```

Returns `EXPLAIN ANALYZE` output with performance hints.

---

## Part 7: Architecture Philosophy - No LLM in the Middle

### The Problem with Traditional AI-to-Database Products

```
User → AI Assistant → [Product's LLM] → SQL → Database → Results
                           ↑
                    RAG / Embeddings
                    Vector Store
                    Fine-tuned Model
```

This introduces:
- **Latency**: Two LLM round-trips instead of one
- **Accuracy loss**: The middle LLM may misinterpret or "hallucinate"
- **Infrastructure burden**: RAG requires embeddings, vector DB, retrieval logic
- **Cost**: Running inference on two models instead of one
- **Staleness**: Embeddings drift out of sync with schema changes

### Ekaya's Differentiated Approach

```
User → AI Assistant (Claude) → SQL → Ekaya (validate + execute) → Results
              ↑
    Ekaya provides rich structured context via MCP
    (No LLM, No RAG, No Embeddings)
```

**Core principle: Don't put an LLM between Claude and the data. Give Claude the context, let Claude write SQL.**

### What Makes Ekaya Different

Most AI-to-database products ask: *"How do we make our LLM write better SQL?"*

Ekaya asks: *"How do we give the client's LLM everything it needs to write perfect SQL?"*

- **They bet on their model** — fine-tuning, RAG, prompt engineering
- **Ekaya bets on context** — rich ontology, structured metadata, MCP features

### What Ekaya DOESN'T Need

| Component | Traditional Products | Ekaya |
|-----------|---------------------|-------|
| Vector database | Required for RAG | Not needed |
| Embedding model | Required for RAG | Not needed |
| SQL-generating LLM | Required | Not needed |
| Fine-tuned model | Often required | Not needed |
| GPU infrastructure | Required for inference | Not needed |

**Ekaya needs**: Postgres + structured ontology + MCP server. That's it.

---

## Part 8: Tool Access Control

### Developer Tools vs Business User Tools

Not all tools should be available to all users. The admin can control which tools are enabled.

**Developer Tools** (ontology updates, questions workflow, probing):
- `update_entity`, `delete_entity`, `get_entity`
- `update_relationship`, `delete_relationship`
- `update_glossary_term`, `delete_glossary_term`
- `update_project_knowledge`, `delete_project_knowledge`
- `update_column`, `delete_column_metadata`
- `probe_column`, `probe_columns`, `probe_relationship`
- `list_ontology_questions`, `resolve_ontology_question`
- `skip_ontology_question`, `escalate_ontology_question`, `dismiss_ontology_question`

**Business User Tools** (query execution):
- `query`, `sample`, `validate`
- `list_approved_queries`, `execute_approved_query`
- `get_context`, `get_glossary_sql`
- `suggest_approved_query` (if admin enables suggestions from users)

**Always Available:**
- `health`, `echo`

---

## Part 9: Implementation Priority

### Phase 1: Foundation (High Impact, Moderate Effort)

1. **[x] `get_context` unified tool** - Consolidates 3 tools, graceful degradation
   - **Implementation:** `pkg/mcp/tools/context.go` + `pkg/mcp/tools/context_test.go`
   - **Registration:** Added to main.go with ContextToolDeps
   - **Registry:** Added to ToolRegistry in pkg/services/mcp_tools_registry.go under ToolGroupApprovedQueries
   - **Key Features Implemented:**
     - Progressive depth levels: `domain`, `entities`, `tables`, `columns`
     - Graceful degradation when ontology unavailable (falls back to schema-only)
     - Consolidates ontology + schema + glossary in single call
     - Reduces token waste by avoiding duplicate entity descriptions
     - Always returns useful context (never fails with "ontology not found")
   - **Testing:** Comprehensive unit tests covering all depth levels, with/without ontology, error cases
   - **Tool Group:** ToolGroupApprovedQueries (available when Developer Tools OR Approved Queries enabled)
   - **Next Session Notes:**
     - The tool properly handles nil ontology (returns schema-only response)
     - Depth filtering works correctly (domain < entities < tables < columns)
     - Glossary integration is optional (gracefully handles missing glossary service)
     - Entity descriptions from ontology are merged with schema table data
     - Relationships are only included if ontology exists and depth >= entities

2. **[x] `get_context` with `include` parameter** - COMPLETED: Add statistics and sample_values options
   - **Implementation:** Added `include` array parameter to `get_context` tool in `pkg/mcp/tools/context.go`
   - **Supported values:**
     - `statistics`: Adds distinct_count, row_count, null_rate, cardinality_ratio, is_joinable, joinability_reason
     - `sample_values`: Placeholder for future implementation (marked with TODO)
   - **Statistics source:** Retrieved from `engine_schema_columns` table via SchemaRepository.GetColumnsByTables()
   - **Architecture:**
     - New `includeOptions` struct with Statistics and SampleValues flags
     - `parseIncludeOptions()` function converts string array to options struct
     - `buildColumnDetails()` enriches column data with statistics when requested
     - `addStatisticsToColumnDetail()` adds computed statistics (null_rate, cardinality_ratio) from SchemaColumn
   - **Error handling:** Partial failure tolerance - if statistics fetch fails, continues with basic column data (logs warning)
   - **Testing:** Added comprehensive unit tests for parseIncludeOptions and addStatisticsToColumnDetail functions
   - **Known limitation:** Sample values fetching on-demand requires datasource adapter access, which is not yet implemented
   - **Next Session Notes:**
     - Statistics are computed from SchemaColumn fields (DistinctCount, NullCount, RowCount, IsJoinable, JoinabilityReason)
     - Only included when depth=columns and include contains "statistics"
     - Sample values will require adding DatasourceAdapterFactory to ContextToolDeps for on-demand fetching
     - The implementation handles missing statistics gracefully - nil pointers are checked before adding to response
     - Calculated fields (null_rate, cardinality_ratio) are only added when required data is available

3. **[x] `update_project_knowledge`** - COMPLETED: Leverages existing `engine_project_knowledge` table
   - **Implementation:** `pkg/mcp/tools/knowledge.go` + `pkg/mcp/tools/knowledge_test.go`
   - **Registration:** Added to main.go with KnowledgeToolDeps (main.go:400-407)
   - **Registry:** Added to ToolRegistry in pkg/services/mcp_tools_registry.go under ToolGroupDeveloper
   - **Key Features Implemented:**
     - `update_project_knowledge` tool with upsert semantics (by project_id + fact_type + key)
     - `delete_project_knowledge` tool for removing incorrect facts by fact_id
     - Support for 4 categories: terminology, business_rule, enumeration, convention
     - Optional fact_id parameter for explicit updates vs natural upsert
     - Optional context parameter for tracking discovery source
     - Default category to "terminology" if not specified
     - Validation of category values with clear error messages
   - **Testing:** Comprehensive unit tests covering tool structure, registration, parameter validation, and response formats
   - **Tool Group:** ToolGroupDeveloper (available when Developer Tools enabled)
   - **Architecture Notes:**
     - Uses existing KnowledgeRepository interface with Upsert and Delete methods
     - Dependencies injected via KnowledgeToolDeps struct (DB, MCPConfigService, KnowledgeRepository, Logger)
     - Tools registered in main.go alongside other MCP tool groups (glossary, context, etc.)
     - Follows standard MCP tool pattern: deps struct → Register function → handler functions
     - Uses unified ToolAccessChecker for consistent access control with tool list filtering
   - **Implementation Details:**
     - Facts are stored in engine_project_knowledge table with upsert on (project_id, fact_type, key)
     - The tool uses fact content as the key for natural upsert behavior
     - fact_id can be explicitly provided to update a specific fact by ID instead of content-based upsert
     - The KnowledgeRepository interface is fully implemented in pkg/repositories/knowledge.go
     - Tools properly filter by project_id from MCP context (no cross-project data leakage)
     - Includes checkKnowledgeToolEnabled for project-scoped access control with tenant context
     - Response includes fact_id, fact, category, context, created_at, and updated_at timestamps

### Phase 2: Probe Tools (High Impact, Low Effort - Data Already Persisted)

4. **[x] `get_entity`** - COMPLETED (2026-01-08): Full entity details for before making updates
   - **Implementation:** `pkg/mcp/tools/entity.go` + `pkg/mcp/tools/entity_test.go`
   - **Registration:** Added to main.go with EntityToolDeps (main.go:423-432)
   - **Registry:** Added to ToolRegistry in pkg/services/mcp_tools_registry.go under ToolGroupDeveloper
   - **Commit:** `2fb9e30 feat: enable AI agents to inspect full entity details via get_entity tool`
   - **Key Features Implemented:**
     - Retrieves full entity details by name from active ontology
     - Returns name, primary_table, description
     - Includes all aliases from engine_ontology_entity_aliases
     - Includes all key columns from engine_ontology_entity_key_columns
     - Builds occurrences from relationships (where entity appears in schema with roles)
     - Lists relationships to/from other entities with labels and column mappings
     - Deduplicates occurrences across source and target relationships
     - Maps entity IDs to names for readable relationship output
   - **Testing:** Comprehensive unit tests covering response building, empty data, aliases, key columns, occurrences, relationships
   - **Tool Group:** ToolGroupDeveloper (available when Developer Tools enabled)
   - **Response Format:**
     ```json
     {
       "name": "User",
       "primary_table": "users",
       "description": "Platform user...",
       "aliases": ["creator", "host"],
       "key_columns": ["user_id", "username"],
       "occurrences": [
         {"table": "billing_transactions", "column": "payee_user_id", "role": "payee"}
       ],
       "relationships": [
         {"direction": "to", "entity": "Account", "label": "owns", "columns": "users.user_id -> accounts.owner_id"}
       ]
     }
     ```
   - **Next Session Notes:**
     - The tool queries engine_ontology_entities, engine_ontology_entity_aliases, engine_ontology_entity_key_columns, and engine_entity_relationships
     - Occurrences are derived from relationships (both source and target) rather than stored separately
     - Returns error if no active ontology or entity not found
     - Uses consistent error handling pattern from other MCP tools
     - Dependencies: OntologyRepo, OntologyEntityRepo, EntityRelationshipRepo
     - **WHY this tool exists:** AI agents need to inspect full entity state (aliases, key columns, occurrences, relationships) before making updates with `update_entity` or similar tools. Without this, agents must query multiple tables or risk overwriting existing data. This tool provides complete context in a single call.
5. **[x] `probe_column`** / **`probe_columns`** - COMPLETED: Column statistics and semantic information
   - **Implementation:** `pkg/mcp/tools/probe.go` + `pkg/mcp/tools/probe_test.go`
   - **Registration:** Added to main.go with ProbeToolDeps (main.go:431-442)
   - **Registry:** Added to ToolRegistry in pkg/services/mcp_tools_registry.go under ToolGroupDeveloper
   - **Key Features Implemented:**
     - `probe_column` tool for deep-diving into specific columns
     - `probe_columns` batch variant for analyzing multiple columns at once (up to 50 columns max)
     - Returns statistics (distinct_count, row_count, non_null_count, null_rate, cardinality_ratio, min_length, max_length)
     - Returns joinability classification (is_joinable, reason)
     - Returns semantic information from ontology (entity, role, description, enum_labels)
     - Graceful error handling - batch mode returns partial results with errors for failed probes
     - sample_values intentionally NOT included (see architecture notes below)
   - **Testing:** Comprehensive unit tests covering tool structure, registration, parameter validation, and response formats
   - **Tool Group:** ToolGroupDeveloper (available when Developer Tools enabled)
   - **Response Format:**
     ```json
     {
       "table": "users",
       "column": "status",
       "statistics": {
         "distinct_count": 5,
         "row_count": 1000,
         "non_null_count": 950,
         "null_rate": 0.05,
         "cardinality_ratio": 0.005,
         "min_length": 6,
         "max_length": 9
       },
       "joinability": {
         "is_joinable": false,
         "reason": "low_cardinality"
       },
       "semantic": {
         "entity": "User",
         "role": "attribute",
         "description": "User account status",
         "enum_labels": {
           "ACTIVE": "Normal active account",
           "SUSPENDED": "Temporarily disabled"
         }
       }
     }
     ```
   - **Architecture Notes:**
     - Handler uses SchemaRepo.GetColumnsByTables() to fetch statistics from engine_schema_columns table
     - Handler uses OntologyRepo.GetActive() to fetch semantic data from engine_ontologies.column_details
     - Batch variant iterates over input columns and builds map of "table.column" -> probe result
     - Partial failure support: if one column fails in batch mode, others still return successfully with error entry
     - Access control via checkProbeToolEnabled (validates Developer Tools enabled for project)
     - Tool filtering handled by unified ToolAccessChecker in MCP server
   - **Design Decision: Why sample_values is NOT included:**
     - The original plan (Part 5, Section 5.2) included sample_values in probe_column response
     - However, Phase 2 item #7 specifies: "Persist sample_values - Store distinct values during extraction (currently discarded)"
     - This means sample_values are NOT currently stored in engine_schema_columns during ontology extraction
     - To implement sample_values would require BOTH:
       1. Changes to ontology extraction workflow to persist values during extraction
       2. Changes to this MCP tool to return persisted values
     - The probe tools were implemented to expose EXISTING persisted data only, not to trigger on-demand data fetching
     - This keeps probe tools fast (single DB query) and avoids datasource adapter complexity
     - Future session implementing Phase 2 #7 can add sample_values to this tool's response once data is persisted
   - **Next Session Notes:**
     - Statistics are pre-computed during schema extraction and stored in engine_schema_columns
     - Semantic information comes from active ontology's column_details JSONB field
     - Enum labels are extracted by parsing column_details[table][column].enum_labels
     - Batch tool has 50-column limit to prevent excessive database queries
     - Dependencies: DB, MCPConfigService, SchemaRepo, OntologyRepo, Logger
     - For sample_values support, implement Phase 2 item #7 first (ontology extraction changes), then add to this tool
6. **[x] `probe_relationship`** - COMPLETED: Relationship exploration with optional entity filtering
   - **Implementation:** `pkg/mcp/tools/probe.go` (registerProbeRelationshipTool function) + `pkg/mcp/tools/probe_test.go`
   - **Registration:** Added to main.go with ProbeToolDeps (main.go:434-444), added EntityRepo and RelationshipRepo dependencies
   - **Registry:** Added to ToolRegistry in pkg/services/mcp_tools_registry.go under ToolGroupDeveloper (line 27)
   - **Key Features Implemented:**
     - Supports optional `from_entity` and `to_entity` parameters for filtering relationships
     - Returns entity relationships with from/to entity names and column mappings
     - Includes description and association label if available
     - Returns empty arrays for data_quality and rejected_candidates (see limitations below)
   - **Testing:** Unit tests covering response structure, empty state, minimal fields, and orphan calculation logic
   - **Tool Group:** ToolGroupDeveloper (available when Developer Tools enabled)
   - **Response Format:**
     ```json
     {
       "relationships": [{
         "from_entity": "Account",
         "to_entity": "User",
         "from_column": "accounts.owner_id",
         "to_column": "users.user_id",
         "description": "The user who owns this account",
         "label": "owns"
       }],
       "rejected_candidates": []
     }
     ```
   - **Architecture Notes:**
     - Queries engine_entity_relationships to get confirmed relationships
     - Filters by from_entity/to_entity parameters if provided
     - Maps entity IDs to names for readable output
     - Access control via checkProbeToolEnabled (validates Developer Tools enabled for project)
     - Dependencies: DB, MCPConfigService, SchemaRepo, OntologyRepo, EntityRepo, RelationshipRepo, Logger
   - **Known Limitations:**
     - **cardinality and data_quality metrics NOT currently returned** - These are stored in engine_schema_relationships table but require additional repository/query logic to fetch and map to entity relationships
     - **rejected_candidates NOT currently returned** - These are stored in engine_schema_relationships with rejection_reason but require similar infrastructure
     - The implementation focuses on entity-level relationships from engine_entity_relationships, not schema-level metrics from engine_schema_relationships
   - **Next Session Notes:**
     - To add cardinality/data_quality metrics, need to:
       1. Query engine_schema_relationships table (via SchemaRepo or new SchemaRelationshipRepo)
       2. Match schema relationships to entity relationships by (source_table, source_column, target_table, target_column)
       3. Extract match_rate, source_distinct, target_distinct, matched_count
       4. Calculate orphan_count = source_distinct - matched_count
     - To add rejected_candidates, need to:
       1. Query engine_schema_relationships WHERE rejection_reason IS NOT NULL
       2. Filter by entity names if from_entity/to_entity parameters provided
       3. Return rejection_reason and match_rate for each candidate
     - For now, tool provides entity-level relationship visibility which is still valuable for understanding domain model
7. **[x] Persist sample_values** - COMPLETED (2026-01-08): Store distinct values during extraction
   - **Commit:** `feat: persist sample values for low-cardinality columns during ontology extraction`
   - **Implementation:** Modified column enrichment workflow to persist sample values for low-cardinality columns (≤50 distinct values)
   - **Migration:** Added `sample_values TEXT[]` column to `engine_schema_columns` table (migrations/033_add_sample_values.{up,down}.sql)
   - **Model Changes:**
     - Added `SampleValues []string` field to `models.SchemaColumn` struct
     - Field is included in JSON serialization and repository scanning
   - **Repository Changes:** Updated `SchemaRepository.UpdateColumnStats` signature to accept `sampleValues []string` parameter
     - Modified all query methods (ListColumnsByTable, ListColumnsByDatasource, GetColumnsByTables, GetColumnByID, GetColumnByName) to SELECT sample_values column
     - Updated UpsertColumn to preserve sample_values when reactivating soft-deleted columns
     - Updated scanSchemaColumn, scanSchemaColumnRow, and scanSchemaColumnWithDiscovery to scan sample_values
   - **Service Changes:**
     - Added `persistSampleValues` method to `ColumnEnrichmentService` (pkg/services/column_enrichment.go:322-342)
     - Method iterates over enumSamples map (already collected by `sampleEnumValues`) and persists to database
     - Calls `schemaRepo.UpdateColumnStats` with nil for other stats to preserve existing values
     - Uses Warn-level logging for persistence failures (non-fatal, continues with other columns)
     - Called from `EnrichTable` after saving column details (line 221-228)
   - **MCP Tool Changes:**
     - Updated `probe_column` tool (pkg/mcp/tools/probe.go:363-367) to return sample_values from SchemaColumn
     - Removed obsolete comment about sample_values not being persisted
     - Updated response struct comment to document sample_values availability for low-cardinality columns
   - **Testing:**
     - Updated all repository test mocks to include sample_values parameter
     - Modified test fixtures across 6 test files to match new signature
     - All tests pass (repository, service, integration)
   - **Key Design Decisions:**
     - Sample values are collected during ontology extraction via existing `sampleEnumValues` method (already identifies low-cardinality columns)
     - No on-demand fetching required - data is persisted during extraction and retrieved from database
     - Only columns with ≤50 distinct values get sample_values (enum candidates)
     - Non-fatal persistence - if UpdateColumnStats fails for one column, others continue
     - MCP tools (probe_column, get_context) can now return sample_values without datasource adapter access
   - **Next Session Notes:**
     - Sample values are only populated for columns that passed enum candidate detection (distinctCount <= 50)
     - The data is already present from previous ontology extractions - no re-extraction needed
     - To add sample_values to `get_context` tool responses, modify `buildColumnDetails` in pkg/mcp/tools/context.go to include SampleValues field
     - The persistence is opportunistic - failures are logged but don't block enrichment workflow
     - **WHY this change was needed:** Without persisted sample_values, MCP tools like probe_column would need on-demand datasource adapter access to fetch distinct values, adding latency and complexity. Persisting during extraction (when values are already sampled) enables instant retrieval via simple SELECT queries.

### Phase 3: Query Intelligence (High Impact, Higher Effort)

8. **[x] `suggest_approved_query`** - COMPLETED (2026-01-08): Allow AI agents to propose reusable queries
   - **Implementation:** `pkg/mcp/tools/queries.go` (registerSuggestApprovedQueryTool function) + `pkg/mcp/tools/queries_test.go`
   - **Migration:** Added `status`, `suggested_by`, and `suggestion_context` fields to `engine_queries` table (migrations/034_query_suggestions.{up,down}.sql)
   - **Model Changes:** Added Status, SuggestedBy, SuggestionContext fields to `models.Query` struct (pkg/models/query.go)
   - **Service Changes:** Extended `CreateQueryRequest` to accept suggestion metadata, updated `QueryService.Create` to handle status (default "approved" for backward compatibility) and metadata (pkg/services/query.go)
   - **Repository Changes:** Updated all SQL queries (Create, GetByID, ListByDatasource, ListEnabled, Update) and scan functions (scanQuery, scanQueryRow) to include new fields (pkg/repositories/query_repository.go)
   - **Registry:** Added to ToolRegistry in pkg/services/mcp_tools_registry.go under ToolGroupApprovedQueries (line 42)
   - **Key Features Implemented:**
     - Validates SQL with example parameters using dry-run execution (via QueryService.Test)
     - Detects output columns from query execution results (column names and types)
     - Merges provided column descriptions with detected columns (buildOutputColumns function)
     - Stores query with status='pending' and suggested_by='agent' by default
     - Returns validation results including detected columns and row count
     - Suggestion context stores validation metadata (sql_valid, dry_run_rows, parameters_used) for review
     - Helper functions: parseParameterDefinitions, validateAndTestQuery, buildOutputColumns, buildColumnInfo
   - **Testing:** Comprehensive unit tests covering tool registration, parameter parsing (parseParameterDefinitions), output column building (buildOutputColumns), and response structure (TestSuggestApprovedQuery_ResponseStructure)
   - **Tool Group:** ToolGroupApprovedQueries (available when Approved Queries enabled)
   - **Response Format:**
     ```json
     {
       "suggestion_id": "uuid",
       "status": "pending",
       "validation": {
         "sql_valid": true,
         "dry_run_rows": 3,
         "detected_output_columns": [
           {"name": "month", "type": "TIMESTAMP"},
           {"name": "total_earned_usd", "type": "NUMERIC"}
         ]
       }
     }
     ```
   - **Tool Parameters:**
     - `name` (required): Human-readable name for the query
     - `description` (required): What business question this query answers
     - `sql` (required): SQL query with {{parameter}} placeholders
     - `parameters` (optional): Parameter definitions array (inferred from SQL if omitted)
     - `output_column_descriptions` (optional): Object mapping column names to descriptions
   - **Parameter Definition Structure:**
     - `name` (required): Parameter name matching {{placeholder}} in SQL
     - `type` (required): "string" | "number" | "date" | "boolean"
     - `description` (optional): User-friendly explanation
     - `required` (optional, default true): Whether parameter is required
     - `example` (required for required params): Example value for dry-run validation
   - **Validation Workflow:**
     1. Extract and validate tool parameters (name, description, sql)
     2. Get default datasource ID from project
     3. Parse optional parameters array and output_column_descriptions object
     4. Call validateAndTestQuery to run dry-run execution with example values
     5. Build output columns by merging detected columns with provided descriptions
     6. Create query via QueryService.Create with status='pending', suggested_by='agent'
     7. Return suggestion_id, status, and validation results
   - **Error Handling:**
     - Returns error if required parameters (name, description, sql) missing
     - Returns error if parameter definitions invalid (missing name or type)
     - Returns error if required parameter has no example value for validation
     - Returns error if SQL validation fails during dry-run execution
     - Returns error if default datasource not configured for project
   - **Next Session Notes:**
     - UI work needed for approval flow (admin review of pending suggestions in /projects/{pid}/approved-queries)
     - Future enhancement: Auto-approve mode (status='approved' if enabled in project config)
     - The tool uses QueryService.Test to validate SQL with example parameters (limit=1 for column detection)
     - Example parameters are required for all required parameters (stored in Default field of QueryParameter)
     - Query is created as disabled (is_enabled=false) until approved by admin
     - Status field defaults to "approved" in QueryService.Create for backward compatibility with existing code paths
     - SuggestionContext stores validation metadata as map[string]any with "validation" key
     - Helper functions are internal to queries.go (not exported) to keep MCP tool surface minimal
     - **WHY this tool exists:** AI agents discover useful query patterns during exploration. Without this tool, those patterns are ephemeral - lost when the session ends. This tool captures them for human review and approval, building a curated library of business queries over time. The system gets smarter with each approved suggestion. The validation workflow ensures suggested queries are syntactically valid and executable before being submitted for review.
9. **[x] Query tags/categories** - COMPLETED (2026-01-08): Add tagging and categorization support
   - **Migration:** Added `tags TEXT[]` column to `engine_queries` table with GIN index for array containment queries (migrations/035_query_tags.{up,down}.sql)
   - **Model Changes:** Added `Tags []string` field to `models.Query` struct (pkg/models/query.go)
   - **Repository Changes:**
     - Updated all SQL queries (Create, Update, GetByID, ListByDatasource, ListEnabled) to include tags column
     - Updated scan functions (scanQuery, scanQueryRow) to scan tags
     - Added `ListEnabledByTags` method for filtering queries by tags (uses PostgreSQL `&&` array overlap operator)
     - Tag filtering uses OR logic: queries matching ANY of the provided tags are returned
   - **Service Changes:**
     - Added `Tags []string` field to `CreateQueryRequest` (pkg/services/query.go)
     - Updated `Create` method to ensure tags is never nil (defaults to empty array)
     - Added `ListEnabledByTags` method to QueryService interface and implementation
   - **MCP Tool Changes:**
     - Added `tags` array parameter to `suggest_approved_query` tool (optional)
     - Added `Tags` field to `approvedQueryInfo` response struct
     - Added `tags` array parameter to `list_approved_queries` tool for filtering (optional)
     - Updated list handler to use `ListEnabledByTags` when tags filter is provided
     - Updated response building to include tags in query info
   - **Testing:** Added comprehensive integration test `TestQueryRepository_ListEnabledByTags` covering:
     - Single tag filtering
     - Multiple tag filtering (OR logic)
     - Filtering by non-existent tags
     - Empty tag list handling
     - Disabled query exclusion
     - Tag array preservation in results
   - **Test Updates:** Updated all mock repositories and services to include `ListEnabledByTags` method:
     - `pkg/services/query_parameterized_test.go` - mockQueryRepository
     - `pkg/mcp/tools/mocks_test.go` - mockQueryService
     - `pkg/services/mcp_config_test.go` - mockQueryServiceForMCP
   - **Design Decision:** Used single `tags` TEXT[] field instead of separate tags/categories for:
     - Flexibility: Can represent categories as special tags (e.g., "category:billing", "category:analytics")
     - Simplicity: One column vs two, simpler schema
     - Ease of querying: PostgreSQL array operations work well for this use case
     - Natural multi-category support: Queries can belong to multiple categories if needed
   - **Usage Examples:**
     - Suggest query with tags: `suggest_approved_query(..., tags: ["billing", "category:finance", "reporting"])`
     - List queries by tag: `list_approved_queries(tags: ["billing"])` - returns queries with "billing" tag
     - List queries by multiple tags: `list_approved_queries(tags: ["billing", "engagement"])` - returns queries with ANY of these tags
   - **WHY this feature exists:** As the approved query library grows, users need ways to organize and find queries. Tags allow flexible categorization by business domain (billing, engagement), use case (reporting, analytics), or custom categories. AI agents can suggest appropriate tags when proposing queries, and users can filter queries by tag when searching for relevant ones. This makes the query library more discoverable and maintainable at scale.
   - **Next Session Notes:**
     - All repository and service mocks have been updated to include ListEnabledByTags method
     - GIN index on tags column enables efficient array containment queries (tags && ARRAY['billing'])
     - The implementation uses PostgreSQL array overlap operator (&&) for OR logic filtering
     - Tags default to empty array (never nil) to prevent null-related issues in queries
     - Future enhancement: UI for managing tags (bulk tagging, tag renaming, tag suggestions)

### Phase 4: Ontology Updates (Medium Impact)

10. **[ ] `update_entity`**, **[ ] `delete_entity`**
11. **[ ] `update_relationship`**, **[ ] `delete_relationship`** (with cardinality)
12. **[ ] `update_glossary_term`**, **[ ] `delete_glossary_term`**
13. **[ ] `update_column`**, **[ ] `delete_column_metadata`** (with entity/role params)

### Phase 5: Questions Workflow (High Impact, Higher Effort)

14. **[ ] `list_ontology_questions`** with filtering and pagination
15. **[ ] `resolve_ontology_question`**
16. **[ ] `skip_ontology_question`**, **[ ] `escalate_ontology_question`**, **[ ] `dismiss_ontology_question`**

### Phase 6: Power Features (Lower Priority)

17. **[ ] `search_schema`** - Full-text search
18. **[ ] `explain_query`** - Performance insights
19. **[ ] `get_query_history`** - Recent queries

---

## Part 10: Database Changes

### New Tables

None required - reuse existing tables.

### Schema Changes

```sql
-- engine_queries additions for suggestion workflow
ALTER TABLE engine_queries ADD COLUMN IF NOT EXISTS status VARCHAR DEFAULT 'approved';
-- Values: 'pending', 'approved', 'rejected'

ALTER TABLE engine_queries ADD COLUMN IF NOT EXISTS suggested_by VARCHAR;
-- Values: 'user', 'agent', 'admin'

ALTER TABLE engine_queries ADD COLUMN IF NOT EXISTS suggestion_context JSONB;
-- Stores: example usage, validation results, etc.

-- engine_project_knowledge additions
ALTER TABLE engine_project_knowledge ADD COLUMN IF NOT EXISTS category VARCHAR;
-- Values: 'terminology', 'business_rule', 'enumeration', 'convention'

ALTER TABLE engine_project_knowledge ADD COLUMN IF NOT EXISTS source VARCHAR DEFAULT 'manual';
-- Values: 'manual', 'agent', 'ontology_extraction'

-- engine_ontology_questions (new or extend existing)
-- Stores questions generated during ontology extraction
CREATE TABLE IF NOT EXISTS engine_ontology_questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    question TEXT NOT NULL,
    category VARCHAR NOT NULL,  -- enumeration, relationship, business_logic, naming, calculation
    priority VARCHAR DEFAULT 'medium',  -- high, medium, low
    status VARCHAR DEFAULT 'pending',  -- pending, resolved, skipped, escalated, dismissed
    context JSONB,  -- entity, table, column, observed_values, etc.
    resolution_notes TEXT,
    status_reason TEXT,  -- reason for skip/escalate/dismiss
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    resolved_at TIMESTAMPTZ
);
```

---

## Part 11: Success Metrics

After implementation, measure:

1. **Zero database probes** - Enum discovery, cardinality checks via probe tools, not SQL
2. **Knowledge accumulation** - Facts added via `update_project_knowledge`
3. **Query reuse** - Suggested queries that get approved and used
4. **Questions resolved** - Percentage of ontology questions answered
5. **Tool call reduction** - Fewer calls needed per task with `get_context`
6. **Token efficiency** - Reduction in context tokens per session
7. **Time to first correct query** - How quickly Claude can write accurate SQL

---

## Closing Thoughts

The current MCP tools treat me as a **consumer** of a static knowledge base. These proposals transform me into a **collaborator** who can:

1. **Learn progressively** - Start shallow, go deep only when needed
2. **Contribute knowledge** - Capture insights that persist
3. **Answer questions** - Bridge the gap between schema and source code
4. **Suggest queries** - Build a curated library of business queries

The underlying infrastructure already exists (ontology tables, knowledge table, query system). These proposals are about exposing that infrastructure through the MCP interface in a way that makes AI agents first-class participants in the knowledge ecosystem.

### The Vision

```
Today:     Claude + Ekaya = Correct SQL on first try
Tomorrow:  Claude + Ekaya = Self-improving knowledge base
Future:    Claude + Ekaya = The database explains itself
```

Ekaya isn't competing to be the best SQL-writing AI. It's competing to be the **best context layer for AI-powered data access**. That's a moat that grows with every ontology extraction, every knowledge update, every approved query.

---

*"The AI Data Liaison isn't just answering questions—it's building a curated library of business queries tailored to your domain."*

*"The smartest model is already in the conversation. Don't duplicate it. Empower it."*
