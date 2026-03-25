# Test: Project Knowledge Deletion

Test deleting domain facts.

## Tools Under Test

- `list_project_knowledge`
- `delete_project_knowledge`

## Prerequisites

Run `240-project-knowledge.md` first.

## Test Cases

### 1. Delete Project Knowledge - Success
Call `list_project_knowledge`, capture an existing `fact_id`, then call `delete_project_knowledge` and verify:
- Fact is deleted
- Returns success confirmation
- A follow-up `list_project_knowledge` call no longer shows the deleted `fact_id`

```
fact_id: "<captured from list_project_knowledge>"
```

### 2. Delete Project Knowledge - Non-Existent
Call `delete_project_knowledge` with a non-existent `fact_id` and verify:
- Returns an error indicating the fact was not found

### 3. Delete All Test Knowledge
Use `list_project_knowledge` to discover all `mcp_test_*` facts, then delete them by `fact_id`:
- Verify all removed
- No orphaned data

## Report Format

```
=== 340-project-knowledge-delete: Project Knowledge Deletion ===

Test 1: Delete Project Knowledge - Success
  fact_id: [captured uuid]
  Deleted: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Delete Project Knowledge - Non-Existent
  Behavior: [error]
  RESULT: [PASS/FAIL]

Test 3: Delete All Test Knowledge
  Facts deleted: [count]
  All removed: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
