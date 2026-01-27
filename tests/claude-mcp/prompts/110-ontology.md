# Test: Ontology Tools

Test ontology retrieval at various depth levels.

## Tools Under Test

- `get_ontology`
- `get_context`

## Test Cases

### 1. Get Ontology - Domain Level
Call `get_ontology` with `depth: "domain"` and verify:
- Returns domain summary
- Contains high-level business description

### 2. Get Ontology - Entities Level
Call `get_ontology` with `depth: "entities"` and verify:
- Returns entity list with summaries
- Each entity has name and description

### 3. Get Ontology - Full Level
Call `get_ontology` with `depth: "full"` and verify:
- Returns complete ontology
- Includes entities, relationships, column details

### 4. Get Context - Domain
Call `get_context` with `depth: "domain"` and verify:
- Returns unified context object
- Contains domain-level information

### 5. Get Context - Entities
Call `get_context` with `depth: "entities"` and verify:
- Returns entity-level context
- More detail than domain level

### 6. Get Context - Tables
Call `get_context` with `depth: "tables"` and verify:
- Returns table-level context
- Includes schema information

### 7. Get Context - Columns
Call `get_context` with `depth: "columns"` and verify:
- Returns most detailed context
- Includes column-level annotations

## Report Format

```
=== 110-ontology: Ontology Tools ===

Test 1: Get Ontology - Domain Level
  Has domain summary: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Get Ontology - Entities Level
  Entity count: [count]
  Has descriptions: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Get Ontology - Full Level
  Has entities: [yes/no]
  Has relationships: [yes/no]
  Has column details: [yes/no]
  RESULT: [PASS/FAIL]

Test 4-7: Get Context at various depths
  Domain: [PASS/FAIL]
  Entities: [PASS/FAIL]
  Tables: [PASS/FAIL]
  Columns: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
