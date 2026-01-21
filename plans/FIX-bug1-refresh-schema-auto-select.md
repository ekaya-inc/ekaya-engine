# FIX: Bug 1 - MCP refresh_schema auto_select Does Not Update UI State

**Priority:** Medium
**Component:** MCP Server / Schema Selection

## Problem Statement

When calling `refresh_schema` with `auto_select: true`, the MCP server reports that tables were auto-selected, but the UI Schema Selection page shows them as unchecked.

**Reproduction Steps:**
1. Create new tables in the datasource database
2. Call MCP `refresh_schema` with `auto_select: true`
3. Response shows `"auto_select_applied": true` and lists tables in `tables_added`
4. Navigate to UI Schema Selection page
5. Newly added tables show as unchecked

## Root Cause Analysis

### Code Flow Traced

**Step 1: MCP Tool Handler** (`pkg/mcp/tools/developer.go:871-996`)

```go
// Line 905: Parse auto_select parameter (defaults to true)
autoSelect := getOptionalBoolWithDefaultDev(req, "auto_select", true)

// Line 908: Refresh schema from datasource
result, err := deps.SchemaService.RefreshDatasourceSchema(tenantCtx, projectID, dsID)

// Lines 919-928: Auto-select if condition met
if autoSelect && len(result.NewTableNames) > 0 {
    if err := deps.SchemaService.SelectAllTables(tenantCtx, dsID); err != nil {
        deps.Logger.Warn("Failed to auto-select tables", ...)
        // Don't fail the entire operation for selection failure
    }
}

// Line 985: Response field set based on CONDITION, not actual success
AutoSelectApplied: autoSelect && len(result.NewTableNames) > 0,
```

**Issue 1:** `AutoSelectApplied` is set based on the input condition, not the actual result of `SelectAllTables`. If `SelectAllTables` fails silently (warning logged), the response still claims `auto_select_applied: true`.

**Step 2: Schema Service** (`pkg/services/schema.go:1221-1227`)

```go
func (s *schemaService) SelectAllTables(ctx context.Context, datasourceID uuid.UUID) error {
    if err := s.schemaRepo.SelectAllTablesAndColumns(ctx, datasourceID); err != nil {
        return fmt.Errorf("failed to select all tables: %w", err)
    }
    return nil
}
```

**Step 3: Repository Update** (`pkg/repositories/schema_repository.go:1677-1708`)

```go
func (r *schemaRepository) SelectAllTablesAndColumns(ctx context.Context, datasourceID uuid.UUID) error {
    scope, ok := database.GetTenantScope(ctx)
    if !ok {
        return fmt.Errorf("no tenant scope in context")
    }

    // Update all tables to selected
    _, err := scope.Conn.Exec(ctx, `
        UPDATE engine_schema_tables
        SET is_selected = true, updated_at = NOW()
        WHERE datasource_id = $1 AND deleted_at IS NULL
    `, datasourceID)
    // ...columns update follows...
}
```

**Step 4: Table Creation During Refresh** (`pkg/services/schema.go:163-173`)

```go
table := &models.SchemaTable{
    ProjectID:    projectID,
    DatasourceID: datasourceID,
    SchemaName:   dt.SchemaName,
    TableName:    dt.TableName,
    RowCount:     &dt.RowCount,
    // NOTE: IsSelected not set, defaults to false
}

if err := s.schemaRepo.UpsertTable(ctx, table); err != nil { ... }
```

**Issue 2:** New tables are created with `IsSelected: false` (Go zero value). The upsert at line 259 inserts this value, so new tables start as unselected.

**Step 5: UI Reading** (`pkg/handlers/schema.go:705`, `pkg/services/schema.go:467`)

The UI reads `IsSelected` from `engine_schema_tables.is_selected` via:
```go
// Handler converts to response
IsSelected: table.IsSelected,

// Service reads from repository model
dt := &models.DatasourceTable{
    IsSelected: t.IsSelected,
}
```

### Potential Root Causes

1. **Response Inaccuracy (Lines 985 vs 920-927):** The `AutoSelectApplied` flag doesn't reflect actual success. If `SelectAllTables` errors (logged as warning), the response still says `true`.

