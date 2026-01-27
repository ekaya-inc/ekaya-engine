# Test: Entity Deletion

Test deleting entities from the ontology.

## Tools Under Test

- `delete_entity`

## Prerequisites

Run `200-entity-create.md` and `201-entity-update.md` first.
Relationships must be deleted before entities (see `310-relationship-delete.md`).

## Test Cases

### 1. Delete Entity with Relationships
Call `delete_entity` on entity that has relationships and verify:
- Returns error indicating relationships exist
- Entity is not deleted
- Must delete relationships first

### 2. Delete Entity - Success
After deleting relationships, call `delete_entity` and verify:
- Entity is deleted
- No longer in `get_ontology` results
- No longer retrievable via `get_entity`

```
name: "BasicEntity_MCP_TEST"
```

### 3. Delete Entity - Non-Existent
Call `delete_entity` with entity that doesn't exist and verify:
- Returns appropriate error or success (idempotent)
- Document actual behavior

### 4. Delete All Test Entities
Delete all `*_MCP_TEST` entities created during testing:
- AliasEntity_MCP_TEST
- Special Chars_MCP_TEST (if created)
- Any others

## Report Format

```
=== 300-entity-delete: Entity Deletion ===

Test 1: Delete Entity with Relationships
  Entity: BasicEntity_MCP_TEST
  Blocked by relationships: [yes/no]
  Error message helpful: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Delete Entity - Success
  Entity: BasicEntity_MCP_TEST
  Deleted: [yes/no]
  Not in ontology: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Delete Entity - Non-Existent
  Behavior: [error / idempotent success]
  RESULT: [PASS/FAIL]

Test 4: Delete All Test Entities
  Entities deleted: [list]
  All removed: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
