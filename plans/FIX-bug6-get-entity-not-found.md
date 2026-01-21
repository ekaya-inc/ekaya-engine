# FIX: Bug 6 - get_entity Returns "Not Found" for Extraction-Created Entities

**Priority:** High
**Component:** MCP Server / Entity Queries
**Discovered:** During ontology extraction testing (TEST-ontology-mix.md)

## Problem Statement

Entities that appear in `get_context(depth='entities')` are not found by `get_entity(name='...')`. This creates an inconsistent user experience where entities are visible through one tool but not another.

**Reproduction Steps:**
1. Start ontology extraction via UI
2. After Entity Discovery completes, call `get_context(depth='entities')`
3. Response shows entities like `{"s1_customers": {"primary_table": "s1_customers", ...}}`
4. Call `get_entity(name='s1_customers')`
5. Returns "entity not found"

## Root Cause Analysis

The two tools use different query methods to find entities:

### get_entity (pkg/mcp/tools/entity.go:87-94)
```go
ontology, err := ensureOntologyExists(tenantCtx, deps.OntologyRepo, projectID)
entity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, name)
```

Uses `GetByName` which queries:
```sql
-- pkg/repositories/ontology_entity_repository.go:178-184
SELECT ... FROM engine_ontology_entities
WHERE ontology_id = $1 AND name = $2 AND NOT is_deleted
```

### get_context (pkg/services/ontology_context.go:177)
```go
entities, err := s.entityRepo.GetByProject(ctx, projectID)
```

Uses `GetByProject` which queries:
```sql
-- pkg/repositories/ontology_entity_repository.go:140-148
SELECT e.id, ... FROM engine_ontology_entities e
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE e.project_id = $1 AND o.is_active = true AND NOT e.is_deleted
```

### The Inconsistency

- `GetByName` requires an exact `ontology_id` match (passed from `ensureOntologyExists`)
- `GetByProject` uses a JOIN to find entities from ANY active ontology for the project

**Scenario where they diverge:**

During or after extraction, if:
1. Multiple ontology records exist for the project
2. `ensureOntologyExists` returns one ontology ID
3. But entities were created under a different ontology ID that is also active

Or if:
1. `ensureOntologyExists` creates a NEW empty ontology (line 30-41 in ontology_helpers.go) when none is found active
2. Meanwhile, entities exist under a DIFFERENT ontology that IS active

The mismatch occurs because `GetByName` is scoped to a specific `ontology_id` while `GetByProject` finds entities across all active ontologies.

## Recommended Fix

### Option A: Make get_entity use GetByProject-style query (Recommended)

Add a new repository method that matches by name but uses the same join pattern as `GetByProject`:

**pkg/repositories/ontology_entity_repository.go:**
```go
// GetByProjectAndName finds an entity by name within the active ontology for a project.
func (r *ontologyEntityRepository) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
    scope, ok := database.GetTenantScope(ctx)
    if !ok {
        return nil, fmt.Errorf("no tenant scope in context")
    }

    query := `
        SELECT e.id, e.project_id, e.ontology_id, e.name, e.description, e.domain,
               e.primary_schema, e.primary_table, e.primary_column,
               e.is_deleted, e.deletion_reason,
               e.created_by, e.updated_by, e.created_at, e.updated_at
        FROM engine_ontology_entities e
        JOIN engine_ontologies o ON e.ontology_id = o.id
        WHERE e.project_id = $1 AND e.name = $2 AND o.is_active = true AND NOT e.is_deleted`

    row := scope.Conn.QueryRow(ctx, query, projectID, name)
    entity, err := scanOntologyEntity(row)
    if err != nil {
        if err == pgx.ErrNoRows {
            return nil, nil
        }
        return nil, err
    }

    return entity, nil
}
```

**pkg/mcp/tools/entity.go** (get_entity handler):
```go
// Before:
ontology, err := ensureOntologyExists(tenantCtx, deps.OntologyRepo, projectID)
entity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, name)

// After:
entity, err := deps.OntologyEntityRepo.GetByProjectAndName(tenantCtx, projectID, name)
```

### Option B: Ensure consistent ontology ID usage

Investigate why `ensureOntologyExists` might return a different ontology ID than what the entities are stored under. This is a deeper fix but may mask other issues.

## Files to Modify

1. [x] **pkg/repositories/ontology_entity_repository.go**
   - Add interface method `GetByProjectAndName(ctx, projectID, name) (*OntologyEntity, error)`
   - Implement the method with JOIN to engine_ontologies

2. [x] **pkg/mcp/tools/entity.go**
   - Modify `registerGetEntityTool` handler to use `GetByProjectAndName` instead of `ensureOntologyExists` + `GetByName`
   - Note: `update_entity` and `delete_entity` may also need this fix

3. [x] **pkg/mcp/tools/entity_test.go** (if exists) or create tests
   - Add test case for finding extraction-created entities

## Testing Verification ✅

Manually verified on 2026-01-21:

1. [x] Run ontology extraction via UI
2. [x] Wait for Entity Discovery to complete
3. [x] Verify `get_context(depth='entities')` shows entities (50+ entities returned)
4. [x] Verify `get_entity(name='Account')` finds extraction-created entities
5. [x] Verify `update_entity` works with extraction-created entities (`created: false`)
6. [x] Verify `delete_entity` works (tested with temporary `ManualTestEntity`)

## Related Issues

This same pattern may affect other MCP tools that use `GetByName` or `GetByOntology`:
- `update_entity` (entity.go:371) - uses `GetByName`
- `delete_entity` (entity.go:565) - uses `GetByName`
- Entity relationship tools - may use `GetByOntology`

Consider auditing all entity lookups to ensure consistency.
