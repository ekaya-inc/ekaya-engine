# TASK: Update Developer Tool Loadouts

**Status:** READY
**Date:** 2026-03-13

## Current Status

Verified on 2026-03-18: implementation is still pending.

The current code still uses the old loadouts:

- `pkg/services/mcp_app_toggles.go` keeps MCP Server developer `addDirectDatabaseAccess` at `echo`, `execute`, `query`
- `pkg/services/mcp_app_toggles.go` keeps Ontology Forge developer `addOntologyMaintenanceTools` without `query` or `validate`
- `pkg/services/mcp_tool_loadouts.go` keeps `LoadoutDeveloperCore` at `echo`, `execute`
- `pkg/services/mcp_tool_loadouts.go` keeps `LoadoutOntologyMaintenance` without `query` or `validate`

This task is ready for implementation, but the approved loadout changes described below have not been applied yet.

## Goal

Update the developer-facing loadouts so they remain independently useful when enabled in isolation.

This task currently covers two developer toggle changes:

### MCP Server > Developer > Direct Database Access

Target tool set:

- `health` (always available)
- `echo`
- `execute`
- `query`
- `validate`
- `explain_query`

### Ontology Forge > Developer > Add Ontology Maintenance Tools

Target behavior:

- keep the existing ontology-maintenance tools
- add `query`
- add `validate`

Rationale:

- Ontology Forge developer tools must remain usable even when Direct Database Access is OFF
- the MCP client maintaining the ontology sometimes needs ad hoc read-only SQL to validate ontology correctness or answer ontology questions
- `explain_query` is not part of this Ontology Forge change

## Decision Summary

Approved changes for this task:

- Add `validate` to MCP Server developer `addDirectDatabaseAccess`
- Add `explain_query` to MCP Server developer `addDirectDatabaseAccess`
- Add `query` to Ontology Forge developer `addOntologyMaintenanceTools`
- Add `validate` to Ontology Forge developer `addOntologyMaintenanceTools`
- Remove nothing from either developer toggle

Important non-changes:

- Keep `validate` in AI Data Liaison user `addRequestTools`
- Keep `query` in AI Data Liaison user `addRequestTools`
- Keep `explain_query` in AI Data Liaison developer `addApprovalTools`
- Do not add `explain_query` to Ontology Forge in this task

## Duplicate Tool Support

Duplicate membership across toggles/loadouts is already supported in the current backend.

Verified behavior:

- `MergeLoadouts(...)` deduplicates by tool name via a `map[string]bool` in `pkg/services/mcp_tool_loadouts.go`
- `ComputeDeveloperTools(...)` and `ComputeUserTools(...)` also collect tool names into a map before returning canonical order
- `ComputeEnabledToolsFromConfig(...)` unions developer and user tools into another map before producing the final tool list

Implication:

- the same tool can safely appear in multiple toggles/loadouts
- the MCP client-facing enabled tool list will still be merged/deduplicated
- if the UI inventory later wants to show duplicate provenance per app/toggle, that would require a separate display-path change, but it is not required for this task

## Current State

### MCP Server developer toggle

In `pkg/services/mcp_app_toggles.go`, the MCP Server developer toggle currently exposes:

```go
Tools: []string{"echo", "execute", "query"}
```

### Ontology Forge developer toggle

In `pkg/services/mcp_app_toggles.go`, the Ontology Forge developer toggle currently exposes ontology-maintenance tools only and does not include `query` or `validate`.

### Related canonical loadout metadata

In `pkg/services/mcp_tool_loadouts.go`:

- `LoadoutDeveloperCore` currently contains only `echo` and `execute`
- `LoadoutQuery` already contains `validate`, `query`, and `explain_query`
- `LoadoutOntologyMaintenance` currently does not include `query` or `validate`
- `TestMergeLoadouts_Deduplication` in `pkg/services/mcp_tools_registry_test.go` already assumes overlap between `LoadoutDeveloperCore` and `LoadoutQuery`

This means duplicate placement is already a known/accepted pattern in the codebase.

## Files To Update

### Primary implementation files

- `pkg/services/mcp_app_toggles.go`
- `pkg/services/mcp_tool_loadouts.go`

### Test files likely requiring updates

- `pkg/services/mcp_tool_loadouts_test.go`
- `pkg/services/tool_access_test.go`
- `pkg/services/mcp_config_test.go`
- `pkg/mcp/tools/developer_filter_test.go`

### Files to inspect during implementation

- `pkg/services/mcp_tools_registry_test.go`
- `pkg/mcp/tools/developer.go`
- `pkg/mcp/tools/mcp_tools_integration_test.go`
- `pkg/mcp/tools/mcp_tools_scenario_test.go`

## Required Changes

### 1. Expand the MCP Server developer toggle

In `pkg/services/mcp_app_toggles.go`, update the MCP Server developer toggle:

From:

```go
Tools: []string{"echo", "execute", "query"}
```

To:

```go
Tools: []string{"echo", "execute", "query", "validate", "explain_query"}
```

Notes:

- this is the actual source used by `ComputeDeveloperTools(...)`
- do not remove `validate` from AI Data Liaison user tools
- do not remove `explain_query` from AI Data Liaison developer tools

### 2. Keep MCP Server canonical loadout metadata in sync

