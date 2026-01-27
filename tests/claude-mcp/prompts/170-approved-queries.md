# Test: Approved Queries Read Operations

Test pre-approved query listing and execution.

## Tools Under Test

- `list_approved_queries`
- `get_query_history`
- `execute_approved_query`

## Test Cases

### 1. List Approved Queries
Call `list_approved_queries` and verify:
- Returns array of approved queries
- Each has `id`, `name`, `description`, `sql`
- May include `parameters` definition

### 2. List Approved Queries - Empty
If no approved queries exist, verify:
- Returns empty array
- Does not error

### 3. Get Query History
Call `get_query_history` and verify:
- Returns recent query executions
- Each has timestamp, query, result summary
- Ordered by recency

### 4. Execute Approved Query - No Parameters
Call `execute_approved_query` with query that has no parameters and verify:
- Executes successfully
- Returns query results
- Logs to query history

### 5. Execute Approved Query - With Parameters
Call `execute_approved_query` with parameterized query and verify:
- Parameters are substituted correctly
- Results reflect parameter values
- SQL injection via parameters is prevented

### 6. Execute Approved Query - Invalid ID
Call `execute_approved_query` with non-existent ID and verify:
- Returns appropriate error
- Does not execute anything

### 7. Execute Approved Query - Missing Parameters
Call `execute_approved_query` without required parameters and verify:
- Returns error indicating missing parameters
- Does not execute partial query

## Report Format

```
=== 170-approved-queries: Approved Queries Read Operations ===

Test 1: List Approved Queries
  Queries returned: [count]
  Has required fields: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: List Approved Queries - Empty
  Behavior when empty: [empty array / message]
  RESULT: [PASS/FAIL]

Test 3: Get Query History
  History entries: [count]
  Has timestamps: [yes/no]
  Ordered by recency: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Execute Approved Query - No Parameters
  Query ID: [id]
  Executed: [yes/no]
  Results returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Execute Approved Query - With Parameters
  Query ID: [id]
  Parameters: [values]
  Substitution correct: [yes/no]
  RESULT: [PASS/FAIL]

Test 6: Execute Approved Query - Invalid ID
  ID: "nonexistent-id-xyz"
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 7: Execute Approved Query - Missing Parameters
  Error returned: [yes/no]
  Error specifies missing params: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
