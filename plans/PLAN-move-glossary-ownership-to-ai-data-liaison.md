# PLAN: Move Glossary Ownership to AI Data Liaison

**Status:** Proposed
**Date:** 2026-03-16

## Goal

Make Glossary an **AI Data Liaison** feature at the product and MCP-tool level while preserving the existing ontology dependency for glossary generation.

This change is not a data-model migration. It is an ownership and UX refactor across:

- MCP tool ownership and app-install enforcement
- App configuration UI and copy
- Dashboard/app navigation
- Tests that encode the current Ontology Forge ownership

## Key Files To Open First In A Fresh Session

### Backend

- `pkg/services/mcp_app_toggles.go`
  - current per-app tool ownership registry
- `pkg/mcp/tools/access.go`
  - execution-time app-install enforcement
- `pkg/services/mcp_config.go`
  - UI/API tool inventory response
- `pkg/mcp/tools/developer.go`
  - MCP tool filtering with installed-app awareness
- `pkg/mcp/tools/context.go`
  - `get_context` still includes glossary output
- `pkg/handlers/glossary_handler.go`
  - standalone glossary REST routes

### Frontend

- `ui/src/pages/AIDataLiaisonPage.tsx`
  - setup checklist, tool configuration copy, uninstall copy
- `ui/src/pages/ProjectDashboard.tsx`
  - current dashboard tile ownership
- `ui/src/pages/GlossaryPage.tsx`
  - existing ontology/readiness behavior
- `ui/src/pages/ApplicationsPage.tsx`
  - AI Data Liaison marketing/install copy and Ontology Forge dependency
- `ui/src/components/mcp/ToolInventory.tsx`
  - tool grouping by `appId`
- `ui/src/services/engineApi.ts`
  - glossary list endpoint used by the UI

### Tests

- `pkg/services/mcp_config_test.go`
- `pkg/services/mcp_tool_loadouts_test.go`
- `pkg/services/tool_access_test.go`
- `pkg/mcp/tools/tool_access_consistency_test.go`
- `pkg/mcp/tools/glossary_test.go`
- `pkg/mcp/tools/glossary_integration_test.go`
- `pkg/mcp/tools/developer_filter_test.go`
- `ui/src/pages/__tests__/AIDataLiaisonPage.test.tsx`
- `ui/src/pages/__tests__/ApplicationsPage.test.tsx`
- `ui/src/pages/__tests__/GlossaryPage.test.tsx`
- `ui/src/pages/__tests__/OntologyForgePage.test.tsx`

## Product Decisions Already Made

These decisions were explicitly confirmed and should be treated as fixed for implementation:

1. Glossary is now an **AI Data Liaison** feature.
2. Glossary MCP tools should move to **AI Data Liaison**.
3. Glossary generation should still depend on **Ontology Forge** readiness.
4. This is acceptable because **AI Data Liaison already requires Ontology Forge**.
5. Do **not** add new hard route gating for `/glossary` in this change.
6. `/glossary` may remain directly accessible even when prerequisites are not met.
7. It is acceptable for the page to show existing readiness messaging such as:
   - extract ontology first
   - answer required ontology questions first
8. In **AI Data Liaison > Setup Checklist**, insert a new step:
   - `2. Glossary set up`
   - this should appear before activation

## Non-Goals

- No glossary storage migration
- No new page-level auth or app-install enforcement
- No change to the ontology extraction pipeline itself
- No attempt to fully disentangle glossary data from ontology-backed context tools such as `get_context`

## Why This Does Not Need a Storage Migration

Glossary is already project-scoped, not ontology-version-scoped:

- `engine_business_glossary` originally had `ontology_id`
- migration `011_remove_ontologies.up.sql` removed `ontology_id`
- uniqueness is now `(project_id, term)`

Relevant files:

- `migrations/011_remove_ontologies.up.sql`
- `pkg/repositories/glossary_repository.go`
- `pkg/handlers/glossary_handler.go`

This means the move is about ownership, configuration, and presentation, not persistence.

## Current State Summary

### 1. Glossary page and REST routes are standalone

- UI route: `ui/src/App.tsx`
- REST routes: `pkg/handlers/glossary_handler.go`

There is no current installed-app gate on `/glossary` or `/api/projects/{pid}/glossary`.

### 2. MCP glossary tool ownership currently points to Ontology Forge

Current mapping in `pkg/services/mcp_app_toggles.go`:

- Ontology Forge user tools:
  - `list_glossary`
  - `get_glossary_sql`
- Ontology Forge developer tools:
  - `create_glossary_term`
  - `update_glossary_term`
  - `delete_glossary_term`

### 3. AI Data Liaison currently only owns query workflow tools

Current mapping in `pkg/services/mcp_app_toggles.go`:

- AI Data Liaison developer:
  - query approval and approved-query management tools
