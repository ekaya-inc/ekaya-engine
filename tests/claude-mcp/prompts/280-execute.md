# Test: Execute Edge Cases

Test edge cases and error handling for DDL/DML execution.

## Tools Under Test

- `execute`

## Prerequisites

Run `020-test-fixtures.md` first - basic execute functionality is tested there.

## Test Cases

### 1. Execute Invalid SQL - Syntax Error
Call `execute` with malformed SQL and verify:
- Returns syntax error
- Error message indicates position/nature of error
- No partial execution

```sql
SELEKT * FORM mcp_test_users
```

### 2. Execute Invalid SQL - Missing Table
Call `execute` referencing non-existent table and verify:
- Returns table not found error
- Error specifies which table

```sql
INSERT INTO nonexistent_table_xyz (col) VALUES (1)
```

### 3. Execute Invalid SQL - Constraint Violation
Call `execute` violating a constraint and verify:
- Returns constraint violation error
- Specifies which constraint (unique, FK, etc.)

```sql
INSERT INTO mcp_test_users (name, email) VALUES ('Duplicate', 'alice@mcp-test.example')
```

### 4. Execute with Transaction - ROLLBACK
Test rollback behavior:
- Begin transaction
- Make change
- Rollback
- Verify change not persisted

### 5. Execute - Empty Statement
Call `execute` with empty or whitespace-only SQL and verify:
- Returns appropriate error
- Does not crash

### 6. Execute - Multiple Statements
Call `execute` with multiple statements and verify:
- Either executes all or rejects
- Document actual behavior (some systems allow, some don't)

```sql
INSERT INTO mcp_test_users (name) VALUES ('Multi1');
INSERT INTO mcp_test_users (name) VALUES ('Multi2');
```

### 7. Execute - SQL Injection Attempt
Call `execute` with potentially dangerous input and verify:
- Proper escaping/rejection
- No unintended side effects

## Report Format

```
=== 280-execute: Execute Edge Cases ===

Test 1: Execute Invalid SQL - Syntax Error
  Error returned: [yes/no]
  Error helpful: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Execute Invalid SQL - Missing Table
  Error returned: [yes/no]
  Table specified: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Execute Invalid SQL - Constraint Violation
  Error returned: [yes/no]
  Constraint identified: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Execute with Transaction - ROLLBACK
  Transaction support: [yes/no]
  Rollback works: [yes/no]
  RESULT: [PASS/FAIL/SKIP]

Test 5: Execute - Empty Statement
  Behavior: [error / no-op]
  RESULT: [PASS/FAIL]

Test 6: Execute - Multiple Statements
  Behavior: [all executed / rejected / first only]
  RESULT: [PASS/FAIL]

Test 7: Execute - SQL Injection Attempt
  Properly handled: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
