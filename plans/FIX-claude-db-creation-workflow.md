# FIX: Claude Database Creation Workflow

## Context

Ekaya Engine supports two ontology building modes:

### Mode 1: Dev Tools Mode (Claude as Ontology Builder)

Claude creates database tables and builds the ontology directly via MCP tools. No AI Config required - Claude IS the AI. The admin enables "Ontology Management Tools" in the MCP Server config, and Claude uses `update_entity()`, `update_relationship()`, `update_column()`, etc. to build semantic context incrementally.

**Use cases:**
- Creating a database from scratch
- Importing CSV files and building schema
- Rapid prototyping where Claude understands intent better than extraction

**Workflow:**
```
execute(DDL) → refresh_schema() → update_entity/relationship/column() → query()
```

### Mode 2: AI Config Mode (Engine as Ontology Builder)

Admin attaches an AI Config to the project. The Engine uses LLM-based extraction to build the ontology automatically. The UI provides "Extract Ontology" functionality.

**Future vision:** Instead of dropping and re-extracting the entire ontology on schema changes, the Engine should:
1. Detect schema changes (new tables, new columns, dropped columns)
2. Detect data changes (new enumeration values, cardinality shifts)
3. Optimally update the ontology (add new entities, update column metadata)
4. Queue these updates when admin manually refreshes schema in UI

This provides continuous ontology freshness without full re-extraction.

### Interoperability

Both modes must coexist:
- Empty ontology created on project creation enables immediate MCP tool use
- Admin can later attach AI Config and run extraction (creates version 2)
- Claude's manual ontology work is preserved unless extraction explicitly replaces it
- UI shows provenance: "Created by Claude Code" vs "Extracted by Engine"

---

## Issue 1: Schema Refresh MCP Tool

### Problem

When Claude creates tables via `execute()` DDL, the schema isn't reflected in Ekaya until a human manually clicks "Refresh Schema" in the UI. There's no MCP tool to trigger this refresh.

### Current Behavior

1. Claude runs: `execute("CREATE TABLE threads (...)")`
2. Table exists in PostgreSQL but Ekaya's `engine_schema_tables` doesn't know about it
3. Claude tries `get_schema()` - doesn't include new table
4. Human must go to UI → Schema page → click Refresh → save selections

### Desired Behavior

1. Claude runs: `execute("CREATE TABLE threads (...)")`
2. Claude runs: `refresh_schema()` - auto-imports and auto-selects new tables
3. Claude runs `get_schema()` - includes new table

### Proposed Solution

Add a new MCP tool `refresh_schema` that:

1. Calls existing `SchemaService.RefreshDatasourceSchema()` to sync from datasource
2. Auto-selects all newly discovered tables/columns (different from UI behavior where user chooses)
3. Returns summary of changes (tables added, columns added, etc.)

**Tool Definition:**

```go
tool := mcp.NewTool(
    "refresh_schema",
    mcp.WithDescription(
        "Refresh schema from datasource and auto-select new tables/columns. "+
        "Use this after creating tables with execute() to make them visible to other tools. "+
        "Returns summary of discovered changes.",
    ),
    mcp.WithBoolean(
        "auto_select",
        mcp.Description("Automatically select all new tables/columns (default: true)"),
    ),
)
```

**Response:**

```json
{
  "tables_added": ["threads", "entries", "context_snapshots"],
  "tables_removed": [],
  "columns_added": 15,
  "relationships": [
    {"from": "entries.thread_id", "to": "threads.id"},
    {"from": "context_snapshots.thread_id", "to": "threads.id"}
  ]
}
```

*Design feedback from Claude Chat:* Return relationship pairs, not just counts. This lets the caller verify FKs were detected correctly without a follow-up `get_schema()` call.

### Implementation

**File:** `pkg/mcp/tools/developer.go` (add to existing developer tools)

The tool uses existing infrastructure:
- `SchemaService.RefreshDatasourceSchema()` already exists at `pkg/services/schema.go:97`
- Need to add `SchemaService.SelectAllTables()` method
- Need to track which tables are "new" in RefreshResult

