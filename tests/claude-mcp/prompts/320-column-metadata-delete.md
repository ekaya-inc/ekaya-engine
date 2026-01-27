# Test: Column Metadata Deletion

Test removing column semantic metadata.

## Tools Under Test

- `delete_column_metadata`

## Prerequisites

Run `220-column-metadata.md` first to add test metadata.

## Test Cases

### 1. Delete Column Metadata - Success
Call `delete_column_metadata` for column with metadata and verify:
- Metadata is removed
- Column still exists in schema
- No semantic annotations remain

```
table: "[table_name]"
column: "[column_name]"
```

### 2. Delete Column Metadata - No Metadata
Call `delete_column_metadata` for column without metadata and verify:
- Returns success (idempotent) or appropriate error
- Document actual behavior

### 3. Delete Column Metadata - Invalid Table
Call `delete_column_metadata` with non-existent table and verify:
- Returns appropriate error
- Does not affect other metadata

### 4. Delete Column Metadata - Invalid Column
Call `delete_column_metadata` with non-existent column and verify:
- Returns appropriate error
- Specifies which column is invalid

## Report Format

```
=== 320-column-metadata-delete: Column Metadata Deletion ===

Test 1: Delete Column Metadata - Success
  Table.Column: [name]
  Deleted: [yes/no]
  Column still in schema: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Delete Column Metadata - No Metadata
  Behavior: [idempotent / error]
  RESULT: [PASS/FAIL]

Test 3: Delete Column Metadata - Invalid Table
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Delete Column Metadata - Invalid Column
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
