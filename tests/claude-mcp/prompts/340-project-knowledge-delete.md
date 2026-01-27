# Test: Project Knowledge Deletion

Test deleting domain facts.

## Tools Under Test

- `delete_project_knowledge`

## Prerequisites

Run `240-project-knowledge.md` first.

## Test Cases

### 1. Delete Project Knowledge - Success
Call `delete_project_knowledge` with existing fact and verify:
- Fact is deleted
- No longer affects ontology context
- Returns success confirmation

```
key: "mcp_test_business_rule"
```

### 2. Delete Project Knowledge - Non-Existent
Call `delete_project_knowledge` with key that doesn't exist and verify:
- Returns appropriate error or success (idempotent)
- Document actual behavior

### 3. Delete All Test Knowledge
Delete all `mcp_test_*` facts:
- Verify all removed
- No orphaned data

## Report Format

```
=== 340-project-knowledge-delete: Project Knowledge Deletion ===

Test 1: Delete Project Knowledge - Success
  Key: mcp_test_business_rule
  Deleted: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Delete Project Knowledge - Non-Existent
  Behavior: [error / idempotent success]
  RESULT: [PASS/FAIL]

Test 3: Delete All Test Knowledge
  Facts deleted: [count]
  All removed: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
