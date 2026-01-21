# PLAN-03: Schema Change Detection

**Parent:** [PLAN-living-ontology-master.md](./PLAN-living-ontology-master.md)
**Dependencies:** PLAN-01 (refresh_schema tool)
**Enables:** PLAN-04 (Data Change Detection), PLAN-05 (Change Queue)

## Goal

When schema refreshes, detect DDL-level changes (new tables, columns, types, dropped objects) and store them as pending changes for review.

## Current State

- `RefreshDatasourceSchema()` syncs tables/columns from datasource
- Returns counts (TablesUpserted, ColumnsUpserted, etc.)
- No record of WHAT specifically changed
- No diff between old and new schema state

## Desired State

After `refresh_schema()`:
```json
{
  "tables_added": ["orders"],
  "tables_removed": ["temp_data"],
  "columns_added": [
    {"table": "users", "column": "status", "type": "text"}
  ],
  "columns_removed": [
    {"table": "users", "column": "legacy_field"}
  ],
  "columns_modified": [
    {"table": "products", "column": "price", "old_type": "integer", "new_type": "numeric"}
  ],
  "pending_changes_created": 5
}
```

Changes queued in `engine_ontology_pending_changes` for review.

## Implementation

### 1. Create Pending Changes Table

**Migration:**

```sql
CREATE TABLE engine_ontology_pending_changes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(project_id),

    -- Change classification
    change_type TEXT NOT NULL,  -- 'new_table', 'dropped_table', 'new_column', 'dropped_column', 'modified_column', 'new_enum_value', 'cardinality_change', etc.
    change_source TEXT NOT NULL DEFAULT 'schema_refresh',  -- 'schema_refresh', 'data_scan', 'manual'

    -- What changed
    table_name TEXT,
    column_name TEXT,
    old_value JSONB,  -- Previous state (type, enum values, etc.)
    new_value JSONB,  -- New state

    -- Suggested ontology action
    suggested_action TEXT,  -- 'create_entity', 'update_entity', 'create_column_metadata', etc.
    suggested_payload JSONB,  -- Parameters for the action

    -- Review state
    status TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'approved', 'rejected', 'auto_applied'
    reviewed_by TEXT,  -- 'admin', 'mcp', 'auto'
    reviewed_at TIMESTAMPTZ,

    -- Metadata
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT valid_change_type CHECK (change_type IN (
        'new_table', 'dropped_table',
        'new_column', 'dropped_column', 'modified_column',
        'new_enum_value', 'cardinality_change', 'new_fk_pattern'
    )),
    CONSTRAINT valid_status CHECK (status IN ('pending', 'approved', 'rejected', 'auto_applied'))
);

-- RLS
ALTER TABLE engine_ontology_pending_changes ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON engine_ontology_pending_changes
    USING (project_id = current_setting('app.current_project_id')::uuid);

-- Indexes
CREATE INDEX idx_pending_changes_project_status ON engine_ontology_pending_changes(project_id, status);
CREATE INDEX idx_pending_changes_created ON engine_ontology_pending_changes(created_at DESC);
```

### 2. Create Change Detection Service

**File:** `pkg/services/schema_change_detection.go`

```go
type SchemaChangeDetectionService interface {
    DetectChanges(ctx context.Context, projectID, datasourceID uuid.UUID, refreshResult *RefreshResult) ([]PendingChange, error)
}

type PendingChange struct {
    ID              uuid.UUID
    ProjectID       uuid.UUID
    ChangeType      string
    ChangeSource    string
    TableName       string
    ColumnName      string
    OldValue        map[string]any
    NewValue        map[string]any
    SuggestedAction string
    SuggestedPayload map[string]any
    Status          string
    CreatedAt       time.Time
}

type schemaChangeDetectionService struct {
    schemaRepo       repositories.SchemaRepository
    pendingChangeRepo repositories.PendingChangeRepository
    logger           *slog.Logger
}

func (s *schemaChangeDetectionService) DetectChanges(
    ctx context.Context,
    projectID, datasourceID uuid.UUID,
    refreshResult *RefreshResult,
) ([]PendingChange, error) {
    var changes []PendingChange

    // New tables → suggest entity creation
    for _, tableName := range refreshResult.NewTableNames {
        change := PendingChange{
            ProjectID:       projectID,
            ChangeType:      "new_table",
            ChangeSource:    "schema_refresh",
            TableName:       tableName,
            SuggestedAction: "create_entity",
            SuggestedPayload: map[string]any{
                "name":        toEntityName(tableName),
                "primary_table": tableName,
            },
            Status: "pending",
        }
        changes = append(changes, change)
    }

    // Dropped tables → flag for review (don't auto-delete entities)
    for _, tableName := range refreshResult.RemovedTableNames {
        change := PendingChange{
            ProjectID:       projectID,
            ChangeType:      "dropped_table",
            ChangeSource:    "schema_refresh",
            TableName:       tableName,
            SuggestedAction: "review_entity",  // Human should decide
            Status:          "pending",
        }
        changes = append(changes, change)
    }

    // New columns → suggest column metadata
    for _, col := range refreshResult.NewColumns {
        change := PendingChange{
            ProjectID:       projectID,
            ChangeType:      "new_column",
            ChangeSource:    "schema_refresh",
            TableName:       col.TableName,
            ColumnName:      col.ColumnName,
            NewValue:        map[string]any{"type": col.DataType},
            SuggestedAction: "create_column_metadata",
            Status:          "pending",
        }
        changes = append(changes, change)
    }

    // Persist changes
    if err := s.pendingChangeRepo.CreateBatch(ctx, changes); err != nil {
        return nil, fmt.Errorf("persist pending changes: %w", err)
    }

    return changes, nil
}
```

