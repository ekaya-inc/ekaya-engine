# FIX: BUG-11 - All Relationships Have Unknown Cardinality

**Bug Reference:** BUGS-ontology-extraction.md, BUG-11
**Severity:** Medium
**Type:** Data Quality Issue

## Problem Summary

All 48 discovered relationships have `cardinality = 'unknown'` instead of proper values (1:1, 1:N, N:1, N:M). Cardinality information is essential for:
- Query optimization hints
- Understanding data relationships
- Generating correct JOIN patterns
- Validating data integrity

## Root Cause Analysis

### Cardinality Never Set

**File:** `pkg/services/deterministic_relationship_service.go`

When creating entity relationships, the `Cardinality` field is **never set**:

**FK Relationships (line 206-219):**
```go
rel := &models.EntityRelationship{
    OntologyID:         ontology.ID,
    SourceEntityID:     sourceEntity.ID,
    TargetEntityID:     targetEntity.ID,
    // ... other fields ...
    DetectionMethod:    detectionMethod,
    Confidence:         1.0,
    Status:             models.RelationshipStatusConfirmed,
    // ❌ Cardinality NOT SET
}
```

**PK-Match Relationships (line 652-665):**
```go
rel := &models.EntityRelationship{
    OntologyID:         ontology.ID,
    // ... other fields ...
    DetectionMethod:    models.DetectionMethodPKMatch,
    Confidence:         confidence,
    Status:             status,
    // ❌ Cardinality NOT SET
}
```

**Reverse Relationships (line 89-103):**
```go
reverse := &models.EntityRelationship{
    // ... swapped fields ...
    DetectionMethod:    rel.DetectionMethod,
    Confidence:         rel.Confidence,
    Status:             rel.Status,
    Description:        nil,
    // ❌ Cardinality NOT SET
}
```

### Repository Defaults to Unknown

**File:** `pkg/repositories/entity_relationship_repository.go`

```go
// Line 54-56: Default cardinality to "unknown" if not specified
if rel.Cardinality == "" {
    rel.Cardinality = "unknown"
}
```

### Cardinality Inference Exists But Not Used

**File:** `pkg/services/relationship_discovery.go`

There IS a cardinality inference function:
```go
func (s *relationshipDiscoveryService) inferCardinality(join *datasource.JoinAnalysis) string {
    if join.SourceMatched == 0 || join.TargetMatched == 0 {
        return models.CardinalityUnknown
    }
    // ... logic to determine 1:1, 1:N, N:1, N:M ...
}
```

But this is only used in `relationship_discovery.go` (line 375), not in `deterministic_relationship_service.go`.

## The Fix

### Option A: Add Cardinality Calculation to Deterministic Service (Recommended)

Calculate cardinality during relationship creation using the join validation data that's already available:

**File:** `pkg/services/deterministic_relationship_service.go`

For PK-Match relationships, add cardinality calculation:
```go
// Calculate cardinality from join analysis if available
cardinality := models.CardinalityUnknown
if joinAnalysis != nil {
    cardinality = s.inferCardinality(joinAnalysis)
}

rel := &models.EntityRelationship{
    OntologyID:         ontology.ID,
    // ... other fields ...
    DetectionMethod:    models.DetectionMethodPKMatch,
    Confidence:         confidence,
    Status:             status,
    Cardinality:        cardinality,  // NEW: Set cardinality
}
```

For FK relationships (from database constraints), infer cardinality from FK definition:
```go
// FK constraints are typically N:1 (many-to-one)
// The FK column (source) has many values pointing to one PK (target)
cardinality := models.CardinalityNTo1

rel := &models.EntityRelationship{
    // ... other fields ...
    Cardinality: cardinality,
}
```

### Option B: Add Post-Processing Step

Add a separate step to calculate cardinality for all relationships after creation:

```go
func (s *service) calculateRelationshipCardinalities(ctx context.Context, ontologyID uuid.UUID) error {
    relationships, err := s.relationshipRepo.GetByOntology(ctx, ontologyID)
    if err != nil {
        return err
    }

    for _, rel := range relationships {
        if rel.Cardinality != models.CardinalityUnknown {
            continue // Already calculated
        }

        cardinality := s.inferCardinalityFromSchema(ctx, rel)
        rel.Cardinality = cardinality
        if err := s.relationshipRepo.Update(ctx, rel); err != nil {
            return err
        }
    }
    return nil
}
```

### Option C: Use Schema Statistics

