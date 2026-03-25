# FIX: Approved query core tools should belong to Ontology Forge

**Status:** Open
**Date:** 2026-03-25

## Problem

The core approved-query tools currently require the AI Data Liaison app even though the product already treats pre-approved queries as part of the Ontology Forge surface.

Current backend ownership in `pkg/services/mcp_app_toggles.go` is split like this:

- AI Data Liaison developer toggle owns `create_approved_query`, `update_approved_query`, `delete_approved_query`
- AI Data Liaison user toggle owns `list_approved_queries`, `execute_approved_query`

That ownership flows through the rest of the stack:

- `pkg/services/mcp_config.go` assigns `appId` per tool from `GetToolAppID(...)`
- `pkg/mcp/tools/developer.go` hides tools when their owning app is not installed
- `pkg/mcp/tools/access.go` returns `app_not_installed` for execution when the owning app is missing
- `ui/src/pages/OntologyForgePage.tsx` and `ui/src/pages/AIDataLiaisonPage.tsx` filter enabled tools by `appId`

So a project with Ontology Forge installed but without AI Data Liaison cannot use the core approved-query catalog and execution tools, even though the product UI already frames those capabilities as Ontology Forge:

- `ui/src/pages/ProjectDashboard.tsx` shows "Pre-Approved Queries" only when Ontology Forge is installed
- `ui/src/pages/OntologyForgePage.tsx` includes "Create Pre-Approved Queries" in the Ontology Forge checklist
- the query management screen lives at `/projects/:pid/queries`, not under an AI Data Liaison-specific route

This makes AI Data Liaison an accidental prerequisite for a core Ontology Forge workflow.

## Scope

Move only the core approved-query catalog/execution tools into Ontology Forge.

Target ownership:

- Ontology Forge developer path:
  - `create_approved_query`
  - `update_approved_query`
  - `delete_approved_query`
  - `list_approved_queries`
  - `execute_approved_query`
- Ontology Forge user path:
  - `list_approved_queries`
  - `execute_approved_query`

Keep these AI Data Liaison-only:

- `list_query_suggestions`
- `approve_query_suggestion`
- `reject_query_suggestion`
- `suggest_approved_query`
- `suggest_query_update`
- `get_query_history`
- `record_query_feedback`

Keep this narrow:

- do not add new toggle fields
- do not add a new app or a new page
- do not change the agent limited-query loadout
- do not keep duplicate ownership in both apps for the moved core tools

The ownership change should be a clean replacement, not a shared/temporary dual-registration.

## Important implementation detail

The user request distinguishes admin/data from user access:

- admin/data should get create/update/delete/list/execute
- user should get list/execute

In this codebase, admin/data access comes from the developer path plus any enabled user path:

- `pkg/mcp/tools/access.go` uses `effectiveRole(...)`
- admin/data roles resolve to `ComputeEnabledToolsFromConfig(state, false)`
- user role resolves to `ComputeUserTools(state)`

Because of that, `list_approved_queries` and `execute_approved_query` must exist in both Ontology Forge toggles:

- Ontology Forge developer toggle
- Ontology Forge user toggle

Do not rely on the Ontology Forge user toggle being enabled in order for admin/data callers to get list/execute.

## Existing implementation context

### Backend ownership and filtering

- `pkg/services/mcp_app_toggles.go`
  - current source of truth for app/toggle/tool ownership
- `pkg/services/mcp_tool_loadouts.go`
  - canonical tool order and loadout metadata
- `pkg/services/mcp_config.go`
  - builds the per-role enabled tool response and filters by installed app
- `pkg/mcp/tools/developer.go`
  - applies the same ownership/install filtering to `tools/list`
- `pkg/mcp/tools/access.go`
  - enforces the same ownership/install rules during tool execution

### Frontend surfaces affected by `appId`

- `ui/src/pages/OntologyForgePage.tsx`
  - shows `config.developerTools.filter(t => t.appId === 'ontology-forge')`
  - shows `config.userTools.filter(t => t.appId === 'ontology-forge')`
- `ui/src/pages/AIDataLiaisonPage.tsx`
  - shows the equivalent `ai-data-liaison`-filtered tool lists

Once backend ownership changes, the tool rows shown on those pages move automatically with no extra routing work.

## Intended fix

### 1. Reassign the core tools in `pkg/services/mcp_app_toggles.go`

Update the toggle registry so that:

- Ontology Forge developer `addOntologyMaintenanceTools` includes:
  - existing ontology-maintenance tools
  - `create_approved_query`
  - `update_approved_query`
  - `delete_approved_query`
  - `list_approved_queries`
  - `execute_approved_query`
- Ontology Forge user `addOntologySuggestions` includes:
  - existing ontology/context user tools
  - `list_approved_queries`
  - `execute_approved_query`
- AI Data Liaison developer `addApprovalTools` removes:
  - `create_approved_query`
  - `update_approved_query`
  - `delete_approved_query`
- AI Data Liaison user `addRequestTools` removes:
  - `list_approved_queries`
  - `execute_approved_query`

The move should make Ontology Forge the sole owner for those five tools.

### 2. Keep canonical loadout metadata aligned

Inspect `pkg/services/mcp_tool_loadouts.go` and update any loadout membership or comments that still describe the moved tools as AI Data Liaison-owned.

At minimum:

- keep `LoadoutLimitedQuery` unchanged for agents
- keep duplicate membership only where it is intentional for role semantics
- if `LoadoutOntologyMaintenance` or related comments are used to describe the Ontology Forge developer experience, include the moved core approved-query tools there as well

