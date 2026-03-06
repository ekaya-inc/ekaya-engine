# Per-App Tool Loadout Architecture

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Each application (MCP Server, Ontology Forge, AI Data Liaison) manages its own tool configuration with Developer and User toggles, while the MCP Server page shows a read-only inventory of all tools grouped by role then by app.

**Architecture:** Replace the flat `ToolGroupConfig` toggle model (AddQueryTools, AddOntologyMaintenance, AllowOntologyMaintenance) with per-app toggle definitions. Each app declares its toggles and the tools they control. The backend computes enabled tools by merging all enabled toggles across apps. The API response includes `app_id` per tool and an `appNames` map for display. The MCP tool filter uses the same computation to ensure UI and MCP Server stay in sync.

**Tech Stack:** Go backend (models, services, loadouts), TypeScript/React frontend (types, components, pages)

**Key files (backend):**
- `pkg/models/mcp_config.go` — ToolGroupConfig, MCPConfig, DefaultMCPConfig
- `pkg/services/mcp_tools_registry.go` — ToolDefinition, ToolRegistry, DataLiaisonTools
- `pkg/services/mcp_tool_loadouts.go` — ToolSpec, Loadouts, AllToolsOrdered, Compute*Tools functions
- `pkg/services/mcp_config.go` — MCPConfigResponse, EnabledToolInfo, buildResponse, Compute functions
- `pkg/services/mcp_state.go` — MCPStateValidator, applyUpdate, normalizeState, deepCopy
- `pkg/services/tool_access.go` — ToolAccessChecker
- `pkg/mcp/tools/developer.go` — NewToolFilter (MCP tool filtering)

**Key files (frontend):**
- `ui/src/types/mcp.ts` — ToolGroupState, EnabledToolInfo, MCPConfigResponse, UpdateMCPConfigRequest
- `ui/src/constants/mcpToolMetadata.tsx` — TOOL_GROUP_IDS, TOOL_GROUP_SUB_OPTIONS
- `ui/src/components/mcp/DeveloperToolsSection.tsx` — Developer tools config UI
- `ui/src/components/mcp/UserToolsSection.tsx` — User tools config UI
- `ui/src/components/mcp/MCPEnabledTools.tsx` — Collapsible tool list
- `ui/src/pages/MCPServerPage.tsx` — MCP Server config page
- `ui/src/pages/OntologyForgePage.tsx` — Ontology Forge config page
- `ui/src/pages/AIDataLiaisonPage.tsx` — AI Data Liaison config page
- `ui/src/services/engineApi.ts` — API client

**Tool categorization (approved design):**

MCP Server (always: `health`):
- Developer > "Direct Database Access": `echo`, `execute`, `query`

Ontology Forge:
- Developer > "Add Ontology Maintenance Tools": `get_schema`, `search_schema`, `probe_column`, `probe_columns`, `get_column_metadata`, `update_column`, `update_columns`, `update_table`, `update_project_knowledge`, `update_glossary_term`, `create_glossary_term`, `delete_column_metadata`, `delete_table_metadata`, `delete_project_knowledge`, `delete_glossary_term`, `refresh_schema`, `scan_data_changes`, `list_pending_changes`, `approve_change`, `reject_change`, `approve_all_changes`, `list_ontology_questions`, `resolve_ontology_question`, `skip_ontology_question`, `escalate_ontology_question`, `dismiss_ontology_question`
- User > "Add Ontology Suggestions [RECOMMENDED]": `get_context`, `get_ontology`, `list_glossary`, `get_glossary_sql`

AI Data Liaison:
- Developer > "Add Approval Tools": `list_query_suggestions`, `approve_query_suggestion`, `reject_query_suggestion`, `create_approved_query`, `update_approved_query`, `delete_approved_query`, `explain_query`
- User > "Add Request Tools [RECOMMENDED]": `query`, `sample`, `validate`, `list_approved_queries`, `execute_approved_query`, `suggest_approved_query`, `suggest_query_update`, `get_query_history`, `record_query_feedback`

