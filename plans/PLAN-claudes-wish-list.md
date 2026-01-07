# PLAN: Claude's MCP Tool Wishlist

**Author:** Claude (as MCP Client)
**Date:** 2025-01-05
**Status:** Design Proposal

## Executive Summary

This document captures my wishlist as an MCP client after hands-on experience with the ekaya-engine MCP server. The core thesis: **the current tools work, but they're designed for humans managing a system rather than AI agents collaborating with it.**

Three major capabilities would transform this from a query tool into a learning system:

1. **Unified Context Tool** - Progressive disclosure with graceful degradation
2. **Ontology Contribution** - Let me enhance what I learn
3. **Query Recommendation** - Let me suggest reusable queries

---

## Part 1: Unified Context Tool (`get_context`)

### Problem Statement

Currently I call multiple tools to understand a database:
- `get_ontology(depth=domain)` → business context
- `get_schema` → table/column details
- `get_glossary` → business terms

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

## Part 2: Ontology Contribution Tools

### Problem Statement

I learn things during queries that the ontology doesn't capture:
- "Oh, `transaction_state='settled'` means the payment completed"
- "The `tikr_share` column is the platform's commission"
- "`visitor` and `host` are the two sides of an engagement"

Currently this knowledge dies with my session. The next Claude instance starts from scratch.

### Proposed Tools

#### 2.1 `suggest_knowledge`

Add project-level facts that persist across sessions.

```
suggest_knowledge(
  fact_type: "terminology" | "business_rule" | "enumeration" | "convention",
  key: string,
  value: string,
  context?: string,
  confidence: "high" | "medium" | "low"
)
```

**Examples:**

```json
// Learning from a query result
{
  "fact_type": "enumeration",
  "key": "billing_transactions.transaction_state",
  "value": "pending, processing, settled, failed, refunded",
  "context": "Discovered while analyzing transaction flow",
  "confidence": "high"
}

// Understanding a business term
{
  "fact_type": "terminology",
  "key": "tik",
  "value": "A billing unit representing approximately 6 seconds of engagement time",
  "context": "Inferred from tiks * 6 ≈ total_duration_s in billing_engagements",
  "confidence": "medium"
}

// Documenting a business rule
{
  "fact_type": "business_rule",
  "key": "platform_fee_calculation",
  "value": "Platform fees are 33% of total_amount, stored in tikr_share column",
  "context": "Verified: tikr_share / total_amount ≈ 0.33 across all transactions",
  "confidence": "high"
}
```

**Storage:** `engine_project_knowledge` table (already exists!)

**Approval Workflow:**
- `confidence: high` → Auto-approved, immediately visible
- `confidence: medium|low` → Queued for human review
- UI shows pending suggestions for admin approval

#### 2.2 `enhance_entity`

Add or update entity metadata.

```
enhance_entity(
  entity_name: string,
  enhancement_type: "alias" | "key_column" | "description",
  value: string | string[],
  source: "query_learning"
)
```

**Examples:**

```json
// Add an alias I discovered
{
  "entity_name": "User",
  "enhancement_type": "alias",
  "value": "creator",
  "source": "query_learning"
}

// Flag an important column
{
  "entity_name": "Billing Transaction",
  "enhancement_type": "key_column",
  "value": "earned_amount",
  "source": "query_learning"
}
```

**Storage:**
- Aliases → `engine_ontology_entity_aliases` with `source='query'`
- Key columns → `engine_ontology_entity_key_columns`

#### 2.3 `enhance_relationship`

Add semantic context to relationships.

```
enhance_relationship(
  from_entity: string,
  to_entity: string,
  association: string,
  description?: string
)
```

**Example:**

```json
{
  "from_entity": "Billing Engagement",
  "to_entity": "User",
  "association": "as visitor",
  "description": "The user who initiated the engagement and will be charged"
}
```

**Storage:** Updates `engine_entity_relationships.association` and `description` fields.

### Permission Model

