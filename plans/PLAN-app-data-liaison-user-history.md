# PLAN: MCP User Query History

## Purpose

Enable the MCP client to learn from past queries - both the user's own history and successful patterns across the project. This creates a feedback loop where the system gets smarter over time.

**Core Insight:** The best query for a new question is often an adaptation of a query that worked before.

---

## Use Cases

### 1. Query Reuse
User asks: "Show me top customers by revenue"
System finds: Previous query "top 10 customers by order value" with 95% similarity
LLM adapts: Reuses the query structure, adjusts column names if needed

### 2. Error Avoidance
User asks: "Orders by region"
System finds: Previous query failed because "region" column doesn't exist, user clarified they meant "shipping_state"
LLM learns: Suggest clarification before generating SQL

### 3. Pattern Discovery
User frequently queries: "X by month for last year"
System learns: This user prefers monthly aggregations with 12-month lookback

---

## Data Model

### New Table: `engine_mcp_query_history`

```sql
CREATE TABLE engine_mcp_query_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id),
    user_id VARCHAR(255) NOT NULL,  -- From auth claims

    -- The query itself
    natural_language TEXT NOT NULL,
    generated_sql TEXT NOT NULL,
    final_sql TEXT,  -- May differ if user edited

    -- Execution details
    executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    execution_duration_ms INTEGER,
    row_count INTEGER,
    was_successful BOOLEAN NOT NULL,
    error_message TEXT,

    -- Learning signals
    user_feedback VARCHAR(20),  -- 'helpful', 'not_helpful', 'edited', NULL
    feedback_comment TEXT,

    -- Query classification (for similarity search)
    query_type VARCHAR(50),  -- 'aggregation', 'lookup', 'report', 'exploration'
    tables_used TEXT[],  -- ['users', 'orders']
    aggregations_used TEXT[],  -- ['SUM', 'COUNT', 'AVG']
    time_filters JSONB,  -- {"type": "relative", "period": "last_quarter"}

    -- Embedding for similarity search (optional, for advanced similarity)
    question_embedding VECTOR(1536),  -- If using pgvector

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT fk_project FOREIGN KEY (project_id) REFERENCES engine_projects(id)
);

-- Indexes
CREATE INDEX idx_query_history_user ON engine_mcp_query_history(project_id, user_id, created_at DESC);
CREATE INDEX idx_query_history_tables ON engine_mcp_query_history USING GIN(tables_used);
CREATE INDEX idx_query_history_success ON engine_mcp_query_history(project_id, was_successful, created_at DESC);
```

---

## MCP Tools

### Tool: get_query_history

```
Purpose: Retrieve the user's recent query history

Input:
{
  "limit": 20,                    // Optional: default 20, max 100
  "filter": "all",                // Optional: "all", "successful", "failed"
  "tables": ["orders"],           // Optional: filter by tables used
  "since": "2024-01-01"           // Optional: filter by date
}

Output:
{
  "queries": [
    {
      "id": "uuid",
      "natural_language": "Show me top 10 customers by total orders",
      "sql": "SELECT u.name, COUNT(o.id) as order_count FROM users u JOIN orders o ON o.customer_id = u.id GROUP BY u.id, u.name ORDER BY order_count DESC LIMIT 10",
      "executed_at": "2024-12-15T10:30:00Z",
      "was_successful": true,
      "row_count": 10,
      "execution_duration_ms": 145,
      "tables_used": ["users", "orders"],
      "query_type": "aggregation",
      "user_feedback": "helpful"
    },
    {
      "id": "uuid",
      "natural_language": "Orders by region last month",
      "sql": "SELECT region, COUNT(*) FROM orders WHERE created_at >= '2024-11-01' GROUP BY region",
      "executed_at": "2024-12-14T15:00:00Z",
      "was_successful": false,
      "error_message": "column \"region\" does not exist",
      "tables_used": ["orders"],
      "query_type": "aggregation"
    }
  ],
  "total_count": 156,
  "has_more": true
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

---

### Tool: find_similar_queries

```
Purpose: Find past queries semantically similar to a new question

Input:
{
  "natural_language": "Who are our biggest customers?",
  "limit": 5  // Optional: default 5
}

