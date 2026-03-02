# PLAN: MCP Relationship Management Tools

**Status:** TODO
**Branch:** TBD
**Created:** 2026-03-01

## Problem

There are no MCP tools for managing relationships. An admin/data role user (or an MCP client like Claude Code) cannot list, review, create, or delete relationships through the MCP server. When the ontology extraction produces false positives (e.g., `content_posts.week_number -> post_channel_steps.id`), the only way to fix them is through the web UI or direct database access.

This is a gap in the MCP admin toolset. All other ontology entities (columns, tables, glossary terms, project knowledge, pending changes, queries) have full CRUD via MCP tools, but relationships do not.

## Tool Design

Five tools, designed for an MCP client that needs to audit and curate relationships:

### 1. `list_relationships`

**Purpose:** List all relationships for the project's default datasource, with optional filtering. This is the primary audit tool — an MCP client uses this to review what exists before deciding what to delete or add.

**Parameters:**
- `table` (string, optional): Filter to relationships involving this table (as source OR target)
- `type` (string, optional): Filter by relationship_type: `fk`, `inferred`, `manual`
- `include_rejected` (boolean, optional): Include soft-deleted/rejected relationships (default: false)

**Returns:** Array of relationship objects with human-readable table/column names:
```json
{
  "relationships": [
    {
      "id": "uuid",
      "source_table": "content_posts",
      "source_column": "app_id",
      "target_table": "applications",
      "target_column": "id",
      "relationship_type": "inferred",
      "cardinality": "N:1",
      "confidence": 0.95,
      "inference_method": "column_features",
      "is_approved": true
    }
  ],
  "total_count": 47,
  "empty_tables": ["weekly_metrics"],
  "orphan_tables": ["competitors"]
}
```

**Implementation:** Calls `SchemaService.GetRelationshipsResponse()` which returns `RelationshipDetail` objects (already has resolved table/column names). Apply optional filters in the tool handler. The existing `GetRelationshipsResponse` already returns `empty_tables` and `orphan_tables` which are useful for the MCP client to understand the graph.

### 2. `create_relationship`

**Purpose:** Manually create a relationship between two columns. Used when the ontology extraction missed a real FK.

**Parameters:**
- `source_table` (string, required): Source table name
- `source_column` (string, required): Source column name (the FK column)
- `target_table` (string, required): Target table name
- `target_column` (string, required): Target column name (the PK/unique column)
- `cardinality` (string, optional): `1:1`, `N:1`, `1:N`, or `N:M` (default: `unknown`, computed from data if possible)

**Returns:**
```json
{
  "id": "uuid",
  "source_table": "content_posts",
  "source_column": "app_id",
  "target_table": "applications",
  "target_column": "id",
  "relationship_type": "manual",
  "cardinality": "N:1",
  "created": true
}
```

**Implementation:** Calls `SchemaService.AddManualRelationship()` which already:
- Validates both table/column names exist in schema
- Checks for duplicate relationships
- Creates with `RelationshipTypeManual`, `confidence: 1.0`, `is_approved: true`

Enhancement needed: pass through the optional `cardinality` parameter. Currently `AddManualRelationship` hardcodes `CardinalityUnknown`. The service method should accept an optional cardinality from the request.

### 3. `delete_relationship`

**Purpose:** Remove a relationship by ID. Used to clean up false positives from inference. Soft-deletes so the relationship is not re-inferred on the next extraction run.

**Parameters:**
- `relationship_id` (string, required): UUID of the relationship to delete

**Returns:**
```json
{
  "relationship_id": "uuid",
  "source": "content_posts.app_id",
  "target": "mcp_directories.id",
  "deleted": true
}
```

**Implementation:** Calls `SchemaService.RemoveRelationship()` which already:
- Verifies relationship exists and belongs to project
- Soft-deletes (sets `deleted_at`) so it persists through re-extraction

Enhancement: before deleting, fetch the relationship details so the response includes human-readable source/target names for confirmation.

### 4. `delete_relationships_bulk`

**Purpose:** Delete multiple relationships at once. Essential for cleaning up mass false positives (e.g., "delete all 11 inferred relationships for content_posts"). Without this, the MCP client would need to make 11 separate `delete_relationship` calls.

**Parameters:**
- `relationship_ids` (string array, required): Array of UUIDs to delete

**Returns:**
```json
{
  "deleted": 11,
  "failed": 0,
  "details": [
    {"id": "uuid", "source": "content_posts.app_id", "target": "mcp_directories.id", "deleted": true},
    {"id": "uuid", "source": "content_posts.app_id", "target": "marketing_tasks.id", "deleted": true}
  ]
}
```

