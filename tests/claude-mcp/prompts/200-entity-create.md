# Test: Entity Creation

Test creating new entities via the ontology.

## Tools Under Test

- `update_entity` (create mode)

## Test Data Convention

All test entities use the `_MCP_TEST` suffix: `TestEntity_MCP_TEST`

## Test Cases

### 1. Create Basic Entity
Call `update_entity` with new entity name and verify:
- Entity is created
- Returns success confirmation
- Entity appears in `get_ontology` results

```
name: "BasicEntity_MCP_TEST"
description: "Test entity for MCP test suite"
```

### 2. Create Entity with Aliases
Call `update_entity` with aliases and verify:
- Entity is created with aliases
- Aliases are searchable (test with `get_entity`)

```
name: "AliasEntity_MCP_TEST"
description: "Entity with aliases for testing"
aliases: ["alias1_test", "alias2_test"]
```

### 3. Create Entity - Duplicate Name
Call `update_entity` with existing entity name and verify:
- Behaves as upsert (updates existing)
- Does not create duplicate
- Document actual behavior

### 4. Create Entity - Empty Description
Call `update_entity` with empty/null description and verify:
- Either succeeds with empty description
- Or returns validation error
- Document actual behavior

### 5. Create Entity - Special Characters
Call `update_entity` with special characters in name and verify:
- Handles unicode, spaces, punctuation appropriately
- Document any restrictions

```
name: "Special Chars_MCP_TEST"
description: "Testing special characters: émojis 🎉 & symbols"
```

## Report Format

```
=== 200-entity-create: Entity Creation ===

Test 1: Create Basic Entity
  Name: BasicEntity_MCP_TEST
  Created: [yes/no]
  Appears in ontology: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Create Entity with Aliases
  Name: AliasEntity_MCP_TEST
  Aliases: [alias1_test, alias2_test]
  Aliases searchable: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Create Entity - Duplicate Name
  Behavior: [upsert / error / duplicate created]
  RESULT: [PASS/FAIL]

Test 4: Create Entity - Empty Description
  Behavior: [accepted / rejected]
  RESULT: [PASS/FAIL]

Test 5: Create Entity - Special Characters
  Name accepted: [yes/no]
  Unicode preserved: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Cleanup

Entities created here will be deleted in `300-entity-delete.md`.
