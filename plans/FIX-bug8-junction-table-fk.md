# FIX: Bug 8 - Junction Table FK Relationships Not Discovered

**Priority:** Medium
**Component:** FK Discovery

## Problem Statement

Junction tables with composite primary keys have their FK relationships ignored.

**Evidence:**
- `s3_enrollments` has FKs: `student_id → s3_students`, `course_code → s3_courses`
- `probe_relationship` returns no Enrollment→Student or Enrollment→Course relationships

## Root Cause Analysis

### Entity Discovery Does Create Entities for Junction Tables

Looking at `pkg/services/entity_discovery_service.go:107-154`, Entity Discovery:
1. Finds ALL primary key columns (line 118: `if col.IsPrimaryKey`)
2. Groups candidates by table (line 148-154: `bestByTable`)
3. Creates ONE entity per table using the first/best PK column

For junction tables with composite PKs (e.g., `student_id + course_code`):
- Both columns are marked as `IsPrimaryKey = true`
- ONE entity is created (e.g., "Enrollment") with one column as `PrimaryColumn`
- **Entity IS created** - this is not the bug

### The Real Issue: Schema Relationships May Not Exist

For FK Discovery to work (pkg/services/deterministic_relationship_service.go:145-228), it needs:
1. Schema relationships in `engine_schema_relationships` table
2. Both source and target entities to exist via `entityByPrimaryTable` lookup

The bug might be in one of these areas:

**Possibility 1: Schema Relationships Not Created for Junction Table FKs**

Check if `scan_data_changes` or schema introspection is populating `engine_schema_relationships` for junction table FKs. If the FK constraints exist but aren't in the schema_relationships table, FK Discovery won't find them.

**Possibility 2: Entity Lookup Fails**

FK Discovery does (lines 181-193):
```go
sourceKey := fmt.Sprintf("%s.%s", sourceTable.SchemaName, sourceTable.TableName)
sourceEntity := entityByPrimaryTable[sourceKey]
if sourceEntity == nil {
    continue // No entity owns this table
}
```

If the junction table's entity isn't in `entityByPrimaryTable`, the FK is skipped.

**Possibility 3: Composite PK Entity Created with Wrong Column**

Entity Discovery picks one column from the composite PK as `PrimaryColumn`. FK Discovery might use this column for lookups, and if it's not the FK column, matching fails.

## Investigation Required

### Step 1: Verify Entities Exist

```sql
-- Check if junction table entity exists
SELECT name, primary_table, primary_column FROM engine_ontology_entities
WHERE primary_table = 's3_enrollments';
```

### Step 2: Verify Schema Relationships Exist

```sql
-- Check if FK relationships are in schema_relationships
SELECT source_column_id, target_column_id, relationship_type
FROM engine_schema_relationships
WHERE relationship_type = 'foreign_key';
```

### Step 3: Check entityByPrimaryTable Map Contents

Add logging to see what entities are in the map:
```go
for key, entity := range entityByPrimaryTable {
    s.logger.Debug("Entity in lookup map",
        zap.String("key", key),
        zap.String("entity", entity.Name))
}
```

## Recommended Fix

### If Schema Relationships Missing (Most Likely)

Fix the schema introspection to detect and store junction table FK constraints:

1. Check `pkg/services/schema_introspection.go` or similar for FK detection
2. Verify it handles composite FK columns
3. Ensure FKs from junction tables are stored in `engine_schema_relationships`

### If Entity Not in Lookup Map

Check that Entity Discovery correctly creates entities for all tables:

1. Verify `entityByPrimaryTable` is populated from ALL entities (line 132-136)
2. Ensure junction table entities aren't filtered out

## Many-to-Many Representation

For a proper data model, junction tables like `s3_enrollments`:
- Should have an entity (e.g., "Enrollment")
- Should have relationships:
  - Enrollment → Student (N:1)
  - Enrollment → Course (N:1)
- The M:N relationship (Student ↔ Course) is implicit via the junction

## Files to Investigate

1. **pkg/services/schema_introspection.go** (or similar) - FK detection from database
2. **pkg/services/schema_change_detection.go** - Schema relationship creation
3. **pkg/services/deterministic_relationship_service.go:132-136** - Entity lookup map
4. **pkg/services/entity_discovery_service.go:148-154** - Entity creation for composite PKs

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
   INSERT INTO test_students VALUES (1, 'Alice'), (2, 'Bob');
   INSERT INTO test_courses VALUES (1, 'Math'), (2, 'Science');
   INSERT INTO test_enrollments VALUES (1, 1, NOW()), (1, 2, NOW()), (2, 1, NOW());
   ```

2. Run schema refresh: `refresh_schema`

3. Run ontology extraction

4. Verify entities exist:
   - Student entity
   - Course entity
   - Enrollment entity (junction)

5. Call `probe_relationship(from_entity='Enrollment')`:
   - Should show: Enrollment → Student
   - Should show: Enrollment → Course

6. Verify via direct query:
   ```sql
   SELECT e1.name as from_entity, e2.name as to_entity,
          r.source_column_table, r.source_column_name,
          r.target_column_table, r.target_column_name
   FROM engine_entity_relationships r
   JOIN engine_ontology_entities e1 ON r.source_entity_id = e1.id
   JOIN engine_ontology_entities e2 ON r.target_entity_id = e2.id
   WHERE e1.primary_table = 'test_enrollments' OR e2.primary_table = 'test_enrollments';
   ```

## Edge Cases

- Tables with composite PKs that are NOT junction tables (audit tables, etc.)
- Junction tables with additional non-PK columns (e.g., `grade`, `enrolled_at`)
- Junction tables with their own surrogate key (`id` + composite unique constraint)
- Self-referential junction tables (rare but possible)
