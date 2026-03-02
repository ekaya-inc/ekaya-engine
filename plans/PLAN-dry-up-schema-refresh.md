# PLAN: DRY Up Schema Refresh Code Paths

**Status:** DONE
**Created:** 2026-03-01
**Related:** DESIGN-pending-changes-workflow.md

## Problem

The MCP `refresh_schema` tool and the HTTP `RefreshSchema` handler (`POST .../schema/refresh`) both call `RefreshDatasourceSchema()` but have divergent behavior afterward:

- **MCP tool** (`pkg/mcp/tools/developer.go:1084-1184`): Calls `SchemaChangeDetectionService.DetectChanges()` to create pending changes, supports `auto_select` parameter (defaults true), returns enriched response with table/column names and pending change count.
- **HTTP handler** (`pkg/handlers/schema.go:263-296`): Does NOT call `DetectChanges()`, hard-codes `autoSelect=false`, returns basic statistics only.

This means clicking [Refresh] in the Schema Selection UI never creates pending changes, while the MCP tool does. The behavior should be identical.

## Solution

Extract a shared `RefreshSchemaWithChangeDetection()` method in the schema service layer that both the MCP tool and HTTP handler call. Both paths produce pending changes. The HTTP handler response is enriched to include pending change information so the UI can display it.

## Implementation

### Task 1: RED Tests - Schema Service shared refresh method

Add tests in `pkg/services/schema_change_detection_test.go` (or a new `schema_refresh_orchestrator_test.go`) that verify:

- [x]Test: `RefreshSchemaWithChangeDetection` calls `RefreshDatasourceSchema` AND `DetectChanges`
- [x]Test: When `DetectChanges` fails, the refresh still succeeds (non-fatal, logged)
- [x]Test: When `RefreshDatasourceSchema` returns no changes, `DetectChanges` is still called (to handle idempotent refreshes)
- [x]Test: Return value includes both the `RefreshResult` and the count of pending changes created

### Task 2: RED Tests - HTTP Handler returns pending change info

Add tests in `pkg/handlers/schema_test.go`:

- [x]Test: `RefreshSchema` handler response includes `pending_changes_created` count
- [x]Test: `RefreshSchema` handler response includes `new_table_names` and `removed_table_names`

### Task 3: GREEN - Implement shared service method

In `pkg/services/schema.go` (or a new orchestrator):

- [x]Create `RefreshSchemaWithChangeDetection(ctx, projectID, dsID, autoSelect bool) (*RefreshResultWithChanges, error)`
- [x]This method calls `RefreshDatasourceSchema()` then `DetectChanges()`
- [x]Define `RefreshResultWithChanges` struct that wraps `RefreshResult` + pending change count

### Task 4: GREEN - Update HTTP handler

In `pkg/handlers/schema.go`:

- [x]Update `RefreshSchema` handler to call the new shared method
- [x]Accept optional `auto_select` query parameter (default: false for backward compat)
- [x]Update `RefreshSchemaResponse` struct to include `pending_changes_created`, `new_table_names`, `removed_table_names`
- [x]Update existing handler tests to pass with new response shape

### Task 5: GREEN - Update MCP tool

In `pkg/mcp/tools/developer.go`:

- [x]Update `registerRefreshSchemaTool` to call the new shared method instead of calling `RefreshDatasourceSchema` and `DetectChanges` separately
- [x]Verify MCP tool response still includes all current fields

### Task 6: GREEN - Update frontend API types

In `ui/src/services/engineApi.ts` and `ui/src/types/schema.ts`:

- [x]Update `SchemaRefreshResponse` type to include `pending_changes_created`, `new_table_names`, `removed_table_names`

### Task 7: REFACTOR

- [x]Remove the direct `DetectChanges` call from `developer.go` (now handled by shared method)
- [x]Verify all existing tests pass
- [x]Run integration tests

## Files Affected

| File | Change |
|------|--------|
| `pkg/services/schema.go` | Add `RefreshSchemaWithChangeDetection()` |
| `pkg/services/schema_test.go` (new or existing) | New tests for shared method |
| `pkg/handlers/schema.go` | Update `RefreshSchema` handler |
| `pkg/handlers/schema_test.go` | Update handler tests |
| `pkg/mcp/tools/developer.go` | Simplify to use shared method |
| `pkg/mcp/tools/refresh_schema_test.go` | Update if needed |
| `ui/src/types/schema.ts` | Update response types |
| `ui/src/services/engineApi.ts` | Update if response shape changes |
