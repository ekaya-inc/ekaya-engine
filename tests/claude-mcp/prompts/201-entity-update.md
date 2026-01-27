# Test: Entity Updates

Test modifying existing entities.

## Tools Under Test

- `update_entity` (update mode)

## Prerequisites

Run `200-entity-create.md` first to create test entities.

## Test Cases

### 1. Update Description
Call `update_entity` on existing entity with new description and verify:
- Description is updated
- Other fields unchanged
- Change reflected in `get_entity`

```
name: "BasicEntity_MCP_TEST"
description: "Updated description for testing"
```

### 2. Add Aliases to Existing Entity
Call `update_entity` to add aliases to entity without aliases and verify:
- Aliases are added
- Existing data preserved

### 3. Update Aliases
Call `update_entity` to modify aliases on entity with aliases and verify:
- New aliases replace old (or merge - document behavior)
- Entity still retrievable by name

### 4. Remove Aliases
Call `update_entity` with empty aliases array and verify:
- Aliases are removed (or document if not possible)

### 5. Update Non-Existent Entity
Call `update_entity` on non-existent entity and verify:
- Creates new entity (upsert behavior)
- Or returns error (strict update behavior)
- Document actual behavior

## Report Format

```
=== 201-entity-update: Entity Updates ===

Test 1: Update Description
  Entity: BasicEntity_MCP_TEST
  Description updated: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Add Aliases to Existing Entity
  Entity: BasicEntity_MCP_TEST
  Aliases added: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update Aliases
  Entity: AliasEntity_MCP_TEST
  Aliases updated: [yes/no]
  Behavior: [replace / merge]
  RESULT: [PASS/FAIL]

Test 4: Remove Aliases
  Behavior: [removed / not supported / error]
  RESULT: [PASS/FAIL]

Test 5: Update Non-Existent Entity
  Behavior: [created / error]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
