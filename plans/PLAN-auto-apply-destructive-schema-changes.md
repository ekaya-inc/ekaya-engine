# PLAN: Auto-Apply Destructive Schema Changes

**Status:** DONE
**Created:** 2026-03-01
**Related:** PLAN-dry-up-schema-refresh.md, DESIGN-pending-changes-workflow.md

## Problem

When a database table or column is dropped from the upstream datasource, the schema refresh detects this and creates `dropped_table` / `dropped_column` pending changes with status `pending`. These sit waiting for user "approval" — but approval is meaningless because the table/column is already gone. Meanwhile, the stale schema entries cause the LLM to generate invalid SQL referencing non-existent objects.

## Solution

Destructive changes (dropped tables/columns) should be **immediately applied** to the schema during refresh:

1. The schema tables (`engine_schema_tables`, `engine_schema_columns`) are updated to remove the dropped items (this already happens via `RefreshDatasourceSchema` which soft-deletes them).
2. Pending change rows ARE still created — but with status `auto_applied` instead of `pending` — so the UI can inform the user what was removed.
3. These `auto_applied` rows are informational only. Approve/reject actions on them are no-ops (the deletion already happened).

## Rationale

- A user cannot "reject" a deletion — the underlying object is gone.
- Keeping non-existent tables/columns in the schema leads to broken SQL generation.
- The user still needs to KNOW that something was removed (hence the informational pending change row).

## Implementation

### Task 1: RED Tests - DetectChanges marks destructive changes as auto_applied

Add tests in `pkg/services/schema_change_detection_test.go`:

- [x] Test: `DetectChanges` sets `Status = "auto_applied"` for `dropped_table` changes (currently sets `pending`)
- [x] Test: `DetectChanges` sets `Status = "auto_applied"` for `dropped_column` changes (currently sets `pending`)
- [x] Test: `DetectChanges` still sets `Status = "pending"` for `new_table`, `new_column`, `modified_column` changes (unchanged)

### Task 2: RED Tests - Approve/reject no-ops for auto_applied changes

Add tests in `pkg/services/change_review_service_test.go`:

- [x] Test: Approving an `auto_applied` change returns success without side effects
- [x] Test: Rejecting an `auto_applied` change returns success without side effects (does not restore the table/column)

### Task 3: RED Tests - MCP tools handle auto_applied status

Add tests for the `list_pending_changes` and `approve_change` MCP tools:

- [x] Test: `list_pending_changes` with `status=auto_applied` returns destructive changes
- [x] Test: `approve_change` on an `auto_applied` change returns a message indicating it was already applied

### Task 4: GREEN - Update DetectChanges

In `pkg/services/schema_change_detection.go`:

- [x] Change `dropped_table` changes (line 78) from `ChangeStatusPending` to `ChangeStatusAutoApplied`
- [x] Change `dropped_column` changes (line 108) from `ChangeStatusPending` to `ChangeStatusAutoApplied`
- [x] Verify `ChangeStatusAutoApplied` constant exists in models (it does: `auto_applied` is a valid status per the CHECK constraint)

### Task 5: GREEN - Update change review service

In `pkg/services/change_review_service.go`:

- [x] When approving a change with `status = auto_applied`, return success immediately (no-op)
- [x] When rejecting a change with `status = auto_applied`, return success immediately (no-op)

### Task 6: REFACTOR

- [x] Verify existing tests still pass
- [x] Verify the UI (once built) correctly displays `auto_applied` changes as informational

## Files Affected

| File | Change |
|------|--------|
| `pkg/services/schema_change_detection.go` | Set `auto_applied` for destructive changes |
| `pkg/services/schema_change_detection_test.go` | New tests |
| `pkg/services/change_review_service.go` | No-op for auto_applied approve/reject |
| `pkg/services/change_review_service_test.go` | New tests |
| `pkg/models/pending_change.go` | Verify `ChangeStatusAutoApplied` constant exists |

## Edge Cases

- **Modified columns** remain `pending` — a type change is not destructive and the user should decide whether to update the ontology metadata.
- **Re-added tables** — if a table is dropped then re-added, the `auto_applied` dropped_table row remains as historical record. The new table gets a fresh `new_table` pending change.
