# Test: Relationship Deletion

Test deleting relationships between entities.

## Tools Under Test

- `delete_relationship`

## Prerequisites

Run `210-relationship-create.md` first.
Run this BEFORE `300-entity-delete.md` (entities with relationships cannot be deleted).

## Test Cases

### 1. Delete Relationship - Success
Call `delete_relationship` with valid from/to entities and verify:
- Relationship is deleted
- No longer appears in ontology
- Entities still exist

```
from_entity: "BasicEntity_MCP_TEST"
to_entity: "AliasEntity_MCP_TEST"
```

### 2. Delete Relationship - Non-Existent
Call `delete_relationship` for relationship that doesn't exist and verify:
- Returns appropriate error or success (idempotent)
- Document actual behavior

### 3. Delete Relationship - Invalid Entity
Call `delete_relationship` with non-existent entity and verify:
- Returns appropriate error
- Specifies which entity is invalid

### 4. Delete All Test Relationships
Delete all relationships between `*_MCP_TEST` entities:
- Verify all removed
- Prepares for entity deletion

## Report Format

```
=== 310-relationship-delete: Relationship Deletion ===

Test 1: Delete Relationship - Success
  From: BasicEntity_MCP_TEST
  To: AliasEntity_MCP_TEST
  Deleted: [yes/no]
  Not in ontology: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Delete Relationship - Non-Existent
  Behavior: [error / idempotent success]
  RESULT: [PASS/FAIL]

Test 3: Delete Relationship - Invalid Entity
  Error returned: [yes/no]
  Invalid entity specified: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Delete All Test Relationships
  Relationships deleted: [count]
  All removed: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