2. **Timing/Transaction Issue:** `RefreshDatasourceSchema` and `SelectAllTables` may run in separate implicit transactions. If the UI reads before `SelectAllTables` commits, it sees `is_selected = false`.

3. **RLS Policy Mismatch:** The UPDATE query filters by `datasource_id` only (line 1688), while RLS policy filters by `project_id`. If RLS isn't properly set, the update might affect wrong rows or none.

## Recommended Fix

### Fix 1: Reflect actual success in response [x]

```go
// pkg/mcp/tools/developer.go, lines 919-987

var autoSelectSucceeded bool
if autoSelect && len(result.NewTableNames) > 0 {
    if err := deps.SchemaService.SelectAllTables(tenantCtx, dsID); err != nil {
        deps.Logger.Warn("Failed to auto-select tables",
            zap.String("project_id", projectID.String()),
            zap.String("datasource_id", dsID.String()),
            zap.Error(err),
        )
        autoSelectSucceeded = false
    } else {
        autoSelectSucceeded = true
    }
}

// Later in response:
AutoSelectApplied: autoSelectSucceeded,
```

### Fix 2: Set IsSelected during table creation [x]

If `auto_select: true`, set the flag during table creation rather than as a separate update:

```go
// pkg/mcp/tools/developer.go - pass autoSelect to service
result, err := deps.SchemaService.RefreshDatasourceSchema(tenantCtx, projectID, dsID, autoSelect)

// pkg/services/schema.go - use it when creating tables
table := &models.SchemaTable{
    ProjectID:    projectID,
    DatasourceID: datasourceID,
    SchemaName:   dt.SchemaName,
    TableName:    dt.TableName,
    RowCount:     &dt.RowCount,
    IsSelected:   autoSelect,  // Set based on parameter
}
```

This eliminates the race condition between insert and update.

### Fix 3: Add explicit project_id filter to update query

```go
// pkg/repositories/schema_repository.go:1685-1689
_, err := scope.Conn.Exec(ctx, `
    UPDATE engine_schema_tables
    SET is_selected = true, updated_at = NOW()
    WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
`, projectID, datasourceID)
```

This makes the query explicit rather than relying solely on RLS.

## Files to Modify

1. **pkg/mcp/tools/developer.go:919-987**
   - Track actual success of `SelectAllTables`
   - Set `AutoSelectApplied` based on actual result

2. **pkg/services/schema.go:163-169** (Option B)
   - Accept `autoSelect` parameter in `RefreshDatasourceSchema`
   - Set `IsSelected: true` during table creation

3. **pkg/repositories/schema_repository.go:1677-1708** (Option C)
   - Add `projectID` parameter to `SelectAllTablesAndColumns`
   - Include `project_id` in WHERE clause

## Testing Verification

After implementing:

1. Create a new table in the datasource:
   ```sql
   CREATE TABLE test_auto_select (id SERIAL PRIMARY KEY, name TEXT);
   ```

2. Call MCP `refresh_schema` with `auto_select: true`

3. Check MCP response:
   - `tables_added` should include the new table
   - `auto_select_applied` should be `true`

4. Query database directly:
   ```sql
   SELECT table_name, is_selected FROM engine_schema_tables
   WHERE table_name = 'test_auto_select';
   ```
   Should return `is_selected = true`

5. Load UI Schema Selection page - new table should appear checked

6. Test error case: Temporarily break `SelectAllTables` and verify:
   - MCP response says `auto_select_applied: false`
   - Warning is logged

## Database Schema Reference

```sql
-- migrations/003_schema.up.sql:5-22
CREATE TABLE engine_schema_tables (
    id uuid DEFAULT gen_random_uuid() NOT NULL PRIMARY KEY,
    project_id uuid NOT NULL,
    datasource_id uuid NOT NULL,
    schema_name text NOT NULL,
    table_name text NOT NULL,
    is_selected boolean DEFAULT false NOT NULL,  -- Default is false
    ...
);

-- Line 121-123: RLS Policy
ALTER TABLE engine_schema_tables ENABLE ROW LEVEL SECURITY;
CREATE POLICY schema_tables_access ON engine_schema_tables FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL
           OR project_id = current_setting('app.current_project_id', true)::uuid);
```