```go
func registerRefreshSchemaTool(s *server.MCPServer, deps *MCPToolDeps) {
    tool := mcp.NewTool(
        "refresh_schema",
        mcp.WithDescription(
            "Refresh schema from datasource and auto-select new tables/columns. "+
            "Use this after creating tables with execute() to make them visible to other tools.",
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

        // Get auto_select param (default true)
        autoSelect := true
        if args, ok := req.Params.Arguments.(map[string]any); ok {
            if v, ok := args["auto_select"].(bool); ok {
                autoSelect = v
            }
        }

        // Refresh schema from datasource
        result, err := deps.SchemaService.RefreshDatasourceSchema(tenantCtx, projectID, datasourceID)
        if err != nil {
            return NewErrorResult("refresh_failed", fmt.Sprintf("failed to refresh schema: %v", err)), nil
        }

        // Auto-select all tables/columns if requested
        if autoSelect {
            if err := deps.SchemaService.SelectAllTables(tenantCtx, projectID, datasourceID); err != nil {
                return NewErrorResult("select_failed", fmt.Sprintf("failed to auto-select tables: %v", err)), nil
            }
        }

        // Get relationships for response
        relationships, err := deps.SchemaService.GetRelationships(tenantCtx, projectID, datasourceID)
        if err != nil {
            return NewErrorResult("relationships_failed", fmt.Sprintf("failed to get relationships: %v", err)), nil
        }

        relPairs := make([]map[string]string, 0, len(relationships))
        for _, rel := range relationships {
            relPairs = append(relPairs, map[string]string{
                "from": rel.SourceTable + "." + rel.SourceColumn,
                "to":   rel.TargetTable + "." + rel.TargetColumn,
            })
        }

        response := map[string]any{
            "tables_added":   result.NewTableNames,
            "tables_removed": result.RemovedTableNames,
            "columns_added":  result.ColumnsUpserted,
            "relationships":  relPairs,
        }

        return NewJSONResult(response), nil
    })
}
```

**New Methods Required:**

1. `SchemaService.SelectAllTables(ctx, projectID, datasourceID)` - marks all tables/columns as `is_selected=true`
2. Modify `RefreshDatasourceSchema()` to track `NewTableNames` and `RemovedTableNames` in result

### Tool Access

Add `refresh_schema` to `developerToolNames` in `pkg/mcp/tools/developer.go:76` and to `services.ToolGroupDeveloper` definition.

---

## Issue 2: Empty Ontology on Project Creation

### Problem

When Claude tries to call `update_entity()` after creating tables, it fails with:

```
{"error":{"code":-32603,"message":"no active ontology found for project"}}
```

This is because ontology requires an "Extract Ontology" run to create the `engine_ontologies` record. But in Dev Tools mode, there's no human to click that button, and no AI Config to run extraction.

### Desired Behavior

1. Project creation creates an empty ontology record (like having an empty schema)
2. Claude can immediately call `update_entity()`, `update_relationship()`, etc.
3. MCP tools return empty results instead of errors when ontology has no content
4. The ontology page shows Claude's work without requiring "Extract Ontology"

### Proposed Solution

**Create empty ontology on project creation.**

There's no downside to having an empty ontology - MCP tools will just return empty results. This mirrors how schema works (empty until populated).

**Implementation Location:** `pkg/services/projects.go` in `Provision()` method (line 129)

```go
// In projectService.Provision(), after creating project
func (s *projectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
    // ... existing project creation at line 129 ...

    project := &models.Project{
        ProjectID:  projectID,
        Name:       name,
        Parameters: params,
        Status:     "active",
    }
    if err := s.projectRepo.Create(ctx, project); err != nil {
        return nil, fmt.Errorf("create project: %w", err)
    }

    // NEW: Create empty ontology for immediate MCP tool use
    emptyOntology := &models.TieredOntology{
        ProjectID:       projectID,
        Version:         1,
        IsActive:        true,
        EntitySummaries: make(map[string]*models.EntitySummary),
        ColumnDetails:   make(map[string][]models.ColumnDetail),
        Metadata:        make(map[string]any),
    }

    // Use tenant context for ontology creation
    tenantCtx := s.db.WithTenant(ctx, projectID)
    if err := s.ontologyRepo.Create(tenantCtx, emptyOntology); err != nil {
        // Log but don't fail - ontology can be created later
        s.logger.Warn("failed to create initial ontology", "project_id", projectID, "error", err)
    }

    // ... rest of method ...
}
```

**Dependencies Required:**
- Add `ontologyRepo repositories.OntologyRepository` to `projectService` struct
- Inject via `NewProjectService()` constructor
- Add `db` connection for `WithTenant()` context

### Migration for Existing Projects

Projects created before this change won't have an ontology. Options:

1. **Lazy creation (recommended):** Modify ontology update tools to create empty ontology if none exists
2. **Migration script:** One-time script to create empty ontologies for all projects without one

Lazy creation is safer and handles edge cases:

```go
// Helper function used by all ontology update tools
func ensureOntologyExists(ctx context.Context, repo repositories.OntologyRepository, projectID uuid.UUID) (*models.TieredOntology, error) {
    ontology, err := repo.GetActive(ctx, projectID)
    if err != nil {
        return nil, err
    }
    if ontology != nil {
        return ontology, nil
    }

    // Create empty ontology
    ontology = &models.TieredOntology{
        ProjectID:       projectID,
        Version:         1,
        IsActive:        true,
        EntitySummaries: make(map[string]*models.EntitySummary),
        ColumnDetails:   make(map[string][]models.ColumnDetail),
        Metadata:        make(map[string]any),
    }
    if err := repo.Create(ctx, ontology); err != nil {
        return nil, fmt.Errorf("failed to create ontology: %w", err)
    }
    return ontology, nil
}
```