Note: `query` appears in two toggles (MCP Server developer, AI Data Liaison user). This is intentional — for developers it's raw DB access; for users it's part of the query workflow.

---

## Task 1: Backend — Define per-app toggle registry

**Files:**
- Create: `pkg/services/mcp_app_toggles.go`
- Modify: `pkg/models/mcp_config.go`

**Context:** Define the mapping from app → role → toggle → tools. This is the new single source of truth for which tools belong to which toggle. Also add per-app toggle fields to `ToolGroupConfig`.

**Step 1: Create `pkg/services/mcp_app_toggles.go`**

This file defines:
- `AppToggle` struct: `AppID`, `Role` (developer/user), `ToggleKey` (config field name), `DisplayName`, `Tools` ([]string)
- `AppToggles` — ordered slice of all AppToggle definitions (MCP Server, Ontology Forge, AI Data Liaison)
- `AppDisplayNames` — map of app_id → display name
- Helper functions: `GetToggleTools(appID, role, toggleKey)`, `GetAppTogglesForRole(role)`

Tool assignments per toggle (exact tool names from `AllToolsOrdered`):

```go
// MCP Server — Developer > Direct Database Access
{"echo", "execute", "query"}

// Ontology Forge — Developer > Add Ontology Maintenance Tools
{"get_schema", "search_schema", "probe_column", "probe_columns", "get_column_metadata",
 "update_column", "update_columns", "update_table", "update_project_knowledge",
 "update_glossary_term", "create_glossary_term",
 "delete_column_metadata", "delete_table_metadata", "delete_project_knowledge", "delete_glossary_term",
 "refresh_schema", "scan_data_changes", "list_pending_changes",
 "approve_change", "reject_change", "approve_all_changes",
 "list_ontology_questions", "resolve_ontology_question", "skip_ontology_question",
 "escalate_ontology_question", "dismiss_ontology_question"}

// Ontology Forge — User > Add Ontology Suggestions
{"get_context", "get_ontology", "list_glossary", "get_glossary_sql"}

// AI Data Liaison — Developer > Add Approval Tools
{"list_query_suggestions", "approve_query_suggestion", "reject_query_suggestion",
 "create_approved_query", "update_approved_query", "delete_approved_query", "explain_query"}

// AI Data Liaison — User > Add Request Tools
{"query", "sample", "validate", "list_approved_queries", "execute_approved_query",
 "suggest_approved_query", "suggest_query_update", "get_query_history", "record_query_feedback"}
```

**Step 2: Add new toggle fields to `ToolGroupConfig` in `pkg/models/mcp_config.go`**

Add these new fields (keep existing fields for backward compatibility):

```go
// MCP Server toggle
AddDirectDatabaseAccess bool `json:"addDirectDatabaseAccess"`

// Ontology Forge toggles
AddOntologyMaintenanceTools bool `json:"addOntologyMaintenanceTools"`
AddOntologySuggestions      bool `json:"addOntologySuggestions"`

// AI Data Liaison toggles
AddApprovalTools bool `json:"addApprovalTools"`
AddRequestTools  bool `json:"addRequestTools"`
```

Note: The existing `AddQueryTools` and `AddOntologyMaintenance` fields stay for backward compat but become unused. The existing `AllowOntologyMaintenance` field stays for backward compat but becomes unused.

**Step 3: Update `DefaultMCPConfig` in `pkg/models/mcp_config.go`**

The default config should have all toggles ON (maximally permissive), stored under a single "tools" key (replacing "user" and "developer" keys):

```go
func DefaultMCPConfig(projectID uuid.UUID) *MCPConfig {
    return &MCPConfig{
        ProjectID: projectID,
        ToolGroups: map[string]*ToolGroupConfig{
            "tools": {
                AddDirectDatabaseAccess:     true,
                AddOntologyMaintenanceTools: true,
                AddOntologySuggestions:      true,
                AddApprovalTools:            true,
                AddRequestTools:             true,
            },
            // Agent tools - enabled by default
            "agent_tools": {Enabled: true},
            // Legacy keys preserved for backward compat
            "user":      {AllowOntologyMaintenance: true},
            "developer": {AddQueryTools: true, AddOntologyMaintenance: true},
        },
    }
}
```

