# PLAN: Schema Selection Shows Pending Changes for Approval

**Status:** DONE
**Created:** 2026-03-01
**Depends on:** PLAN-dry-up-schema-refresh.md
**Related:** DESIGN-pending-changes-workflow.md

## Problem

The Schema Selection screen (`ui/src/pages/SchemaPage.tsx`) shows all tables/columns with checkboxes but has no visual distinction between existing schema items and newly detected additions from a refresh. The user saves the schema without knowing they are implicitly approving pending changes. The pending changes in `engine_ontology_pending_changes` are never resolved.

## Solution

1. **Backend**: The schema API endpoint returns pending change information alongside the schema, so the frontend knows which tables/columns are new or modified.
2. **Frontend**: New/modified items are visually distinguished (badge, highlight, or grouping). Auto-applied deletions are shown as informational notices.
3. **Backend**: `SaveSelections` resolves matching pending changes — checked items are approved, unchecked new items are rejected.

## Implementation

### Task 1: RED Tests - Backend API includes pending change summary

Add tests in `pkg/handlers/schema_test.go`:

- [ ] Test: `GetSchema` response includes a `pending_changes` map keyed by `"schema.table_name"` (for table-level changes) and `"schema.table_name.column_name"` (for column-level changes)
- [ ] Test: Each pending change entry includes `change_id`, `change_type`, and `status`
- [ ] Test: When no pending changes exist, the `pending_changes` map is empty (not null)

### Task 2: RED Tests - SaveSelections resolves pending changes

Add tests in `pkg/handlers/schema_test.go` and `pkg/services/schema_test.go`:

- [ ] Test: `SaveSelections` with a checked new table approves the matching `new_table` pending change
- [ ] Test: `SaveSelections` with a checked new column approves the matching `new_column` pending change
- [ ] Test: `SaveSelections` with an unchecked new table rejects the matching `new_table` pending change
- [ ] Test: `SaveSelections` with an unchecked new column rejects the matching `new_column` pending change
- [ ] Test: `SaveSelections` ignores `auto_applied` pending changes (destructive changes already handled)
- [ ] Test: `SaveSelections` with no pending changes behaves as before (backward compatible)
- [ ] Test: `SaveSelections` response includes count of approved and rejected changes

### Task 3: GREEN - Backend: Add pending changes to schema API response

In `pkg/handlers/schema.go` and `pkg/services/schema.go`:

- [ ] In the `GetSchema` handler (or service), query `engine_ontology_pending_changes` for pending changes matching this project
- [ ] Build a map of `table_name` -> change info and `table_name.column_name` -> change info
- [ ] Add to the schema response as `pending_changes` field

Update types:

- [ ] Add `PendingChangeInfo` struct: `{ ChangeID uuid, ChangeType string, Status string }`
- [ ] Add `PendingChanges map[string]PendingChangeInfo` field to schema response

### Task 4: GREEN - Backend: SaveSelections resolves pending changes

In `pkg/services/schema.go` (the `SaveSelections` method at line 797):

- [ ] After saving selections, query pending changes with `status = 'pending'` for the project
- [ ] For each `new_table` pending change: if the table is selected, approve it; if not, reject it
- [ ] For each `new_column` pending change: if the column is selected, approve it; if not, reject it
- [ ] For each `modified_column` pending change: if the column is selected, approve it; if not, reject it
- [ ] Skip `auto_applied` changes (already handled)
- [ ] Use `PendingChangeRepository.UpdateStatus()` for each change
- [ ] Return counts in the response

### Task 5: GREEN - Frontend: Update types

In `ui/src/types/schema.ts`:

- [ ] Add `PendingChangeInfo` type: `{ change_id: string, change_type: string, status: string }`
- [ ] Add `pending_changes?: Record<string, PendingChangeInfo>` to `DatasourceSchema`
- [ ] Update `SaveSelectionsResponse` to include `approved_count` and `rejected_count`

### Task 6: GREEN - Frontend: Visual distinction for pending items

In `ui/src/pages/SchemaPage.tsx`:

- [ ] For tables with a matching `new_table` pending change, show a "New" badge (e.g., small blue pill)
- [ ] For columns with a matching `new_column` pending change, show a "New" badge
- [ ] For columns with a matching `modified_column` pending change, show a "Modified" badge (e.g., small amber pill)
- [ ] For `auto_applied` `dropped_table`/`dropped_column` changes, show an informational banner at the top listing what was removed during the last refresh
- [ ] Style: badges should be subtle but clearly visible — use the existing UI component library

### Task 7: GREEN - Frontend: Toast notification for resolved changes

In `ui/src/pages/SchemaPage.tsx`:

- [ ] After successful save, if changes were resolved, show toast: "Schema saved. X changes approved, Y changes rejected."
- [ ] If auto-applied deletions exist, include in toast: "Z items were automatically removed (no longer in datasource)."

### Task 8: REFACTOR

- [ ] Verify all existing tests pass
- [ ] Verify backward compatibility: schema response with no pending changes works as before

## Files Affected

| File | Change |
|------|--------|
| `pkg/handlers/schema.go` | GetSchema includes pending changes; SaveSelections resolves them |
| `pkg/handlers/schema_test.go` | New tests |
| `pkg/services/schema.go` | SaveSelections resolves pending changes |
| `pkg/services/schema_test.go` | New tests |
| `ui/src/types/schema.ts` | Add PendingChangeInfo type |
| `ui/src/pages/SchemaPage.tsx` | Visual badges, informational banner |
| `ui/src/services/engineApi.ts` | Update response type if needed |

## UI Mockup (Conceptual)

```
Schema Selection
Select the tables and columns to include in your ontology

[ Info Banner: "Last refresh removed: orders (table dropped), users.legacy_field (column dropped)" ]

Datasource Schema
Schema: public - 12 tables, 145 columns selected

[x] Select All Tables and Columns

  [x] applications          9 columns
  [x] competitors           11 columns
  [x] content_posts         23 columns
  [x] invoices  [NEW]       5 columns        <-- new table from refresh
      [x] id
      [x] amount  [NEW]                      <-- new column
      [x] status  [MODIFIED]                 <-- type changed
```
