# Test: Project Knowledge

Test creating and updating domain facts.

## Tools Under Test

- `update_project_knowledge`

## Test Data Convention

All test facts use identifiable keys: `mcp_test_*`

## Test Cases

### 1. Create Domain Fact
Call `update_project_knowledge` with new fact and verify:
- Fact is created
- Returns success confirmation
- Fact is used in ontology context

```
fact_type: "domain_rule"
key: "mcp_test_business_rule"
value: "Test business rule: All test entities use _MCP_TEST suffix"
```

### 2. Create Multiple Facts
Call `update_project_knowledge` for several facts and verify:
- All facts are stored
- Each has unique key
- Facts are retrievable

### 3. Update Existing Fact
Call `update_project_knowledge` with existing key and verify:
- Value is updated
- Key unchanged
- Upsert behavior works

### 4. Create Fact - Empty Value
Call `update_project_knowledge` with empty value and verify:
- Either accepted (clears fact)
- Or rejected with validation error

### 5. Create Fact - Special Characters
Call `update_project_knowledge` with unicode/special chars and verify:
- Characters preserved correctly
- No encoding issues

## Report Format

```
=== 240-project-knowledge: Project Knowledge ===

Test 1: Create Domain Fact
  Key: mcp_test_business_rule
  Created: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Create Multiple Facts
  Facts created: [count]
  All retrievable: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update Existing Fact
  Updated: [yes/no]
  Previous value replaced: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Create Fact - Empty Value
  Behavior: [accepted / rejected]
  RESULT: [PASS/FAIL]

Test 5: Create Fact - Special Characters
  Characters preserved: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Cleanup

Facts created here will be deleted in `340-project-knowledge-delete.md`.
