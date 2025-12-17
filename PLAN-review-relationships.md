# Plan: Review Relationships for Orphan Tables

## Overview

Add a secondary discovery pass that identifies potential integer-based relationships for tables that have no relationships after the FK import and deterministic inference. These candidates are marked as `type='review'` for LLM-assisted verification on the frontend.

## Motivation

The deterministic algorithm skips all numeric-to-numeric joins because auto-increment IDs and counts cause false positives. However, this misses legitimate FK relationships where:
- The database doesn't have FK constraints defined
- The column naming doesn't follow conventions
- The relationship is still semantically valid

For orphan tables (tables with no relationships), we can be more aggressive and flag potential matches for human/LLM review rather than auto-approving them.

## Algorithm

### Trigger Condition
Only run for tables that have **zero relationships** after:
1. FK constraint import (`type='fk'`)
2. Deterministic inference (`type='inferred'`)

### Source Column Selection
For each orphan table:
- **If table has a primary key:** Use only the PK column(s) as source
- **If table has no primary key:** Use all columns that would be joinable with numeric types allowed (uuid, text, AND integer types)

### Candidate Matching Criteria
For each source column, find target columns where ALL of these are true:

1. **Not a primary key** - We're looking for the FK side, not the PK side
2. **Exact type match** - `integer→integer`, `bigint→bigint` (no cross-type matching like `int→bigint`)
3. **High cardinality** - Target column has high distinct ratio (>10% of rows are distinct, or >1000 distinct values)
4. **Zero orphan rate** - 100% of source values exist in target column (no unmatched values)

### Relationship Storage
- `relationship_type = 'review'` (new enumeration value)
- `is_approved = NULL` (pending review)
- `confidence` = match_rate from value overlap
- Store validation metrics for LLM context

---

## Implementation Phases

### Phase 1: Database Changes

**Migration: `007_review_relationship_type.up.sql`**

```sql
-- Add 'review' to relationship_type options
-- (If using enum, alter type; if using varchar, no change needed)
COMMENT ON COLUMN engine_schema_relationships.relationship_type IS
  'fk=foreign key constraint, inferred=auto-discovered, manual=user-created, review=pending LLM review';
```

**Files:**
- `migrations/007_review_relationship_type.up.sql`
- `migrations/007_review_relationship_type.down.sql`

### Phase 2: Model Updates

**Add new constant in `pkg/models/schema.go`:**

```go
const (
    RelationshipTypeFk       = "fk"
    RelationshipTypeInferred = "inferred"
    RelationshipTypeManual   = "manual"
    RelationshipTypeReview   = "review"  // NEW: pending LLM review
)
```

**Files:**
- `pkg/models/schema.go`

### Phase 3: Discovery Service Updates

**Add review candidate discovery in `pkg/services/relationship_discovery.go`:**

1. After main discovery completes, identify orphan tables (tables with zero relationships)
2. For each orphan table, run the review candidate algorithm
3. Store matches as `type='review'`

**New helper functions:**
- `findReviewCandidates()` - Main orchestrator for orphan tables
- `getSourceColumnsForReview()` - Returns PK or all joinable columns
- `isHighCardinality()` - Checks if column looks like a join key vs count
- `hasZeroOrphanRate()` - Verifies 100% value match

**Key logic:**
```go
// Pseudo-code for review candidate matching
func (s *service) findReviewCandidates(ctx, projectID, datasourceID, orphanTables) {
    for _, table := range orphanTables {
        sourceColumns := s.getSourceColumnsForReview(table)

        for _, source := range sourceColumns {
            // Only consider numeric types for review candidates
            if !isNumericType(source.DataType) {
                continue
            }

            // Find potential targets (non-PK columns with exact type match)
            targets := s.findExactTypeMatches(source.DataType, excludePKs=true)

            for _, target := range targets {
                // Must have high cardinality (not a count column)
                if !s.isHighCardinality(target) {
                    continue
                }

                // Must have 100% value overlap (no orphans)
                overlap := s.checkValueOverlap(source, target)
                if overlap.OrphanRate > 0 {
                    continue
                }

                // Store as review candidate
                s.createReviewRelationship(source, target, overlap)
            }
        }
    }
}
```

**Files:**
- `pkg/services/relationship_discovery.go`

### Phase 4: Repository Updates

**Add method to find non-PK columns by exact type:**

```go
// GetNonPKColumnsByType returns columns that are NOT primary keys and match the exact data type
func (r *schemaRepository) GetNonPKColumnsByType(ctx, projectID, datasourceID, dataType string) ([]*models.SchemaColumn, error)
```

**Files:**
- `pkg/repositories/schema_repository.go`

### Phase 5: Handler/Response Updates

**Update `GetRelationshipsResponse` to include review relationships:**
- Review relationships should appear in the relationships list
- Frontend can filter/display them differently based on `relationship_type`

**Files:**
- `pkg/handlers/schema.go` (if response format changes needed)