- AI Data Liaison user:
  - request/query execution tools

### 4. App-install enforcement is duplicated

The MCP config/UI path derives tool ownership from the app-toggle registry:

- `pkg/services/mcp_app_toggles.go`
- `pkg/services/mcp_config.go`
- `pkg/mcp/tools/developer.go`

But execution-time access has an additional hard-coded check:

- `pkg/mcp/tools/access.go`
- `pkg/services/mcp_tools_registry.go` (`DataLiaisonTools`)

That duplication is the main technical risk in this change. If only the toggle registry is updated, UI inventory and actual tool execution can drift.

### 5. Dashboard/product ownership still points to Ontology Forge

- `ui/src/pages/ProjectDashboard.tsx` shows Glossary in the purple Ontology Forge intelligence cluster
- `ui/src/pages/AIDataLiaisonPage.tsx` does not surface Glossary as an owned feature
- `ui/src/pages/ApplicationsPage.tsx` still describes AI Data Liaison primarily as query workflow/business-user liaison

Additionally, the current AI Data Liaison setup checklist only has:

1. Ontology Forge set up
2. Activate AI Data Liaison

That checklist needs a new intermediate Glossary step.

### 6. Glossary generation is already ontology-gated

The existing Glossary page behavior already matches the desired dependency model:

- checks pending required ontology questions
- routes users to `/ontology` and `/ontology-questions`
- auto-generation depends on ontology-derived context

Relevant files:

- `ui/src/pages/GlossaryPage.tsx`
- `pkg/handlers/glossary_handler.go`
- `pkg/services/glossary_service.go`

### 7. Current AI Data Liaison checklist implementation details

The current `AIDataLiaisonPage` implementation is important because the new checklist step must fit into its existing structure rather than being invented from scratch:

- `fetchChecklistData()` currently loads:
  - `engineApi.getMCPConfig(pid)`
  - `engineApi.getInstalledApp(pid, "ai-data-liaison")`
  - `engineApi.getInstalledApp(pid, "ontology-forge")`
- the page stores:
  - `ontologyForgeReady`
  - `installedApp`
  - `mcpConfig`
- `getChecklistItems()` currently returns only:
  - `Ontology Forge set up`
  - `Activate AI Data Liaison`

The new Glossary step should be inserted into this existing pattern rather than creating a separate checklist component or a different data-loading path.

### 8. Current glossary data source for UI

For the new `Glossary set up` checklist step, use the existing glossary list endpoint rather than inventing a new status endpoint.

Current client method:

- `engineApi.listGlossaryTerms(projectId)`

Current response shape already contains enough information:

- `terms`
- `total`
- `generation_status`

The simplest completion rule for the checklist step is:

- complete when `terms.length > 0`
- otherwise pending

This is sufficient for the current requirement and avoids expanding the API surface.

## Implementation Decisions For This Change

### Decision 1: Reuse existing AI Data Liaison toggles

Do not introduce new MCP config fields or a separate glossary toggle in this change.

Instead:

- Move glossary read tools into AI Data Liaison's existing **user** toggle
- Move glossary write tools into AI Data Liaison's existing **developer** toggle
- Update AI Data Liaison page copy so the toggle descriptions mention glossary management/access

This keeps the config model smaller and avoids a larger migration of MCP config semantics.

Implementation note:

- this means no new `ToolGroupConfig` fields should be introduced just for glossary ownership
- the change should reuse the existing AI Data Liaison toggles and update their descriptions/copy accordingly

### Decision 2: Keep `get_context` owned by Ontology Forge for now

`get_context` always includes glossary data in its response and is currently owned by Ontology Forge.

This plan does **not** change that contract.

Rationale:

- `get_context` is not a glossary-specific tool
- changing its payload or ownership is a broader MCP contract change
- AI Data Liaison already depends on Ontology Forge

Known limitation after this change:

- a project with only Ontology Forge can still receive glossary content indirectly through `get_context`

Treat that as acceptable for this iteration unless product requirements later require strict glossary entitlement separation.

### Decision 3: Keep `/glossary` directly reachable

Do not add app-install route guards.

If the page is reachable without the prerequisites, existing empty-state and readiness messaging is sufficient.

## Tool Ownership After This Change

### Ontology Forge

Developer toggle should continue to own:

- schema exploration and ontology maintenance tools
- ontology question tools
- pending change review tools

Developer toggle should no longer own:

- `create_glossary_term`
- `update_glossary_term`
- `delete_glossary_term`

User toggle should continue to own:

- `get_context`
- `get_ontology`

User toggle should no longer own:

- `list_glossary`
- `get_glossary_sql`

### AI Data Liaison

Developer toggle (`Add Approval Tools`) should own its current query-approval tools plus:

- `create_glossary_term`
- `update_glossary_term`
- `delete_glossary_term`