**Implementation:** Loop over IDs, call `SchemaService.RemoveRelationship()` for each. Collect results. Continue on individual failures (partial success is acceptable). Fetch relationship details before deletion for readable output.

A new `SchemaService` method is NOT needed — the tool handler loops over the existing `RemoveRelationship` method.

### 5. `update_relationship`

**Purpose:** Update the cardinality of an existing relationship. Useful when inference gets the cardinality wrong (e.g., inferred N:M when it should be N:1).

**Parameters:**
- `relationship_id` (string, required): UUID of the relationship
- `cardinality` (string, optional): `1:1`, `N:1`, `1:N`, or `N:M`
- `is_approved` (boolean, optional): Explicitly approve or reject a relationship

**Returns:**
```json
{
  "id": "uuid",
  "source_table": "content_posts",
  "source_column": "app_id",
  "target_table": "applications",
  "target_column": "id",
  "cardinality": "N:1",
  "is_approved": true,
  "updated": true
}
```

**Implementation:** Requires a new `SchemaService` method (or extend existing repo methods). The repo already has `UpdateRelationshipApproval` for the `is_approved` field. For cardinality updates, add a simple update method or extend the existing one.

## File-by-File Changes

### 1. New file: `pkg/mcp/tools/relationship.go`

Create a new tool file following the pattern of `column.go` and `table.go`.

**Deps struct:**
```go
type RelationshipToolDeps struct {
    BaseMCPToolDeps
    SchemaRepo     repositories.SchemaRepository
    SchemaService  services.SchemaService
    ProjectService services.ProjectService
}
```

Note: needs both `SchemaRepo` (for `GetRelationshipByID`, `GetRelationshipDetails`) and `SchemaService` (for `AddManualRelationship`, `RemoveRelationship`, `GetRelationshipsResponse`).

**Registration function:**
```go
func RegisterRelationshipTools(s *mcp.Server, deps *RelationshipToolDeps)
```

Registers all 5 tools. Each uses `AcquireToolAccess` (write provenance) since these are admin write operations, except `list_relationships` which uses `AcquireToolAccessWithoutProvenance` (read-only).

**Tool definitions:**
- `list_relationships`: Read-only hint, open-world false
- `create_relationship`: Not read-only, not destructive, idempotent (upsert)
- `delete_relationship`: Not read-only, destructive, idempotent
- `delete_relationships_bulk`: Not read-only, destructive, not idempotent
- `update_relationship`: Not read-only, not destructive, idempotent

### 2. `pkg/services/mcp_tool_loadouts.go`

**`AllToolsOrdered` (after line 87, before the closing `}`):** Add 5 new tool specs in a new "Relationship Management" section:
```go
// Relationship Management
{Name: "list_relationships", Description: "List all relationships with optional filtering by table or type"},
{Name: "create_relationship", Description: "Create a manual relationship between two columns"},
{Name: "update_relationship", Description: "Update cardinality or approval status of a relationship"},
{Name: "delete_relationship", Description: "Delete a relationship by ID (soft-delete, persists through re-extraction)"},
{Name: "delete_relationships_bulk", Description: "Delete multiple relationships at once"},
```

**`LoadoutOntologyMaintenance` (line 133-157):** Add the 5 tools:
```go
// Relationship management
"list_relationships",
"create_relationship",
"update_relationship",
"delete_relationship",
"delete_relationships_bulk",
```

### 3. `pkg/services/mcp_tools_registry.go`

**`ToolRegistry` (around line 52, with other developer tools):** Add 5 entries:
```go
{Name: "list_relationships", Description: "List all relationships with optional filtering by table or type", ToolGroup: ToolGroupDeveloper},
{Name: "create_relationship", Description: "Create a manual relationship between two columns", ToolGroup: ToolGroupDeveloper},
{Name: "update_relationship", Description: "Update cardinality or approval status of a relationship", ToolGroup: ToolGroupDeveloper},
{Name: "delete_relationship", Description: "Delete a relationship by ID", ToolGroup: ToolGroupDeveloper},
{Name: "delete_relationships_bulk", Description: "Delete multiple relationships at once", ToolGroup: ToolGroupDeveloper},
```

### 4. `pkg/models/schema.go`

**Extend `AddRelationshipRequest` (line 293-298):** Add optional cardinality field:
```go
type AddRelationshipRequest struct {
    SourceTableName  string `json:"source_table"`
    SourceColumnName string `json:"source_column"`
    TargetTableName  string `json:"target_table"`
    TargetColumnName string `json:"target_column"`
    Cardinality      string `json:"cardinality,omitempty"` // Optional: 1:1, N:1, 1:N, N:M
}
```

### 5. `pkg/services/schema.go`

