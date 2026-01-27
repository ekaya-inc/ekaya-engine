# Test: Relationship Updates

Test modifying existing relationships.

## Tools Under Test

- `update_relationship` (update mode)

## Prerequisites

Run `210-relationship-create.md` first to create test relationships.

## Test Cases

### 1. Update Description
Call `update_relationship` on existing relationship with new description and verify:
- Description is updated
- Relationship direction unchanged
- Change reflected in ontology

### 2. Update Cardinality
Call `update_relationship` to change cardinality and verify:
- Cardinality is updated
- Other fields unchanged

### 3. Update Non-Existent Relationship
Call `update_relationship` for relationship that doesn't exist and verify:
- Creates new relationship (upsert behavior)
- Or returns error (strict update behavior)
- Document actual behavior

## Report Format

```
=== 211-relationship-update: Relationship Updates ===

Test 1: Update Description
  Relationship: BasicEntity_MCP_TEST -> AliasEntity_MCP_TEST
  Description updated: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Update Cardinality
  New cardinality: [value]
  Updated correctly: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update Non-Existent Relationship
  Behavior: [created / error]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