User toggle (`Add Request Tools`) should own its current request/query tools plus:

- `list_glossary`
- `get_glossary_sql`

## Recommended Task Breakdown

## Task 1: Reassign glossary MCP tool ownership to AI Data Liaison

**Files:**

- `pkg/services/mcp_app_toggles.go`

**Changes:**

1. Remove glossary read tools from the Ontology Forge user toggle.
2. Remove glossary write tools from the Ontology Forge developer toggle.
3. Add glossary read tools to the AI Data Liaison user toggle.
4. Add glossary write tools to the AI Data Liaison developer toggle.

**Expected result:**

- `MCPConfigResponse.userTools` and `developerTools` report glossary tools with `appId: "ai-data-liaison"`
- MCP tool inventory groups glossary under AI Data Liaison instead of Ontology Forge

## Task 2: Replace hard-coded app-install enforcement with ownership-derived enforcement

**Files:**

- `pkg/mcp/tools/access.go`
- `pkg/services/mcp_tools_registry.go`
- possibly `pkg/services/mcp_app_toggles.go` for new helper(s)

**Problem to solve:**

`checkAppInstallation(...)` currently hard-codes AI Data Liaison app gating through `services.DataLiaisonTools`.

That works for the current query-suggestion tools but is the wrong abstraction for this refactor.

**Recommended approach:**

Add a helper that derives the owning app from the same app-toggle registry used by the UI.

Suggested helper shape:

```go
func GetToolOwningAppIDs(toolName string) []string
```

Behavior:

- scan all `AppToggles`
- collect app IDs for any toggle that contains the tool
- ignore `mcp-server` if the tool is always available there
- return de-duplicated app IDs

Then update `checkAppInstallation(...)` to:

1. keep the special `ai-agents` handling for agent auth
2. for non-agent auth, derive required app IDs from the registry
3. enforce installation for each owning app

This is safer than using role-based lookup in execution-time access because admin/data callers can execute tools that are modeled as "user" tools.

Fresh-session warning:

- do not derive app ownership in `checkAppInstallation(...)` from the caller's effective role
- admin/data users can execute tools that are owned by a "user" toggle
- scan the toggle registry directly for the requested tool instead

**Preferred cleanup:**

- stop relying on `DataLiaisonTools` for app-install enforcement
- either delete it or narrow its purpose so it no longer represents ownership

## Task 3: Update AI Data Liaison UI copy to include glossary

**Files:**

- `ui/src/pages/AIDataLiaisonPage.tsx`
- optionally `ui/src/pages/ApplicationsPage.tsx`

**Changes:**

1. Update the page description so AI Data Liaison is no longer framed only as query suggestion/approval.
2. Update tool-toggle descriptions:
   - developer toggle should mention managing glossary terms in addition to query approvals
   - user toggle should mention accessing glossary terms in addition to query/request tools
3. Update application card subtitle/copy if needed so demo/marketing language in the product matches the new ownership.
4. Update the AI Data Liaison setup checklist to:
   - `1. Ontology Forge set up`
   - `2. Glossary set up`
   - `3. Activate AI Data Liaison`

**AI Data Liaison page copy that must change:**

The current page copy is too query-centric. Update it so Glossary is explicitly part of the feature set in these areas:

- page description under the header
- `Add Approval Tools` description
- `Add Request Tools [RECOMMENDED]` description
- `Danger Zone` uninstall description

**Required copy intent:**

- `Add Approval Tools` should mention:
  - reviewing/managing query suggestions
  - managing glossary terms
- `Add Request Tools` should mention:
  - business users requesting data access / suggesting queries
  - business users accessing glossary terminology/definitions
- `Danger Zone` uninstall copy should mention:
  - disabling the query suggestion workflow
  - removing AI Data Liaison ownership/access to glossary functionality

**Concrete guidance for implementation:**

The exact wording can be refined during implementation, but it should be materially equivalent to:

- `Add Approval Tools`
  - `Include tools to review and manage query suggestions and glossary terms: approve, reject, manage approved queries, and maintain shared business terminology.`
- `Add Request Tools [RECOMMENDED]`
  - `Enable business users to suggest queries, request data access, and access glossary terms through the MCP Client.`
- `Danger Zone`
  - `Uninstalling AI Data Liaison will disable the query suggestion workflow and remove AI Data Liaison access to glossary functionality. Business users will no longer be able to suggest queries or access glossary terms through AI Data Liaison, and data engineers will lose access to suggestion and glossary management tools.`

**Recommended checklist behavior for the new Glossary step:**

- fetch glossary state in `AIDataLiaisonPage` via `engineApi.listGlossaryTerms(pid)`
- mark the step **complete** when at least one glossary term exists
- mark the step **pending** when no glossary terms exist yet
- link the step to `/projects/${pid}/glossary`

Implementation note:

