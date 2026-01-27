# Test: Column Metadata

Test adding and updating column semantic metadata.

## Tools Under Test

- `update_column`

## Prerequisites

Run `get_schema` to identify valid table and column names.

## Test Cases

### 1. Add Column Description
Call `update_column` to add description to a column and verify:
- Description is added
- Appears in `get_schema` output
- Appears in `probe_column` output

```
table: "[existing_table]"
column: "[existing_column]"
description: "MCP_TEST: Test column description"
```

### 2. Add Column Semantic Type
Call `update_column` to add semantic type annotation and verify:
- Semantic type is stored
- Used in ontology context

```
table: "[existing_table]"
column: "[existing_column]"
semantic_type: "email_address"
```

### 3. Update Existing Metadata
Call `update_column` on column with existing metadata and verify:
- Metadata is updated
- Previous values replaced

### 4. Add Metadata - Invalid Table
Call `update_column` with non-existent table and verify:
- Returns appropriate error
- Does not create phantom metadata

### 5. Add Metadata - Invalid Column
Call `update_column` with non-existent column and verify:
- Returns appropriate error
- Specifies which column is invalid

### 6. Clear Column Metadata
Call `update_column` with empty/null values and verify:
- Metadata is cleared (or document behavior)
- Column still exists in schema

## Report Format

```
=== 220-column-metadata: Column Metadata ===

Test 1: Add Column Description
  Table.Column: [name]
  Description added: [yes/no]
  Visible in schema: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Add Column Semantic Type
  Semantic type: email_address
  Stored correctly: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update Existing Metadata
  Updated: [yes/no]
  Previous value replaced: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Add Metadata - Invalid Table
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Add Metadata - Invalid Column
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 6: Clear Column Metadata
  Behavior: [cleared / not supported]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Cleanup

Column metadata added here will be deleted in `320-column-metadata-delete.md`.
