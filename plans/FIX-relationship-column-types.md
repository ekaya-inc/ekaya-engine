# FIX: Add Column Types to Entity Relationships

**Priority:** 7 (Low)
**Status:** In Progress
**Parent:** PLAN-ontology-next.md
**Design Reference:** FIX-add-column-type-to-entity-relationships.md (archived)

## Problem

The `engine_entity_relationships` table stores column names but not column types. The API response struct has `source_column_type` and `target_column_type` fields, but they're never populated.

## Solution

Add foreign keys from `engine_entity_relationships` to `engine_schema_columns` for both source and target columns. This allows JOINing to get column types without duplicating data.

## Implementation

### Step 1: Migration ✓

```sql
ALTER TABLE engine_entity_relationships
    ADD COLUMN source_column_id UUID REFERENCES engine_schema_columns(id) ON DELETE SET NULL,
    ADD COLUMN target_column_id UUID REFERENCES engine_schema_columns(id) ON DELETE SET NULL;

CREATE INDEX idx_entity_rel_source_col ON engine_entity_relationships(source_column_id);
CREATE INDEX idx_entity_rel_target_col ON engine_entity_relationships(target_column_id);
```

### Step 2: Update Model ✓

**File:** `pkg/models/entity_relationship.go`
```go
type EntityRelationship struct {
    // ... existing fields ...
    SourceColumnID   *uuid.UUID `json:"source_column_id,omitempty"`
    TargetColumnID   *uuid.UUID `json:"target_column_id,omitempty"`
    SourceColumnType string     `json:"source_column_type,omitempty"` // From JOIN
    TargetColumnType string     `json:"target_column_type,omitempty"` // From JOIN
}
```

### Step 3: Update Repository ✓

**File:** `pkg/repositories/entity_relationship_repository.go`

Update `GetByProject` query to JOIN for types:
```sql
SELECT r.*,
       COALESCE(sc.data_type, '') as source_column_type,
       COALESCE(tc.data_type, '') as target_column_type
FROM engine_entity_relationships r
LEFT JOIN engine_schema_columns sc ON r.source_column_id = sc.id
LEFT JOIN engine_schema_columns tc ON r.target_column_id = tc.id
WHERE ...
```

### Step 4: Update Service ✓

**File:** `pkg/services/deterministic_relationship_service.go`

When creating relationships, store column IDs:
```go
rel := &models.EntityRelationship{
    // ... existing fields ...
    SourceColumnID: &sourceCol.ID,
    TargetColumnID: &targetCol.ID,
}
```

### Step 5: Update Handler ✓

Map column types from model to response.

## Files to Modify

| File | Change |
|------|--------|
| `migrations/0XX_relationship_column_ids.sql` | New migration |
| `pkg/models/entity_relationship.go` | Add column ID and type fields |
| `pkg/repositories/entity_relationship_repository.go` | Update queries with JOIN |
| `pkg/services/deterministic_relationship_service.go` | Store column IDs |
| `pkg/handlers/entity_relationship_handler.go` | Map types to response |

## Testing

1. Run ontology extraction
2. Verify relationships have column IDs populated
3. Verify API returns column types
4. UI can optionally display types (was removed due to this bug)

## Success Criteria

- [ ] Column IDs stored in relationships
- [ ] API returns source_column_type and target_column_type
- [ ] New extractions populate the fields
