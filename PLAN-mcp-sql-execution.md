# PLAN: MCP SQL Execution Layer

## Purpose

Define how SQL queries are executed through the MCP interface, including permissions, limits, timeouts, and result formatting. This layer sits between the LLM's generated SQL and the actual database execution.

**Scope:** This plan covers the execution mechanics. Query generation is covered in PLAN-mcp-ontology.md.

---

## Current State

The existing MCP server provides:

| Tool | SQL Type | Limits | Current Behavior |
|------|----------|--------|------------------|
| `query` | SELECT only | 1000 rows | Direct execution, read-only |
| `execute` | DDL/DML | None | Requires explicit enablement |
| `execute_approved_query` | Any (pre-approved) | 1000 rows | Parameter substitution |

---

## Execution Model

### Query Types

```
┌─────────────────────────────────────────────────────────────────┐
│                    SQL Execution Categories                      │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  READ (query tool)                                               │
│  ├── SELECT statements                                           │
│  ├── WITH (CTE) that ends in SELECT                              │
│  └── EXPLAIN/EXPLAIN ANALYZE                                     │
│                                                                  │
│  WRITE (execute tool - requires explicit enablement)             │
│  ├── INSERT, UPDATE, DELETE                                      │
│  ├── MERGE/UPSERT                                                │
│  └── TRUNCATE                                                    │
│                                                                  │
│  DDL (execute tool - requires explicit enablement)               │
│  ├── CREATE TABLE/VIEW/INDEX                                     │
│  ├── ALTER TABLE                                                 │
│  ├── DROP (dangerous!)                                           │
│  └── GRANT/REVOKE                                                │
│                                                                  │
│  APPROVED (execute_approved_query)                               │
│  └── Any SQL that admin has pre-approved                         │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

---

## Tool Specifications

### Tool: query

**Purpose:** Execute read-only SELECT statements

```
Input:
{
  "sql": "SELECT * FROM users WHERE created_at > '2024-01-01'",
  "limit": 100,  // Optional: default 100, max 1000
  "explain": false,  // Optional: return query plan instead of results
  "natural_language_context": "users created this year"  // Optional: for history
}

Output (success):
{
  "columns": [
    {"name": "id", "type": "uuid"},
    {"name": "name", "type": "text"},
    {"name": "created_at", "type": "timestamptz"}
  ],
  "rows": [
    {"id": "abc123", "name": "John", "created_at": "2024-03-15T10:00:00Z"},
    ...
  ],
  "row_count": 47,
  "truncated": false,
  "execution_time_ms": 145,
  "query_id": "uuid"  // For feedback/history
}

Output (explain mode):
{
  "plan": [
    "Seq Scan on users  (cost=0.00..25.00 rows=500 width=64)",
    "  Filter: (created_at > '2024-01-01'::date)"
  ],
  "estimated_rows": 500,
  "execution_time_ms": 12
}

Output (error):
{
  "error": true,
  "error_type": "syntax_error",
  "message": "syntax error at or near 'SELEC'",
  "position": 1,
  "hint": "Did you mean 'SELECT'?",
  "sql_state": "42601"
}
```

**Validation Rules:**
- Must start with SELECT or WITH
- No data-modifying CTEs (WITH ... INSERT/UPDATE/DELETE)
- No function calls that modify data

---

### Tool: execute

**Purpose:** Execute DDL/DML statements (when enabled)

```
Input:
{
  "sql": "INSERT INTO orders (customer_id, total) VALUES ('abc', 100.00) RETURNING id",
  "confirm_destructive": false  // Required true for DROP/TRUNCATE/DELETE without WHERE
}

Output (success with RETURNING):
{
  "columns": [{"name": "id", "type": "uuid"}],
  "rows": [{"id": "new-uuid-123"}],
  "rows_affected": 1,
  "execution_time_ms": 45
}

Output (success without RETURNING):
{
  "rows_affected": 1,
  "message": "1 row inserted",
  "execution_time_ms": 45
}

