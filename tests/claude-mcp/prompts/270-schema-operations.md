# Test: Schema Operations

Test schema refresh and data change scanning.

## Tools Under Test

- `refresh_schema`
- `scan_data_changes`

## Warning

These operations may affect the ontology state. Run with caution.

## Test Cases

### 1. Refresh Schema
Call `refresh_schema` and verify:
- Schema is re-read from datasource
- New tables/columns detected (if any)
- Returns summary of changes

### 2. Refresh Schema - No Changes
Call `refresh_schema` when schema hasn't changed and verify:
- Reports no changes
- Ontology unchanged
- Idempotent operation

### 3. Scan Data Changes
Call `scan_data_changes` and verify:
- Scans for new enum values
- Scans for new FK patterns
- Returns summary of findings

### 4. Scan Data Changes - Specific Table
Call `scan_data_changes` with table filter (if supported) and verify:
- Only specified table scanned
- More efficient than full scan

### 5. Scan After Data Modification
After adding test data, call `scan_data_changes` and verify:
- New patterns detected
- Suggestions generated (if applicable)

## Report Format

```
=== 270-schema-operations: Schema Operations ===

Test 1: Refresh Schema
  Completed: [yes/no]
  Changes detected: [count/description]
  RESULT: [PASS/FAIL]

Test 2: Refresh Schema - No Changes
  Reports no changes: [yes/no]
  Idempotent: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Scan Data Changes
  Completed: [yes/no]
  Findings: [summary]
  RESULT: [PASS/FAIL]

Test 4: Scan Data Changes - Specific Table
  Table filter supported: [yes/no]
  RESULT: [PASS/FAIL/SKIP]

Test 5: Scan After Data Modification
  New patterns detected: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
