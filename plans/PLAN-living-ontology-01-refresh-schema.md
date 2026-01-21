# PLAN-01: refresh_schema MCP Tool

**Parent:** [PLAN-living-ontology-master.md](./PLAN-living-ontology-master.md)
**Dependencies:** None
**Enables:** PLAN-03 (Schema Change Detection)

## Goal

Add MCP tool to sync schema from datasource to Ekaya's registry, enabling Claude to create tables and immediately use them.

## Current State

- `SchemaService.RefreshDatasourceSchema()` exists at `pkg/services/schema.go:97`
- Called by UI via `POST /api/projects/{pid}/datasources/{did}/schema/refresh`
- No MCP tool exposes this functionality
- After `execute(DDL)`, Claude must wait for human to refresh via UI

## Desired State

```
Claude: execute("CREATE TABLE orders (...)")
Claude: refresh_schema()  → { tables_added: ["orders"], ... }
Claude: get_schema()      → includes orders table
```

## Implementation

### 1. Add Tool to Developer Group

**File:** `pkg/services/mcp_config.go`

Add `refresh_schema` to `ToolGroupDeveloper`:

```go
var ToolGroupDeveloper = ToolGroup{
    Name: "developer",
    Tools: []string{
        "echo", "query", "sample", "execute", "validate", "explain_query",
        "refresh_schema",  // ADD THIS
    },
}
```

### 2. Update Tool Name Maps

**File:** `pkg/mcp/tools/developer.go`

The `developerToolNames` map is built from `ToolGroupDeveloper` at init, so adding to the group handles this.

### 3. Add SchemaService to MCPToolDeps

**File:** `pkg/mcp/tools/developer.go`

```go
type MCPToolDeps struct {
    BaseDeps      *BaseDeps
    SchemaService services.SchemaService  // ADD THIS
}
```

### 4. Wire Up in main.go

**File:** `main.go` (around line 270)

```go
mcpToolDeps := &mcptools.MCPToolDeps{
    BaseDeps:      baseDeps,
    SchemaService: schemaService,  // ADD THIS
}
```

### 5. Implement Tool

**File:** `pkg/mcp/tools/developer.go`

```go
func registerRefreshSchemaTool(s *server.MCPServer, deps *MCPToolDeps) {
    tool := mcp.NewTool(
        "refresh_schema",
        mcp.WithDescription(
            "Refresh schema from datasource and auto-select new tables/columns. "+
            "Use after execute() to make new tables visible to other tools. "+
            "Returns summary: tables added/removed, columns added, relationships discovered.",
        ),
        mcp.WithBoolean(
            "auto_select",
            mcp.Description("Automatically select all new tables/columns (default: true)"),
        ),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, datasourceID, tenantCtx, cleanup, err := AcquireToolAccessWithDatasource(ctx, deps.BaseDeps, "refresh_schema")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        autoSelect := getBoolParam(req, "auto_select", true)

        // Refresh schema
        result, err := deps.SchemaService.RefreshDatasourceSchema(tenantCtx, projectID, datasourceID)
        if err != nil {
            return NewErrorResult("refresh_failed", err.Error()), nil
        }

        // Auto-select if requested
        if autoSelect {
            if err := deps.SchemaService.SelectAllTables(tenantCtx, datasourceID); err != nil {
                return NewErrorResult("select_failed", err.Error()), nil
            }
        }

        // Get relationships for response
        rels, _ := deps.SchemaService.GetRelationships(tenantCtx, datasourceID)

        relPairs := make([]map[string]string, 0, len(rels))
        for _, r := range rels {
            relPairs = append(relPairs, map[string]string{
                "from": r.SourceTable + "." + r.SourceColumn,
                "to":   r.TargetTable + "." + r.TargetColumn,
            })
        }

        return NewJSONResult(map[string]any{
            "tables_added":   result.NewTableNames,
            "tables_removed": result.RemovedTableNames,
            "columns_added":  result.ColumnsUpserted,
            "relationships":  relPairs,
        }), nil
    })
}
```

### 6. Register Tool

**File:** `pkg/mcp/tools/developer.go`

```go
func RegisterMCPTools(s *server.MCPServer, deps *MCPToolDeps) {
    registerEchoTool(s, deps)
    registerQueryTool(s, deps)
    registerSampleTool(s, deps)
    registerExecuteTool(s, deps)
    registerValidateTool(s, deps)
    registerExplainQueryTool(s, deps)
    registerRefreshSchemaTool(s, deps)  // ADD THIS
}
```

### 7. Add SelectAllTables Method

**File:** `pkg/services/schema.go`

```go
// SelectAllTables marks all tables and columns for this datasource as selected
func (s *schemaService) SelectAllTables(ctx context.Context, datasourceID uuid.UUID) error {
    return s.schemaRepo.SelectAll(ctx, datasourceID)
}
```

**File:** `pkg/repositories/schema_repository.go`

```go
func (r *schemaRepository) SelectAll(ctx context.Context, datasourceID uuid.UUID) error {
    _, err := r.db.Exec(ctx, `
        UPDATE engine_schema_tables SET is_selected = true WHERE datasource_id = $1;
        UPDATE engine_schema_columns SET is_selected = true
        WHERE table_id IN (SELECT id FROM engine_schema_tables WHERE datasource_id = $1);
    `, datasourceID)
    return err
}
```

### 8. Track New/Removed Table Names in RefreshResult

**File:** `pkg/services/schema.go`

Modify `RefreshDatasourceSchema` to populate `NewTableNames` and `RemovedTableNames`:

```go
type RefreshResult struct {
    TablesUpserted    int
    TablesDeleted     int64
    ColumnsUpserted   int
    ColumnsDeleted    int64
    RelationshipsCreated int
    RelationshipsDeleted int64
    NewTableNames     []string  // ADD
    RemovedTableNames []string  // ADD
}
```

Track during refresh loop which tables are newly inserted vs updated.

## Tasks

1. [x] Add `refresh_schema` to `ToolGroupDeveloper` in `pkg/services/mcp_tools_registry.go`
2. [x] Add `SchemaService` to `MCPToolDeps` struct (already present)
3. [x] Wire `SchemaService` into `mcpToolDeps` in `main.go` (already present)
4. [x] Implement `registerRefreshSchemaTool()` in `pkg/mcp/tools/developer.go`
5. [x] Add to `RegisterMCPTools()` call
6. [x] Add `SelectAllTables()` to SchemaService interface and implementation
7. [x] Add `SelectAllTablesAndColumns()` to SchemaRepository
8. [x] Modify `RefreshResult` to include `NewTableNames` and `RemovedTableNames`
9. [x] Update `RefreshDatasourceSchema()` to populate new fields
10. [x] Test: `execute(DDL)` → `refresh_schema()` → `get_schema()` shows new table

## Testing

```sql
-- Setup: empty datasource

-- Step 1: Create table
execute("CREATE TABLE test_refresh (id uuid PRIMARY KEY, name text)")

-- Step 2: Refresh
refresh_schema()
-- Expected: { "tables_added": ["test_refresh"], "columns_added": 2, ... }

-- Step 3: Verify
get_schema()
-- Expected: includes test_refresh table with id, name columns

-- Step 4: Add column
execute("ALTER TABLE test_refresh ADD COLUMN status text")
refresh_schema()
-- Expected: { "tables_added": [], "columns_added": 1, ... }
```