Output (blocked - destructive):
{
  "error": true,
  "error_type": "confirmation_required",
  "message": "DELETE without WHERE clause affects all rows. Set confirm_destructive: true to proceed.",
  "estimated_affected_rows": 50000
}
```

**Safety Rules:**
- DROP TABLE/DATABASE requires `confirm_destructive: true`
- DELETE/TRUNCATE without WHERE requires `confirm_destructive: true`
- All executions logged to audit trail
- 30-second timeout (configurable)

---

### Tool: execute_approved_query

**Purpose:** Execute pre-approved parameterized queries

```
Input:
{
  "query_id": "uuid",
  "parameters": {
    "customer_id": "abc123",
    "start_date": "2024-01-01"
  },
  "limit": 100  // Optional
}

Output:
{
  "columns": [...],
  "rows": [...],
  "row_count": 47,
  "truncated": false,
  "execution_time_ms": 145,
  "query_name": "Customer Orders Report",
  "parameters_used": {
    "customer_id": "abc123",
    "start_date": "2024-01-01"
  }
}
```

**Security:**
- SQL is not exposed to MCP client (unless allow_suggestions is on)
- Parameters are type-checked and sanitized
- SQL injection attempts are detected and logged

---

## Limits & Quotas

### Row Limits

| Context | Default | Max | Configurable |
|---------|---------|-----|--------------|
| query tool | 100 | 1000 | Per project |
| approved queries | 100 | 1000 | Per project |
| execute with RETURNING | 100 | 1000 | Per project |

### Timeout Limits

| Operation | Default | Max | Configurable |
|-----------|---------|-----|--------------|
| SELECT queries | 30s | 120s | Per project |
| DDL/DML | 30s | 60s | Per project |
| Approved queries | 60s | 120s | Per project |

### Rate Limits (Future)

| Limit | Default | Window |
|-------|---------|--------|
| Queries per minute | 60 | 1 minute |
| Total rows per hour | 100,000 | 1 hour |
| Concurrent queries | 5 | N/A |

---

## Error Handling

### Error Categories

```
┌─────────────────────────────────────────────────────────────────┐
│ Error Type           │ User-Facing Message                     │
├──────────────────────┼─────────────────────────────────────────┤
│ syntax_error         │ SQL syntax error with position          │
│ column_not_found     │ Column X doesn't exist, suggest similar │
│ table_not_found      │ Table X doesn't exist, suggest similar  │
│ permission_denied    │ User doesn't have access to this table  │
│ timeout              │ Query exceeded time limit               │
│ row_limit_exceeded   │ Results truncated to limit              │
│ rate_limit_exceeded  │ Too many queries, try again later       │
│ connection_error     │ Database connection failed              │
│ validation_failed    │ Query failed pre-execution validation   │
└──────────────────────┴─────────────────────────────────────────┘
```

### Error Response Format

```json
{
  "error": true,
  "error_type": "column_not_found",
  "message": "Column 'usres.name' does not exist",
  "sql_state": "42703",
  "position": 7,
  "suggestions": [
    {"correction": "users.name", "reason": "Likely typo in table alias"}
  ],
  "context": {
    "available_columns": ["users.id", "users.name", "users.email"],
    "similar_columns": ["users.name"]
  }
}
```

**LLM-Friendly Errors:**
- Include suggestions for common mistakes
- Provide context for self-correction
- Reference available columns/tables when relevant

---

## Permission Model

### Table-Level Permissions

Permissions are derived from the admin-approved schema:

```
┌─────────────────────────────────────────────────────────────────┐
│                    Permission Hierarchy                          │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  Admin selects tables in Schema view                             │
│            ↓                                                     │
│  Selected tables → engine_schema_tables.is_selected = true       │
│            ↓                                                     │
│  MCP queries can only access selected tables                     │
│            ↓                                                     │
│  Database-level permissions are final enforcement                │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

### Query Validation Pipeline

