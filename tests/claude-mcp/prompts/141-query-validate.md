# Test: SQL Validation

Test SQL syntax validation without execution.

## Tools Under Test

- `validate`

## Test Cases

### 1. Valid SELECT
Call `validate` with valid SELECT and verify:
- Returns valid/success indicator
- Does not execute the query
- May return query plan info

### 2. Valid Complex Query
Call `validate` with complex multi-table query and verify:
- Validates successfully
- Handles JOINs, subqueries, CTEs

### 3. Invalid Syntax
Call `validate` with malformed SQL and verify:
- Returns invalid indicator
- Provides helpful error message
- Indicates position of error if possible

### 4. Invalid Table Reference
Call `validate` with non-existent table and verify:
- Returns invalid indicator
- Specifies which table doesn't exist

### 5. Invalid Column Reference
Call `validate` with non-existent column and verify:
- Returns invalid indicator
- Specifies which column doesn't exist

### 6. Write Statement Validation
Call `validate` with INSERT/UPDATE and verify:
- Either validates syntax (without executing) or rejects
- Document actual behavior

## Report Format

```
=== 141-query-validate: SQL Validation ===

Test 1: Valid SELECT
  Query: [query]
  Valid: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Valid Complex Query
  Query: [query with JOINs]
  Valid: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Invalid Syntax
  Query: [malformed]
  Invalid detected: [yes/no]
  Error message helpful: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Invalid Table Reference
  Query: SELECT * FROM nonexistent_xyz
  Invalid detected: [yes/no]
  Table identified: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Invalid Column Reference
  Query: SELECT bad_column FROM [table]
  Invalid detected: [yes/no]
  Column identified: [yes/no]
  RESULT: [PASS/FAIL]

Test 6: Write Statement Validation
  Query: INSERT INTO ...
  Behavior: [validates syntax / rejects]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