Output:
{
  "similar_queries": [
    {
      "id": "uuid",
      "original_question": "Show me top 10 customers by total orders",
      "sql": "SELECT u.name, COUNT(o.id) as order_count FROM users u JOIN orders o ON o.customer_id = u.id GROUP BY u.id, u.name ORDER BY order_count DESC LIMIT 10",
      "similarity_score": 0.92,
      "was_successful": true,
      "tables_used": ["users", "orders"],
      "can_reuse": true,
      "adaptation_hint": "Consider changing metric from order count to revenue if 'biggest' means by value"
    },
    {
      "id": "uuid",
      "original_question": "Top customers by revenue last year",
      "sql": "SELECT u.name, SUM(o.total_amount) as revenue FROM users u JOIN orders o ON o.customer_id = u.id WHERE o.created_at >= '2024-01-01' GROUP BY u.id, u.name ORDER BY revenue DESC LIMIT 10",
      "similarity_score": 0.88,
      "was_successful": true,
      "tables_used": ["users", "orders"],
      "can_reuse": true,
      "adaptation_hint": "Remove date filter if all-time is desired"
    }
  ],
  "recommendation": {
    "suggested_action": "adapt",
    "base_query_id": "uuid",
    "reasoning": "Previous query for 'top customers by revenue' is highly similar and was successful. Consider adapting this query."
  }
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

**Similarity Calculation Options:**

1. **Simple (V1):** TF-IDF or keyword matching on natural language
2. **Advanced (V2):** Embedding similarity using pgvector
3. **Structural:** Compare tables_used, aggregations_used, query patterns

---

### Tool: record_query_feedback

```
Purpose: Record whether a generated query was helpful

Input:
{
  "query_id": "uuid",             // Required: from query history
  "feedback": "helpful",          // Required: "helpful", "not_helpful", "edited"
  "comment": "Had to add date filter"  // Optional
}

Output:
{
  "recorded": true,
  "message": "Feedback recorded. Thank you for helping improve query suggestions."
}

MCP Annotations:
- read_only_hint: false
- destructive_hint: false
- idempotent_hint: true
```

---

### Tool: get_query_patterns

```
Purpose: Get common query patterns for this project/user

Input:
{
  "scope": "user"  // "user" or "project"
}

Output:
{
  "patterns": [
    {
      "pattern": "time_series_aggregation",
      "description": "Aggregations grouped by time period",
      "frequency": 45,  // Used 45 times
      "example_sql": "SELECT DATE_TRUNC('month', created_at), SUM(amount) FROM orders GROUP BY 1",
      "common_tables": ["orders", "invoices"],
      "common_metrics": ["SUM", "COUNT"]
    },
    {
      "pattern": "top_n_ranking",
      "description": "Top N items by some metric",
      "frequency": 32,
      "example_sql": "SELECT name, COUNT(*) as cnt FROM ... GROUP BY name ORDER BY cnt DESC LIMIT 10",
      "common_tables": ["users", "products"],
      "common_metrics": ["COUNT", "SUM"]
    }
  ],
  "preferences": {
    "default_limit": 10,
    "preferred_date_format": "relative",  // "relative" or "absolute"
    "common_time_ranges": ["last_month", "last_quarter", "ytd"]
  }
}

MCP Annotations:
- read_only_hint: true
- idempotent_hint: true
```

---

## Query Recording Flow

When a query is executed via MCP tools, automatically record it:

```
1. User sends natural language question
2. LLM generates SQL using ontology tools
3. LLM calls query() or execute_approved_query()
4. System intercepts and records:
   - Natural language (from conversation context or explicit parameter)
   - Generated SQL
   - Execution result
   - Tables used (parsed from SQL or from query plan)
5. Query ID returned in response for later feedback
```

### Recording Integration Point

Modify the `query` tool to optionally accept natural language context:

```go
// query tool input
{
  "sql": "SELECT ...",
  "limit": 100,
  "natural_language_context": "top customers by revenue"  // Optional
}
```

If provided, automatically creates a history entry.

---

## Privacy & Security Considerations

### Data Isolation
- Query history is scoped by `project_id` and `user_id`
- Users can only see their own history
- Project admins may see aggregated patterns (not individual queries) - future feature

### Retention Policy
- Default: 90 days retention
- Configurable per project
- Option to disable history recording entirely

### Sensitive Data
- SQL queries may contain literals (dates, IDs, etc.)
- Consider: Option to strip literals before storing
- Consider: Parameterize queries before storing

---

## Admin Audit Screen

### Overview

An **Audit** tile/screen appears in the **Intelligence** section of the UI for users with Admin role when the AI Data Liaison app is installed. This gives admins visibility into all MCP query activity across the project.

### Purpose

Admins need to understand how the AI Data Liaison is being used: who is querying what, how often, whether queries are succeeding, and what data is being accessed or modified. This is essential for governance, compliance, and understanding usage patterns.

### Location

- **Navigation:** Intelligence section (alongside existing tiles like Ontology)
- **Visibility:** Admin role only
- **Prerequisite:** AI Data Liaison app must be installed for the project

### Features

#### Search & Filter

Admins can search and filter the query history by:

- **User** - Filter by specific user or view all users
- **Time range** - Date picker for start/end, plus presets (today, last 7 days, last 30 days, custom)
- **Data modification** - Filter to show only queries that modified data (INSERT, UPDATE, DELETE) vs read-only (SELECT)
- **Tables/columns** - Filter by specific tables or columns referenced in the SQL (uses `tables_used` from history, plus parsed column references)
- **Success/failure** - Filter by execution outcome
- **Query type** - Filter by classification (aggregation, lookup, report, exploration)

#### Display

Each query entry shows:
- User who executed it
- Natural language question
- Generated SQL (expandable)
- Final SQL if edited (expandable, with diff highlighting)
- Execution timestamp
- Duration and row count
- Success/failure status with error details
- Tables and columns accessed
- Whether the query modified data

#### Export

- Export filtered results as CSV for compliance reporting

### Data Access

The audit screen queries the same `engine_mcp_query_history` table but without the `user_id` filter — admins see all users' queries within their project. Access is still scoped by `project_id`.

### API Endpoint

```
GET /api/projects/{project_id}/mcp/query-audit

Query params:
  user_id       - Filter by user (optional)
  from          - Start timestamp (optional)
  to            - End timestamp (optional)
  tables        - Comma-separated table names (optional)
  columns       - Comma-separated column names (optional)
  has_mutations - Boolean, filter to data-modifying queries (optional)
  status        - "successful", "failed", "all" (default: "all")
  query_type    - Filter by classification (optional)
  search        - Full-text search across natural_language and SQL (optional)
  limit         - Default 50, max 500
  offset        - For pagination

Requires: Admin role
```

---

## UI Considerations (Future)

While this PLAN focuses on the MCP interface, the history data enables:

1. **Query History Panel** - View/rerun past queries
2. **"Did you mean?" Suggestions** - During query building
3. **Query Templates** - Save and share successful queries
4. **Usage Analytics** - Most queried tables, common patterns

---

## Implementation Phases

### Phase 1: Core History
- Create table and migration
- Implement `get_query_history` tool
- Auto-record queries from `query` tool

### Phase 2: Similarity Search
- Implement `find_similar_queries` with keyword matching
- Add `record_query_feedback` tool
- Parse and store query metadata (tables, aggregations)

### Phase 3: Admin Audit Screen
- Add `/mcp/query-audit` API endpoint (admin-only, cross-user query access)
- Build Audit tile in Intelligence section (admin visibility, requires AI Data Liaison app)
- Implement search/filter UI (user, time range, tables/columns, mutations, status)
- Add CSV export for compliance reporting

### Phase 4: Advanced Matching
- Add vector embeddings for semantic similarity
- Implement `get_query_patterns`
- Add pattern detection algorithms

---

## Open Questions

1. **Should failed queries be recorded?**
   - Pro: Learn from mistakes
   - Con: Noise in similarity search
   - Recommendation: Record with flag, filter by default

2. **How to handle edited queries?**
   - Record both generated and final SQL?
   - Track diffs for learning?

3. **Cross-user learning?**
   - Should one user's successful queries help another?
   - Privacy implications
   - Recommendation: Aggregate patterns only, not specific queries

4. **Embedding model for similarity?**
   - Use OpenAI embeddings?
   - Use local model?
   - Start with keyword matching, add embeddings later