### UI Handling

The ontology page needs to handle the empty ontology case gracefully:

**Current behavior:** Shows "Extract Ontology" button, blocks on AI Config

**Desired behavior:**
- Shows entities/relationships tables (empty state: "No entities yet")
- If AI Config attached: "Extract Ontology" button available
- If no AI Config: Show message "Build ontology via MCP tools or attach AI Config to extract"
- UI indicates provenance when displaying entities: "via MCP" or "via extraction"

**Empty state check:**

```typescript
// In OntologyPage.tsx
const hasOntologyContent = entities.length > 0 || relationships.length > 0;

if (!hasOntologyContent) {
    return (
        <EmptyOntologyState
            hasAIConfig={!!project.aiConfigId}
            onExtract={handleExtractOntology}
        />
    );
}
```

---

## Implementation Tasks

### Phase 1: Empty Ontology on Project Creation

1. [ ] Add `ontologyRepo` dependency to `projectService` struct in `pkg/services/projects.go`
2. [ ] Update `NewProjectService()` constructor to accept ontology repo
3. [ ] Modify `Provision()` to create empty ontology after project creation
4. [ ] Add `ensureOntologyExists()` helper for ontology update tools (handles existing projects)
5. [ ] Update ontology update tools to use `ensureOntologyExists()` instead of failing
6. [ ] Update UI OntologyPage to show empty state gracefully
7. [ ] Test: create project → call `update_entity()` → succeeds

### Phase 2: Schema Refresh Tool

1. [ ] Add `SelectAllTables()` method to `SchemaService` interface and implementation
2. [ ] Modify `RefreshDatasourceSchema()` to return `NewTableNames` and `RemovedTableNames`
3. [ ] Add `GetRelationships()` method to `SchemaService` (or expose existing)
4. [ ] Add `refresh_schema` to `developerToolNames` map in `pkg/mcp/tools/developer.go`
5. [ ] Add `refresh_schema` to `services.ToolGroupDeveloper` definition
6. [ ] Implement `registerRefreshSchemaTool()` in `pkg/mcp/tools/developer.go`
7. [ ] Add `SchemaService` to `MCPToolDeps` struct
8. [ ] Test: create table via execute → refresh_schema → get_schema shows table

### Phase 3: UI Provenance Tracking (Future)

1. [ ] Add `created_by` field to entity/relationship records (values: "mcp", "extraction", "manual")
2. [ ] Display provenance badge in UI entity list
3. [ ] Allow filtering by provenance

---

## Testing Scenarios

### 1. New Project - Dev Tools Mode

```
1. Create project via API (no AI Config)
2. Connect MCP client
3. execute("CREATE TABLE foo (id uuid PRIMARY KEY, name text)")
4. refresh_schema() → returns { tables_added: ["foo"], ... }
5. get_schema() → includes foo table
6. update_entity(name='Foo', description='Test entity') → succeeds
7. get_ontology(depth='entities') → shows Foo entity
```

### 2. New Project - AI Config Mode

```
1. Create project via API
2. Attach AI Config
3. Connect datasource, refresh schema via UI
4. Click "Extract Ontology" → creates version 2, deactivates version 1
5. Entities populated via LLM extraction
```

### 3. Mixed Mode - Claude Enhances Extracted Ontology

```
1. Project with AI Config, extraction completed
2. Claude connects via MCP
3. update_entity(name='Order', description='Improved description') → updates existing
4. Ontology shows hybrid: some from extraction, some from Claude
```

### 4. Existing Project Without Ontology (Migration)

```
1. Connect to existing project created before this fix
2. update_entity() → ensureOntologyExists creates empty ontology → succeeds
3. Subsequent calls work normally
```

---

## Future: Incremental Ontology Updates

When admin refreshes schema in UI (AI Config mode), instead of requiring full re-extraction:

1. **Detect changes:**
   - New tables → queue entity discovery
   - New columns → queue column enrichment
   - Dropped tables → mark entities as stale
   - New enum values → update column metadata

2. **Optimal updates:**
   - Only run LLM on changed portions
   - Preserve manually edited descriptions
   - Merge new discoveries with existing ontology

3. **Admin approval:**
   - Show pending ontology changes in UI
   - Admin can approve/reject individual updates
   - Prevents unwanted PII columns from being added

This creates a "living ontology" that stays current without full re-extraction cost.
