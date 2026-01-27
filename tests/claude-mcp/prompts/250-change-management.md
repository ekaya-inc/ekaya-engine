# Test: Change Management

Test approving and rejecting pending ontology changes.

## Tools Under Test

- `approve_change`
- `approve_all_changes`
- `reject_change`

## Prerequisites

Make ontology changes first (via 200-series tests) to create pending changes.
Use `list_pending_changes` to get change IDs.

## Test Cases

### 1. List Changes Before
Call `list_pending_changes` to establish baseline:
- Note current pending changes
- Get change IDs for testing

### 2. Approve Single Change
Call `approve_change` with valid change ID and verify:
- Change is approved
- Removed from pending list
- Applied to ontology

### 3. Reject Single Change
Call `reject_change` with valid change ID and verify:
- Change is rejected
- Removed from pending list
- Not applied to ontology
- May require reason parameter

### 4. Approve All Changes
Call `approve_all_changes` and verify:
- All pending changes approved
- Pending list is empty
- Changes applied to ontology

### 5. Approve/Reject - Invalid ID
Call `approve_change` with non-existent ID and verify:
- Returns appropriate error
- Does not affect other changes

### 6. Approve Already Approved
Call `approve_change` on already-approved change and verify:
- Returns appropriate message
- Idempotent behavior (or error)

## Report Format

```
=== 250-change-management: Change Management ===

Test 1: List Changes Before
  Pending changes: [count]
  Change IDs: [list]
  RESULT: [PASS/FAIL]

Test 2: Approve Single Change
  Change ID: [id]
  Approved: [yes/no]
  Applied: [yes/no]
  RESULT: [PASS/FAIL]

Test 3: Reject Single Change
  Change ID: [id]
  Rejected: [yes/no]
  Reason required: [yes/no]
  RESULT: [PASS/FAIL]

Test 4: Approve All Changes
  Changes approved: [count]
  Pending now: [count]
  RESULT: [PASS/FAIL]

Test 5: Approve/Reject - Invalid ID
  Error returned: [yes/no]
  RESULT: [PASS/FAIL]

Test 6: Approve Already Approved
  Behavior: [idempotent / error]
  RESULT: [PASS/FAIL]

OVERALL: [PASS/FAIL]
```
