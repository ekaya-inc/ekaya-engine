# Test: Entity Lookup

Test retrieving specific entity details.

## Tools Under Test

- `get_entity`

## Prerequisites

Run `get_ontology` with `depth: "entities"` first to get a list of valid entity names.

## Test Cases

### 1. Get Entity - Valid Name
Call `get_entity` with a known entity name and verify:
- Returns entity details
- Includes `name`, `description`
- Includes `aliases` if any
- Includes `occurrences` (where entity appears in schema)
- Includes `key_columns` if any
- Includes `relationships` if any

### 2. Get Entity - Case Insensitive
Call `get_entity` with entity name in different case and verify:
- Still returns the entity (or returns appropriate error)
- Document actual behavior

### 3. Get Entity - Invalid Name
Call `get_entity` with non-existent entity name and verify:
- Returns appropriate error message
- Does not crash

### 4. Get Entity - By Alias
If entity has aliases, call `get_entity` with an alias and verify:
- Returns the entity (or document if aliases aren't searchable)

## Report Format

```
=== 111-ontology-entities: Entity Lookup ===

Test 1: Get Entity - Valid Name
  Entity: [name]
  Has description: [yes/no]
  Has occurrences: [yes/no]
  Occurrence count: [count]
  RESULT: [PASS/FAIL]

Test 2: Get Entity - Case Insensitive
  Query: [value]
  Behavior: [found/not found/error]
  RESULT: [PASS/FAIL]

Test 3: Get Entity - Invalid Name
  Query: "NonExistentEntity_XYZ"
  Error returned: [yes/no]
  Error message: [value]
  RESULT: [PASS/FAIL]

Test 4: Get Entity - By Alias
  Alias used: [value]
  Found entity: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
