# Test: Relationship Probing

Test relationship metrics and data quality checks.

## Tools Under Test

- `probe_relationship`

## Prerequisites

Run `get_ontology` with `depth: "full"` to identify existing relationships.

## Test Cases

### 1. Probe Valid Relationship
Call `probe_relationship` with valid from_entity and to_entity and verify:
- Returns relationship metrics
- Includes cardinality information
- Includes data quality metrics (orphans, etc.)

### 2. Probe Relationship - Referential Integrity
Call `probe_relationship` and examine:
- Count of records with valid references
- Count of orphaned records (if any)
- Percentage of referential integrity

### 3. Probe Relationship - Invalid Entities
Call `probe_relationship` with non-existent entity names and verify:
- Returns appropriate error
- Indicates which entity is invalid

### 4. Probe Relationship - No Relationship Exists
Call `probe_relationship` with valid entities that have no relationship and verify:
- Returns appropriate message
- Does not crash

## Report Format

```
=== 121-probe-relationships: Relationship Probing ===

Test 1: Probe Valid Relationship
  From: [entity]
  To: [entity]
  Has metrics: [yes/no]
  Cardinality: [value]
  RESULT: [PASS/FAIL]

Test 2: Referential Integrity
  Valid references: [count/percent]
  Orphaned records: [count]
  RESULT: [PASS/FAIL]

Test 3: Invalid Entities
  Error returned: [yes/no]
  Error indicates which entity: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: No Relationship Exists
  Behavior: [describe]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
