# FIX: MCP tool bundle

**Status:** Open
**Date:** 2026-03-25

## Why this file exists

This file replaces the older overlapping MCP planning documents:

- `plans/FIX-mcp-tool-toggle-inventories-and-ownership.md`
- `plans/TASK-update-tool-loadouts.md`
- `plans/PLAN-mcp-tool-catalog-resource.md`
- `plans/FIX-mcp-glossary-tools-should-expose-sql-availability.md`

Those files mixed together:

- ownership changes that are already landed
- stale assumptions about current tool inventories
- glossary-contract cleanup that is still unimplemented
- a larger discovery/catalog feature that is not required for the current fix

This bundled FIX is the single source of truth for the remaining MCP-tool work that still matters in the current codebase.

## Current code snapshot

### App toggle ownership is already updated

`pkg/services/mcp_app_toggles.go` is now the real source of truth for per-role MCP tool ownership.

Current inventories in code:

- MCP Server developer `addDirectDatabaseAccess`:
  - `echo`
  - `execute`
  - `query`
  - `validate`
  - `sample`
  - `explain_query`
- MCP Server user `addDirectDatabaseAccess`:
  - `query`
  - `validate`
  - `sample`
- Ontology Forge user `addOntologySuggestions`:
  - `get_context`
  - `get_ontology`
  - `list_approved_queries`
  - `execute_approved_query`
  - `search_schema`
  - `get_schema`
  - `get_column_metadata`
  - `probe_column`
  - `probe_columns`
  - `list_project_knowledge`
- AI Data Liaison developer `addApprovalTools`:
  - `list_query_suggestions`
  - `approve_query_suggestion`
  - `reject_query_suggestion`
  - `create_glossary_term`
  - `update_glossary_term`
  - `delete_glossary_term`
  - `list_glossary`
  - `get_glossary_sql`
  - `get_query_history`
- AI Data Liaison user `addRequestTools`:
  - `list_glossary`
  - `get_glossary_sql`
  - `suggest_approved_query`
  - `suggest_query_update`
  - `get_query_history`
  - `record_query_feedback`

### Access behavior depends on keeping same-role ownership unique

Current behavior is split across two layers:

- `pkg/mcp/tools/access.go` uses `GetEnabledToolOwningAppIDs(...)` and `GetToolOwningAppIDs(...)` for installation gating
- `pkg/services/mcp_config.go` and `pkg/mcp/tools/developer.go` still use `GetToolAppID(toolName, role)` to assign `appId` and do role-specific installation filtering

That means the codebase still does **not** safely support the same tool being owned by two different apps for the same role.

The current `AppToggles` definitions avoid that problem. This FIX must keep that property intact.

### UI app-page copy is already mostly aligned

The user-facing app pages already reflect the current tool surfaces closely enough:

- `ui/src/pages/MCPServerPage.tsx`
- `ui/src/pages/OntologyForgePage.tsx`
- `ui/src/pages/AIDataLiaisonPage.tsx`

This bundled FIX should not reopen those page descriptions unless a concrete test or code review shows a remaining mismatch.

## What is already done and should not be redone

- Do not move `query`, `validate`, or `sample` back into AI Data Liaison user tools.
- Do not move `explain_query` back into AI Data Liaison developer tools.
- Do not add `query` or `validate` to Ontology Forge developer tools as part of this fix.
- Do not refactor the ownership model to support same-role duplicate owners in this fix.
- Do not treat the deleted `TASK-update-tool-loadouts.md` as still active guidance.

## Remaining required work

### 1. Make glossary read contracts explicit about SQL availability

This is the clearest still-open MCP usability issue.

Current problem:

- `list_glossary` does not say whether a term actually has SQL
- `get_glossary_sql` can return an empty `defining_sql` without an explicit signal
- several descriptions still lean toward “every glossary term has SQL” even though definition-only terms are valid

Current implementation context:

- `pkg/mcp/tools/glossary.go`
  - `listGlossaryResponse` has no `has_sql`
  - `getGlossarySQLResponse` has no `has_sql`
  - handler descriptions still imply SQL-backed terms more strongly than the actual model does
- `pkg/services/mcp_tools_registry.go`
  - glossary one-line descriptions still assume simpler SQL-backed behavior
- `pkg/services/mcp_tool_loadouts.go`
  - canonical descriptions still need the same clarification

Required changes:

- In `pkg/mcp/tools/glossary.go`:
  - add `HasSQL bool \`json:"has_sql"\`` to `listGlossaryResponse`
  - add `HasSQL bool \`json:"has_sql"\`` to `getGlossarySQLResponse`
  - populate `HasSQL` from whether the stored SQL is non-empty after trimming
  - update `list_glossary` description so it explicitly supports definition-only terms
  - update `get_glossary_sql` description so it says valid terms may have `has_sql = false`
  - tighten `create_glossary_term` and `update_glossary_term` wording so SQL is clearly optional and SQL-backed terms are usually single-row metric-style definitions
