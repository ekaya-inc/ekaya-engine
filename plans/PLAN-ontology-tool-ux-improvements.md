# PLAN: Ontology Tool UX Improvements

## Context

When an AI agent answers ontology questions using MCP tools, two friction points were identified:

1. **No easy way to inspect current state** before updating
2. **No batch updates** for repetitive patterns (e.g., 15 tables with `deleted_at`)

These are quick wins that improve the agent workflow without changing question structure.

## Task 1: Add Inspection Tools [x]

### Problem

To update a column, agents currently must:
1. Call `get_context(depth='columns', tables=['billing_transactions'])`
2. Parse large JSON response
3. Find the specific column in the response
4. Decide what to change

### Solution

Add focused inspection tools:

**`get_column_metadata`** - Returns current metadata for a single column

```go
// Tool definition
mcp.NewTool(
    "get_column_metadata",
    mcp.WithDescription(
        "Get current ontology metadata for a specific column. "+
        "Returns description, semantic_type, enum_values, entity, role, and other metadata. "+
        "Use this before update_column to see what's already documented.",
    ),
    mcp.WithString("table", mcp.Required(), mcp.Description("Table name")),
    mcp.WithString("column", mcp.Required(), mcp.Description("Column name")),
)
```

**Response:**
```json
{
  "table": "billing_transactions",
  "column": "transaction_state",
  "metadata": {
    "description": "Current state of the transaction",
    "semantic_type": "status",
    "enum_values": [
      {"value": "0", "label": "Pending"},
      {"value": "1", "label": "Started"}
    ],
    "entity": null,
    "role": null
  },
  "schema": {
    "data_type": "integer",
    "is_nullable": true,
    "is_primary_key": false
  }
}
```

**`get_entity_metadata`** - Returns current metadata for an entity

```go
mcp.NewTool(
    "get_entity_metadata",
    mcp.WithDescription(
        "Get current ontology metadata for a specific entity. "+
        "Returns description, aliases, key_columns, and relationships. "+
        "Use this before update_entity to see what's already documented.",
    ),
    mcp.WithString("name", mcp.Required(), mcp.Description("Entity name")),
)
```

**Response:**
```json
{
  "name": "User",
  "primary_table": "users",
  "description": "Platform user who can host or visit",
  "aliases": ["host", "visitor", "creator"],
  "key_columns": ["user_id", "username"],
  "relationships": [
    {"to": "Account", "label": "belongs_to", "cardinality": "N:1"}
  ]
}
```

### Implementation

**File:** `pkg/mcp/tools/ontology_inspect.go` (new file)

```go
func RegisterOntologyInspectionTools(s *server.MCPServer, deps *OntologyToolDeps) {
    registerGetColumnMetadataTool(s, deps)
    registerGetEntityMetadataTool(s, deps)
}
```

**Acceptance criteria:**
- `get_column_metadata(table, column)` returns focused column info
- `get_entity_metadata(name)` returns focused entity info
- Returns clear error if column/entity not found
- Response is concise (not the full context dump)

---

## Task 2: Add Batch Update Tools

### Problem

When the same pattern applies to multiple tables (e.g., GORM soft delete), agents must call `update_column` repeatedly:

```
update_column(table='users', column='deleted_at', description='Soft delete...')
update_column(table='accounts', column='deleted_at', description='Soft delete...')
update_column(table='sessions', column='deleted_at', description='Soft delete...')
... (15 more times)
```

### Solution

**`update_columns`** - Batch update multiple columns at once

```go
mcp.NewTool(
    "update_columns",
    mcp.WithDescription(
        "Update metadata for multiple columns in a single call. "+
        "Useful for applying the same pattern across tables (e.g., soft delete, audit timestamps). "+
        "Each update specifies table, column, and the fields to update.",
    ),
    mcp.WithArray(
        "updates",
        mcp.Required(),
        mcp.Description("Array of column updates, each with table, column, and metadata fields"),
    ),
)
```

**Request:**
```json
{
  "updates": [
    {
      "table": "users",
      "column": "deleted_at",
      "description": "Soft delete timestamp. NULL = active, timestamp = deleted."
    },
    {
      "table": "accounts",
      "column": "deleted_at",
      "description": "Soft delete timestamp. NULL = active, timestamp = deleted."
    },
    {
      "table": "sessions",
      "column": "deleted_at",
      "description": "Soft delete timestamp. NULL = active, timestamp = deleted."
    }
  ]
}
```

**Response:**
```json
{
  "updated": 3,
  "results": [
    {"table": "users", "column": "deleted_at", "status": "success"},
    {"table": "accounts", "column": "deleted_at", "status": "success"},
    {"table": "sessions", "column": "deleted_at", "status": "success"}
  ]
}
```

### Implementation

**File:** `pkg/mcp/tools/ontology_batch.go` (new file)

```go
type ColumnUpdateBatch struct {
    Table       string   `json:"table"`
    Column      string   `json:"column"`
    Description *string  `json:"description,omitempty"`
    EnumValues  []string `json:"enum_values,omitempty"`
    Entity      *string  `json:"entity,omitempty"`
    Role        *string  `json:"role,omitempty"`
    // ... other update_column fields
}

func registerUpdateColumnsTool(s *server.MCPServer, deps *OntologyToolDeps) {
    // Parse array of updates
    // Validate each update
    // Apply in transaction
    // Return batch result
}
```

**Acceptance criteria:**
- Accepts array of column updates
- Applies all updates in a single transaction (all-or-nothing)
- Returns per-update status
- Validates all updates before applying any
- Limit batch size (e.g., max 50 updates per call)

---

## Task 3: Enhance probe_column to Include Metadata

### Alternative to Task 1

Instead of adding new tools, enhance existing `probe_column` to include ontology metadata alongside statistics.

**Current `probe_column` response:**
```json
{
  "table": "users",
  "column": "status",
  "statistics": {
    "distinct_count": 5,
    "null_rate": 0.02,
    "sample_values": ["active", "suspended", "deleted"]
  }
}
```

**Enhanced response:**
```json
{
  "table": "users",
  "column": "status",
  "statistics": { ... },
  "metadata": {
    "description": "User account status",
    "semantic_type": "status",
    "enum_values": [...]
  }
}
```

**Implementation:**

**File:** `pkg/mcp/tools/column_tools.go`

Add metadata fetch to existing `probe_column` handler.

**Acceptance criteria:**
- `probe_column` response includes `metadata` field
- Metadata is fetched from ontology if available
- Backwards compatible (new field, existing fields unchanged)

---

## Recommendation

**Implement in this order:**

1. **Task 3 first** - Enhance `probe_column` (lowest effort, immediate value)
2. **Task 1 if needed** - Add dedicated inspection tools (if probe_column isn't sufficient)
3. **Task 2** - Batch updates (higher effort, significant time savings for repetitive work)

---

## Testing

1. **probe_column enhancement:**
   - Call on column with metadata → metadata returned
   - Call on column without metadata → metadata field empty/null
   - Performance: no significant slowdown

2. **Batch updates:**
   - Batch of valid updates → all succeed
   - Batch with one invalid → all rejected (transaction rollback)
   - Empty batch → error
   - Oversized batch (>50) → error with guidance

---

## Success Metrics

| Metric | Before | After |
|--------|--------|-------|
| Tool calls for soft-delete pattern (15 tables) | 15 | 1 |
| Steps to inspect before update | 3 (get_context → parse → find) | 1 |
| Agent knows current state before update | Often skipped | Easy to check |
