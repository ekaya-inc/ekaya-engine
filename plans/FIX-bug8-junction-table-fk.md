# FIX: Bug 8 - Junction Table FK Relationships Not Discovered

**Priority:** Medium
**Component:** FK Discovery / Schema Discovery
**Status:** ✅ FIXED

## Problem Statement

Junction tables with composite primary keys have their FK relationships ignored.

**Evidence:**
- `s3_enrollments` has FKs: `student_id → s3_students`, `course_code → s3_courses`
- `probe_relationship` returns no Enrollment→Student or Enrollment→Course relationships

## Root Cause Analysis ✅ COMPLETE

### The Bug Chain

The issue is a **data discovery problem** in schema introspection, not business logic.

### Primary Root Cause: Composite PK Columns Excluded

**File:** `pkg/adapters/datasource/postgres/schema.go:143`

```sql
-- In DiscoverColumns query (lines 135-144)
LEFT JOIN (
    SELECT a.attname as column_name, true as is_pk
    FROM pg_index ix
    ...
    WHERE ix.indisprimary = true
      AND n.nspname = $1
      AND t.relname = $2
      AND array_length(ix.indkey, 1) = 1  -- *** BUG: Single-column PKs only ***
) pk ON c.column_name = pk.column_name
```

This filter `array_length(ix.indkey, 1) = 1` **explicitly excludes composite PKs**.

### The Cascade Effect

1. **Schema Discovery** marks `s3_enrollments.student_id` and `s3_enrollments.course_code` as `IsPrimaryKey = false` (because they're part of a composite PK)

2. **Entity Discovery** (`entity_discovery_service.go:118`) looks for columns where `col.IsPrimaryKey == true`:
   ```go
   if col.IsPrimaryKey {
       candidates = append(candidates, entityCandidate{...})
   }
   ```
   Neither junction table column qualifies → No entity candidate created

3. **Fallback to Unique Constraint** (`entity_discovery_service.go:131-143`) only triggers if columns have individual unique constraints (rare for junction tables)

4. **FK Discovery** (`deterministic_relationship_service.go:181-186`) looks up source entity:
   ```go
   sourceEntity := entityByPrimaryTable[sourceKey]
   if sourceEntity == nil {
       continue // No entity owns this table - SKIPPED!
   }
   ```
   No entity for `s3_enrollments` → FKs from junction table are skipped

### What Should Happen vs. What Actually Happens

| Step | Expected | Actual |
|------|----------|--------|
| Schema discovery | `s3_enrollments.student_id` marked as PrimaryKey | Marked as NOT PrimaryKey |
| Entity discovery | Creates "Enrollment" entity | No entity created (no PK candidates) |
| FK discovery | Discovers Enrollment → Student | Skips (no Enrollment entity) |
| `probe_relationship` | Returns relationships | Returns `[]` (empty) |

## Recommended Fix

### Remove the Single-Column PK Filter

**File:** `pkg/adapters/datasource/postgres/schema.go:143`

```sql
-- Before:
AND array_length(ix.indkey, 1) = 1  -- Single-column PKs only

-- After: Remove this line entirely
-- All columns in composite PKs should be marked as IsPrimaryKey = true
```

The query already uses `a.attnum = ANY(ix.indkey)` which correctly joins all columns in the index. The `array_length` filter is unnecessarily restrictive.

### Also Consider: Unique Constraint Filter (Line 157)

Same issue exists for unique constraints:
```sql
AND array_length(ix.indkey, 1) = 1  -- Single-column unique indexes only
```

This may also need removal to support composite unique constraints.

## Files Modified

1. [x] **pkg/adapters/datasource/postgres/schema.go:143**
   - Removed `AND array_length(ix.indkey, 1) = 1` from PK detection
   - All columns in composite PKs now marked as `IsPrimaryKey = true`

2. [x] **pkg/adapters/datasource/postgres/schema.go:157**
   - Removed `AND array_length(ix.indkey, 1) = 1` from unique constraint detection
   - All columns in composite unique constraints now detected

## Testing Verification

After implementing:

1. Create junction table with composite PK:
   ```sql
   CREATE TABLE test_students (id SERIAL PRIMARY KEY, name VARCHAR(100));
   CREATE TABLE test_courses (id SERIAL PRIMARY KEY, title VARCHAR(100));
   CREATE TABLE test_enrollments (
       student_id INTEGER REFERENCES test_students(id),
       course_id INTEGER REFERENCES test_courses(id),
       enrolled_at TIMESTAMP,
       PRIMARY KEY (student_id, course_id)
   );
   ```

2. Run schema refresh: `refresh_schema`

3. Verify both columns are marked as PK:
   ```sql
   SELECT table_name, column_name, is_primary_key
   FROM engine_schema_columns
   WHERE table_name = 'test_enrollments';
   ```
   Expected: Both `student_id` and `course_id` have `is_primary_key = true`

4. Run ontology extraction

5. Verify entity exists:
   ```sql
   SELECT name, primary_table FROM engine_ontology_entities
   WHERE primary_table = 'test_enrollments';
   ```

6. Verify relationships via `probe_relationship(from_entity='Enrollment')`:
   - Should show: Enrollment → Student
   - Should show: Enrollment → Course

## Edge Cases to Consider

- Tables with composite PKs that are NOT junction tables (audit tables, etc.)
- Junction tables with additional non-PK columns (e.g., `grade`, `enrolled_at`)
- Junction tables with their own surrogate key (`id` + composite unique constraint)
- Self-referential junction tables (rare but possible)

## Why the Filter Was Added (Historical Context)

The `array_length = 1` filter was likely added to:
1. Avoid complexity in entity discovery (picking one "best" column)
2. Simplify cardinality calculations
3. Handle edge cases where multiple columns share PK status

However, excluding composite PKs entirely breaks junction table support, which is a common and important pattern.
