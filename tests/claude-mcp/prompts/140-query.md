# Test: SQL Query Execution

Test read-only SQL query execution.

## Tools Under Test

- `query`

## Test Cases

### 1. Simple SELECT
Call `query` with simple SELECT statement and verify:
- Returns result rows
- Column names included
- Data types preserved

### 2. SELECT with WHERE
Call `query` with WHERE clause and verify:
- Filtering works correctly
- Returns subset of data

### 3. SELECT with JOIN
Call `query` with JOIN between tables and verify:
- Join executes correctly
- Combined columns returned

### 4. SELECT with Aggregation
Call `query` with COUNT/SUM/AVG and verify:
- Aggregation executes
- Returns single row with result

### 5. Write Statement Blocked
Call `query` with INSERT/UPDATE/DELETE and verify:
- Statement is rejected
- Appropriate error message returned
- No data modified

### 6. Invalid SQL
Call `query` with malformed SQL and verify:
- Returns syntax error
- Error message is helpful

### 7. Query Timeout (if applicable)
Call `query` with expensive query and verify:
- Timeout handling works
- Returns appropriate error

## Report Format

```
=== 140-query: SQL Query Execution ===

Test 1: Simple SELECT
  Query: SELECT * FROM [table] LIMIT 5
  Rows returned: [count]
  RESULT: [PASS/FAIL]

Test 2: SELECT with WHERE
  Query: [query]
  Filtering worked: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: SELECT with JOIN
  Query: [query]
  Join worked: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: SELECT with Aggregation
  Query: [query]
  Result: [value]
  RESULT: [PASS/FAIL]

Test 5: Write Statement Blocked
  Query: INSERT INTO ...
  Blocked: [yes/no]
  Error message: [value]
  RESULT: [PASS/FAIL]

Test 6: Invalid SQL
  Query: "SELEKT * FORM table"
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 7: Query Timeout
  Behavior: [describe]
  RESULT: [PASS/FAIL/SKIP]

OVERALL: [PASS/FAIL]
```
