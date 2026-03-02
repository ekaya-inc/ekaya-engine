# ISSUE: row_count = -1 stored for all tables, breaks empty/orphan classification

**Date:** 2026-03-02
**Status:** FIXED
**Priority:** HIGH
**Observed on:** ekaya_marketing datasource (project 21bfc3bf)

## Observation

All 12 tables in `engine_schema_tables` have `row_count = -1`:

```sql
SELECT table_name, row_count FROM engine_schema_tables
WHERE project_id = '21bfc3bf-...' AND deleted_at IS NULL;
-- All 12 rows: row_count = -1
```

Actual data reality: `weekly_metrics` has 0 rows, `competitors` has 4 rows, most other tables have data. The stored row_count is wrong for every table.

## Root Cause

PostgreSQL's `pg_class.reltuples` returns `-1` for tables that have never been `ANALYZE`d. The discovery query in `pkg/adapters/datasource/postgres/schema.go:103-114` uses:

```sql
COALESCE(c.reltuples::bigint, 0) as row_count
```

`COALESCE` only handles NULL (no pg_class entry). It does NOT handle `-1` (pg_class entry exists but table was never analyzed). The `-1` passes through and gets stored in `engine_schema_tables.row_count`.

## Impact

`GetEmptyTables` and `GetOrphanTables` in `pkg/repositories/schema_repository.go` both have blind spots for `-1`:

| Query | Condition | Catches -1? |
|-------|-----------|-------------|
| `GetEmptyTables` | `row_count IS NULL OR row_count = 0` | No |
| `GetOrphanTables` (before hotfix) | `row_count IS NOT NULL AND row_count > 0` | No |

This caused the Relationships page to show "2 tables without relationships" in the warning banner (from a computed fallback) but the "No Relationships" section had no tables listed.

A hotfix was applied to `GetOrphanTables` to use `row_count IS NULL OR row_count != 0` so tables with unknown counts are treated as non-empty. This is a workaround — it means `weekly_metrics` (actually 0 rows) shows as an orphan table instead of an empty table.

## Fix

### 1. Fix the discovery query (root cause)

In `pkg/adapters/datasource/postgres/schema.go:107`, change:

```sql
COALESCE(c.reltuples::bigint, 0) as row_count
```

to:

```sql
GREATEST(COALESCE(c.reltuples::bigint, 0), 0) as row_count
```

This clamps `-1` to `0`, which correctly means "no known rows." Tables with 0 row_count will then be caught by `GetEmptyTables`.

### 2. Check MSSQL adapter

Verify `pkg/adapters/datasource/mssql/schema.go` doesn't have the same issue with its row count discovery query.

### 3. Add tests

There are no tests covering:
- `DiscoverTables` returning `-1` from `pg_class.reltuples` for unanalyzed tables
- `GetEmptyTables` / `GetOrphanTables` behavior when `row_count = -1`
- The classification correctness: every table should appear in exactly one of {has relationships, orphan, empty}

### 4. Consider re-running schema refresh

Existing datasources with `-1` row counts will need a schema refresh to get corrected values. Alternatively, a migration could clamp existing `-1` values to `0`.

## Files

| File | Issue |
|------|-------|
| `pkg/adapters/datasource/postgres/schema.go:107` | COALESCE doesn't handle -1 from reltuples |
| `pkg/repositories/schema_repository.go:1261-1305` | GetEmptyTables misses row_count = -1 |
| `pkg/repositories/schema_repository.go:1307-1360` | GetOrphanTables misses row_count = -1 (hotfixed) |
| `pkg/adapters/datasource/mssql/schema.go` | Needs audit for same issue |
