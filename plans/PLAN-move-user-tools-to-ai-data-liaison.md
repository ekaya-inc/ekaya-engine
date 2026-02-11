# PLAN: Move User Tools to AI Data Liaison Application

**Status:** IMPLEMENTED
**Branch:** `ddanieli/move-user-tools-to-ai-data-liaison`

## Problem

The MCP Server page currently shows both "User Tools" and "Developer Tools" sections. User Tools should be part of the AI Data Liaison application, not the base MCP Server. The MCP Server page should only show Developer Tools.

## Architecture Context

### How MCP tool filtering works (no changes needed)

The MCP tool filter (`pkg/mcp/tools/developer.go:NewToolFilter`) computes the tool set for non-agent users via `ComputeEnabledToolsFromConfig()`, which uses **only the developer config** (`addQueryTools`, `addOntologyMaintenance`). It does NOT use the user tool group config. The `DataLiaisonTools` map is then applied to remove suggest/approve query tools when the app isn't installed.

The `UserTools` field in the API response is computed by `ComputeUserTools()` separately in `buildResponse()` - it's **only for UI display**, not MCP filtering.

This means: the MCP filter behavior does not change. Developers get query tools through the developer config regardless.

### What the "user" tool group config controls

The `user` tool group config has one sub-option: `AllowOntologyMaintenance`. This controls whether `ComputeUserTools()` includes the OntologyMaintenance loadout in the `userTools` API response. It's a UI-only concern.

## Implementation

This is primarily a **frontend change** - moving the User Tools UI section from the MCP Server page to the AI Data Liaison page. The backend already handles everything correctly.

### 1. Remove User Tools from MCP Server page

**File:** `ui/src/pages/MCPServerPage.tsx`

- [x] Remove `<UserToolsSection>` component and its import
- [x] Remove `handleAllowOntologyMaintenanceChange` handler (lines 60-88)
- [x] Remove `approvedQueriesState` and `allowOntologyMaintenance` state derivations (lines 26-27)
- [x] Remove the `UserToolsSection` import (line 7)
- [x] Remove unused imports: `TOOL_GROUP_IDS` if no longer needed — still used for developer state, kept

### 2. Add User Tools section to AI Data Liaison page

**File:** `ui/src/pages/AIDataLiaisonPage.tsx`

- [x] Import `UserToolsSection` from `../components/mcp/UserToolsSection`
- [x] Import `TOOL_GROUP_IDS` from `../constants/mcpToolMetadata`
- [x] Derive `allowOntologyMaintenance` from `mcpConfig` (already fetched): `mcpConfig?.toolGroups[TOOL_GROUP_IDS.USER]?.allowOntologyMaintenance ?? true`
- [x] Add `handleAllowOntologyMaintenanceChange` handler that calls `engineApi.updateMCPConfig(pid, { allowOntologyMaintenance: enabled })` and refetches config
- [x] Add `updating` state for the toggle
- [x] Render `<UserToolsSection>` after the "Setup Checklist" card, passing:
  - `projectId={pid}`
  - `allowOntologyMaintenance` from derived state
  - `onAllowOntologyMaintenanceChange` handler
  - `enabledTools={mcpConfig?.userTools ?? []}`
  - `disabled={updating}`

### 3. Simplify the "Enabled Tools" card on AI Data Liaison page

**File:** `ui/src/pages/AIDataLiaisonPage.tsx`

The current page has a static "Enabled Tools" card showing 8 data-liaison-specific tools. Now that `<UserToolsSection>` shows all user tools dynamically (including the 2 suggest tools), the static card is redundant for user tools.

- [x] Remove the `DATA_LIAISON_USER_TOOLS` constant
- [x] Keep `DATA_LIAISON_DEVELOPER_TOOLS` constant - these are the developer-side tools added by AI Data Liaison (query suggestion management)
- [x] Replace the "Enabled Tools" card with a simpler "Additional Developer Tools" card that shows only the developer-side tools enabled by AI Data Liaison installation
- [x] Update the card title/description to reflect these are developer tools added to the MCP Server by installing AI Data Liaison

## Files Modified

| File | Change |
|------|--------|
| `ui/src/pages/MCPServerPage.tsx` | Remove `<UserToolsSection>` and related state/handlers |
| `ui/src/pages/AIDataLiaisonPage.tsx` | Add `<UserToolsSection>`, simplify Enabled Tools card |

## Files NOT Modified

| File | Why |
|------|-----|
| `pkg/services/mcp_tools_registry.go` | `DataLiaisonTools` map stays as-is (gates suggest/approve tools correctly) |
| `pkg/services/mcp_config.go` | `buildResponse()` already computes `userTools` correctly |
| `pkg/services/mcp_tool_loadouts.go` | Loadout computation unchanged |
| `pkg/mcp/tools/developer.go` | MCP tool filter unchanged |
| `pkg/handlers/mcp_config.go` | API endpoints unchanged |

## Out of Scope

- MCP tool filter changes (not needed - developer tools work through developer config)
- Backend API changes (not needed - `userTools` field is already computed and available)
- Agent tools (unchanged)
- Database migrations (none needed)