- do not block rendering of the page on glossary fetch failure
- if glossary fetch fails, fail soft and treat the step as pending with a safe description rather than breaking the entire AI Data Liaison page

**Recommended copy for the new step:**

- complete:
  - `Glossary is configured and ready`
- pending when Ontology Forge is ready:
  - `Set up the business glossary for consistent business terminology`
- pending when Ontology Forge is not ready:
  - `Complete step 1 first`

**Optional but recommended:**

Add a simple card or CTA on the AI Data Liaison page linking to `/projects/:pid/glossary`.

This is not strictly required for correctness, but it helps the product ownership change become visible in the UI.

## Task 4: Move dashboard ownership from Ontology Forge to AI Data Liaison

**Files:**

- `ui/src/pages/ProjectDashboard.tsx`

**Changes:**

1. Stop showing the Glossary tile as an Ontology Forge-owned purple tile.
2. Show Glossary when AI Data Liaison is installed.
3. Use AI Data Liaison styling/color for the Glossary tile so ownership is visually consistent.
4. Keep the existing enable/disable requirements tied to datasource, selected tables, and AI config.

**Important:**

Do not remove the ontology dependency from the actual page behavior.
This task is about ownership and presentation, not changing prerequisites.

## Task 5: Leave page reachability alone, keep readiness messaging

**Files:**

- `ui/src/pages/GlossaryPage.tsx`

**Changes:**

Only make copy updates if necessary.

Keep the existing behavior that:

- allows visiting `/glossary`
- blocks generation until ontology prerequisites are met
- points the user to `/ontology` or `/ontology-questions` when needed

Do not add a new app-install route guard in this change.

## Task 6: Review context-tool behavior and document the accepted limitation in code comments if needed

**Files:**

- `pkg/mcp/tools/context.go`
- optionally `pkg/services/mcp_app_toggles.go`

**Why this matters:**

`get_context` always includes glossary output and remains owned by Ontology Forge.

This plan intentionally leaves that behavior in place. If the implementation feels ambiguous, add a short code comment explaining that Glossary page/tool ownership moved to AI Data Liaison, but `get_context` remains an ontology context tool for now.

This is optional unless the new code would otherwise be confusing in review.

## Tests To Update

### Backend

- `pkg/services/mcp_config_test.go`
  - update assertions about which installed app filters glossary tools
- `pkg/services/mcp_tool_loadouts_test.go`
  - update ownership/loadout expectations if glossary app IDs are asserted
- `pkg/services/tool_access_test.go`
  - ensure accessible-tool computation still matches new ownership
- `pkg/mcp/tools/tool_access_consistency_test.go`
  - verify list-vs-call consistency after ownership move
- `pkg/mcp/tools/glossary_test.go`
  - update any ownership assumptions in glossary tool tests
- `pkg/mcp/tools/glossary_integration_test.go`
  - update comments and any config/setup that assumes glossary write tools are under Ontology Forge
- `pkg/mcp/tools/developer_filter_test.go`
  - update expected visible tool groupings if app ownership is asserted

### Frontend

- `ui/src/pages/__tests__/AIDataLiaisonPage.test.tsx`
  - update copy expectations
  - update checklist numbering/order expectations
  - add coverage for the new Glossary checklist step
  - add/update assertions for the glossary-related copy in tool configuration and danger zone
  - add assertions for any new glossary CTA if added
- `ui/src/pages/__tests__/ApplicationsPage.test.tsx`
  - update app subtitle/copy expectations if touched
- `ui/src/pages/__tests__/GlossaryPage.test.tsx`
  - update copy assertions only if Glossary page text changes
- `ui/src/pages/__tests__/OntologyForgePage.test.tsx`
  - update tool-copy expectations if glossary is removed from page descriptions

There is currently no dedicated `ProjectDashboard` page test in `ui/src/pages/__tests__`.
If dashboard behavior is changed materially, add one rather than leaving the ownership move untested.

## Suggested Implementation Order

1. Reassign tool ownership in `mcp_app_toggles.go`
2. Replace execution-time hard-coded app gating with registry-derived gating
3. Update backend tests first so the MCP contract is stable
4. Update AI Data Liaison checklist logic and copy
5. Move dashboard ownership and AI Data Liaison copy
6. Add any optional glossary CTA on the AI Data Liaison page
7. Update frontend tests
8. Run full checks

## Verification

Because backend code changes are involved, finish with:

```sh
make check
```

If the dashboard or AI Data Liaison page copy/layout changes substantially, also run the relevant frontend test subset while iterating.

## Expected Outcome

After implementation:

- Glossary appears to users as an AI Data Liaison feature
- glossary MCP tools are grouped under AI Data Liaison in tool inventory/config
- glossary tool execution is gated by the same ownership model the UI reports
- glossary generation still depends on ontology readiness
- `/glossary` remains directly reachable without adding new page gating
