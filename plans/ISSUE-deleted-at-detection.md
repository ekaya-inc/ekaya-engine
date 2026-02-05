# Issue: deleted_at Columns Not Detected as Soft-Delete Timestamps

**Status:** FIXED (2026-02-05)

Timestamp classification has been improved with:
1. Semantic guidance: "null rate indicates frequency, not purpose - a 2% non-null rate can still indicate soft delete"
2. Explicit `soft_delete` purpose in classification options
3. Cross-column validation phase for soft delete timestamps
4. `IsSoftDelete` field in timestamp features

See `pkg/services/column_feature_extraction.go` lines 962, 971, 998.

## Observed Behavior (Historical)

The `deleted_at` column on `media` table is incorrectly classified:

```
Column: deleted_at
semantic_type: audit_updated
role: attribute
description: Records when the record was last modified.
```

This is wrong. The column is a **soft-delete timestamp**, not an audit field for last modification.

## Expected Behavior

The column should be classified as:

```
semantic_type: soft_delete (or audit_deleted)
role: attribute
description: Timestamp indicating when the record was soft-deleted. NULL means active/not deleted.
timestamp_features:
  is_audit_field: true
  is_soft_delete: true
  timestamp_purpose: soft_delete
```

## Data Pattern That Should Be Detected

On the `media` table:
- ~5% of rows have `deleted_at IS NULL` (active records)
- ~95% of rows have `deleted_at IS NOT NULL` (soft-deleted records)

This distribution pattern is a strong signal:
1. Column name contains "deleted"
2. High percentage of non-NULL values
3. NULL means "active", non-NULL means "deleted"

## Root Cause

The column feature extraction is likely:
1. Not giving enough weight to the column name pattern (`deleted_at`, `deleted_on`, etc.)
2. Not considering the NULL/non-NULL distribution as a semantic signal
3. Possibly confusing it with `updated_at` due to similar timestamp data type

## Suggested Fix

1. **Column name priority**: If column name matches `deleted_at`, `deleted_on`, `deleted_date`, etc., strongly prefer `soft_delete` classification over `audit_updated`

2. **NULL distribution signal**: When a timestamp column has:
   - Mostly NULL values → likely optional audit field
   - Mostly non-NULL values with column name containing "deleted" → soft-delete pattern

3. **Ambiguity handling**: If the LLM detects ambiguity between `audit_updated` and `soft_delete`, it should create an ontology question asking the user to clarify the column's purpose

## Impact

Incorrect classification affects:
- Query generation (WHERE clauses should filter `deleted_at IS NULL` for active records)
- Data understanding (users need to know which records are "live")
- Relationship discovery (FKs pointing to soft-deleted records may appear as orphans)

## Related Tables

Check other tables with `deleted_at` columns to see if the same misclassification occurs.
