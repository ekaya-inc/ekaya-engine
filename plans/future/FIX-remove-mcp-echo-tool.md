# FIX: Remove the MCP `echo` Tool

**Status:** Open
**Date:** 2026-04-01

## Problem

The MCP server still exposes an `echo` tool as a first-class developer tool for testing round trips.

That tool no longer appears necessary. If an MCP client needs to verify end-to-end request/response behavior, it can already use the existing `query()` tool with a trivial read-only statement such as:

```sql
select 'hello there'
```

Keeping a dedicated `echo` tool adds surface area without adding meaningful product capability.

## Root Cause

`echo` was introduced as a lightweight developer/testing utility, but it became wired into multiple product layers instead of remaining disposable test scaffolding.

Today it is represented in:

- MCP tool registration
- tool metadata registries and loadouts
- MCP Server UI copy for Direct Database Access
- automated tests and MCP test-suite docs

Because of that spread, removing it now requires a coordinated cleanup rather than deleting a single handler.

## Why this matters

- The tool inventory is larger and noisier than necessary
- `echo` creates a special-case testing path when `query()` already provides a real round trip
- MCP Server UI copy currently advertises `echo` as part of the direct database access toolset, which is misleading because it is not a real datasource capability
- The repo’s MCP tests and reference docs teach clients to use a non-essential tool

## Existing implementation context

### MCP tool registration

`pkg/mcp/tools/developer.go` registers `echo` via `registerEchoTool(...)`, and `RegisterMCPTools(...)` adds it alongside the real developer/query tools.

The current implementation:

- exposes tool name `echo`
- accepts a single `message` string parameter
- returns the same message in JSON form
- checks access through the normal MCP tool access path

### Tool metadata and loadouts

`echo` is currently included in:

- `pkg/services/mcp_tools_registry.go`
- `pkg/services/mcp_tool_loadouts.go`
- `pkg/services/mcp_app_toggles.go`

That means it is treated as part of the product’s supported developer tool catalog, not as an internal-only test helper.

### UI copy

`ui/src/pages/MCPServerPage.tsx` lists `echo` in the Direct Database Access description:

- query
- validate
- sample
- execute
- explain_query
- echo

That copy should stop referencing `echo` once the tool is removed.

### Tests and test docs

The MCP test materials currently document and/or call `echo`, including:

- `tests/TEST_SUITE.md`
- `tests/claude-mcp/CLAUDE.md`

Automated tests across `pkg/services` and `pkg/mcp/tools` also assert that `echo` appears in loadouts, filtered tool lists, and integration scenarios.

## Intended fix

Remove `echo` completely rather than deprecating it.

Preferred approach:

1. Delete the MCP `echo` tool registration and stop advertising it anywhere in product code.
2. Remove `echo` from tool registries, loadouts, access expectations, and toggle-controlled tool lists.
3. Update MCP test documentation and automated tests to use `query()` for round-trip checks instead of `echo`.

For the test/docs replacement, prefer a minimal read-only query such as:

```sql
select 'hello there'
```

Do not keep an alias, compatibility shim, or hidden no-op implementation of `echo`.

## File-by-file changes

### 1. `pkg/mcp/tools/developer.go`

- [ ] Remove `registerEchoTool(...)`
- [ ] Remove the `registerEchoTool(s, deps)` call from `RegisterMCPTools(...)`
- [ ] Remove any now-unused imports that only existed for `echo`

### 2. `pkg/services/mcp_tools_registry.go`

- [ ] Remove the `echo` tool definition from `ToolRegistry`
- [ ] Keep the remaining developer tools in a coherent order

### 3. `pkg/services/mcp_tool_loadouts.go`

- [ ] Remove `echo` from `AllToolsOrdered`
- [ ] Remove `echo` from `LoadoutDeveloperCore`
- [ ] Keep `execute` as the remaining developer-core database mutation tool

### 4. `pkg/services/mcp_app_toggles.go`

- [ ] Remove `echo` from the MCP Server developer `addDirectDatabaseAccess` toggle tool list
- [ ] Leave the rest of the toggle behavior unchanged

### 5. `ui/src/pages/MCPServerPage.tsx`

- [ ] Update the Direct Database Access description to stop mentioning `echo`
- [ ] Keep the copy aligned with the actual enabled tool set after removal

### 6. MCP test docs

- [ ] Update `tests/TEST_SUITE.md` to remove the `echo(...)` step
- [ ] Replace that step with a simple `query()` round-trip example
- [ ] Update `tests/claude-mcp/CLAUDE.md` to remove `echo` from the MCP tools reference

### 7. Automated tests

- [ ] Update tests that currently expect `echo` in tool registries, loadouts, access checks, and filtered tool lists
- [ ] Update MCP integration/scenario tests that currently enumerate `echo`
- [ ] Remove `echo`-specific assertions rather than replacing them with a new synthetic test-only tool

## Automated tests to update

At minimum, review and update the existing automated coverage in these areas:

- `pkg/services/tool_access_test.go`
- `pkg/services/mcp_state_test.go`
- `pkg/services/mcp_tool_loadouts_test.go`
- `pkg/mcp/tools/access_test.go`
- `pkg/mcp/tools/developer_filter_test.go`
- `pkg/mcp/tools/mcp_tools_integration_test.go`
- `pkg/mcp/tools/mcp_tools_scenario_test.go`

The exact assertions may change, but the intended post-fix state is:

- `echo` is absent from exposed tool inventories
- `echo` is absent from enabled-tool/loadout calculations
- developer/user/agent filtering behavior still works for the remaining tools

## Non-goals

- [ ] Do not add a replacement `ping`/`round_trip` tool
- [ ] Do not keep `echo` behind a hidden flag or backward-compatibility path
- [ ] Do not broaden this into a redesign of MCP tool groups or app-toggle architecture
- [ ] Do not change `query()` semantics beyond using it as the recommended testing round-trip

## Expected outcome

After this fix:

- the MCP server no longer exposes `echo`
- the UI no longer advertises `echo`
- MCP test docs use `query()` for round-trip verification
- tool metadata, loadouts, and access-control tests all reflect the smaller supported tool set
