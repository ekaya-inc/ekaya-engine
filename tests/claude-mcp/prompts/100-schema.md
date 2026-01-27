# Test: Schema Tools

Test schema retrieval and search capabilities.

## Tools Under Test

- `get_schema`
- `search_schema`

## Test Cases

### 1. Get Full Schema
Call `get_schema` with no parameters and verify:
- Returns JSON with `tables` array
- Each table has `name`, `columns` array
- Columns have `name`, `type`, and optional semantic annotations

### 2. Get Schema for Specific Table
Call `get_schema` with `table_name` parameter and verify:
- Returns only the specified table
- Error handling for non-existent table

### 3. Search Schema - Table Name
Call `search_schema` with a known table name fragment and verify:
- Returns matching tables
- Results include relevance information

### 4. Search Schema - Column Name
Call `search_schema` with a known column name and verify:
- Returns tables containing that column
- Shows column context in results

### 5. Search Schema - No Results
Call `search_schema` with nonsense query and verify:
- Returns empty results gracefully
- No error thrown

## Report Format

```
=== 100-schema: Schema Tools ===

Test 1: Get Full Schema
  Tables returned: [count]
  Has columns: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Get Schema for Specific Table
  Table requested: [name]
  Table returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Search Schema - Table Name
  Query: [value]
  Results found: [count]
  RESULT: [PASS/FAIL]

Test 4: Search Schema - Column Name
  Query: [value]
  Results found: [count]
  RESULT: [PASS/FAIL]

Test 5: Search Schema - No Results
  Query: [value]
  Empty result handled: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
