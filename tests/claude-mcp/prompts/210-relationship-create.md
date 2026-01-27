# Test: Relationship Creation

Test creating relationships between entities.

## Tools Under Test

- `update_relationship` (create mode)

## Prerequisites

Run `200-entity-create.md` first to create test entities.

## Test Data Convention

Relationships should only be between `*_MCP_TEST` entities.

## Test Cases

### 1. Create Basic Relationship
Call `update_relationship` between two test entities and verify:
- Relationship is created
- Returns success confirmation
- Appears in ontology with correct direction

```
from_entity: "BasicEntity_MCP_TEST"
to_entity: "AliasEntity_MCP_TEST"
description: "Test relationship for MCP suite"
```

### 2. Create Relationship with Cardinality
Call `update_relationship` with cardinality info and verify:
- Cardinality is stored
- Reflected in `probe_relationship`

```
from_entity: "BasicEntity_MCP_TEST"
to_entity: "AliasEntity_MCP_TEST"
cardinality: "one-to-many"
```

### 3. Create Duplicate Relationship
Call `update_relationship` for existing relationship and verify:
- Behaves as upsert (updates existing)
- Does not create duplicate
- Document actual behavior

### 4. Create Relationship - Invalid Entity
Call `update_relationship` with non-existent entity and verify:
- Returns appropriate error
- Specifies which entity is invalid

### 5. Create Self-Referencing Relationship
Call `update_relationship` where from_entity equals to_entity and verify:
- Either allowed (some domains have self-references)
- Or rejected with appropriate error
- Document actual behavior

## Report Format

```
=== 210-relationship-create: Relationship Creation ===

Test 1: Create Basic Relationship
  From: BasicEntity_MCP_TEST
  To: AliasEntity_MCP_TEST
  Created: [yes/no]
  Direction correct: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Create Relationship with Cardinality
  Cardinality: one-to-many
  Stored correctly: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Create Duplicate Relationship
  Behavior: [upsert / error / duplicate]
  RESULT: [PASS/FAIL]

Test 4: Create Relationship - Invalid Entity
  Error returned: [yes/no]
  Invalid entity specified: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Create Self-Referencing Relationship
  Behavior: [allowed / rejected]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Cleanup

Relationships created here will be deleted in `310-relationship-delete.md`.