- In `pkg/services/mcp_tools_registry.go`:
  - align the one-line descriptions for:
    - `list_glossary`
    - `get_glossary_sql`
    - `create_glossary_term`
    - `update_glossary_term`
- In `pkg/services/mcp_tool_loadouts.go`:
  - align the matching `AllToolsOrdered` descriptions with the MCP tool descriptions

Expected response shape after the fix:

```json
{
  "term": "Acquisition Channel",
  "definition": "Original acquisition source recorded on users.traffic_source",
  "has_sql": false,
  "defining_sql": ""
}
```

### 2. Keep MCP metadata and tests aligned with the current ownership model

The runtime ownership model in `AppToggles` is already correct, but helper metadata and tests can still drift because older docs assumed different inventories.

Important implementation rule:

- `pkg/services/mcp_app_toggles.go` is the source of truth for current user/developer role ownership
- this fix is about metadata consistency and test coverage around that model
- this fix is **not** a reason to change ownership again

Files to inspect and update as needed:

#### `pkg/services/mcp_tool_loadouts.go`

- Keep canonical descriptions aligned with current glossary contract wording.
- Do not assume `LoadoutDeveloperCore` or `LoadoutOntologyMaintenance` drive current per-role exposure; current role exposure comes from `AppToggles`.
- If comments still imply the old loadout-driven role model, update the comments instead of changing behavior.

#### `pkg/services/mcp_tools_registry.go`

- Keep descriptions synchronized with `AllToolsOrdered` and the MCP tool handlers.
- Preserve the current AI Data Liaison request/approval split:
  - request tools = glossary reads, query suggestion request/update, query history, feedback
  - approval tools = query suggestion review + glossary write tools + glossary reads/query history

#### `pkg/services/mcp_app_toggles_test.go`

Current assertions are already aligned with the current code and should remain true:

- user `query` / `validate` / `sample` belong to MCP Server
- developer `explain_query` belongs to MCP Server
- glossary reads and query history for developer role belong to AI Data Liaison
- approved-query core tools belong to Ontology Forge

Only change this file if implementation changes demand it.

#### `pkg/services/mcp_tool_loadouts_test.go`

Preserve expectations that match the current ownership model:

- request tools do **not** include `query`, `sample`, or `validate`
- approval tools do include:
  - `list_glossary`
  - `get_glossary_sql`
  - `get_query_history`
- approval tools do **not** include `explain_query`
- ontology suggestions do include:
  - `search_schema`
  - `get_schema`
  - `get_column_metadata`
  - `probe_column`
  - `probe_columns`
  - `list_project_knowledge`

#### `pkg/services/mcp_config_test.go`

Preserve current app assignment expectations in API responses:

- `query` in `userTools` is tagged with `appId = mcp-server`
- `list_approved_queries` and `execute_approved_query` are tagged with `appId = ontology-forge`
- `suggest_approved_query` is tagged with `appId = ai-data-liaison`
- developer glossary reads and `get_query_history` are tagged with `appId = ai-data-liaison`

#### `pkg/mcp/tools/developer_filter_test.go`

If adding or updating filter assertions, make sure the test fixtures are broad enough to cover the current surfaces.

In particular, the minimal fixtures often need to include tools such as:

- `explain_query`
- `search_schema`
- `get_column_metadata`
- `probe_column`
- `probe_columns`
- `list_project_knowledge`
- `get_query_history`

Preserve the current expectation that AI Data Liaison approval tools do **not** expose `explain_query`.

## Tests to update

Backend tests that should move with this fix:

- `pkg/mcp/tools/glossary_test.go`
- `pkg/mcp/tools/glossary_integration_test.go`
- `pkg/services/mcp_tool_loadouts_test.go`
- `pkg/services/mcp_config_test.go`
- `pkg/mcp/tools/developer_filter_test.go`

Use automated verification only. Do not add manual testing tasks to this plan.

## Explicitly deferred from this bundled fix

The deleted `PLAN-mcp-tool-catalog-resource.md` is intentionally **not** part of this bundled FIX.

Reason:

- it was a broader MCP discovery feature, not a current correctness fix
- its examples depended on older assumptions, especially around multi-path `query` ownership
- if the catalog/resource work is revisited later, it should be replanned from scratch against the current `AppToggles` model

## Non-goals

- No new MCP tool implementations
- No new MCP resource/catalog implementation
- No ownership-model refactor
- No duplicate same-role tool owners across apps
- No database schema changes
- No MCP config schema changes
- No frontend redesign

## Completion criteria

- `list_glossary` and `get_glossary_sql` expose `has_sql`
- glossary tool descriptions clearly support definition-only terms
- glossary descriptions are aligned across:
  - `pkg/mcp/tools/glossary.go`
  - `pkg/services/mcp_tool_loadouts.go`
  - `pkg/services/mcp_tools_registry.go`
- existing ownership tests continue to reflect the current `AppToggles` model
- filter/config tests do not regress toward the deleted stale plan assumptions
