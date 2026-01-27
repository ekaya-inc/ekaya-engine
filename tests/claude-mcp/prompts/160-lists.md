# Test: List Operations

Test various list/enumeration tools.

## Tools Under Test

- `list_pending_changes`
- `list_ontology_questions`

## Test Cases

### 1. List Pending Changes
Call `list_pending_changes` and verify:
- Returns array (may be empty)
- Each change has `id`, `type`, `description`
- Shows what would be changed

### 2. List Pending Changes - After Modification
After making an ontology change (in later tests), verify:
- New change appears in list
- Change details are accurate

### 3. List Ontology Questions - All
Call `list_ontology_questions` with no filters and verify:
- Returns array of questions
- Each question has `id`, `question`, `status`
- May include `context`, `suggested_answer`

### 4. List Ontology Questions - By Status
Call `list_ontology_questions` with status filter and verify:
- Returns only questions matching status
- Valid statuses: pending, resolved, skipped, dismissed, escalated

### 5. List Ontology Questions - Empty
If no questions exist, verify:
- Returns empty array
- Does not error

## Report Format

```
=== 160-lists: List Operations ===

Test 1: List Pending Changes
  Changes returned: [count]
  Has required fields: [yes/no]
  RESULT: [PASS/FAIL]

Test 2: List Pending Changes - After Modification
  (Run after 200-series tests)
  Change detected: [yes/no]
  RESULT: [PASS/FAIL/DEFERRED]

Test 3: List Ontology Questions - All
  Questions returned: [count]
  Has required fields: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: List Ontology Questions - By Status
  Status filter: [value]
  Results: [count]
  All match status: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: List Ontology Questions - Empty
  Behavior when empty: [empty array / message]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