The goal is to avoid another split-brain situation where `AppToggles` says one thing and the conceptual loadout metadata says another.

### 3. Update page copy where ownership is described

The page plumbing already filters by `appId`, but text still implies AI Data Liaison owns approved-query management.

Expected follow-up in the two application pages:

- `ui/src/pages/OntologyForgePage.tsx`
  - developer copy should mention approved-query catalog management
  - user copy should mention access to reusable approved queries
- `ui/src/pages/AIDataLiaisonPage.tsx`
  - approval-tools copy should focus on suggestion review workflow, glossary management, and `explain_query`
  - it should no longer claim AI Data Liaison manages the approved-query catalog directly

No dashboard navigation change is needed:

- "Pre-Approved Queries" already belongs to Ontology Forge
- "Pending Queries" remains under AI Data Liaison

## RED-first test plan

Start with failing tests before changing the registry.

### Backend ownership/access tests to add or update first

#### `pkg/services/mcp_tool_loadouts_test.go`

- [ ] Ontology Forge developer toggle alone exposes:
  - `create_approved_query`
  - `update_approved_query`
  - `delete_approved_query`
  - `list_approved_queries`
  - `execute_approved_query`
- [ ] Ontology Forge user toggle alone exposes:
  - `list_approved_queries`
  - `execute_approved_query`
- [ ] AI Data Liaison developer toggle no longer exposes core approved-query CRUD
- [ ] AI Data Liaison user toggle no longer exposes `list_approved_queries` or `execute_approved_query`

#### `pkg/services/mcp_config_test.go`

- [ ] With only Ontology Forge installed, `Get(...)` returns the five core approved-query tools under Ontology Forge `appId`
- [ ] With only AI Data Liaison installed, those five tools are absent
- [ ] Suggestion/review/history tools still remain under AI Data Liaison

#### `pkg/services/tool_access_test.go`

- [ ] Non-agent access with only `addOntologyMaintenanceTools=true` includes create/update/delete/list/execute
- [ ] Non-agent access with only `addOntologySuggestions=true` includes list/execute

#### `pkg/mcp/tools/tool_access_consistency_test.go`

- [ ] User listing/calling `list_approved_queries` succeeds with Ontology Forge installed and AI Data Liaison absent
- [ ] Admin/data calling `create_approved_query` succeeds with Ontology Forge installed and AI Data Liaison absent
- [ ] Missing Ontology Forge produces `app_not_installed` for the moved tools

#### `pkg/mcp/tools/developer_filter_test.go`

- [ ] Replace expectations that core approved-query tools are hidden when AI Data Liaison is missing
- [ ] Add expectations that they are visible when Ontology Forge is installed
- [ ] Keep expectations that suggestion/review/history tools remain hidden without AI Data Liaison

#### New direct ownership test recommended

Add `pkg/services/mcp_app_toggles_test.go` to pin the registry directly:

- [ ] `GetToolAppID("create_approved_query", "developer") == ontology-forge`
- [ ] `GetToolAppID("list_approved_queries", "developer") == ontology-forge`
- [ ] `GetToolAppID("list_approved_queries", "user") == ontology-forge`
- [ ] `GetToolAppID("execute_approved_query", "user") == ontology-forge`
- [ ] AI Data Liaison-only tools still resolve to `ai-data-liaison`

### Frontend tests to add after the backend move

#### `ui/src/pages/__tests__/OntologyForgePage.test.tsx`

- [ ] Verify Ontology Forge copy mentions approved-query management/access
- [ ] Provide mock `developerTools`/`userTools` with `appId: 'ontology-forge'` and assert the moved tool rows appear under Ontology Forge

#### `ui/src/pages/__tests__/AIDataLiaisonPage.test.tsx`

- [ ] Update copy assertions so AI Data Liaison no longer claims direct approved-query catalog management
- [ ] If tool rows are asserted, ensure only suggestion/review/history tools remain under `ai-data-liaison`

### Suggested execution order

1. Add/update the backend tests above and confirm RED.
2. Move ownership in `pkg/services/mcp_app_toggles.go` and any supporting loadout metadata.
3. Update frontend copy/tests.
4. Run focused packages first.
5. Run `make check` before calling the implementation complete.

Useful focused commands:

```bash
go test ./pkg/services ./pkg/mcp/tools -count=1
cd ui && npm test -- --run src/pages/__tests__/OntologyForgePage.test.tsx src/pages/__tests__/AIDataLiaisonPage.test.tsx
make check
```

## Non-goals

- [ ] Do not move `suggest_approved_query`, `suggest_query_update`, `get_query_history`, or `record_query_feedback` into Ontology Forge in this fix
- [ ] Do not move glossary tools/routes in this fix
- [ ] Do not add a new "approved queries" toggle just for Ontology Forge
- [ ] Do not change agent authentication behavior or `LoadoutLimitedQuery`
- [ ] Do not preserve dual ownership of the moved tools across both apps

## Expected outcome

After this fix:

- Ontology Forge alone is enough for the core approved-query catalog/execution workflow
- AI Data Liaison remains optional and focused on suggestion/review/collaboration workflows
- the MCP config API, MCP tool filter, execution-time access checks, and application pages all agree on the same ownership model

## Planning note

This fix supersedes the core approved-query ownership described in `plans/PLAN-per-app-tool-loadouts.md`.
