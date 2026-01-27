# Test: Approved Query Deletion

Test deleting approved queries.

## Tools Under Test

- `delete_approved_query`

## Prerequisites

Run `290-approved-queries-write.md` first.

## Test Cases

### 1. Delete Approved Query - Success
Call `delete_approved_query` with existing query ID and verify:
- Query is deleted
- No longer in `list_approved_queries`
- `execute_approved_query` fails for this ID

```
id: "[MCP_TEST query ID]"
```

### 2. Delete Approved Query - Non-Existent
Call `delete_approved_query` with ID that doesn't exist and verify:
- Returns appropriate error or success (idempotent)
- Document actual behavior

### 3. Delete All Test Queries
Delete all `MCP_TEST_*` approved queries:
- Verify all removed from list
- No orphaned suggestions

## Report Format

```
=== 350-approved-queries-delete: Approved Query Deletion ===

Test 1: Delete Approved Query - Success
  Query: MCP_TEST_Simple_Query
  Deleted: [yes/no]
  Not in list: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Delete Approved Query - Non-Existent
  Behavior: [error / idempotent success]
  RESULT: [PASS/FAIL]

Test 3: Delete All Test Queries
  Queries deleted: [list]
  All removed: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
