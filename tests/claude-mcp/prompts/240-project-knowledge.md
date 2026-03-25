# Test: Project Knowledge

Test creating and updating domain facts.

## Tools Under Test

- `list_project_knowledge`
- `update_project_knowledge`

## Test Data Convention

All test facts use identifiable prefixes in the `fact` text: `mcp_test_*`

## Test Cases

### 1. Create Domain Fact
Call `update_project_knowledge` with new fact and verify:
- Fact is created
- Returns `fact_id`
- `list_project_knowledge` shows the new fact with the same `fact_id`

```
fact: "mcp_test_business_rule: All test entities use _MCP_TEST suffix"
category: "business_rule"
context: "Created by MCP prompt fixture"
```

### 2. Create Multiple Facts
Call `update_project_knowledge` for several facts and verify:
- All facts are stored
- Each returns a unique `fact_id`
- Facts are retrievable with `list_project_knowledge`

### 3. Update Existing Fact
Call `list_project_knowledge`, capture a returned `fact_id`, then call `update_project_knowledge` with that `fact_id` and verify:
- Fact text is updated
- `fact_id` is unchanged
- Updated context is visible in a follow-up `list_project_knowledge` call

### 4. Create Fact - Empty Value
Call `update_project_knowledge` with empty `fact` and verify:
- Rejected with validation error

### 5. Create Fact - Special Characters
Call `update_project_knowledge` with unicode/special chars and verify:
- Characters preserved correctly
- No encoding issues

## Report Format

```
=== 240-project-knowledge: Project Knowledge ===

Test 1: Create Domain Fact
  Fact: mcp_test_business_rule...
  fact_id returned: [yes/no]
  Created: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: Create Multiple Facts
  Facts created: [count]
  All retrievable: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Update Existing Fact
  fact_id reused: [yes/no]
  Updated: [yes/no]
  Update visible in list_project_knowledge: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Create Fact - Empty Value
  Behavior: [rejected]
  RESULT: [PASS/FAIL]

Test 5: Create Fact - Special Characters
  Characters preserved: [yes/no]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```

## Cleanup

Facts created here will be deleted in `340-project-knowledge-delete.md`.
