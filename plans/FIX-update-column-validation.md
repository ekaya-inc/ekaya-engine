# FIX: update_column Accepts Non-Existent Tables/Columns

## Bug Report

**Source:** Claude Chat via ClaudeDB
**Severity:** Low
**Component:** `update_column` MCP tool

### Reproduction

```
update_column(table="nonexistent_table", column="foo", description="test")
```

**Response:**
```json
{"table":"nonexistent_table","column":"foo","description":"test","created":true}
```

**Verification:**
- `get_ontology(depth="columns", tables=["nonexistent_table"])` returns `{"tables":{}}`
- `get_schema()` does not show the table
- Data is either not persisted or orphaned (not surfaced in views)

### Root Cause

Looking at `pkg/mcp/tools/column.go:85-202`:

1. **No schema validation** - The tool writes column metadata directly to the ontology's `column_details` JSONB field without checking if the table/column exists in the schema registry (`engine_schema_tables`/`engine_schema_columns`).

2. **Data is orphaned** - The metadata IS persisted to the ontology, but since there's no corresponding table in the schema registry, `get_ontology(depth="columns")` filters it out when building the response.

3. **Misleading response** - Returns `{"created": true}` suggesting success, but the data is effectively unusable.

### Current Flow

```
update_column(table, column)
  → GetActive ontology
  → Get/create column in ontology.ColumnDetails[table]
  → Save to ontology (NO SCHEMA CHECK)
  → Return {"created": true}
```

### Why This Matters

In the "Claude creates database" workflow, Claude might:
1. Create tables via `execute()`
2. Forget to call `refresh_schema()`
3. Call `update_column()` - appears to succeed
4. Later discover the ontology data is invisible

The tool should catch this mistake immediately.

---

## Proposed Fix

### Option A: Strict Validation (Recommended)

Validate that table and column exist in schema registry before persisting. Return an error if not found.

**Implementation:**

```go
// In registerUpdateColumnTool handler, after getting required params:

// Get datasourceID from project
datasourceID, err := deps.DatasourceService.GetPrimaryDatasourceID(tenantCtx, projectID)
if err != nil {
    return nil, fmt.Errorf("failed to get datasource: %w", err)
}

// Validate table exists in schema registry
schemaTable, err := deps.SchemaRepo.FindTableByName(tenantCtx, projectID, datasourceID, table)
if err != nil {
    return nil, fmt.Errorf("failed to lookup table: %w", err)
}
if schemaTable == nil {
    return NewErrorResult("TABLE_NOT_FOUND",
        fmt.Sprintf("table '%s' not found in schema registry. Run refresh_schema() after creating tables.", table))
}

// Validate column exists
schemaColumn, err := deps.SchemaRepo.GetColumnByName(schemaTable.ID, column)
if err != nil {
    return nil, fmt.Errorf("failed to lookup column: %w", err)
}
if schemaColumn == nil {
    return NewErrorResult("COLUMN_NOT_FOUND",
        fmt.Sprintf("column '%s' not found in table '%s'", column, table))
}

// Continue with existing logic...
```

**Pros:**
- Catches errors immediately
- Clear guidance to run `refresh_schema()`
- Prevents orphaned data

**Cons:**
- Breaking change for any existing workflows that rely on creating orphan metadata

### Option B: Warning in Response

Allow the operation but return a warning in the response.

```go
// After looking up schema
var warning string
if schemaTable == nil {
    warning = fmt.Sprintf("table '%s' not found in schema registry - metadata may not be surfaced", table)
} else if schemaColumn == nil {
    warning = fmt.Sprintf("column '%s' not found in table '%s' - metadata may not be surfaced", column, table)
}

// In response:
response := updateColumnResponse{
    Table:   table,
    Column:  column,
    // ... other fields
    Warning: warning,  // Add this field
}
```

**Pros:**
- Non-breaking
- Still informs the caller

**Cons:**
- Caller might ignore warning
- Still creates orphaned data

---

## Recommendation

**Go with Option A (strict validation).**

Rationale:
1. The "Claude creates database" workflow is new - no existing workflows to break
2. Orphaned data provides zero value and confuses the caller
3. The error message guides the user to the solution (`refresh_schema()`)
4. Uses the error result helper from FIX-mcp-error-handling-and-logging.md

---

## Implementation Tasks

### Dependencies

1. [ ] Add `SchemaRepo` to `ColumnToolDeps`
2. [ ] Add `DatasourceService` (or method to get datasourceID from projectID)

### Code Changes

1. [ ] Update `ColumnToolDeps` struct:
   ```go
   type ColumnToolDeps struct {
       DB               *database.DB
       MCPConfigService services.MCPConfigService
       OntologyRepo     repositories.OntologyRepository
       SchemaRepo       repositories.SchemaRepository  // NEW
       DatasourceService services.DatasourceService    // NEW (or just datasourceID lookup)
       Logger           *zap.Logger
   }
   ```

2. [ ] Add validation logic to `registerUpdateColumnTool` before saving

3. [ ] Update tool registration in `pkg/mcp/server.go` to pass new deps

4. [ ] Apply same fix to `delete_column_metadata` tool (validate table/column before deleting)

### Testing

1. [ ] Test: `update_column` with non-existent table returns `TABLE_NOT_FOUND` error
2. [ ] Test: `update_column` with valid table but non-existent column returns `COLUMN_NOT_FOUND` error
3. [ ] Test: `update_column` with valid table/column succeeds as before
4. [ ] Test: `delete_column_metadata` with non-existent table returns appropriate error

---

## Related Issues

- **FIX-claude-db-creation-workflow.md** - Adds `refresh_schema()` tool that Claude should call after creating tables
- **FIX-mcp-error-handling-and-logging.md** - Provides `NewErrorResult()` helper for returning errors in result payload