| Tool | User | Agent (API Key) | Auto-Approve |
|------|------|-----------------|--------------|
| `suggest_knowledge` | Yes | Yes | High confidence only |
| `enhance_entity` | Yes | No | Aliases only |
| `enhance_relationship` | Yes | No | Never |

**Rationale:** Agents can suggest but not directly modify schema semantics. This prevents prompt injection attacks where malicious web content tries to poison the ontology.

---

## Part 3: Query Recommendation Tool

### Problem Statement

When I write a useful query, it's ephemeral. The admin must manually:
1. Copy the SQL
2. Navigate to Queries page
3. Add natural language prompt
4. Define parameters with types
5. Test to capture output columns
6. Add constraints
7. Save

This friction means most useful queries never get saved.

### Proposed Tool

#### `recommend_query`

Suggest a query for approval with full metadata.

```
recommend_query(
  name: string,
  description: string,
  sql: string,
  parameters: QueryParameter[],
  output_columns: OutputColumn[],
  constraints?: string,
  example_usage?: string
)
```

**Example:**

```json
{
  "name": "Top hosts by revenue",
  "description": "Returns hosts ranked by total earnings with transaction counts",
  "sql": "SELECT payee_user_id, payee_username, COUNT(*) as transactions, SUM(earned_amount) / 100.0 as total_earned_usd FROM billing_transactions WHERE deleted_at IS NULL AND created_at >= {{start_date}} AND created_at < {{end_date}} GROUP BY payee_user_id, payee_username ORDER BY total_earned_usd DESC LIMIT {{limit}}",
  "parameters": [
    { "name": "start_date", "type": "date", "description": "Start of date range", "required": true },
    { "name": "end_date", "type": "date", "description": "End of date range", "required": true },
    { "name": "limit", "type": "integer", "description": "Max results", "required": false, "default": 10 }
  ],
  "output_columns": [
    { "name": "payee_user_id", "type": "uuid", "description": "Host's user ID" },
    { "name": "payee_username", "type": "string", "description": "Host's username" },
    { "name": "transactions", "type": "integer", "description": "Number of completed transactions" },
    { "name": "total_earned_usd", "type": "decimal", "description": "Total earnings in USD" }
  ],
  "constraints": "Only includes transactions with deleted_at IS NULL. Amounts converted from cents to dollars.",
  "example_usage": "Find top 5 hosts in January 2024: start_date='2024-01-01', end_date='2024-02-01', limit=5"
}
```

### Workflow

```
Claude writes ad-hoc query
        ↓
Query works well
        ↓
Claude calls recommend_query()
        ↓
System validates SQL + parameters
        ↓
Recommendation stored (is_enabled=false, status='pending')
        ↓
Admin sees notification in UI
        ↓
Admin reviews, optionally edits, approves
        ↓
Query becomes available in list_approved_queries
```

### Storage

New table or extend `engine_queries`:

```sql
-- Option A: New status field
ALTER TABLE engine_queries ADD COLUMN status VARCHAR DEFAULT 'approved';
-- Values: 'pending_review', 'approved', 'rejected'

ALTER TABLE engine_queries ADD COLUMN recommended_by VARCHAR;
-- Values: 'user', 'agent', 'system'

ALTER TABLE engine_queries ADD COLUMN recommendation_context JSONB;
-- Stores: original conversation context, example usage, etc.
```

### UI Changes

**Queries Page:**
- New tab or filter: "Pending Recommendations"
- Badge showing count of pending recommendations
- One-click approve/reject with optional edits
- Show recommendation context (who suggested, when, example usage)

---

## Part 4: Additional Improvements

### 4.1 Query History/Favorites

**Problem:** I write the same queries repeatedly across sessions.

**Solution:** New tool `get_query_history` returning recent successful queries:

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

**Privacy:** Only return queries from same user/agent, last 24 hours, de-duplicated.

### 4.2 Schema Search

**Problem:** With 38 tables, finding relevant ones is hard.

**Solution:** New tool `search_schema`:

```
search_schema(query: "billing")
→ Returns tables/columns matching "billing" with relevance ranking
```

**Implementation:** Full-text search over table names, column names, entity descriptions, aliases.

### 4.3 Explain Plan

**Problem:** I write slow queries without knowing.

**Solution:** Enhance `validate` tool or new `explain_query` tool:

```
explain_query(sql: "SELECT ...")
→ Returns EXPLAIN ANALYZE output with performance hints
```

### 4.4 Ontology Extraction Status

**Problem:** I don't know if ontology is being extracted, complete, or failed.

**Solution:** Include in `get_context` response:

```json
{
  "ontology_status": "extracting",
  "extraction_progress": {
    "current_node": "ColumnEnrichment",
    "progress": { "current": 15, "total": 38 },
    "started_at": "2024-01-05T10:00:00Z"
  }
}
```

---

## Part 5: Architecture Philosophy — No LLM in the Middle

### The Problem with Traditional AI-to-Database Products

Most AI-to-database solutions follow this pattern:

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

The MCP server's role shifts from "LLM-powered query generator" to:
1. **Context provider** — Rich semantic metadata via ontology
2. **Guardrails** — Validation, conventions enforcement, injection detection
3. **Coordinator** — Orchestrates advanced MCP features that leverage the CLIENT's intelligence

### Why This Works Better

| Dimension | Traditional (LLM Middleware) | Ekaya (Smart Context) |
|-----------|------------------------------|----------------------|
| Latency | 2x LLM calls | 1x LLM call |
| SQL accuracy | Dependent on middleware model | Claude's full reasoning |
| Token throughput | Limited by middleware | Claude's batched inference |
| Infrastructure | Vector DB + Embeddings + LLM hosting | Postgres only |
| Context freshness | Embedding lag | Real-time from schema |
| Reasoning depth | Constrained by middleware context window | Full client context |

**Real numbers**: Claude runs ~6x faster (tokens/sec) than typical local models and has superior reasoning for complex joins. Why add a slower, less capable model in the middle?

### MCP Advanced Features: Leveraging Client Intelligence

MCP provides three mechanisms that let a "dumb" server leverage a "smart" client:

#### 5.1 Prompts: Ontology-Driven Query Recipes

The server auto-generates MCP prompts from the ontology — no LLM required, just structured data:

```
prompts/list → ["revenue_analysis", "user_engagement", "billing_troubleshooting"]

prompts/get("revenue_analysis") →
{
  "name": "Revenue Analysis Patterns",
  "messages": [{
    "role": "user",
    "content": "When analyzing revenue in this database:

      **Key tables:** billing_transactions, billing_engagements

      **Conventions:**
      - Currency stored in cents → divide by 100 for display
      - Soft delete: always filter deleted_at IS NULL
      - Host earnings: use earned_amount (not total_amount)
      - Platform commission: tikr_share column

      **Entity roles:**
      - visitor_id = the user being charged
      - host_id = the user receiving payment

      **Common patterns:**
      - Revenue by host: GROUP BY payee_user_id, payee_username
      - Time filtering: created_at BETWEEN {{start}} AND {{end}}

      **Pre-approved queries available:**
      - 'Top hosts by revenue' (params: start_date, end_date, limit)
      - 'Daily revenue trend' (params: days_back)
      - 'Revenue by offer type' (params: start_date, end_date)"
  }]
}
```

**Implementation**: Pure template generation from:
- `engine_ontologies.conventions` → Currency, soft delete rules
- `engine_ontology_entities` → Entity names and key columns
- `engine_entity_relationships` → Role semantics (visitor/host)
- `engine_queries` → Available pre-approved queries
- `engine_project_knowledge` → Business rules and terminology

**No LLM call**. The prompt is assembled from structured data at request time.

#### 5.2 Elicitation: Rule-Based Disambiguation

When I write an ambiguous query, the server can ask for clarification without using an LLM:

```
Me: "Show me this quarter's revenue"
Server: detects date ambiguity, returns elicitation

{
  "type": "elicitation",
  "question": "Which quarter did you mean?",
  "options": [
    {
      "label": "Q4 2024 (Oct 1 - Dec 31)",
      "value": { "start": "2024-10-01", "end": "2025-01-01" }
    },
    {
      "label": "Q1 2025 (Jan 1 - Mar 31)",
      "value": { "start": "2025-01-01", "end": "2025-04-01" }
    }
  ],
  "context": "Project fiscal year starts October 1"
}

Me: presents options to user
User: selects Q4 2024
Me: re-executes with resolved dates
```

**Rule-based triggers** (no LLM):
- Date ranges: "this quarter", "last month", "YTD"
- Entity ambiguity: Multiple entities with same name
- Enum values: Unknown status value
- Fiscal calendar: Project-specific date logic from `engine_project_knowledge`

**Implementation**: Pattern matching on query text + project configuration. Date math, not language understanding.

#### 5.3 Sampling: Server Asks Claude to Do the Work

The most powerful inversion. Instead of running its own LLM, the server asks ME (via MCP sampling) to do reasoning:

**Use case: SQL validation with business context**

```
Me: calls query("SELECT * FROM users WHERE id = 'abc-123'")

Server: detects potential issue (id vs user_id confusion)
Server → Me (via sampling request):
  "The submitted query uses users.id (internal surrogate key, bigint)
   but the value 'abc-123' looks like a business identifier.

   Schema context:
   - users.id: bigint, internal primary key
   - users.user_id: text, business identifier (UUID format)

   Should this query use user_id instead of id?"

Me: "Yes, I meant user_id. The corrected query is:
     SELECT * FROM users WHERE user_id = 'abc-123'"

Server: executes corrected query, returns results with correction note
```

**Use case: Query recommendation enrichment**

```
Me: calls recommend_query(sql="SELECT payee_username, SUM(earned_amount)...")

Server: wants high-quality metadata without its own LLM
Server → Me (via sampling):
  "Given this SQL, please provide:
   1. A concise name (5 words max)
   2. A description of what business question it answers
   3. For each parameter, a user-friendly description
   4. Any constraints or assumptions users should know"

Me: returns structured metadata

Server: stores recommendation with my enrichment
```

**Use case: Result explanation for non-technical users**

```
User: "Why is revenue down this month?"
Me: executes comparison query, gets results
Me: asks server for explanation help via sampling

Server → Me (via sampling):
  "Given these results and the user's question, explain:
   - billing_transactions decreased 23% month-over-month
   - Top host 'damon-and' had 0 transactions (vs 42 last month)
   - New host signups down 15%

   Provide a business-friendly explanation."

Me: synthesizes explanation for user
```

**Why this works**: The server contributes structured context (schema, relationships, conventions). I contribute reasoning. Neither needs to duplicate the other's capability.

### What Ekaya DOESN'T Need

| Component | Traditional Products | Ekaya |
|-----------|---------------------|-------|
| Vector database | Required for RAG | Not needed |
| Embedding model | Required for RAG | Not needed |
| SQL-generating LLM | Required | Not needed |
| Fine-tuned model | Often required | Not needed |
| GPU infrastructure | Required for inference | Not needed |

**Ekaya needs**: Postgres + structured ontology + MCP server. That's it.

### The Competitive Moat

Other products can copy features. They can't easily copy this architecture because:

1. **They've built on LLM middleware** — Ripping it out means rewriting everything
2. **They've invested in RAG** — Sunk cost in vector infrastructure
3. **MCP is new** — Most products don't support advanced features (Prompts, Elicitation, Sampling)

Ekaya's architecture is **designed for the AI-native future** where:
- Client LLMs are fast, cheap, and highly capable
- Context is more valuable than computation
- Structured metadata beats fuzzy embeddings

### Implementation: MCP Advanced Features

**Phase 1: Prompts from Ontology**

New endpoint: `prompts/list` and `prompts/get`