In `pkg/services/mcp_tool_loadouts.go`, update `LoadoutDeveloperCore`:

From:

```go
LoadoutDeveloperCore: {
    "echo",
    "execute",
},
```

To:

```go
LoadoutDeveloperCore: {
    "echo",
    "execute",
    "validate",
    "query",
    "explain_query",
},
```

Why:

- `LoadoutDeveloperCore` is still part of the canonical loadout model
- tests already reason about overlap between Developer Core and Query
- keeping it aligned with `AppToggles` reduces drift

### 3. Expand the Ontology Forge developer toggle

In `pkg/services/mcp_app_toggles.go`, update the Ontology Forge developer toggle to include:

- `query`
- `validate`

The resulting tool list should include those two read-only SQL tools in addition to the existing ontology-maintenance tools.

Notes:

- this is intentional overlap with other toggles
- do not add `execute`
- do not add `explain_query`

### 4. Keep Ontology Forge canonical loadout metadata in sync

In `pkg/services/mcp_tool_loadouts.go`, update `LoadoutOntologyMaintenance` to include:

- `query`
- `validate`

Recommendation:

- place them near the top of the loadout, before the ontology mutation tools, so the loadout definition makes the intended “investigate then update” workflow obvious

### 5. Update comments and expectations that still describe the old isolated loadouts

There are now two developer-toggle stories to update in tests/comments:

#### Direct Database Access only

Old assumption:

- `health`
- `echo`
- `execute`
- `query`

New assumption:

- `health`
- `echo`
- `execute`
- `query`
- `validate`
- `explain_query`

#### Ontology Maintenance only

Old assumption:

- ontology tools only
- no ad hoc SQL tools

New assumption:

- ontology tools
- `query`
- `validate`
- still no `echo`
- still no `execute`
- still no `explain_query`

Useful search terms:

- `AddDirectDatabaseAccess`
- `AddOntologyMaintenanceTools`
- `Direct Database Access =`
- `expected 4 tools`
- `query should NOT be included`
- `query should be filtered`

## Test Changes Expected

### `pkg/services/mcp_tool_loadouts_test.go`

Update expectations so:

- `AddDirectDatabaseAccess` includes `validate` and `explain_query`
- `AddOntologyMaintenanceTools` includes `query` and `validate`
- any assertions that ontology maintenance excludes `query` are updated

### `pkg/services/tool_access_test.go`

Update `IsToolAccessible(...)` and `GetAccessibleTools(...)` assertions for:

- `AddDirectDatabaseAccess` to include `validate` and `explain_query`
- `AddOntologyMaintenanceTools` to include `query` and `validate`

### `pkg/services/mcp_config_test.go`

Update enabled-tools response assertions for:

- default config
- explicit `AddDirectDatabaseAccess`
- explicit `AddOntologyMaintenanceTools`
- any combined-toggle inventory assertions that hardcode the old tool sets

### `pkg/mcp/tools/developer_filter_test.go`

This file likely needs the most churn.

Implementation notes:

- `createTestTools()` currently includes `validate` but not `explain_query`
- add `explain_query` to the test fixture(s) so the filter can actually surface it
- update AddDirectDatabaseAccess-only tests from 4 tools to 6 tools
- update Ontology Maintenance-only expectations so `query` and `validate` are present
- update any helper tests/comments that still treat `query`/`validate` as exclusively business-user tools when evaluating final visible tool lists

Important nuance:

- some legacy helper tests still model `query`, `sample`, and `validate` under approved-query visibility concepts
- do not broaden this task into a large categorization rewrite unless failing tests force it
- prefer the smallest consistent changes that make the current loadout system reflect the new intended inventories

### Additional files that may need expectation updates

Based on the current test corpus, also inspect:

- `pkg/services/mcp_tools_registry_test.go`
- `pkg/mcp/tools/mcp_tools_integration_test.go`
- `pkg/mcp/tools/mcp_tools_scenario_test.go`

## Expected Final Behavior

### When only `AddDirectDatabaseAccess=true`

For an admin/data role, the accessible tool set should be:

- `health`
- `echo`
- `execute`
- `query`
- `validate`
- `explain_query`

No ontology tools, approval tools, or request-workflow tools should be added by this toggle alone.

### When only `AddOntologyMaintenanceTools=true`

For an admin/data role, the accessible tool set should include:

- ontology maintenance tools
- ontology question tools
- schema inspection/search/probe tools already in that toggle
- `query`
- `validate`

And should still exclude:

- `echo`
- `execute`
- `explain_query`

This is the independence requirement for Ontology Forge when Direct Database Access is OFF.

## Out Of Scope

Do not do any of the following in this task unless explicitly asked:

- move `query` or `validate` out of AI Data Liaison user tools
- move `explain_query` out of AI Data Liaison developer tools
- add `explain_query` to Ontology Forge
- add `execute` to Ontology Forge
- recategorize `ToolRegistry` developer/user groups more broadly
- change agent tool loadouts
- redesign UI inventory presentation to show duplicate provenance per tool

## Verification

After implementation:

1. Run targeted tests while iterating:
   - `go test ./pkg/services`
   - `go test ./pkg/mcp/tools`
2. Run the full check:
   - `make check`

## Suggested Commit Message

`Update developer tool loadouts`
