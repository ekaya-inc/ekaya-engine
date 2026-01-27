# Test: Final Cleanup

Ensure all test data is removed after test suite completion.

## Purpose

This test verifies that the test suite properly cleans up after itself,
leaving no `_MCP_TEST` artifacts in the system.

## Cleanup Verification

### 1. Verify No Test Entities
Call `get_ontology` with `depth: "entities"` and verify:
- No entities with `_MCP_TEST` suffix exist
- If found, delete them

### 2. Verify No Test Relationships
Check ontology for relationships between `*_MCP_TEST` entities:
- Should be none
- If found, delete them

### 3. Verify No Test Glossary Terms
Call `list_glossary` and verify:
- No terms with `_MCP_TEST` suffix exist
- If found, delete them

### 4. Verify No Test Project Knowledge
Check for `mcp_test_*` facts:
- Should be none
- If found, delete them

### 5. Verify No Test Approved Queries
Call `list_approved_queries` and verify:
- No queries with `MCP_TEST_` prefix exist
- If found, delete them

### 6. Verify No Test Tables
Check schema for `mcp_test_*` tables:
- Should be none
- If found, drop them via `execute`

### 7. Verify No Test Column Metadata
Check for column metadata with `MCP_TEST` in description:
- Should be none
- If found, delete it

## Report Format

```
=== 900-cleanup: Final Cleanup ===

Test 1: Verify No Test Entities
  Found: [count]
  Cleaned: [yes/no/N/A]
  RESULT: [PASS/FAIL]

Test 2: Verify No Test Relationships
  Found: [count]
  Cleaned: [yes/no/N/A]
  RESULT: [PASS/FAIL]

Test 3: Verify No Test Glossary Terms
  Found: [count]
  Cleaned: [yes/no/N/A]
  RESULT: [PASS/FAIL]

Test 4: Verify No Test Project Knowledge
  Found: [count]
  Cleaned: [yes/no/N/A]
  RESULT: [PASS/FAIL]

Test 5: Verify No Test Approved Queries
  Found: [count]
  Cleaned: [yes/no/N/A]
  RESULT: [PASS/FAIL]

Test 6: Verify No Test Tables
  Found: [count]
  Cleaned: [yes/no/N/A]
  RESULT: [PASS/FAIL]

Test 7: Verify No Test Column Metadata
  Found: [count]
  Cleaned: [yes/no/N/A]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Final State

After this test completes:
- System should be in same state as before test suite ran
- No test artifacts remain
- Ready for next test run