### 3. Integrate with RefreshDatasourceSchema

**File:** `pkg/services/schema.go`

Modify `RefreshDatasourceSchema` to track detailed changes:

```go
type RefreshResult struct {
    // Counts (existing)
    TablesUpserted       int
    TablesDeleted        int64
    ColumnsUpserted      int
    ColumnsDeleted       int64
    RelationshipsCreated int
    RelationshipsDeleted int64

    // Detailed changes (new)
    NewTableNames     []string
    RemovedTableNames []string
    NewColumns        []ColumnChange
    RemovedColumns    []ColumnChange
    ModifiedColumns   []ColumnModification
}

type ColumnChange struct {
    TableName  string
    ColumnName string
    DataType   string
}

type ColumnModification struct {
    TableName  string
    ColumnName string
    OldType    string
    NewType    string
}
```

### 4. Add MCP Tool to List Pending Changes

**File:** `pkg/mcp/tools/ontology_changes.go`

```go
func registerListPendingChangesTool(s *server.MCPServer, deps *OntologyToolDeps) {
    tool := mcp.NewTool(
        "list_pending_changes",
        mcp.WithDescription(
            "List pending ontology changes detected from schema or data analysis. "+
            "Review these changes and approve/reject them to update the ontology.",
        ),
        mcp.WithString("status", mcp.Description("Filter by status: pending, approved, rejected (default: pending)")),
        mcp.WithNumber("limit", mcp.Description("Max changes to return (default: 50)")),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps.BaseDeps, "list_pending_changes")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        status := getStringParam(req, "status", "pending")
        limit := getIntParam(req, "limit", 50)

        changes, err := deps.PendingChangeRepo.List(tenantCtx, projectID, status, limit)
        if err != nil {
            return NewErrorResult("list_failed", err.Error()), nil
        }

        return NewJSONResult(map[string]any{
            "changes": changes,
            "count":   len(changes),
        }), nil
    })
}
```

### 5. Update refresh_schema to Trigger Detection

**File:** `pkg/mcp/tools/developer.go`

```go
// In registerRefreshSchemaTool, after RefreshDatasourceSchema:

// Detect changes and queue for review
changes, err := deps.ChangeDetectionService.DetectChanges(tenantCtx, projectID, datasourceID, result)
if err != nil {
    // Log but don't fail - refresh succeeded
    deps.Logger.Warn("change detection failed", "error", err)
}

response := map[string]any{
    "tables_added":           result.NewTableNames,
    "tables_removed":         result.RemovedTableNames,
    "columns_added":          len(result.NewColumns),
    "relationships":          relPairs,
    "pending_changes_created": len(changes),
}
```

## Tasks

1. [ ] Create migration for `engine_ontology_pending_changes` table
2. [ ] Create `PendingChange` model in `pkg/models/`
3. [ ] Create `PendingChangeRepository` interface and implementation
4. [ ] Create `SchemaChangeDetectionService` interface and implementation
5. [ ] Modify `RefreshResult` to include detailed change tracking
6. [ ] Update `RefreshDatasourceSchema` to populate detailed changes
7. [ ] Add `ChangeDetectionService` to `MCPToolDeps`
8. [ ] Update `refresh_schema` tool to call change detection
9. [ ] Implement `list_pending_changes` MCP tool
10. [ ] Add `list_pending_changes` to ontology tool group
11. [ ] Test: create table → refresh → pending change created for new entity

## Testing

```
1. Start with empty schema
2. execute("CREATE TABLE orders (id uuid PRIMARY KEY, customer_id uuid, total numeric)")
3. refresh_schema()
   → Returns: { pending_changes_created: 1, ... }
4. list_pending_changes()
   → Returns: [{
       change_type: "new_table",
       table_name: "orders",
       suggested_action: "create_entity",
       suggested_payload: { name: "Order", primary_table: "orders" }
     }]
5. execute("ALTER TABLE orders ADD COLUMN status text")
6. refresh_schema()
   → Returns: { pending_changes_created: 1, ... }
7. list_pending_changes()
   → Includes new_column change for orders.status
```
