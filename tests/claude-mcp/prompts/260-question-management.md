# Test: Ontology Question Management

Test managing ontology questions generated during analysis.

## Tools Under Test

- `resolve_ontology_question`
- `skip_ontology_question`
- `dismiss_ontology_question`
- `escalate_ontology_question`

## Prerequisites

Use `list_ontology_questions` to get question IDs.
Questions are typically generated during ontology extraction.

## Test Cases

### 1. List Questions Before
Call `list_ontology_questions` to establish baseline:
- Note current questions
- Get question IDs for testing

### 2. Resolve Question
Call `resolve_ontology_question` with answer and verify:
- Question marked as resolved
- Answer is stored
- No longer appears in pending list

```
question_id: "[id]"
answer: "Test answer for MCP test suite"
```

### 3. Skip Question
Call `skip_ontology_question` and verify:
- Question marked as skipped
- Can be revisited later
- Status reflects "skipped"

### 4. Dismiss Question
Call `dismiss_ontology_question` and verify:
- Question marked as dismissed
- Will not be asked again
- May require reason

### 5. Escalate Question
Call `escalate_ontology_question` and verify:
- Question marked for human review
- Status reflects "escalated"
- May trigger notification (document behavior)

### 6. Action on Invalid Question ID
Call any action with non-existent ID and verify:
- Returns appropriate error
- Does not affect other questions

### 7. Action on Already Resolved
Call `skip_ontology_question` on resolved question and verify:
- Returns appropriate error/message
- Does not change resolved status

## Report Format

```
=== 260-question-management: Ontology Question Management ===

Test 1: List Questions Before
  Questions: [count]
  Statuses: [breakdown]
  RESULT: [PASS/FAIL]

Test 2: Resolve Question
  Question ID: [id]
  Resolved: [yes/no]
  Answer stored: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Skip Question
  Question ID: [id]
  Skipped: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Dismiss Question
  Question ID: [id]
  Dismissed: [yes/no]
  Reason required: [yes/no]
  RESULT: [PASS/FAIL]

Test 5: Escalate Question
  Question ID: [id]
  Escalated: [yes/no]
  RESULT: [PASS/FAIL]

Test 6: Action on Invalid Question ID
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 7: Action on Already Resolved
  Behavior: [error / no-op]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