```
1. Parse SQL (syntax check)
        ↓
2. Extract referenced tables
        ↓
3. Check tables against is_selected
        ↓
4. If approved_queries_only mode:
   - Reject dynamic SQL
   - Only allow execute_approved_query
        ↓
5. Pass to database with connection credentials
        ↓
6. Database enforces final permissions
```

---

## Result Formatting

### Data Type Handling

| Database Type | JSON Representation | Notes |
|--------------|---------------------|-------|
| uuid | string | As-is |
| integer/bigint | number | Safe for JavaScript |
| numeric/decimal | string | Preserve precision |
| text/varchar | string | As-is |
| boolean | boolean | true/false |
| date | string | ISO 8601 (YYYY-MM-DD) |
| timestamp/timestamptz | string | ISO 8601 with timezone |
| json/jsonb | object/array | Parsed JSON |
| bytea | string | Base64 encoded |
| array | array | Native JSON array |
| NULL | null | JSON null |

### Large Value Handling

| Scenario | Handling |
|----------|----------|
| Text > 10KB | Truncate with "...[truncated]" marker |
| Binary data | Base64 encode, include size |
| Very wide tables (>50 cols) | Include all, warn about size |

---

## Execution Context

### Connection Management

```go
type ExecutionContext struct {
    ProjectID     uuid.UUID
    UserID        string
    DatasourceID  uuid.UUID
    SessionID     string

    // Limits
    RowLimit      int
    TimeoutMS     int

    // Audit
    NaturalLanguageContext string
    ToolName      string
}
```

### Connection Pooling

- Use existing datasource connection pools
- Acquire connection with project context
- Set statement timeout before execution
- Return connection to pool after use

---

## Transaction Handling

### Default Behavior
- Each query/execute runs in auto-commit mode
- No multi-statement transactions via MCP (for now)

### Future: Explicit Transactions
```
begin_transaction() → returns transaction_id
query(sql, transaction_id) → uses same transaction
commit_transaction(transaction_id)
rollback_transaction(transaction_id)
```

**Considerations:**
- Transaction timeout (prevent stuck connections)
- Maximum transaction duration
- Audit logging for transaction boundaries

---

## Query Cancellation (Future)

### Cancel Long-Running Query

```
Tool: cancel_query
Input:
{
  "query_id": "uuid"  // From query response
}

Output:
{
  "cancelled": true,
  "message": "Query cancelled after 15.3 seconds"
}
```

**Implementation:**
- Map query_id to backend PID
- Send pg_cancel_backend()
- Clean up resources

---

## Implementation Notes

### Refactoring Existing Code

The current `query` and `execute` tools in `pkg/mcp/tools/developer.go` should be:

1. Extracted to a shared execution layer
2. Enhanced with better error formatting
3. Integrated with audit logging
4. Made configurable via project settings

### Execution Flow

```
MCP Tool Handler
       ↓
Validation Layer (syntax, permissions)
       ↓
Audit Logger (pre-execution)
       ↓
Query Executor
       ↓
Result Formatter
       ↓
Audit Logger (post-execution)
       ↓
Return to MCP Client
```

---

## Configuration Options

### Project-Level Settings

```go
type QueryExecutionConfig struct {
    // Limits
    DefaultRowLimit     int  // Default: 100
    MaxRowLimit         int  // Default: 1000
    QueryTimeoutSeconds int  // Default: 30
    ExecuteTimeoutSeconds int // Default: 30

    // Features
    AllowExplain        bool // Default: true
    AllowDynamicSQL     bool // Default: true (false = approved only)
    AllowDDL            bool // Default: false
    AllowDML            bool // Default: false

    // Safety
    RequireConfirmDestructive bool // Default: true
    LogAllQueries       bool       // Default: true
}
```

---

## File Changes Summary

| Area | Changes |
|------|---------|
| `pkg/mcp/tools/` | Refactor query/execute into execution layer |
| `pkg/adapters/datasource/` | Enhance error formatting, timeout handling |
| `pkg/models/` | Query execution config model |
| `pkg/services/` | Execution service with validation pipeline |
| `pkg/repositories/` | Config storage for execution settings |
