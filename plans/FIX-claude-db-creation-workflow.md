# FIX: Claude Database Creation Workflow

## Context

In the traditional Ekaya workflow, an admin imports an existing database schema, reviews and saves relevant tables/columns, then runs "Extract Ontology" to build the semantic layer. This manual flow doesn't work well for a new mode where Claude creates the database tables directly.

**New Mode:** Claude creates database tables → Claude builds ontology → Claude queries data

This document addresses two blockers in this workflow.

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

**File:** `pkg/mcp/tools/schema.go` (create new or extend existing)

```go
func registerRefreshSchemaTool(s *server.MCPServer, deps *SchemaToolDeps) {
    tool := mcp.NewTool(
        "refresh_schema",
        mcp.WithDescription("..."),
        mcp.WithBoolean("auto_select", mcp.Description("...")),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, datasourceID, tenantCtx, cleanup, err := AcquireToolAccessWithDatasource(ctx, deps, "refresh_schema")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        // Get auto_select param (default true)
        autoSelect := true
        if v, ok := req.Params.Arguments.(map[string]any)["auto_select"].(bool); ok {
            autoSelect = v
        }

        // Refresh schema
        result, err := deps.SchemaService.RefreshDatasourceSchema(tenantCtx, projectID, datasourceID)
        if err != nil {
            return nil, fmt.Errorf("failed to refresh schema: %w", err)
        }

        // Auto-select new tables/columns if requested
        if autoSelect && result.TablesUpserted > 0 {
            // Mark all tables/columns as selected
            if err := deps.SchemaService.SelectAllNewTables(tenantCtx, projectID, datasourceID); err != nil {
                return nil, fmt.Errorf("failed to auto-select tables: %w", err)
            }
        }

        // Get newly discovered relationships for response
        relationships, err := deps.SchemaService.GetRelationshipsForDatasource(tenantCtx, projectID, datasourceID)
        if err != nil {
            return nil, fmt.Errorf("failed to get relationships: %w", err)
        }

        // Build response with relationship pairs (not just counts)
        relPairs := make([]relationshipPair, 0, len(relationships))
        for _, rel := range relationships {
            relPairs = append(relPairs, relationshipPair{
                From: rel.SourceTableName + "." + rel.SourceColumnName,
                To:   rel.TargetTableName + "." + rel.TargetColumnName,
            })
        }

        response := refreshSchemaResponse{
            TablesAdded:    result.NewTableNames,  // Need to track these in RefreshResult
            TablesRemoved:  result.RemovedTableNames,
            ColumnsAdded:   result.ColumnsUpserted,
            Relationships:  relPairs,
        }

        jsonResult, _ := json.Marshal(response)
        return mcp.NewToolResultText(string(jsonResult)), nil
    })
}

type relationshipPair struct {
    From string `json:"from"`
    To   string `json:"to"`
}
```

**New Method:** `SchemaService.SelectAllNewTables()` - mark all unselected tables/columns as selected

### Tool Access

This tool should be in the **developer** tool group since it modifies schema state.

---

## Issue 2: Empty Ontology on Project Creation

### Problem

When Claude tries to call `update_entity()` after creating tables, it fails with:

```
{"error":{"code":-32603,"message":"no active ontology found for project"}}
```

This is because ontology requires an "Extract Ontology" run to create the `engine_ontologies` record. But in Claude-driven mode, there's no human to click that button.

**Additional Complication:** The UI blocks "Extract Ontology" unless AI Config is set. In Claude-driven mode, Claude IS the AI - no LLM config needed.

### Desired Behavior

1. Project creation creates an empty ontology record
2. Claude can immediately call `update_entity()`, `update_relationship()`, etc.
3. Claude builds the ontology incrementally as it creates tables
4. The ontology page shows Claude's work without requiring "Extract Ontology"

### Proposed Solution

**Option A: Create empty ontology on project creation (Recommended)**

When a project is created, automatically create an empty active ontology:

```go
// In project service Create()
func (s *projectService) Create(ctx context.Context, project *models.Project) error {
    // ... existing project creation ...

    // Create empty ontology for immediate MCP use
    emptyOntology := &models.TieredOntology{
        ProjectID: project.ID,
        Version:   1,
        IsActive:  true,
        // All other fields nil/empty
    }
    if err := s.ontologyRepo.Create(ctx, emptyOntology); err != nil {
        return fmt.Errorf("failed to create initial ontology: %w", err)
    }

    return nil
}
```

**Benefits:**
- Simple, one-time change
- All MCP ontology tools work immediately
- No special handling needed in tool code

**Considerations:**
- UI needs to handle empty ontology gracefully (show "No entities yet" instead of error)
- "Extract Ontology" should still work (creates version 2, deactivates version 1)

**Option B: Auto-create ontology on first update_entity call**

Have `update_entity` create the ontology if none exists:

```go
// In update_entity handler
ontology, err := deps.OntologyRepo.GetActive(tenantCtx, projectID)
if err != nil {
    return nil, err
}
if ontology == nil {
    // Create empty ontology
    ontology = &models.TieredOntology{
        ProjectID: projectID,
        Version:   1,
        IsActive:  true,
    }
    if err := deps.OntologyRepo.Create(tenantCtx, ontology); err != nil {
        return nil, fmt.Errorf("failed to create ontology: %w", err)
    }
}
// ... continue with entity update
```

**Benefits:**
- Only creates ontology when needed
- No change to project creation

**Drawbacks:**
- Must be done in every ontology update tool
- Race condition risk if multiple tools called concurrently

### Recommendation

Go with **Option A** - create empty ontology on project creation. It's cleaner and prevents the "no active ontology" error entirely.

### UI Handling

The ontology page needs to handle the empty ontology case:

**Current (before extract):** Shows "Extract Ontology" button, blocks on AI Config

**Desired (empty ontology exists):**
- Shows entities/relationships (empty state: "No entities discovered yet")
- "Extract Ontology" button available (runs LLM-based extraction)
- OR hide extraction if AI Config not set but allow manual entity creation

**Check for empty ontology:**

```typescript
// In OntologyPage.tsx
const hasOntologyContent = entities.length > 0 || relationships.length > 0;

if (!hasOntologyContent) {
    return <EmptyOntologyState />;  // "No entities yet. Extract or create manually."
}
```

---

## Implementation Tasks

### Phase 1: Empty Ontology on Project Creation

1. [ ] Modify `ProjectService.Create()` to create empty ontology
2. [ ] Add migration if needed (for existing projects without ontologies)
3. [ ] Update OntologyPage to handle empty ontology gracefully
4. [ ] Test: create project → call `update_entity()` → succeeds

### Phase 2: Schema Refresh Tool

1. [ ] Create `pkg/mcp/tools/schema.go` with `refresh_schema` tool
2. [ ] Add `SelectAllNewTables()` method to SchemaService
3. [ ] Register tool in developer tool group
4. [ ] Test: create table via execute → refresh_schema → get_schema shows table

### Testing Scenarios

1. **New project workflow:**
   - Create project via API
   - Connect MCP client
   - `execute("CREATE TABLE foo (...)")`
   - `refresh_schema()` → returns tables_added: 1
   - `update_entity(name='Foo', description='...')` → succeeds
   - `get_ontology(depth='entities')` → shows Foo entity

2. **Existing project with ontology:**
   - `refresh_schema()` works normally
   - Empty ontology migration doesn't break existing data

3. **UI still works:**
   - Project with empty ontology shows sensible state
   - "Extract Ontology" creates version 2 with LLM enrichment
