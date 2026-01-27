# Test: Glossary Term Updates

Test modifying existing glossary terms.

## Tools Under Test

- `update_glossary_term`

## Prerequisites

Run `230-glossary-create.md` first to create test terms.

## Test Cases

### 1. Update Definition
Call `update_glossary_term` with new definition and verify:
- Definition is updated
- SQL unchanged
- Change reflected in `list_glossary`

### 2. Update SQL
Call `update_glossary_term` with new SQL and verify:
- SQL is updated
- New SQL is validated
- Definition unchanged

### 3. Update SQL - Invalid
Call `update_glossary_term` with invalid SQL and verify:
- Validation error returned
- Original SQL preserved

### 4. Update Non-Existent Term
Call `update_glossary_term` for term that doesn't exist and verify:
- Returns appropriate error
- Does not create new term

### 5. Update Term Name
Call `update_glossary_term` to change the term name (if supported) and verify:
- Name is updated
- Or document if name changes aren't supported

## Report Format

```
=== 231-glossary-update: Glossary Term Updates ===

Test 1: Update Definition
  Term: Active Users_MCP_TEST
  Definition updated: [yes/no]
  SQL unchanged: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Update SQL
  New SQL valid: [yes/no]
  SQL updated: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update SQL - Invalid
  Validation error: [yes/no]
  Original preserved: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Update Non-Existent Term
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Update Term Name
  Behavior: [supported / not supported]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
