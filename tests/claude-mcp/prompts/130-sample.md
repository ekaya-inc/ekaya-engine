# Test: Data Sampling

Test quick data preview functionality.

## Tools Under Test

- `sample`

## Prerequisites

Run `get_schema` first to identify valid table names.

## Test Cases

### 1. Sample - Default Limit
Call `sample` with just table name and verify:
- Returns rows from the table
- Uses reasonable default limit
- Includes column names

### 2. Sample - Custom Limit
Call `sample` with `limit` parameter and verify:
- Returns exactly the requested number of rows (or less if table is smaller)
- Respects the limit

### 3. Sample - Large Limit
Call `sample` with very large limit (e.g., 10000) and verify:
- Either returns capped results or appropriate error
- Does not timeout or crash

### 4. Sample - Invalid Table
Call `sample` with non-existent table and verify:
- Returns appropriate error message
- Does not expose internal details

### 5. Sample - Empty Table
If a known empty table exists, call `sample` and verify:
- Returns empty result set gracefully
- Column names still present

## Report Format

```
=== 130-sample: Data Sampling ===

Test 1: Sample - Default Limit
  Table: [name]
  Rows returned: [count]
  Default limit appears to be: [value]
  RESULT: [PASS/FAIL]

Test 2: Sample - Custom Limit
  Requested: [limit]
  Returned: [count]
  RESULT: [PASS/FAIL]

Test 3: Sample - Large Limit
  Requested: [limit]
  Behavior: [capped at X / returned all / error]
  RESULT: [PASS/FAIL]

Test 4: Sample - Invalid Table
  Table: "nonexistent_table_xyz"
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Sample - Empty Table
  Table: [name]
  Empty result handled: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