```go
// pkg/mcp/prompts/ontology_prompts.go

func GenerateQueryPrompt(ctx context.Context, projectID uuid.UUID, domain string) *mcp.Prompt {
    // Fetch ontology
    ontology := ontologyRepo.GetActive(ctx, projectID)
    entities := entityRepo.GetByDomain(ctx, projectID, domain)
    queries := queryRepo.ListEnabled(ctx, projectID)
    knowledge := knowledgeRepo.GetAll(ctx, projectID)

    // Build prompt from structured data (no LLM)
    return &mcp.Prompt{
        Name: fmt.Sprintf("%s_patterns", domain),
        Messages: []mcp.PromptMessage{{
            Role: "user",
            Content: buildPromptContent(ontology, entities, queries, knowledge),
        }},
    }
}
```

**Phase 2: Rule-Based Elicitation**

```go
// pkg/mcp/elicitation/date_resolver.go

type DateElicitor struct {
    fiscalYearStart time.Month  // From project_knowledge
}

func (e *DateElicitor) Resolve(query string) *mcp.Elicitation {
    if matches := datePatterns.FindStringSubmatch(query); matches != nil {
        ambiguity := e.detectAmbiguity(matches[1])  // "this quarter", etc.
        if ambiguity != nil {
            return &mcp.Elicitation{
                Question: ambiguity.Question,
                Options:  ambiguity.Options,
            }
        }
    }
    return nil
}
```

**Phase 3: Sampling Integration**

```go
// pkg/mcp/sampling/client.go

func (s *SamplingClient) AskClientForHelp(ctx context.Context, prompt string) (string, error) {
    // MCP sampling request - asks the CLIENT (Claude) to generate a response
    return s.mcpServer.CreateSamplingRequest(ctx, mcp.SamplingRequest{
        Messages: []mcp.Message{{Role: "user", Content: prompt}},
        MaxTokens: 500,
    })
}
```

### Summary: The "No LLM in the Middle" Principle

```
┌─────────────────────────────────────────────────────────────────┐
│                         EKAYA PHILOSOPHY                        │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│   ❌  Don't run inference on the server                         │
│   ❌  Don't embed documents for RAG                             │
│   ❌  Don't add latency between client and data                 │
│                                                                 │
│   ✅  DO provide rich structured context                        │
│   ✅  DO leverage client LLM via MCP Sampling                   │
│   ✅  DO use rule-based disambiguation (Elicitation)            │
│   ✅  DO generate prompts from ontology (not embeddings)        │
│                                                                 │
│   The smartest model is already in the conversation.            │
│   Don't duplicate it. Empower it.                               │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

---

## Implementation Priority

### Phase 1: Foundation (High Impact, Moderate Effort)

1. **`get_context` unified tool** - Consolidates 3 tools, graceful degradation
2. **`suggest_knowledge`** - Leverages existing `engine_project_knowledge` table
3. **Ontology status in responses** - Simple addition to existing tools
4. **MCP Prompts from ontology** - Auto-generate query recipes (no LLM)

### Phase 2: Query Intelligence (High Impact, Higher Effort)

5. **`recommend_query`** - Requires UI work for approval flow
6. **Query history** - New table + tool
7. **Rule-based Elicitation** - Date disambiguation, entity resolution

### Phase 3: Power Features (Medium Impact)

8. **`enhance_entity`** - Alias and key column additions
9. **`search_schema`** - Full-text search (no embeddings, just trigram/GIN)
10. **`explain_query`** - Performance insights

### Phase 4: Advanced MCP Features (Requires Client Support)

11. **MCP Sampling integration** - Server asks client for help
12. **`enhance_relationship`** - Requires careful permission model
13. **Query favorites** - User preference storage

---

## Technical Considerations

### Backwards Compatibility

- Keep existing tools (`get_schema`, `get_ontology`, `get_glossary`) working
- Add deprecation notices in tool descriptions
- New tools are additive, not replacing

### Security

- All contribution tools require authentication
- Agent API keys have limited write access (suggest only, not direct modify)
- SQL injection detection on all recommended queries
- Rate limiting on contribution endpoints

### Token Efficiency

The unified `get_context` tool should reduce typical token usage:

| Current Pattern | Tokens | New Pattern | Tokens |
|-----------------|--------|-------------|--------|
| `get_ontology(domain)` + `get_schema` | ~9k | `get_context(entities)` | ~3k |
| `get_schema` + `get_glossary` | ~8.5k | `get_context(tables)` | ~4k |

### Database Changes

**New tables:** None required (reuse existing)

**Schema changes:**
```sql
-- engine_queries additions
ALTER TABLE engine_queries ADD COLUMN status VARCHAR DEFAULT 'approved';
ALTER TABLE engine_queries ADD COLUMN recommended_by VARCHAR;
ALTER TABLE engine_queries ADD COLUMN recommendation_context JSONB;

