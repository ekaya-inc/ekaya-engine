# Test: Column Probing

Test column statistics and metadata retrieval.

## Tools Under Test

- `probe_column`
- `probe_columns`

## Prerequisites

Run `get_schema` first to identify valid table and column names.

## Test Cases

### 1. Probe Single Column - Numeric
Call `probe_column` with a numeric column and verify:
- Returns statistics (min, max, avg, etc.)
- Returns null count
- Returns distinct count

### 2. Probe Single Column - Text
Call `probe_column` with a text/varchar column and verify:
- Returns appropriate statistics for text
- May include sample values or common values

### 3. Probe Single Column - Date/Timestamp
Call `probe_column` with a date column and verify:
- Returns date range (min, max)
- Handles timezone appropriately

### 4. Probe Single Column - Invalid
Call `probe_column` with non-existent column and verify:
- Returns appropriate error
- Specifies which part is invalid (table vs column)

### 5. Probe Multiple Columns
Call `probe_columns` with array of columns and verify:
- Returns results for each column
- Handles mix of valid/invalid gracefully
- More efficient than multiple single calls

### 6. Probe Columns - Empty Array
Call `probe_columns` with empty array and verify:
- Returns empty result or appropriate error
- Does not crash

## Report Format

```
=== 120-probe-columns: Column Probing ===

Test 1: Probe Numeric Column
  Column: [table.column]
  Has min/max: [yes/no]
  Has null count: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Probe Text Column
  Column: [table.column]
  Stats returned: [describe]
  RESULT: [PASS/FAIL]

Test 3: Probe Date Column
  Column: [table.column]
  Date range: [min] to [max]
  RESULT: [PASS/FAIL]

Test 4: Probe Invalid Column
  Column: [table.column]
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Probe Multiple Columns
  Columns requested: [count]
  Results returned: [count]
  RESULT: [PASS/FAIL]

Test 6: Probe Empty Array
  Behavior: [describe]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
