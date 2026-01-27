# Test: Glossary Term Deletion

Test deleting business glossary terms.

## Tools Under Test

- `delete_glossary_term`

## Prerequisites

Run `230-glossary-create.md` first.

## Test Cases

### 1. Delete Glossary Term - Success
Call `delete_glossary_term` with existing term and verify:
- Term is deleted
- No longer in `list_glossary`
- `get_glossary_sql` returns not found

```
term: "Active Users_MCP_TEST"
```

### 2. Delete Glossary Term - Non-Existent
Call `delete_glossary_term` with term that doesn't exist and verify:
- Returns appropriate error or success (idempotent)
- Document actual behavior

### 3. Delete All Test Terms
Delete all `*_MCP_TEST` glossary terms:
- Verify all removed from list
- No orphaned data

## Report Format

```
=== 330-glossary-delete: Glossary Term Deletion ===

Test 1: Delete Glossary Term - Success
  Term: Active Users_MCP_TEST
  Deleted: [yes/no]
  Not in list: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Delete Glossary Term - Non-Existent
  Behavior: [error / idempotent success]
  RESULT: [PASS/FAIL]

Test 3: Delete All Test Terms
  Terms deleted: [list]
  All removed: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
