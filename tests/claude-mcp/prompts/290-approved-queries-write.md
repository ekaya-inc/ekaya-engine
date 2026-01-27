# Test: Approved Queries Write Operations

Test creating, updating, and suggesting approved queries.

## Tools Under Test

- `create_approved_query`
- `update_approved_query`
- `suggest_approved_query`
- `suggest_query_update`
- `list_query_suggestions`
- `approve_query_suggestion`
- `reject_query_suggestion`

## Note

Some tools require the AI Data Liaison application to be installed.
Tests will be skipped if tools are not available.

## Test Data Convention

All test queries use identifiable names: `MCP_TEST_*`

## Test Cases

### 1. Create Approved Query (Admin)
Call `create_approved_query` and verify:
- Query is created
- Appears in `list_approved_queries`
- Executable via `execute_approved_query`

```
name: "MCP_TEST_Simple_Query"
description: "Test query for MCP test suite"
sql: "SELECT COUNT(*) as count FROM users"
```

### 2. Create Parameterized Query
Call `create_approved_query` with parameters and verify:
- Parameters are defined
- Validation rules stored
- Executable with parameters

```
name: "MCP_TEST_Param_Query"
description: "Parameterized test query"
sql: "SELECT * FROM users WHERE id = $1"
parameters: [{"name": "user_id", "type": "integer"}]
```

### 3. Update Approved Query
Call `update_approved_query` on existing query and verify:
- Query is updated
- Changes reflected
- Execution works with new SQL

### 4. Suggest Approved Query
Call `suggest_approved_query` and verify:
- Suggestion is created
- Appears in `list_query_suggestions`
- Status is "pending"

### 5. Suggest Query Update
Call `suggest_query_update` for existing query and verify:
- Suggestion references original
- Includes proposed changes
- Status is "pending"

### 6. Approve Query Suggestion
Call `approve_query_suggestion` and verify:
- Suggestion is approved
- Query is created/updated
- Suggestion removed from pending

### 7. Reject Query Suggestion
Call `reject_query_suggestion` and verify:
- Suggestion is rejected
- Reason is recorded
- Original query unchanged

## Report Format

```
=== 290-approved-queries-write: Approved Queries Write Operations ===

Test 1: Create Approved Query (Admin)
  Name: MCP_TEST_Simple_Query
  Created: [yes/no]
  Executable: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Create Parameterized Query
  Parameters defined: [yes/no]
  Executable with params: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update Approved Query
  Updated: [yes/no]
  Changes reflected: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Suggest Approved Query
  Suggestion created: [yes/no]
  Status: pending
  RESULT: [PASS/FAIL/SKIP if not available]

Test 5: Suggest Query Update
  Suggestion created: [yes/no]
  References original: [yes/no]
  RESULT: [PASS/FAIL/SKIP]

Test 6: Approve Query Suggestion
  Approved: [yes/no]
  Query created: [yes/no]
  RESULT: [PASS/FAIL/SKIP]

Test 7: Reject Query Suggestion
  Rejected: [yes/no]
  Reason recorded: [yes/no]
  RESULT: [PASS/FAIL/SKIP]

OVERALL: [PASS/FAIL]
```

## Cleanup

Queries created here will be deleted in `350-approved-queries-delete.md`.
