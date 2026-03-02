# FIX: False-Positive Schema Changes After Refresh

**Status:** FIXED
**Branch:** TBD
**Created:** 2026-03-02

## Problem

When a user refreshes the schema via the UI, the system reports "Schema changes detected: 12 modified tables, 146 modified columns" even when no actual schema changes occurred. This causes a misleading prompt to "Refresh Ontology" when no refresh is needed.

### Root Cause

The chain of events:

1. **Schema refresh** calls `UpsertTable()` and `UpsertColumn()` for every table/column
2. **`UpsertTable`** (schema_repository.go:255) unconditionally sets `updated_at = now()` before the upsert
3. **`UpsertColumn`** (schema_repository.go:722) does the same
4. **Both tables have a `BEFORE UPDATE` trigger** (`update_updated_at_column()` from migration 001) that sets `NEW.updated_at = NOW()` on ANY row update
5. **`ComputeChangeSet`** (ontology_dag_incremental.go:259-263, 334-342) detects "modified" items via `WHERE updated_at > builtAt AND created_at <= builtAt`
6. **Result:** Every row gets `updated_at` bumped, so everything appears modified

### What Should Trigger an Ontology Refresh

**Tables:** new table added, table dropped, `is_selected` changed
**Columns:** new column added, column dropped, `data_type` changed, `is_nullable` changed, `is_primary_key` changed, `is_unique` changed, `default_value` changed, `is_selected` changed

**Should NOT trigger:** `row_count` changes (tables), `ordinal_position` changes (columns), `distinct_count`/`null_count`/`min_length`/`max_length` stat updates (columns)

## Solution: Conditional `updated_at` Triggers

Replace the generic `update_updated_at_column()` trigger on `engine_schema_tables` and `engine_schema_columns` with table-specific trigger functions that only bump `updated_at` when ontology-relevant fields change. This keeps the existing change detection logic (`ComputeChangeSet`) working unchanged.

**Why this over a new flag column:**
- No new schema columns to manage
- No flag lifecycle to maintain (set on change, clear after extraction)
- Existing `ComputeChangeSet` queries work unchanged
- `updated_at` naturally gets the correct semantics: "when schema content that matters last changed"

## Implementation

### Task 1: Migration — Replace triggers with conditional versions

**File:** `migrations/013_conditional_schema_triggers.up.sql`

```sql
-- Replace the generic trigger on engine_schema_tables with a conditional one
-- that only bumps updated_at when ontology-relevant fields change.

CREATE OR REPLACE FUNCTION update_schema_tables_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Only bump updated_at when ontology-relevant fields change.
    -- row_count changes alone should NOT trigger ontology re-extraction.
    IF (OLD.is_selected IS DISTINCT FROM NEW.is_selected)
       OR (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    THEN
        NEW.updated_at = NOW();
    ELSE
        NEW.updated_at = OLD.updated_at;
    END IF;
    RETURN NEW;
END;
$$;

-- Drop old trigger and create new one
DROP TRIGGER IF EXISTS update_engine_schema_tables_updated_at ON engine_schema_tables;
CREATE TRIGGER update_engine_schema_tables_updated_at
    BEFORE UPDATE ON engine_schema_tables
    FOR EACH ROW EXECUTE FUNCTION update_schema_tables_updated_at();


-- Replace the generic trigger on engine_schema_columns with a conditional one.
CREATE OR REPLACE FUNCTION update_schema_columns_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Only bump updated_at when ontology-relevant fields change.
    -- Stats (distinct_count, null_count, min_length, max_length) and
    -- ordinal_position changes should NOT trigger ontology re-extraction.
    IF (OLD.data_type IS DISTINCT FROM NEW.data_type)
       OR (OLD.is_nullable IS DISTINCT FROM NEW.is_nullable)
       OR (OLD.is_primary_key IS DISTINCT FROM NEW.is_primary_key)
       OR (OLD.is_unique IS DISTINCT FROM NEW.is_unique)
       OR (OLD.default_value IS DISTINCT FROM NEW.default_value)
       OR (OLD.is_selected IS DISTINCT FROM NEW.is_selected)
       OR (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    THEN
        NEW.updated_at = NOW();
    ELSE
        NEW.updated_at = OLD.updated_at;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS update_engine_schema_columns_updated_at ON engine_schema_columns;
CREATE TRIGGER update_engine_schema_columns_updated_at
    BEFORE UPDATE ON engine_schema_columns
    FOR EACH ROW EXECUTE FUNCTION update_schema_columns_updated_at();
```

**File:** `migrations/013_conditional_schema_triggers.down.sql`

```sql
-- Revert to the generic trigger function
DROP TRIGGER IF EXISTS update_engine_schema_tables_updated_at ON engine_schema_tables;
CREATE TRIGGER update_engine_schema_tables_updated_at
    BEFORE UPDATE ON engine_schema_tables
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_engine_schema_columns_updated_at ON engine_schema_columns;
CREATE TRIGGER update_engine_schema_columns_updated_at
    BEFORE UPDATE ON engine_schema_columns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Clean up custom functions
DROP FUNCTION IF EXISTS update_schema_tables_updated_at();
DROP FUNCTION IF EXISTS update_schema_columns_updated_at();
```

- [x] Create migration up/down files

### Task 2: Remove explicit `updated_at` assignment in upsert Go code

The Go code currently sets `updated_at = now()` before the upsert, and also passes it in the `DO UPDATE SET updated_at = EXCLUDED.updated_at` clause. The trigger now controls `updated_at`, so:

**File:** `pkg/repositories/schema_repository.go`

**`UpsertTable` (line 249):**
- Remove `table.UpdatedAt = now` (line 256) for the upsert path — the trigger handles it
- Keep `updated_at` in the INSERT VALUES (for new records, since the trigger is BEFORE UPDATE only)
- Remove `updated_at = EXCLUDED.updated_at` from the DO UPDATE SET clause
- The reactivation query (soft-delete → reactivate) should keep setting `updated_at` since that IS an ontology-relevant change

**`UpsertColumn` (line 716):**
- Same pattern: remove `updated_at` from DO UPDATE SET, keep in INSERT VALUES
- Keep `updated_at` in the reactivation query

- [x] Update `UpsertTable` — remove `updated_at` from DO UPDATE SET
- [x] Update `UpsertColumn` — remove `updated_at` from DO UPDATE SET

### Task 3: Integration test — verify no false positives

Add an integration test that:
1. Creates a datasource with tables/columns
2. Runs ontology extraction (sets `completed_at`)
3. Refreshes the schema (re-upserts same tables/columns)
4. Calls `ComputeChangeSet` and asserts `IsEmpty() == true`
5. Then modifies a column's `data_type` and verifies `IsEmpty() == false`

**File:** `pkg/services/ontology_dag_incremental_test.go` (or integration test file)

- [x] Add test: schema refresh without changes produces empty ChangeSet
- [x] Add test: actual schema change (e.g., data_type) produces non-empty ChangeSet

### Task 4: Verify existing tests pass

- [x] Run `make check` to ensure all existing tests still pass
- [x] Verify the `schema_change_detection_test.go` tests still work correctly
