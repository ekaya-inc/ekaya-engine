# FIX: Bug 5 - Entity Discovery Deletes Manually Created Entities

**Priority:** High
**Component:** Ontology Extraction / Entity Management
**Status:** Implemented

## Implementation Tasks

- [x] Add `DeleteInferenceEntitiesByOntology` to repository interface and implementation
- [x] Update `StartExtraction` to use provenance-aware deletion

## Problem Statement

When ontology extraction starts, the Entity Discovery step unconditionally deletes all existing entities, including those created manually via MCP tools.

**Reproduction Steps:**
1. Create entity via `update_entity(name='S1Customer', description='...')`
2. Verify entity exists via `get_entity(name='S1Customer')` - works
3. Start ontology extraction via UI
4. After Entity Discovery completes, call `get_entity(name='S1Customer')`
5. Returns "entity not found"

## Root Cause Analysis

### The Delete Location

**pkg/services/ontology_dag_service.go:174-177**:

```go
// Delete existing entities for fresh discovery
if err := s.entityRepo.DeleteByOntology(ctx, ontology.ID); err != nil {
    return nil, fmt.Errorf("delete existing entities: %w", err)
}
```

This is called in `StartExtraction` before creating the new DAG and running Entity Discovery.

### The Delete Implementation

**pkg/repositories/ontology_entity_repository.go:228-242**:

```go
func (r *ontologyEntityRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
    scope, ok := database.GetTenantScope(ctx)
    if !ok {
        return fmt.Errorf("no tenant scope in context")
    }

    query := `DELETE FROM engine_ontology_entities WHERE ontology_id = $1`

    _, err := scope.Conn.Exec(ctx, query, ontologyID)
    if err != nil {
        return fmt.Errorf("failed to delete ontology entities: %w", err)
    }

    return nil
}
```

This is a **hard delete** that removes ALL entities for the ontology, regardless of:
- `created_by` provenance (manual, mcp, inference)
- `is_deleted` soft-delete flag
- Any business value or manual enrichment

### Why This Happens

The extraction design assumes a "clean slate" approach:
1. Get or create ontology (`getOrCreateOntology`)
2. Delete all existing entities (`DeleteByOntology`)
3. Create fresh DAG for discovery
4. Run Entity Discovery (creates new entities with `inference` provenance)

This approach loses any manual or MCP-created entities, their descriptions, aliases, and key columns.

## Recommended Fix

### Option A: Provenance-Aware Delete (Recommended)

Modify `DeleteByOntology` to only delete inference-created entities, preserving manual/MCP entities:

**pkg/repositories/ontology_entity_repository.go** - Add new method:

```go
// DeleteInferenceEntitiesByOntology deletes only inference-created entities.
// Preserves entities created by manual or MCP sources.
func (r *ontologyEntityRepository) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
    scope, ok := database.GetTenantScope(ctx)
    if !ok {
        return fmt.Errorf("no tenant scope in context")
    }

    // Only delete inference entities, preserve manual/mcp
    query := `DELETE FROM engine_ontology_entities
              WHERE ontology_id = $1 AND created_by = 'inference'`

    _, err := scope.Conn.Exec(ctx, query, ontologyID)
    if err != nil {
        return fmt.Errorf("failed to delete inference entities: %w", err)
    }

    return nil
}
```

**pkg/services/ontology_dag_service.go:174-177** - Use new method:

```go
// Delete only inference-created entities for fresh discovery
// Manual and MCP entities are preserved
if err := s.entityRepo.DeleteInferenceEntitiesByOntology(ctx, ontology.ID); err != nil {
    return nil, fmt.Errorf("delete inference entities: %w", err)
}
```

### Option B: Soft-Delete Instead of Hard-Delete

Mark entities as deleted instead of removing them, then restore manual/MCP entities:

```go
// Soft-delete all existing entities
if err := s.entityRepo.SoftDeleteByOntology(ctx, ontology.ID, "extraction_restart"); err != nil {
    return nil, fmt.Errorf("soft delete entities: %w", err)
}

// Restore manual/MCP entities
if err := s.entityRepo.RestoreByProvenance(ctx, ontology.ID, []string{"manual", "mcp"}); err != nil {
    return nil, fmt.Errorf("restore manual entities: %w", err)
}
```

### Option C: Merge After Discovery

After Entity Discovery creates new entities, merge with preserved manual/MCP entities:

```go
// Before deletion, save manual/MCP entities
manualEntities, _ := s.entityRepo.GetByOntologyWithProvenance(ctx, ontology.ID, []string{"manual", "mcp"})

// After discovery completes, restore or merge
for _, entity := range manualEntities {
    // Check if discovery created same entity
    existing, _ := s.entityRepo.GetByName(ctx, ontology.ID, entity.Name)
    if existing == nil {
        // Restore the manual entity
        entity.IsDeleted = false
        s.entityRepo.Update(ctx, entity)
    } else {
        // Merge: keep manual description/aliases but update other fields
        existing.Description = entity.Description
        existing.CreatedBy = entity.CreatedBy  // Preserve provenance
        s.entityRepo.Update(ctx, existing)
    }
}
```

## Recommended Approach: Option A

**Option A is preferred** because:
1. Single-line change in the deletion query
2. No complex migration or merge logic
3. Respects the existing provenance system
4. Clear semantics: inference entities are regenerated, manual entities are preserved

## Files to Modify

1. **pkg/repositories/ontology_entity_repository.go**
   - Add interface method `DeleteInferenceEntitiesByOntology(ctx, ontologyID) error`
   - Implement with provenance filter: `WHERE created_by = 'inference'`

2. **pkg/services/ontology_dag_service.go:174-177**
   - Change `DeleteByOntology` to `DeleteInferenceEntitiesByOntology`

3. **Consider also**: Relationships should follow the same pattern
   - `pkg/repositories/entity_relationship_repository.go` - Add similar method
   - Check if relationships are also deleted during extraction

## Testing Verification

After implementing:

1. Create entity via MCP `update_entity(name='TestEntity', description='Manual description')`
2. Create relationship via MCP `update_relationship`
3. Add aliases and key columns
4. Verify all exist via `get_entity`
5. Run full ontology extraction via UI
6. After extraction completes:
   - Verify manually created entity still exists
   - Verify description is preserved
   - Verify aliases are preserved
   - Verify relationships are preserved
7. Verify new extraction-discovered entities also exist
8. Verify there are no duplicate entities (same table, different provenance)

## Related Cascade Deletes

Also check these related tables for similar issues:
- `engine_ontology_entity_aliases` - CASCADE from entity delete
- `engine_ontology_entity_key_columns` - CASCADE from entity delete
- `engine_entity_relationships` - Also deleted in extraction?
- `engine_ontology_entity_occurrences` - Managed separately?

Ensure the fix preserves all related data for manual/MCP entities.
