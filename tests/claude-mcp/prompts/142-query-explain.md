# Test: Query Explain/Analyze

Test query performance analysis.

## Tools Under Test

- `explain_query`

## Test Cases

### 1. Explain Simple Query
Call `explain_query` with simple SELECT and verify:
- Returns execution plan
- Shows scan type (seq scan, index scan, etc.)
- Shows estimated costs

### 2. Explain with ANALYZE
Call `explain_query` with analyze option (if available) and verify:
- Returns actual execution times
- Shows actual row counts vs estimates

### 3. Explain Join Query
Call `explain_query` with JOIN and verify:
- Shows join strategy (hash, merge, nested loop)
- Shows order of operations

### 4. Explain with Index Usage
Call `explain_query` with query that should use an index and verify:
- Shows index scan in plan
- Can identify if index is being used

### 5. Explain Invalid Query
Call `explain_query` with invalid SQL and verify:
- Returns error rather than plan
- Error message is helpful

## Report Format

```
=== 142-query-explain: Query Explain/Analyze ===

Test 1: Explain Simple Query
  Query: [query]
  Has execution plan: [yes/no]
  Shows costs: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Explain with ANALYZE
  Actual times included: [yes/no]
  Actual rows included: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Explain Join Query
  Join strategy shown: [value]
  RESULT: [PASS/FAIL]

Test 4: Explain with Index Usage
  Index usage shown: [yes/no]
  Index name: [value or N/A]
  RESULT: [PASS/FAIL]

Test 5: Explain Invalid Query
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