**`AddManualRelationship` (line 641):** Use `req.Cardinality` if provided, fall back to `CardinalityUnknown`:
```go
cardinality := models.CardinalityUnknown
if req.Cardinality != "" {
    cardinality = req.Cardinality
}
```

**Add new method `UpdateRelationship`:**
```go
func (s *schemaService) UpdateRelationship(ctx context.Context, projectID, relationshipID uuid.UUID, cardinality *string, isApproved *bool) error
```
- Validates relationship exists and belongs to project
- Updates cardinality and/or is_approved fields
- Calls repo methods

**Update `SchemaService` interface (line 20):** Add `UpdateRelationship` method signature.

### 6. `pkg/repositories/schema_repository.go`

**Add new method to interface and implementation:**
```go
UpdateRelationshipFields(ctx context.Context, projectID, relationshipID uuid.UUID, updates map[string]interface{}) error
```
A generic field update method that accepts a map of column->value pairs. Validates only known columns (`cardinality`, `is_approved`) to prevent SQL injection.

Alternatively, add a specific `UpdateRelationshipCardinality` method if the generic approach feels too loose.

### 7. `main.go`

**Add registration block (after table tools, around line 555):**
```go
// Register relationship management tools
relationshipToolDeps := &mcptools.RelationshipToolDeps{
    BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
        DB:                  db,
        MCPConfigService:    mcpConfigService,
        Logger:              logger,
        InstalledAppService: installedAppService,
    },
    SchemaRepo:     schemaRepo,
    SchemaService:  schemaService,
    ProjectService: projectService,
}
mcptools.RegisterRelationshipTools(mcpServer.MCP(), relationshipToolDeps)
```

### 8. Tests: `pkg/mcp/tools/relationship_test.go`

Follow the pattern from existing tool tests (e.g., `column_test.go`). Test each tool with:
- Happy path (valid parameters, expected results)
- Missing required parameters
- Invalid relationship_id (not found)
- Duplicate relationship (conflict on create)
- Bulk delete with mix of valid/invalid IDs
- Filter parameters on list

## Checklist

- [ ] Create `pkg/mcp/tools/relationship.go` with `RelationshipToolDeps`, `RegisterRelationshipTools`, and all 5 tool handlers
- [ ] Add `list_relationships` tool (read-only, uses `GetRelationshipsResponse` + optional filters)
- [ ] Add `create_relationship` tool (calls `AddManualRelationship`)
- [ ] Add `delete_relationship` tool (calls `RemoveRelationship`)
- [ ] Add `delete_relationships_bulk` tool (loops `RemoveRelationship`)
- [ ] Add `update_relationship` tool (new service method)
- [ ] Extend `AddRelationshipRequest` in `pkg/models/schema.go` with optional `Cardinality`
- [ ] Update `AddManualRelationship` in `pkg/services/schema.go` to use provided cardinality
- [ ] Add `UpdateRelationship` method to `SchemaService` interface and implementation
- [ ] Add repo method for updating relationship fields (cardinality, is_approved)
- [ ] Register tools in `AllToolsOrdered` in `pkg/services/mcp_tool_loadouts.go`
- [ ] Add tools to `LoadoutOntologyMaintenance` in `pkg/services/mcp_tool_loadouts.go`
- [ ] Add tools to `ToolRegistry` in `pkg/services/mcp_tools_registry.go`
- [ ] Wire deps and register in `main.go`
- [ ] Add tests in `pkg/mcp/tools/relationship_test.go`
- [ ] Run: `go test ./pkg/mcp/tools/... -run Relationship`
- [ ] Run: `go test ./pkg/services/... -run Schema`

## Notes

- The service layer already has `AddManualRelationship`, `RemoveRelationship`, and `GetRelationshipsResponse` — this plan mostly wraps existing functionality as MCP tools
- `list_relationships` uses the existing `RelationshipDetail` model which already joins table/column names — no raw UUIDs in MCP output
- Soft-delete semantics are critical: deleted relationships must persist so re-extraction doesn't re-create them
- The `delete_relationships_bulk` tool is essential for the primary use case (cleaning up dozens of false positives from a single extraction run)
- All write tools go in `LoadoutOntologyMaintenance` which requires Admin/Data role + "Add Ontology Maintenance" config
- `list_relationships` is also in `LoadoutOntologyMaintenance` (not `LoadoutQuery`) because it's primarily an admin audit tool; the `get_context` and `get_ontology` tools already expose relationship info for query generation
- Dead code: `pkg/services/fk_semantic_evaluation.go` and `pkg/services/fk_semantic_evaluation_test.go` are orphaned (no callers outside their own test). Should be deleted in a separate cleanup.