**Step 4: Run `make check`** to verify compilation.

**Step 5: Commit** — `feat: define per-app toggle registry and config fields`

---

## Task 2: Backend — Rewrite tool computation to use per-app toggles

**Files:**
- Modify: `pkg/services/mcp_tool_loadouts.go`
- Modify: `pkg/services/mcp_tools_registry.go`

**Context:** Replace the old loadout-based `ComputeDeveloperTools`/`ComputeUserTools` with new functions that iterate over `AppToggles` and check the corresponding config field. Also fix the three pre-existing inconsistencies (missing tools in registry/loadouts).

**Step 1: Fix pre-existing tool inconsistencies in `mcp_tools_registry.go`**

- Add `create_glossary_term` to `ToolRegistry` (it's in AllToolsOrdered but missing from registry)
- Add `update_columns` to `AllToolsOrdered` (it's in registry but missing from AllToolsOrdered — insert after `update_column`)
- Add `record_query_feedback` to `AllToolsOrdered` (it's in registry but missing — insert after `get_query_history`)

**Step 2: Rewrite `ComputeDeveloperTools` in `mcp_tool_loadouts.go`**

New logic:
1. Always start with `health` (LoadoutDefault)
2. Read the "tools" config from state (fall back to "developer" for backward compat)
3. For each developer-role AppToggle, check if the corresponding config field is true
4. Collect all enabled tool names into a set
5. Return tools in canonical `AllToolsOrdered` order

```go
func ComputeDeveloperTools(state map[string]*models.ToolGroupConfig) []ToolSpec {
    cfg := getToolsConfig(state)

    enabled := make(map[string]bool)
    // health always included
    for _, name := range Loadouts[LoadoutDefault] {
        enabled[name] = true
    }

    for _, toggle := range AppToggles {
        if toggle.Role != "developer" {
            continue
        }
        if isToggleEnabled(cfg, toggle.ToggleKey) {
            for _, toolName := range toggle.Tools {
                enabled[toolName] = true
            }
        }
    }

    return toolsInCanonicalOrder(enabled)
}
```

**Step 3: Rewrite `ComputeUserTools` in `mcp_tool_loadouts.go`**

Same pattern but for "user" role toggles:

```go
func ComputeUserTools(state map[string]*models.ToolGroupConfig) []ToolSpec {
    cfg := getToolsConfig(state)

    enabled := make(map[string]bool)
    for _, name := range Loadouts[LoadoutDefault] {
        enabled[name] = true
    }

    for _, toggle := range AppToggles {
        if toggle.Role != "user" {
            continue
        }
        if isToggleEnabled(cfg, toggle.ToggleKey) {
            for _, toolName := range toggle.Tools {
                enabled[toolName] = true
            }
        }
    }

    return toolsInCanonicalOrder(enabled)
}
```

**Step 4: Rewrite `ComputeEnabledToolsFromConfig`**

For non-agent, non-custom: merge developer + user tools (union). For agent: keep existing logic (LoadoutDefault + LoadoutLimitedQuery when enabled).

**Step 5: Add helper functions**

- `getToolsConfig(state)` — returns `state["tools"]` if present, else constructs from legacy "developer"/"user" keys for backward compat
- `isToggleEnabled(cfg, toggleKey)` — switch on toggleKey to read the corresponding bool field
- `toolsInCanonicalOrder(enabled map[string]bool)` — filter AllToolsOrdered by enabled set

**Step 6: Keep old Loadout constants and `MergeLoadouts` for backward compat** (agent tools still use them, and tests reference them). But `UILoadoutMapping` can be removed as it's unused.

**Step 7: Run `make check`** to verify all tests pass.

**Step 8: Commit** — `feat: rewrite tool computation to use per-app toggles`

---

## Task 3: Backend — Update API response with app_id per tool and appNames map

**Files:**
- Modify: `pkg/services/mcp_config.go` (EnabledToolInfo, MCPConfigResponse, buildResponse)
- Modify: `pkg/services/mcp_app_toggles.go` (add function to get app_id for a tool in a role)

**Context:** The API response needs to include which app controls each tool (per role) so the frontend can group tools by app in the inventory.

**Step 1: Add `AppID` field to `EnabledToolInfo`**

```go
type EnabledToolInfo struct {
    Name        string `json:"name"`
    Description string `json:"description"`
    AppID       string `json:"appId"`
}
```

**Step 2: Add `AppNames` to `MCPConfigResponse`**

```go
type MCPConfigResponse struct {
    ServerURL      string                             `json:"serverUrl"`
    ToolGroups     map[string]*models.ToolGroupConfig `json:"toolGroups"`
    UserTools      []EnabledToolInfo                  `json:"userTools"`
    DeveloperTools []EnabledToolInfo                  `json:"developerTools"`
    AgentTools     []EnabledToolInfo                  `json:"agentTools"`
    EnabledTools   []EnabledToolInfo                  `json:"enabledTools"`   // Deprecated
    AppNames       map[string]string                  `json:"appNames"`
}
```

**Step 3: Add `GetToolAppID(toolName, role)` to `mcp_app_toggles.go`**

Returns the app_id for the toggle that controls a given tool in a given role. For `health`, returns `"mcp-server"`.

**Step 4: Update `buildResponse` to populate `AppID` and `AppNames`**

When converting tool specs to EnabledToolInfo, look up the app_id. Always include `AppNames` in the response using `AppDisplayNames`.

**Step 5: Update `filterAndConvertToolSpecs` signature**

Add `role` parameter so it can call `GetToolAppID(tool.Name, role)` for each tool.

**Step 6: Replace data liaison filtering with app-installation-based filtering**

Instead of the hardcoded `DataLiaisonTools` map, check if each tool's owning app is installed (for apps that require installation). MCP Server tools are always available. Ontology Forge and AI Data Liaison tools require the respective app to be installed. This replaces the existing `dataLiaisonInstalled` check with a more general pattern.

For now, keep the existing filtering approach but extend it: check both Ontology Forge and AI Data Liaison installation status, and filter tools whose app requires installation.

**Step 7: Run `make check`**.

**Step 8: Commit** — `feat: add appId per tool and appNames map to API response`

---

## Task 4: Backend — Update MCP config update endpoint for new toggle fields

**Files:**
- Modify: `pkg/services/mcp_config.go` (UpdateMCPConfigRequest, Update method)
- Modify: `pkg/services/mcp_state.go` (applyUpdate, deepCopy, normalizeState)

**Context:** The update endpoint needs to accept the new toggle fields and persist them.

**Step 1: Add new fields to `UpdateMCPConfigRequest`**

```go
type UpdateMCPConfigRequest struct {
    // New per-app toggles
    AddDirectDatabaseAccess     *bool `json:"addDirectDatabaseAccess,omitempty"`
    AddOntologyMaintenanceTools *bool `json:"addOntologyMaintenanceTools,omitempty"`
    AddOntologySuggestions      *bool `json:"addOntologySuggestions,omitempty"`
    AddApprovalTools            *bool `json:"addApprovalTools,omitempty"`
    AddRequestTools             *bool `json:"addRequestTools,omitempty"`

    // Legacy fields (kept for backward compat)
    AllowOntologyMaintenance *bool `json:"allowOntologyMaintenance,omitempty"`
    AddQueryTools            *bool `json:"addQueryTools,omitempty"`
    AddOntologyMaintenance   *bool `json:"addOntologyMaintenance,omitempty"`
}
```

**Step 2: Update the `Update` method to handle new fields**

When any new toggle field is present, update the "tools" key in ToolGroups. Backward compat: old fields still update the legacy "developer"/"user" keys.

**Step 3: Update `applyUpdate` in `mcp_state.go` to copy new fields**

**Step 4: Update `deepCopy` in `mcp_state.go` to include new fields**

**Step 5: Update `validToolGroups` to include `"tools"`**

**Step 6: Run `make check`**.

**Step 7: Commit** — `feat: update config endpoint to accept per-app toggle fields`

---

## Task 5: Backend — Update MCP tool filter

**Files:**
- Modify: `pkg/mcp/tools/developer.go`

**Context:** The MCP tool filter in `NewToolFilter` must use the new computation. It already calls `services.GetEnabledTools(state)` which delegates to `ComputeEnabledToolsFromConfig`. Since Task 2 rewrites that function, the filter should work automatically. But the app-installation filtering needs to be updated to match Task 3's approach.

**Step 1: Update app installation filtering**

Replace the hardcoded `DataLiaisonTools` check with per-app installation checks matching the buildResponse logic from Task 3. Check both Ontology Forge and AI Data Liaison installation, remove tools from uninstalled apps.

**Step 2: Run `make check`**.

**Step 3: Commit** — `feat: update MCP tool filter for per-app installation checks`

---

## Task 6: Backend — Write tests for new tool computation

**Files:**
- Modify: `pkg/services/mcp_tool_loadouts_test.go`
- Create: `pkg/services/mcp_app_toggles_test.go`

**Context:** Test the new per-app toggle computation thoroughly.

**Step 1: Test `mcp_app_toggles.go`**

- Test that all 49 tools are accounted for (no tool left behind)
- Test `GetToolAppID` returns correct app for each role
- Test that `AppToggles` tool lists don't have duplicates within a role
- Test that every tool in AllToolsOrdered appears in exactly one toggle per role it belongs to (except `query` which appears in 2)

**Step 2: Test new `ComputeDeveloperTools`**

- All toggles on: should return health + all developer tools (echo, execute, query, all ontology tools, all approval tools)
- Only MCP Server toggle on: health + echo + execute + query
- Only Ontology Forge toggle on: health + 26 ontology tools
- Only AI Data Liaison toggle on: health + 7 approval tools
- All toggles off: health only

**Step 3: Test new `ComputeUserTools`**

- All toggles on: health + ontology reading tools + request tools
- Only Ontology Forge toggle on: health + get_context, get_ontology, list_glossary, get_glossary_sql
- Only AI Data Liaison toggle on: health + 9 request tools
- All toggles off: health only

**Step 4: Test backward compatibility**

- Config with only legacy "developer" key (no "tools" key) should still compute correctly by mapping old fields to new toggles

**Step 5: Run `make check`**.

**Step 6: Commit** — `test: add tests for per-app toggle computation`

---

## Task 7: Frontend — Update types and constants

**Files:**
- Modify: `ui/src/types/mcp.ts`
- Modify: `ui/src/constants/mcpToolMetadata.tsx`

**Context:** Update frontend types to match new API response.

**Step 1: Update `EnabledToolInfo` in `ui/src/types/mcp.ts`**

```typescript
export interface EnabledToolInfo {
  name: string;
  description: string;
  appId: string;
}
```

**Step 2: Update `MCPConfigResponse` in `ui/src/types/mcp.ts`**

Add `appNames`:

```typescript
export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: Record<string, ToolGroupState>;
  userTools: EnabledToolInfo[];
  developerTools: EnabledToolInfo[];
  agentTools: EnabledToolInfo[];
  enabledTools: EnabledToolInfo[];
  appNames: Record<string, string>;
}
```

**Step 3: Update `ToolGroupState` in `ui/src/types/mcp.ts`**

Add new toggle fields:

```typescript
export interface ToolGroupState {
  // New per-app toggles
  addDirectDatabaseAccess?: boolean;
  addOntologyMaintenanceTools?: boolean;
  addOntologySuggestions?: boolean;
  addApprovalTools?: boolean;
  addRequestTools?: boolean;

  // Legacy fields (backward compat)
  allowOntologyMaintenance?: boolean;
  addQueryTools?: boolean;
  addOntologyMaintenance?: boolean;
  // ... existing legacy fields
}
```

**Step 4: Update `UpdateMCPConfigRequest` in `ui/src/types/mcp.ts`**

Add new toggle fields.

**Step 5: Update `TOOL_GROUP_IDS` and `TOOL_GROUP_SUB_OPTIONS` in `mcpToolMetadata.tsx`**

Add `TOOLS: 'tools'` to TOOL_GROUP_IDS. Add new sub-option keys.

**Step 6: Run `make check`**.

**Step 7: Commit** — `feat: update frontend types for per-app tool loadouts`

---

## Task 8: Frontend — Create read-only tool inventory component

**Files:**
- Create: `ui/src/components/mcp/ToolInventory.tsx`

**Context:** A new component for the MCP Server page that displays all enabled tools grouped by role (Developer/User), then by app within each role. Each app group has a "Configure" link to that app's page. Uses collapsible sections matching existing UI patterns.

**Step 1: Create `ToolInventory` component**

Props:
```typescript
interface ToolInventoryProps {
  developerTools: EnabledToolInfo[];
  userTools: EnabledToolInfo[];
  appNames: Record<string, string>;
  projectId: string;
}
```

Behavior:
- Two collapsible sections: "Developer Tools (N)" and "User Tools (N)" where N is the count of enabled tools
- Within each section, group tools by `appId` using the order: mcp-server, ontology-forge, ai-data-liaison
- Each app group shows: app display name (from `appNames`) + "Configure" link
- Configure link maps: `mcp-server` → `/projects/{pid}/mcp-server`, `ontology-forge` → `/projects/{pid}/ontology-forge`, `ai-data-liaison` → `/projects/{pid}/ai-data-liaison`
- Tools within each group are listed in the order they appear in the API response (canonical order)
- If a section has 0 tools, show "No tools enabled" with guidance
- If a section has only `health`, show it but no app grouping needed (health is always present)

**Step 2: Run `make check`**.

**Step 3: Commit** — `feat: create ToolInventory component for read-only tool display`

---

## Task 9: Frontend — Update MCP Server page

**Files:**
- Modify: `ui/src/pages/MCPServerPage.tsx`
- Modify: `ui/src/pages/__tests__/MCPServerPage.test.tsx`

**Context:** Replace DeveloperToolsSection with the read-only ToolInventory component. Add the "Direct Database Access" toggle as MCP Server's own developer tool config. Keep Deployment checklist, URL section, and existing functionality.

**Step 1: Add "Direct Database Access" toggle**

Add a card section for MCP Server's tool configuration:
- Title: "Tool Configuration"
- One toggle: "Direct Database Access" — controls echo, execute, query for developers
- When toggled, calls `updateMCPConfig` with `{ addDirectDatabaseAccess: value }`
- Read state from `config.toolGroups['tools']?.addDirectDatabaseAccess`

**Step 2: Replace DeveloperToolsSection with ToolInventory**

Remove the DeveloperToolsSection import and usage. Add ToolInventory showing the read-only view of all tools across all apps, grouped by role then by app.

**Step 3: Update tests**

Update MCPServerPage tests to reflect the new UI: "Direct Database Access" toggle + ToolInventory instead of DeveloperToolsSection.

**Step 4: Run `make check`**.

**Step 5: Commit** — `feat: update MCP Server page with tool inventory and direct DB toggle`

---

## Task 10: Frontend — Update Ontology Forge page

**Files:**
- Modify: `ui/src/pages/OntologyForgePage.tsx`
- Modify: `ui/src/pages/__tests__/OntologyForgePage.test.tsx`

**Context:** Replace the DeveloperToolsSection with Ontology Forge's own tool config section with two toggles: Developer > "Add Ontology Maintenance Tools" and User > "Add Ontology Suggestions [RECOMMENDED]".

**Step 1: Replace DeveloperToolsSection**

Remove DeveloperToolsSection import. Create a new "Tool Configuration" card with:
- Developer section: "Add Ontology Maintenance Tools" toggle
  - When toggled: `updateMCPConfig({ addOntologyMaintenanceTools: value })`
  - Read from: `config.toolGroups['tools']?.addOntologyMaintenanceTools`
  - Show MCPEnabledTools with developer tools filtered to ontology-forge appId
- User section: "Add Ontology Suggestions" toggle (with RECOMMENDED badge)
  - When toggled: `updateMCPConfig({ addOntologySuggestions: value })`
  - Read from: `config.toolGroups['tools']?.addOntologySuggestions`
  - Show MCPEnabledTools with user tools filtered to ontology-forge appId

**Step 2: Remove old toggle handlers**

Remove `handleAddQueryToolsChange` and `handleAddOntologyMaintenanceChange`. Add new handlers for the two new toggles.

**Step 3: Update tests**

Update OntologyForgePage tests: remove DeveloperToolsSection mocking, add tests for new toggles.

**Step 4: Run `make check`**.

**Step 5: Commit** — `feat: add Ontology Forge tool configuration with two toggles`

---

## Task 11: Frontend — Update AI Data Liaison page

**Files:**
- Modify: `ui/src/pages/AIDataLiaisonPage.tsx`
- Modify: `ui/src/pages/__tests__/AIDataLiaisonPage.test.tsx`

**Context:** Replace the UserToolsSection and static developer tools list with AI Data Liaison's own tool config section with two toggles.

**Step 1: Replace UserToolsSection and static developer tools card**

Remove UserToolsSection import. Remove `DATA_LIAISON_DEVELOPER_TOOLS` constant and its static card. Create a new "Tool Configuration" card with:
- Developer section: "Add Approval Tools" toggle
  - When toggled: `updateMCPConfig({ addApprovalTools: value })`
  - Read from: `config.toolGroups['tools']?.addApprovalTools`
  - Show MCPEnabledTools with developer tools filtered to ai-data-liaison appId
- User section: "Add Request Tools" toggle (with RECOMMENDED badge)
  - When toggled: `updateMCPConfig({ addRequestTools: value })`
  - Read from: `config.toolGroups['tools']?.addRequestTools`
  - Show MCPEnabledTools with user tools filtered to ai-data-liaison appId

**Step 2: Remove old toggle handler**

Remove `handleAllowOntologyMaintenanceChange`. Add new handlers for the two new toggles.

**Step 3: Update tests**

Update AIDataLiaisonPage tests.

**Step 4: Run `make check`**.

**Step 5: Commit** — `feat: add AI Data Liaison tool configuration with two toggles`

---

## Task 12: Cleanup — Remove dead code

**Files:**
- Modify: `ui/src/components/mcp/DeveloperToolsSection.tsx` — delete file if no longer imported
- Modify: `ui/src/components/mcp/UserToolsSection.tsx` — delete file if no longer imported
- Modify: `pkg/services/mcp_tools_registry.go` — remove `DataLiaisonTools` map if replaced by app-based filtering
- Modify: `pkg/services/mcp_tool_loadouts.go` — remove `UILoadoutMapping` (unused)

**Step 1: Check for any remaining imports of DeveloperToolsSection and UserToolsSection**

If no other files import them, delete both component files.

**Step 2: Remove `UILoadoutMapping`** from mcp_tool_loadouts.go (it was only used for documentation, not code).

**Step 3: Remove `DataLiaisonTools`** if fully replaced by per-app installation checks.

**Step 4: Run `make check`** to verify nothing broke.

**Step 5: Commit** — `chore: remove dead code from old tool loadout system`

---

## Task 13: Final verification

**Step 1: Run `make check`** — full lint + test + build.

**Step 2: Manual verification checklist** (for the user):
- MCP Server page: shows Direct Database Access toggle + read-only tool inventory
- Ontology Forge page: shows two toggles (Ontology Maintenance + Ontology Suggestions)
- AI Data Liaison page: shows two toggles (Approval Tools + Request Tools)
- Toggling any toggle updates the MCP Server inventory immediately
- MCP tool filter serves the same tools the UI shows
