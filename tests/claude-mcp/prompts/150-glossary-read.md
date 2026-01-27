# Test: Glossary Read Operations

Test business glossary retrieval.

## Tools Under Test

- `list_glossary`
- `get_glossary_sql`

## Test Cases

### 1. List All Glossary Terms
Call `list_glossary` with no filters and verify:
- Returns array of terms
- Each term has `term`, `definition`
- May include `sql` or `sql_preview`

### 2. List Glossary - With Search
Call `list_glossary` with search/filter parameter (if supported) and verify:
- Returns filtered results
- Search is case-insensitive (or document behavior)

### 3. Get Glossary SQL - Valid Term
Call `get_glossary_sql` with known term and verify:
- Returns the SQL definition
- SQL is valid and executable

### 4. Get Glossary SQL - Invalid Term
Call `get_glossary_sql` with non-existent term and verify:
- Returns appropriate error
- Does not crash

### 5. List Glossary - Empty
If no glossary terms exist, verify:
- Returns empty array
- Does not error

## Report Format

```
=== 150-glossary-read: Glossary Read Operations ===

Test 1: List All Glossary Terms
  Terms returned: [count]
  Has definitions: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: List Glossary - With Search
  Search term: [value]
  Results: [count]
  RESULT: [PASS/FAIL/SKIP if not supported]

Test 3: Get Glossary SQL - Valid Term
  Term: [name]
  SQL returned: [yes/no]
  SQL valid: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Get Glossary SQL - Invalid Term
  Term: "NonExistentTerm_XYZ"
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: List Glossary - Empty
  Behavior when empty: [empty array / message]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
