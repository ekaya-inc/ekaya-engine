# FIX: BUG-3 - Missing FK Relationships in Billing Engagement

**Bug Reference:** BUGS-ontology-extraction.md, BUG-3
**Severity:** High
**Type:** Data Quality Issue

## Problem Summary

The "Billing Engagement" entity has 0 relationships despite having obvious FK columns:
- `visitor_id` → users.user_id
- `host_id` → users.user_id
- `session_id` → sessions.session_id
- `offer_id` → offers.offer_id

These columns are filtered out because they have `DistinctCount = NULL`.

## Root Cause

**File:** `pkg/services/deterministic_relationship_service.go`

**Lines 553-559:**
```go
// Require stats to exist (fail-fast on missing data)
// Note: While IsJoinable=true typically implies stats exist (from classifyJoinability),
// PK columns can be marked joinable without stats. This defensive check prevents
// nil pointer access during cardinality filtering.
if col.DistinctCount == nil {
    continue // No stats = cannot evaluate = skip
}
```

Columns with `DistinctCount = NULL` are unconditionally skipped. This affects:
- `visitor_id` (NULL distinct_count, is_joinable = true)
- `host_id` (NULL distinct_count, is_joinable = true)
- `offer_id` (NULL distinct_count, is_joinable = true)

Only `session_id` (distinct_count = 69) passed this filter.

## Why Stats Are NULL

Stats collection runs during `DiscoverRelationships` (line 301):
```go
stats, err := discoverer.AnalyzeColumnStats(ctx, table.SchemaName, table.TableName, columnNames)
if err != nil {
    s.logger.Warn("Failed to analyze column stats", ...)
    continue  // Skip entire table on error
}
```

Possible causes:
1. Stats collection failed for the billing_engagements table
2. Column stats weren't persisted correctly
3. Text UUID columns have different collection behavior
4. Race condition between stats collection and relationship discovery

**This is directly connected to BUG-9** which documents the systemic stats collection failure affecting 27% of joinable columns.

## The Fix Options

### Option A: Add Pattern-Based Exemption (Quick Fix, NOT Recommended)

Add another `_id` exemption like existing ones:
```go
if col.DistinctCount == nil {
    if !isLikelyFKColumn(col.ColumnName) {
        continue
    }
    // _id columns with nil stats proceed to validation
}
```

**Why NOT recommended:** This perpetuates the pattern-matching approach (BUG-1). We should fix stats collection (BUG-9) instead.

### Option B: Use Join Validation for NULL-Stats Columns (Recommended)

Instead of skipping columns with NULL stats, pass them through join validation:
```go
// For columns with stats, apply cardinality filters
if col.DistinctCount != nil {
    if *col.DistinctCount < 20 {
        continue
    }
    if table.RowCount != nil && *table.RowCount > 0 {
        ratio := float64(*col.DistinctCount) / float64(*table.RowCount)
        if ratio < 0.05 && !isJoinableColumn(col) {
            continue
        }
    }
}
// Columns with NULL stats but is_joinable=true proceed to validation
// Join validation will determine if they're valid FK candidates
```

**Why recommended:** Let actual data validation (CheckValueOverlap) decide instead of pre-filtering.

### Option C: Fix Stats Collection (Root Cause Fix)

Fix BUG-9 to ensure stats are collected for all columns. Once stats are reliable, the NULL check becomes a safety net rather than a filter.

**Why recommended:** Addresses root cause. Should be done alongside Option B.

## Implementation Plan

### Phase 1: Remove Blocking Filter (Option B)

**File:** `pkg/services/deterministic_relationship_service.go`

Change lines 553-573 to only apply cardinality filters when stats exist:

```go
// Apply cardinality filters only if stats exist
if col.DistinctCount != nil {
    // Check cardinality threshold
    if *col.DistinctCount < 20 {
        continue
    }
    // Check cardinality ratio if row count available
    if table.RowCount != nil && *table.RowCount > 0 {
        ratio := float64(*col.DistinctCount) / float64(*table.RowCount)
        // Skip ratio check for likely FK columns
        if ratio < 0.05 && !isLikelyFKColumn(col.ColumnName) {
            continue
        }
    }
}
// Columns without stats but with is_joinable=true proceed to validation
// Join validation will determine actual FK validity
```

### Phase 2: Add Logging for NULL-Stats Candidates

Add debug logging to track columns passing through without stats:
```go
if col.DistinctCount == nil {
    s.logger.Debug("Including column with NULL stats for validation",
        zap.String("table", table.TableName),
        zap.String("column", col.ColumnName),
        zap.Bool("is_joinable", col.IsJoinable != nil && *col.IsJoinable))
}
```

### Phase 3: Fix Stats Collection (BUG-9)

This is documented in FIX-ontology-extraction-bug9.md. Fixing stats collection will reduce the number of NULL-stats columns, making the filtering more effective.

## Testing

### Verify Billing Engagement Relationships

After fix, run relationship discovery and verify:
```sql
SELECT e1.name as from_entity, e2.name as to_entity, r.relationship_type
FROM engine_entity_relationships r
JOIN engine_ontology_entities e1 ON r.source_entity_id = e1.id
JOIN engine_ontology_entities e2 ON r.target_entity_id = e2.id
WHERE e1.name = 'Billing Engagement';

-- Expected results:
-- Billing Engagement | User    | visitor
-- Billing Engagement | User    | host
-- Billing Engagement | Session | references
-- Billing Engagement | Offer   | references
```

### Verify Join Validation Works

Check that columns without stats are validated via CheckValueOverlap:
```sql
-- visitor_id should have high overlap with users.user_id
SELECT COUNT(*) as valid_refs
FROM billing_engagements be
WHERE be.visitor_id IN (SELECT user_id FROM users);

-- Should return close to total rows (high overlap = valid FK)
```

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/deterministic_relationship_service.go` | Remove blocking NULL stats filter |
| `pkg/services/deterministic_relationship_service_test.go` | Add test for NULL-stats columns |

## Success Criteria

- [ ] Billing Engagement shows 4+ relationships (visitor, host, session, offer)
- [ ] Columns with NULL stats but `is_joinable=true` are evaluated
- [ ] Join validation correctly identifies valid FK relationships
- [ ] Performance impact measured and acceptable
- [ ] Integration test verifies NULL-stats handling

## Dependencies

- **BUG-9** (Stats collection): Fixing stats collection reduces NULL-stats columns
- **BUG-1** (Pattern matching): Avoid adding more pattern-based exemptions

## Notes

The existing code has `_id` exemptions in two places:
1. Lines 544-548: IsJoinable=NULL but `_id` suffix → include anyway
2. Lines 565-572: Low cardinality ratio but `_id` suffix → skip ratio check

These exemptions partially work around the issue but only for pattern-matching column names. The recommended fix works for ALL columns regardless of naming convention.