-- engine_project_knowledge additions
ALTER TABLE engine_project_knowledge ADD COLUMN confidence VARCHAR DEFAULT 'high';
ALTER TABLE engine_project_knowledge ADD COLUMN status VARCHAR DEFAULT 'approved';
ALTER TABLE engine_project_knowledge ADD COLUMN suggested_by VARCHAR;
```

---

## Success Metrics

After implementation, measure:

1. **Tool call reduction** - Fewer calls needed per task
2. **Knowledge accumulation** - Facts added via `suggest_knowledge`
3. **Query reuse** - Recommended queries that get approved and used
4. **Token efficiency** - Reduction in context tokens per session
5. **Time to first query** - How quickly Claude can write correct SQL

---

## Appendix: Current Tool Inventory

| Tool | Purpose | Keep/Deprecate |
|------|---------|----------------|
| `health` | Health check | Keep |
| `echo` | Auth testing | Keep |
| `query` | Execute SQL | Keep |
| `sample` | Quick preview | Keep |
| `execute` | DDL/DML | Keep |
| `validate` | SQL validation | Keep |
| `get_schema` | Schema + entities | Deprecate → `get_context` |
| `get_ontology` | Ontology tiers | Deprecate → `get_context` |
| `get_glossary` | Business terms | Deprecate → `get_context` |
| `list_approved_queries` | List queries | Keep |
| `execute_approved_query` | Run query | Keep |

---

## Closing Thoughts

The current MCP tools treat me as a **consumer** of a static knowledge base. These proposals transform me into a **collaborator** who can:

1. **Learn progressively** - Start shallow, go deep only when needed
2. **Contribute knowledge** - Capture insights that persist
3. **Suggest improvements** - Recommend queries for the team

The underlying infrastructure already exists (ontology tables, knowledge table, query system). These proposals are about exposing that infrastructure through the MCP interface in a way that makes AI agents first-class participants in the knowledge ecosystem.

### What Makes Ekaya Different

Most AI-to-database products ask: *"How do we make our LLM write better SQL?"*

Ekaya asks: *"How do we give the client's LLM everything it needs to write perfect SQL?"*

This is a fundamentally different bet:
- **They bet on their model** — fine-tuning, RAG, prompt engineering
- **Ekaya bets on context** — rich ontology, structured metadata, MCP features

The client LLMs (Claude, GPT-4, Gemini) are getting faster, cheaper, and smarter every quarter. Middleware LLMs add latency and potential errors. The winning architecture minimizes hops between the user's intent and the data.

### The Vision

```
Today:     Claude + Ekaya = Correct SQL on first try
Tomorrow:  Claude + Ekaya = Self-improving knowledge base
Future:    Claude + Ekaya = The database explains itself
```

Ekaya isn't competing to be the best SQL-writing AI. It's competing to be the **best context layer for AI-powered data access**. That's a moat that grows with every ontology extraction, every knowledge suggestion, every approved query.

---

*"The best database documentation is the one that writes itself through use."*

*"The smartest model is already in the conversation. Don't duplicate it. Empower it."*
