# FIX: Relationship Column IDs Not Populated

**Date:** 2026-01-25
**Priority:** High
**Status:** Complete

## Problem

The `engine_entity_relationships` table has `source_column_id` and `target_column_id` columns (added in migration 023), but they are never populated during ontology extraction. All 64 relationships have NULL for both columns.

This breaks the feature where the API should return `source_column_type` and `target_column_type` via JOIN to `engine_schema_columns`.

## Root Cause

The repository's `Create` and `Upsert` methods don't include `source_column_id` and `target_column_id` in their INSERT statements, even though:

1. **Migration exists:** `migrations/023_relationship_column_ids.up.sql` added the columns
2. **Model has fields:** `pkg/models/entity_relationship.go:34-39` defines `SourceColumnID` and `TargetColumnID`
3. **Services populate them:** Both FK and PK-match discovery correctly set these fields:
   - `deterministic_relationship_service.go:221,225` (FK discovery)
   - `deterministic_relationship_service.go:728,732` (PK-match discovery)
   - `deterministic_relationship_service.go:99,103` (reverse relationship swap)
4. **GetByProject expects them:** Repository has JOIN logic to get column types (lines 189-201)

**The INSERT statements simply omit the columns.**

## Evidence

```sql
-- All relationships have NULL column IDs
SELECT
    source_column_id IS NOT NULL as has_src_id,
    target_column_id IS NOT NULL as has_tgt_id,
    COUNT(*) as cnt
FROM engine_entity_relationships
GROUP BY 1, 2;

-- Result:
-- has_src_id | has_tgt_id | cnt
-- f          | f          |  64
```

## Fix

### File: `pkg/repositories/entity_relationship_repository.go`

#### 1. Update `Create` method (lines 75-99)

**Current INSERT (line 76-82):**
```go
query := `
    INSERT INTO engine_entity_relationships (
        id, ontology_id, source_entity_id, target_entity_id,
        source_column_schema, source_column_table, source_column_name,
        target_column_schema, target_column_table, target_column_name,
        detection_method, confidence, status, cardinality, description, association,
        is_stale, created_by, created_at
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
```

**Fixed INSERT:**
```go
query := `
    INSERT INTO engine_entity_relationships (
        id, ontology_id, source_entity_id, target_entity_id,
        source_column_schema, source_column_table, source_column_name, source_column_id,
        target_column_schema, target_column_table, target_column_name, target_column_id,
        detection_method, confidence, status, cardinality, description, association,
        is_stale, created_by, created_at
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
```

**Update Exec call (line 92-99) to include the new parameters:**
```go
_, err := scope.Conn.Exec(ctx, query,
    rel.ID, rel.OntologyID, rel.SourceEntityID, rel.TargetEntityID,
    rel.SourceColumnSchema, rel.SourceColumnTable, rel.SourceColumnName, rel.SourceColumnID,
    rel.TargetColumnSchema, rel.TargetColumnTable, rel.TargetColumnName, rel.TargetColumnID,
    rel.DetectionMethod, rel.Confidence, rel.Status, rel.Cardinality, rel.Description, rel.Association,
    rel.IsStale, rel.CreatedBy, rel.CreatedAt,
    now,
)
```

**Note:** The ON CONFLICT DO UPDATE clause should also update column IDs:
```sql
ON CONFLICT (...) DO UPDATE SET
    is_stale = false,
    confidence = EXCLUDED.confidence,
    detection_method = EXCLUDED.detection_method,
    source_column_id = EXCLUDED.source_column_id,
    target_column_id = EXCLUDED.target_column_id,
    updated_at = $22
```

#### 2. Update `Upsert` method (lines 552-581)

Apply the same changes:
- Add `source_column_id` and `target_column_id` to INSERT column list
- Add corresponding `$N` placeholders
- Add `rel.SourceColumnID` and `rel.TargetColumnID` to Exec parameters
- Add to ON CONFLICT DO UPDATE SET clause

## Testing

### Unit Test

The test file `pkg/services/deterministic_relationship_service_test.go` already has assertions for column IDs (lines 164-177). After the fix, these tests should pass when run against a real database.

### Integration Test

```sql
-- After re-running ontology extraction:
SELECT
    source_column_id IS NOT NULL as has_src_id,
    target_column_id IS NOT NULL as has_tgt_id,
    COUNT(*) as cnt
FROM engine_entity_relationships
GROUP BY 1, 2;

-- Expected result:
-- has_src_id | has_tgt_id | cnt
-- t          | t          |  64  (all relationships have column IDs)
```

### API Verification

```bash
curl -s "http://localhost:3443/api/projects/{project-id}/entities/relationships" | \
  jq '.relationships[0] | {source_column_type, target_column_type}'

# Expected: actual type values like "uuid", "bigint", etc.
# Current: null, null
```

## Success Criteria

- [x] All new relationships have `source_column_id` and `target_column_id` populated
- [x] API returns `source_column_type` and `target_column_type` via JOIN
- [x] Existing tests pass
- [ ] Re-extraction populates column IDs for all relationships (requires re-run)

## Files to Modify

| File | Change |
|------|--------|
| `pkg/repositories/entity_relationship_repository.go` | Add column IDs to Create and Upsert INSERT statements |
