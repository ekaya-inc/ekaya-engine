# Test: DDL/DML Execution

Test executing DDL and DML statements.

## Tools Under Test

- `execute`

## Warning

This tool can modify data. Use test tables only.

## Test Data Convention

All test tables use the `_mcp_test` suffix.

## Test Cases

### 1. Create Test Table
Call `execute` with CREATE TABLE and verify:
- Table is created
- Appears in schema
- Correct structure

```sql
CREATE TABLE IF NOT EXISTS mcp_test_table (
  id SERIAL PRIMARY KEY,
  name VARCHAR(100),
  created_at TIMESTAMP DEFAULT NOW()
)
```

### 2. Insert Data
Call `execute` with INSERT and verify:
- Row is inserted
- Returns affected row count
- Data is queryable

```sql
INSERT INTO mcp_test_table (name) VALUES ('Test Row 1')
```

### 3. Update Data
Call `execute` with UPDATE and verify:
- Row is updated
- Returns affected row count
- Change is persisted

### 4. Delete Data
Call `execute` with DELETE and verify:
- Row is deleted
- Returns affected row count
- Data is removed

### 5. Execute Invalid SQL
Call `execute` with malformed SQL and verify:
- Returns syntax error
- No partial execution

### 6. Execute with Transaction (if supported)
Test transaction support:
- BEGIN/COMMIT behavior
- ROLLBACK behavior

### 7. Drop Test Table
Call `execute` with DROP TABLE and verify:
- Table is removed
- No longer in schema

```sql
DROP TABLE IF EXISTS mcp_test_table
```

## Report Format

```
=== 280-execute: DDL/DML Execution ===

Test 1: Create Test Table
  Created: [yes/no]
  In schema: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Insert Data
  Rows affected: [count]
  Data queryable: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update Data
  Rows affected: [count]
  Change persisted: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Delete Data
  Rows affected: [count]
  Data removed: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Execute Invalid SQL
  Error returned: [yes/no]
  No partial execution: [yes/no]
  RESULT: [PASS/FAIL]

Test 6: Execute with Transaction
  Transaction support: [yes/no/partial]
  RESULT: [PASS/FAIL/SKIP]

Test 7: Drop Test Table
  Dropped: [yes/no]
  Removed from schema: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