### Phase 6: Test Data Setup

The test database (`test_data`) may not have tables in the orphan state needed for testing. Need to add:

**New test tables in test container setup:**

```sql
-- Table with PK but no FK constraints defined
CREATE TABLE test_orphan_with_pk (
    id BIGINT PRIMARY KEY,
    name TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Table that SHOULD reference test_orphan_with_pk but has no FK
CREATE TABLE test_orphan_referencer (
    id BIGINT PRIMARY KEY,
    orphan_id BIGINT,  -- Should reference test_orphan_with_pk.id
    amount DECIMAL(10,2),
    count INTEGER,  -- Low cardinality count column (should NOT match)
    created_at TIMESTAMP DEFAULT NOW()
);

-- Populate with data where orphan_id values all exist in test_orphan_with_pk.id
INSERT INTO test_orphan_with_pk (id, name) VALUES
    (1, 'Item 1'), (2, 'Item 2'), (3, 'Item 3'),
    (4, 'Item 4'), (5, 'Item 5'), (6, 'Item 6'),
    (7, 'Item 7'), (8, 'Item 8'), (9, 'Item 9'), (10, 'Item 10');

INSERT INTO test_orphan_referencer (id, orphan_id, amount, count) VALUES
    (100, 1, 99.99, 5),
    (101, 1, 49.99, 3),
    (102, 2, 149.99, 5),
    (103, 3, 29.99, 2),
    (104, 5, 199.99, 5),
    (105, 7, 79.99, 1),
    (106, 10, 59.99, 5);
```

**Expected behavior:**
- `test_orphan_referencer.orphan_id` → `test_orphan_with_pk.id` should be found as review candidate
- `test_orphan_referencer.count` should NOT match (low cardinality, only values 1-5)
- `test_orphan_referencer.amount` should NOT match (decimal type, different from bigint)

**Files:**
- `scripts/test_data/init.sql` or equivalent test setup
- `pkg/services/relationship_discovery_integration_test.go`

### Phase 7: Integration Tests

**New test cases:**

1. `TestReviewCandidates_OrphanTableWithPK` - Verifies PK-only source selection
2. `TestReviewCandidates_OrphanTableWithoutPK` - Verifies all-columns source selection
3. `TestReviewCandidates_ExactTypeMatch` - Verifies bigint only matches bigint
4. `TestReviewCandidates_HighCardinalityRequired` - Verifies count columns are skipped
5. `TestReviewCandidates_ZeroOrphanRequired` - Verifies 100% match requirement
6. `TestReviewCandidates_SkipsExistingRelationships` - Verifies tables with FK/inferred are skipped

**Files:**
- `pkg/services/relationship_discovery_integration_test.go`

---

## Configuration Constants

```go
const (
    // ReviewMinCardinalityRatio is the minimum distinct/total ratio for review candidates
    // Columns with lower ratios look like counts, not join keys
    ReviewMinCardinalityRatio = 0.10  // 10%

    // ReviewMinDistinctCount is the minimum distinct values for review candidates
    // Small counts (1-100) are likely enums or status codes
    ReviewMinDistinctCount = 100

    // ReviewMaxOrphanRate for review candidates (must be 0 for strict matching)
    ReviewMaxOrphanRate = 0.0  // 0% - all values must exist in target
)
```

---

## Frontend Integration Notes

The frontend already has access to:
- Relationship type filtering
- LLM integration for the relationships page

Review relationships should:
1. Appear in the relationships list with a distinct visual indicator
2. Have an "Approve" / "Reject" action
3. Optionally show LLM-generated explanation of why this might be a valid relationship

Frontend changes are out of scope for this plan but the backend will provide all necessary data.

---

## File Summary

| File | Phase | Changes |
|------|-------|---------|
| `migrations/007_review_relationship_type.up.sql` | 1 | Add 'review' type documentation |
| `migrations/007_review_relationship_type.down.sql` | 1 | Rollback |
| `pkg/models/schema.go` | 2 | Add `RelationshipTypeReview` constant |
| `pkg/services/relationship_discovery.go` | 3 | Add review candidate discovery logic |
| `pkg/repositories/schema_repository.go` | 4 | Add `GetNonPKColumnsByType` method |
| `scripts/test_data/init.sql` | 6 | Add orphan test tables |
| `pkg/services/relationship_discovery_integration_test.go` | 7 | Add review candidate tests |

---

## Success Criteria

1. Tables with FK or inferred relationships are NOT processed for review candidates
2. Orphan tables with PK only use PK column as source
3. Orphan tables without PK use all numeric columns as source
4. Only exact type matches are considered (bigint→bigint, not int→bigint)
5. Low cardinality columns (counts) are excluded
6. Only 0% orphan rate candidates are stored
7. Stored with `relationship_type='review'` and `is_approved=NULL`
8. All existing tests continue to pass
9. New integration tests verify the algorithm
