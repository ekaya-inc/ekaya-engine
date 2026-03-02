# PLAN: Schema Selection Approve/Reject Button Behavior

**Status:** DONE
**Created:** 2026-03-01
**Depends on:** PLAN-schema-selection-pending-changes-ui.md

## Problem

The Schema Selection screen has [Cancel] and [Save Schema] buttons. When pending changes are present (new tables/columns detected from a refresh), the user is implicitly approving or rejecting them by saving or canceling — but the button labels don't communicate this. The buttons should adapt to reflect what the user is actually doing.

## Solution

When pending changes with `status = 'pending'` exist:
- [Save Schema] becomes [Approve Changes] (the user is approving checked items and rejecting unchecked new items)
- [Cancel] becomes [Reject Changes] (the user is rejecting all pending changes and navigating away)

When no pending changes exist (steady state):
- Buttons remain [Cancel] and [Save Schema] as today

## Behavior Details

**[Approve Changes]** (replaces Save Schema):
- Saves all selection state (same as Save Schema)
- Additionally resolves pending changes: checked new items = approved, unchecked new items = rejected
- This is handled by the backend changes in PLAN-schema-selection-pending-changes-ui.md

**[Reject Changes]** (replaces Cancel):
- Rejects ALL pending changes (sets status to `rejected`)
- Reverts the selection state to what it was before the refresh added new items
- Navigates back to dashboard
- NOTE: This requires a backend call to reject pending changes before navigating away (unlike current Cancel which just navigates)

## Implementation

### Task 1: RED Tests - Frontend: Button labels change based on pending changes

These are component/UI tests (React Testing Library or similar):

- [ ] Test: When `pending_changes` is empty, Save button shows "Save Schema"
- [ ] Test: When `pending_changes` is empty, Cancel button shows "Cancel"
- [ ] Test: When `pending_changes` has entries with `status = 'pending'`, Save button shows "Approve Changes"
- [ ] Test: When `pending_changes` has entries with `status = 'pending'`, Cancel button shows "Reject Changes"
- [ ] Test: When `pending_changes` only has `auto_applied` entries (no pending), buttons show default labels

### Task 2: RED Tests - Backend: Reject all pending changes endpoint

Add tests in `pkg/handlers/schema_test.go`:

- [ ] Test: `POST .../schema/reject-pending-changes` rejects all pending changes for the project/datasource
- [ ] Test: Reject endpoint returns count of rejected changes
- [ ] Test: Reject endpoint ignores `auto_applied` changes
- [ ] Test: Reject endpoint with no pending changes returns 0 count

### Task 3: GREEN - Backend: Add reject-all endpoint

In `pkg/handlers/schema.go`:

- [ ] Add new handler: `RejectPendingChanges(w http.ResponseWriter, r *http.Request)`
- [ ] Route: `POST /api/projects/{pid}/datasources/{dsid}/schema/reject-pending-changes`
- [ ] Queries all pending changes with `status = 'pending'` for the project
- [ ] Updates each to `status = 'rejected'`
- [ ] Returns `{ rejected_count: N }`

### Task 4: GREEN - Frontend: Dynamic button labels

In `ui/src/pages/SchemaPage.tsx`:

- [ ] Derive `hasPendingChanges` boolean from the schema response's `pending_changes` field (filter to `status = 'pending'` only)
- [ ] Save button label: `hasPendingChanges ? 'Approve Changes' : 'Save Schema'`
- [ ] Cancel button label: `hasPendingChanges ? 'Reject Changes' : 'Cancel'`
- [ ] Save button styling: optionally use a slightly different color when approving (e.g., green instead of blue) to reinforce the action

### Task 5: GREEN - Frontend: Reject Changes behavior

In `ui/src/pages/SchemaPage.tsx`:

- [ ] When [Reject Changes] is clicked (and pending changes exist):
  1. Call `engineApi.rejectPendingChanges(projectId, datasourceId)`
  2. On success, navigate back to dashboard
  3. On error, show error toast
- [ ] When [Cancel] is clicked (no pending changes): navigate back to dashboard as before

In `ui/src/services/engineApi.ts`:

- [ ] Add `rejectPendingChanges(projectId, datasourceId)` method calling the new endpoint

### Task 6: GREEN - Frontend: Button disabled states

In `ui/src/pages/SchemaPage.tsx`:

- [ ] [Approve Changes] is enabled when pending changes exist (even if no manual selection changes were made — the user is approving the current state)
- [ ] [Save Schema] remains disabled when no changes from initial state (current behavior)
- [ ] [Reject Changes] is always enabled when pending changes exist
- [ ] [Cancel] disabled state unchanged from current behavior

### Task 7: REFACTOR

- [ ] Verify all existing tests pass
- [ ] Verify button behavior matches expected UX in both states (with and without pending changes)

## Files Affected

| File | Change |
|------|--------|
| `pkg/handlers/schema.go` | Add `RejectPendingChanges` handler |
| `pkg/handlers/schema_test.go` | New tests for reject endpoint |
| `ui/src/pages/SchemaPage.tsx` | Dynamic button labels, reject behavior |
| `ui/src/services/engineApi.ts` | Add `rejectPendingChanges()` method |

## State Diagram

```
[Schema Selection Screen Loaded]
         |
         v
  pending_changes in response?
        / \
       /   \
     Yes    No
      |      |
      v      v
  [Approve Changes]  [Save Schema]
  [Reject Changes]   [Cancel]
      |                  |
      v                  v
  Approve: save +       Save: save selections
  resolve changes        (no pending change logic)
      |
  Reject: reject all +
  navigate away
```
