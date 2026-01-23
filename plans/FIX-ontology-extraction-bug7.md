# FIX: BUG-7 - Relationship Discovery Processes Deselected Tables

**Bug Reference:** BUGS-ontology-extraction.md, BUG-7
**Severity:** Medium
**Type:** Performance/Efficiency Issue

## Problem Summary

Relationship discovery processes **all tables** (including deselected ones) even though entities are only created from selected tables. This wastes processing time and may cause errors when trying to create relationships to non-existent entities.

## Root Cause

Multiple services call `ListTablesByDatasource` with `selectedOnly=false`:

### Comparison: Entity Discovery (Correct)

```go
// pkg/services/entity_discovery_service.go:97
// ✅ Uses selectedOnly=true
tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID, true)
```

### Affected Code Locations (Need Fix)

| File | Line | Function | Purpose |
|------|------|----------|---------|
| `pkg/services/relationship_discovery.go` | 138 | `DiscoverRelationships` | FK discovery via join validation |
| `pkg/services/deterministic_relationship_service.go` | 146 | `DiscoverRelationships` | FK discovery via stats |
| `pkg/services/deterministic_relationship_service.go` | 448 | `DiscoverPKMatches` | PK matching |
| `pkg/services/data_change_detection.go` | 88 | `DetectEnumChanges` | Enum value monitoring |
| `pkg/services/data_change_detection.go` | 134 | `DetectSchemaChanges` | Schema change detection |
| `pkg/services/dag_adapters.go` | 51 | Adapter method | Table listing |

## Impact

### Performance
- 27 deselected sample tables (s1_* through s10_*) analyzed unnecessarily
- Stats collection runs on deselected tables
- Join validation queries executed for deselected tables
- LLM tokens consumed for tables that won't be in ontology

### Errors/Warnings
- Relationship discovery tries to find relationships between deselected tables
- Can't create relationships (entities don't exist)
- May log warnings about missing entities

### Resource Usage
- Database queries for deselected tables
- Memory for deselected table/column data
- Network I/O for stats collection

## Why This Is Safe to Fix

**Database constraints prevent invalid relationships:**
```sql
-- engine_entity_relationships table constraints:
source_entity_id uuid NOT NULL REFERENCES engine_ontology_entities(id)
target_entity_id uuid NOT NULL REFERENCES engine_ontology_entities(id)
```

Relationships REQUIRE both entities to exist. Since deselected tables don't have entities:
- Current behavior: Process deselected tables → try to create relationships → fail silently or error
- Fixed behavior: Skip deselected tables entirely → no wasted processing

## The Fix

Change all affected calls from `selectedOnly=false` to `selectedOnly=true`:

### 1. `pkg/services/relationship_discovery.go:138`

```go
// Before:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)

// After:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
```

### 2. `pkg/services/deterministic_relationship_service.go:146`

```go
// Before:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)

// After:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
```

### 3. `pkg/services/deterministic_relationship_service.go:448`

```go
// Before:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)

// After:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
```

### 4. `pkg/services/data_change_detection.go:88`

```go
// Before:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)

// After:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
```

### 5. `pkg/services/data_change_detection.go:134`

```go
// Before:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)

// After:
tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, true)
```

### 6. `pkg/services/dag_adapters.go:51`

```go
// Before:
tables, err := a.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID, false)

// After:
tables, err := a.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID, true)
```

## Implementation Steps

1. **Search and Replace:** Change all `ListTablesByDatasource(ctx, projectID, datasourceID, false)` to use `true`
2. **Verify no legitimate use cases:** Confirm no service needs deselected tables
3. **Update tests:** Ensure tests pass with selected-only tables
4. **Performance measurement:** Compare processing time before/after

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/services/relationship_discovery.go` | Line 138: change `false` to `true` |
| `pkg/services/deterministic_relationship_service.go` | Lines 146, 448: change `false` to `true` |
| `pkg/services/data_change_detection.go` | Lines 88, 134: change `false` to `true` |
| `pkg/services/dag_adapters.go` | Line 51: change `false` to `true` |

## Testing

### Verify Reduced Processing

```sql
-- Count selected vs total tables
SELECT
  COUNT(*) FILTER (WHERE is_selected) as selected,
  COUNT(*) as total
FROM engine_schema_tables
WHERE project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70';

-- Expected: selected < total
-- After fix: only selected tables processed
```

### Performance Test

1. Clear ontology data
2. Run extraction (note time)
3. Apply fix
4. Run extraction again (note time)
5. Processing time should decrease proportionally to deselected tables

### Integration Test

```go
func TestRelationshipDiscovery_OnlySelectedTables(t *testing.T) {
    // Setup: Create project with some deselected tables
    // Run relationship discovery
    // Verify: No processing for deselected tables (check logs)
    // Verify: Relationships only reference entities from selected tables
}
```

## Success Criteria

- [x] Relationship discovery only processes selected tables
- [x] PK match discovery only processes selected tables
- [x] Data change detection only monitors selected tables (DetectSchemaChanges fixed)
- [ ] Processing time reduced (proportional to deselected table count)
- [ ] No errors/warnings about missing entities from deselected tables
- [ ] All tests pass
- [ ] Relationships still correctly discovered for selected tables

## Edge Cases

### Cross-Selection Relationships

**Q:** What if a selected table references a deselected table?
**A:** The relationship cannot be created (target entity doesn't exist). This is correct behavior - if admin deselected a table, relationships to it should not exist.

### Reselection

**Q:** What if admin reselects a previously deselected table?
**A:** Re-run ontology extraction. Entity discovery creates the entity, then relationship discovery finds relationships.

## Notes

This is a straightforward fix with no architectural changes. The entity discovery pattern (using `selectedOnly=true`) should be followed consistently across all services.
