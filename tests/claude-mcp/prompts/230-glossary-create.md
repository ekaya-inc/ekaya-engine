# Test: Glossary Term Creation

Test creating new business glossary terms.

## Tools Under Test

- `create_glossary_term`

## Test Data Convention

All test terms use the `_MCP_TEST` suffix: `Test Term_MCP_TEST`

## Test Cases

### 1. Create Basic Term
Call `create_glossary_term` with term, definition, and SQL and verify:
- Term is created
- Returns success confirmation
- Appears in `list_glossary`

```
term: "Active Users_MCP_TEST"
definition: "Users who have logged in within the last 30 days"
sql: "SELECT * FROM users WHERE last_login > NOW() - INTERVAL '30 days'"
```

### 2. Create Term - SQL Validation
Call `create_glossary_term` with invalid SQL and verify:
- SQL is validated before creation
- Returns validation error
- Term is not created with invalid SQL

### 3. Create Term - Duplicate Name
Call `create_glossary_term` with existing term name and verify:
- Returns duplicate error
- Or updates existing (document behavior)

### 4. Create Term - Missing Required Fields
Call `create_glossary_term` without SQL and verify:
- Returns validation error
- Specifies which field is missing

### 5. Create Term - Long Definition
Call `create_glossary_term` with very long definition and verify:
- Either accepted or truncated/rejected
- Document any length limits

## Report Format

```
=== 230-glossary-create: Glossary Term Creation ===

Test 1: Create Basic Term
  Term: Active Users_MCP_TEST
  Created: [yes/no]
  In list_glossary: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Create Term - SQL Validation
  Invalid SQL provided: SELECT * FORM bad_syntax
  Validation error: [yes/no]
  Term created: [no]
  RESULT: [PASS/FAIL]

Test 3: Create Term - Duplicate Name
  Behavior: [error / update]
  RESULT: [PASS/FAIL]

Test 4: Create Term - Missing Required Fields
  Error returned: [yes/no]
  Missing field identified: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Create Term - Long Definition
  Definition length: [chars]
  Behavior: [accepted / truncated / rejected]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Cleanup

Terms created here will be deleted in `330-glossary-delete.md`.
