# FIX: Bug 3 - probe_relationship Returns Empty for MCP-Created Relationships

**Priority:** Medium
**Component:** MCP Server / probe_relationship

## Problem Statement

Relationships created via `update_relationship` MCP tool don't appear in `probe_relationship` results.

**Reproduction Steps:**
1. Create relationship via `update_relationship(from_entity='S1Order', to_entity='S1Customer', ...)`
2. Verify relationship exists via `get_ontology` (it shows up)
3. Call `probe_relationship(from_entity='S1Order', to_entity='S1Customer')`
4. Returns empty `{"relationships":[]}`

## Root Cause Analysis

### Entity Lookup Inconsistency

The two tools use **different query patterns** for entity lookups:

**get_ontology** (pkg/services/ontology_context.go:177):
```go
entities, err := s.entityRepo.GetByProject(ctx, projectID)
```

Uses `GetByProject` which queries:
```sql
-- pkg/repositories/ontology_entity_repository.go:141-148
SELECT e.id, ... FROM engine_ontology_entities e
JOIN engine_ontologies o ON e.ontology_id = o.id
WHERE e.project_id = $1 AND o.is_active = true AND NOT e.is_deleted
```

**probe_relationship** (pkg/mcp/tools/probe.go:488):
```go
entities, err := deps.EntityRepo.GetByOntology(ctx, ontology.ID)
```

Uses `GetByOntology` which queries:
```sql
-- pkg/repositories/ontology_entity_repository.go:104-110
SELECT id, ... FROM engine_ontology_entities
WHERE ontology_id = $1 AND NOT is_deleted
```

### The Problem

1. `GetByProject` uses a JOIN to `engine_ontologies WHERE is_active = true`, finding entities across ALL active ontologies for the project

2. `GetByOntology(ontology.ID)` requires an EXACT ontology ID match - if there's any mismatch between the ontology ID returned by `GetActive` and the ontology ID entities were created under, those entities won't be found

3. Similarly, `GetByProject` for relationships (line 482) uses a JOIN:
   ```sql
   -- pkg/repositories/entity_relationship_repository.go:179-181
   JOIN engine_ontologies o ON r.ontology_id = o.id
   WHERE o.project_id = $1 AND o.is_active = true
   ```

4. **The mismatch**: Relationships are found via JOIN to any active ontology, but entities are looked up from ONE specific ontology ID. If entities and relationships were created under different ontology IDs (both active), the entity lookup fails.

### Where Entity Matching Fails

In `probeRelationships` (pkg/mcp/tools/probe.go:530-546):

```go
// Build entity ID → name map from GetByOntology result
entityIDToName := make(map[uuid.UUID]string)
for _, entity := range entities {
    entityIDToName[entity.ID] = entity.Name  // Only entities from ONE ontology
}

// Filter relationships using this map
for _, rel := range entityRelationships {
    // rel might be from ontology A
    fromName := entityIDToName[rel.SourceEntityID]  // Map only has ontology B entities
    if fromName != *fromEntity {  // fromName is "" if not found
        continue  // Relationship is filtered out!
    }
    // ...
}
```

If `entityRelationships` contains relationships from ontology A but `entityIDToName` only has entities from ontology B, the filter excludes valid relationships.

## Recommended Fix

### Change entity lookup to use GetByProject (like get_ontology does)

**pkg/mcp/tools/probe.go** (around line 488):

```go
// Before:
entities, err := deps.EntityRepo.GetByOntology(ctx, ontology.ID)

// After:
entities, err := deps.EntityRepo.GetByProject(ctx, projectID)
```

This ensures `probe_relationship` uses the same entity lookup pattern as `get_ontology`, making them consistent.

## Files to Modify

1. **pkg/mcp/tools/probe.go:488**
   - Change `GetByOntology(ctx, ontology.ID)` to `GetByProject(ctx, projectID)`
   - This is a one-line fix

## Testing Verification

After implementing:

1. Create entities via extraction or MCP
2. Create a relationship via `update_relationship(from_entity='Entity1', to_entity='Entity2')`
3. Verify via `get_ontology(depth='entities')` - relationship should appear
4. Call `probe_relationship(from_entity='Entity1', to_entity='Entity2')`
5. Verify the relationship appears in results (no longer empty)

Test edge cases:
- Relationship created before extraction
- Relationship created after extraction
- Multiple active ontologies (if that's even possible)

## Related

This is the same inconsistency pattern that caused Bug 6 (get_entity returns "not found" for extraction-created entities). Bug 6 was fixed by using `GetByProjectAndName` instead of `GetByName(ontology.ID, ...)`. This bug requires the same pattern fix for entity listing in probe_relationship.