For FK relationships, use column statistics to determine cardinality:

```go
func inferCardinalityFromStats(sourceCol, targetCol *models.SchemaColumn, sourceTable, targetTable *models.SchemaTable) string {
    // N:1: Source has many duplicates, target is unique
    if targetCol.IsPrimaryKey || targetCol.IsUnique {
        if sourceCol.DistinctCount != nil && sourceTable.RowCount != nil {
            ratio := float64(*sourceCol.DistinctCount) / float64(*sourceTable.RowCount)
            if ratio < 0.9 { // Not 1:1
                return models.CardinalityNTo1
            }
            return models.Cardinality1To1
        }
        return models.CardinalityNTo1 // Default FK assumption
    }
    return models.CardinalityUnknown
}
```

## Implementation Steps

### Step 1: Move/Share Cardinality Inference

[x] Move `inferCardinality` from `relationship_discovery.go` to a shared `relationship_utils.go`

### Step 2: Update FK Relationship Creation

**File:** `pkg/services/deterministic_relationship_service.go`

Line 206-219: Add cardinality calculation for FK relationships.

### Step 3: Update PK-Match Relationship Creation

**File:** `pkg/services/deterministic_relationship_service.go`

Line 652-665: Add cardinality calculation using join analysis data.

### Step 4: Update Reverse Relationship Creation

**File:** `pkg/services/deterministic_relationship_service.go`

Line 89-103: Include cardinality (with direction swap: N:1 becomes 1:N for reverse).

### Step 5: Handle Cardinality Swapping for Reverse

When creating reverse relationship, swap cardinality direction:
```go
func reverseCardinality(cardinality string) string {
    switch cardinality {
    case models.CardinalityNTo1:
        return models.Cardinality1ToN
    case models.Cardinality1ToN:
        return models.CardinalityNTo1
    default:
        return cardinality // 1:1, N:M, unknown stay the same
    }
}
```

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/deterministic_relationship_service.go` | Add cardinality calculation |
| `pkg/services/relationship_discovery.go` | Share/export `inferCardinality` or move to common package |
| `pkg/services/deterministic_relationship_service_test.go` | Add tests for cardinality |

## Testing

### Verify Cardinality Values

```sql
-- After fix, check cardinality distribution:
SELECT cardinality, COUNT(*) as count
FROM engine_entity_relationships r
JOIN engine_ontologies o ON r.ontology_id = o.id
WHERE o.project_id = '2bb984fc-a677-45e9-94ba-9f65712ade70'
AND o.is_active = true
GROUP BY cardinality;

-- Expected results:
-- 1:1     |  X
-- 1:N     |  Y
-- N:1     |  Z
-- N:M     |  W
-- unknown |  0  (or minimal)
```

### Verify Specific Relationships

```sql
-- User → Account should be N:1 (many users belong to one account)
SELECT e1.name, e2.name, r.cardinality
FROM engine_entity_relationships r
JOIN engine_ontology_entities e1 ON r.source_entity_id = e1.id
JOIN engine_ontology_entities e2 ON r.target_entity_id = e2.id
WHERE e1.name = 'User' AND e2.name = 'Account';

-- Expected: N:1
```

### Unit Tests

```go
func TestCardinalityCalculation_ForeignKey(t *testing.T) {
    // FK relationships should be N:1 by default
    // Source column has many values, target is unique PK
}

func TestCardinalityCalculation_Reverse(t *testing.T) {
    // N:1 should become 1:N when reversed
    // 1:1 and N:M should stay the same
}
```

## Success Criteria

- [ ] FK relationships have N:1 cardinality (or 1:1 if unique)
- [ ] PK-Match relationships have calculated cardinality
- [ ] Reverse relationships have correct swapped cardinality
- [ ] No relationships have `cardinality = 'unknown'` unless truly ambiguous
- [ ] Cardinality values match schema constraints

## Connection to Other Bugs

| Bug | Connection |
|-----|------------|
| **BUG-9** (Stats collection) | Cardinality inference may need stats; fixing BUG-9 improves accuracy |
| **BUG-3** (Missing relationships) | If relationships aren't created, their cardinality doesn't matter |

## Notes

The cardinality of an FK relationship is typically N:1:
- **N (source)**: The FK column can have duplicate values
- **1 (target)**: The PK column values are unique

For explicit database FK constraints, this is almost always the case. For inferred relationships, cardinality depends on actual data patterns.
