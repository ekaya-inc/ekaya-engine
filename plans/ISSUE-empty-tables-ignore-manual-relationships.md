# ISSUE: Empty tables with manual relationships still appear in "No Relationships" section

**Date:** 2026-03-02
**Status:** FIXED
**Priority:** MEDIUM

## Observation

After manually adding relationships for empty tables (e.g., `paid_placements.id -> lead_magnet_leads.post_id`), the Relationships page still shows these tables in the "No Relationships" section with the "Empty" badge. Expected behavior: tables with any relationship (including manual) should not appear in the "No Relationships" section.

## Steps to Reproduce

1. Go to the Relationships page
2. Scroll to the "No Relationships" section
3. Click "+ Add Relationship" on an empty table (e.g., `lead_magnet_leads`)
4. Create a manual relationship
5. Observe that the table still appears in the "No Relationships" section as "Empty"

## Root Cause

`GetEmptyTables` in `pkg/repositories/schema_repository.go:1261-1305` returns tables based solely on `row_count`:

```sql
SELECT table_name
FROM engine_schema_tables
WHERE project_id = $1
  AND deleted_at IS NULL
  AND (row_count IS NULL OR row_count <= 0)
```

It has **no check against `engine_schema_relationships`**. Compare with `GetOrphanTables` which correctly excludes tables that have relationships via a `NOT EXISTS` subquery.

The assumption was that empty tables can't have relationships (since auto-discovery requires data). But admins can manually add relationships for empty tables, breaking this assumption.

## Fix

Add the same `NOT EXISTS` relationship check to `GetEmptyTables`:

```sql
SELECT t.table_name
FROM engine_schema_tables t
WHERE t.project_id = $1
  AND t.deleted_at IS NULL
  AND (t.row_count IS NULL OR t.row_count <= 0)
  AND NOT EXISTS (
      SELECT 1 FROM engine_schema_relationships r
      WHERE r.deleted_at IS NULL
        AND r.rejection_reason IS NULL
        AND (r.source_table_id = t.id OR r.target_table_id = t.id)
  )
```

This mirrors the existing logic in `GetOrphanTables` and ensures tables with any relationship type (FK, inferred, or manual) are excluded from the "empty" list.

## Files

| File | Change |
|------|--------|
| `pkg/repositories/schema_repository.go` | Add `NOT EXISTS` relationship check to `GetEmptyTables` query |
