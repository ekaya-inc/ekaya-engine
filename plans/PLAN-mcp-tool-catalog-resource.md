# PLAN: MCP Tool Catalog Resource

**Status:** TODO
**Created:** 2026-03-16

## Problem

MCP clients currently discover tools only through the filtered `tools/list` response. By the time a client sees that list, the server has already removed anything blocked by:

- app installation state
- per-app toggle state
- agent-vs-JWT auth mode
- no-datasource gating

That makes hidden capabilities effectively invisible. A client cannot tell that:

- `suggest_approved_query` exists but requires the AI Data Liaison app
- `update_project_knowledge` exists but comes from Ontology Forge
- `query` can come from more than one application path
- the current session is restricted because the project has no datasource

This is a discovery and upsell gap. The client can only react to currently enabled tools, not to potentially relevant tools that the project could enable.

The codebase already contains most of the metadata needed to explain this:

- `pkg/services/mcp_app_toggles.go` defines app/toggle/tool relationships
- `pkg/services/mcp_tool_loadouts.go` defines canonical tool ordering and descriptions
- `pkg/services/tool_access.go` computes current tool accessibility
- `pkg/services/mcp_config.go` already resolves normalized config and installed-app filtering
- `pkg/mcp/tools/developer.go` applies the live tool filter

But the MCP server currently enables tool capabilities only, registers no resources, and therefore exposes no read-only discovery surface beyond the live tool list.

## Goals

- Add one authenticated MCP resource that advertises both currently visible tools and potentially available tools.
- Explain which application and toggle path each tool comes from.
- Preserve the distinction between "callable right now" and "informational only".
- Give MCP clients enough structured information to say things like "AI Data Liaison provides tools for this workflow" without guessing.
- Keep the resource project-aware so it reflects installed apps, current config, auth mode, and datasource state.

## Non-Goals

- Do not change tool execution authorization.
- Do not make hidden tools callable through the resource.
- Do not replace `tools/list` as the authoritative source of callable tool schemas.
- Do not add multiple resources or templates in the first pass.
- Do not add UI changes in this plan.

## Current State

### Tool visibility

The MCP tool filter hides tools that are not currently accessible for the session:

- `pkg/mcp/tools/developer.go`
- `pkg/services/tool_access.go`

This is correct for execution safety, but it means clients cannot inspect the wider capability graph.

### Tool metadata

Tool metadata is distributed across several backend sources:

- `pkg/services/mcp_app_toggles.go`
  - app ID
  - role
  - toggle key
  - display name
  - tools per toggle
- `pkg/services/mcp_tool_loadouts.go`
  - canonical order
  - one-line descriptions
- `pkg/services/mcp_tools_registry.go`
  - legacy UI-facing tool metadata
- `pkg/services/mcp_config.go`
  - installed-app filtering
  - normalized config state
  - enabled tool response format

### Missing resource surface

`pkg/mcp/server.go` enables tool capabilities only. No resource capabilities are enabled and no resources are registered. As a result, `resources/list` is empty even though the backend has enough information to produce a useful catalog.

## Proposed Design

### 1. Add one static authenticated resource

Add a single read-only resource:

- URI: `ekaya://mcp/tool-catalog`
- Name: `MCP Tool Catalog`
- MIME type: `application/json`

This resource should appear in `resources/list` for authenticated project-scoped MCP sessions, even when the current `tools/list` would only return `health`.

The resource URI stays constant. The content is recomputed on each read from current project/session state. Because the URI itself does not appear or disappear, the first version does not need dynamic resource-template machinery.

### 2. Model the catalog as "current visibility + possible paths"

The resource should not merely dump the current enabled tools. It should show both:

- what is visible in the current session
- what other tool paths exist for this project/server

The catalog should be driven by tool access paths, not by a single `app_id` per tool. This matters because some tools intentionally appear in more than one toggle path, for example `query`.

Each tool entry should include:

- `name`
- `description`
- `visible_in_current_session`
- `current_visibility_reason`
- `access_paths`

Each access path should include:

- `app_id`
- `app_name`
- `role`
- `toggle_key`
- `toggle_name`
- `app_installed`
- `toggle_enabled`
- `path_status`
- `enablement_hint`

`path_status` should be deterministic and explainable. Initial states:

- `enabled`
- `disabled_by_toggle`
- `app_not_installed`
- `session_blocked_no_datasource`
- `session_blocked_agent_mode`
- `always_available`

### 3. Include application summaries for upsell/discovery

The resource should also include an `applications` section that explains what each installed or installable application contributes.

Each application entry should include:

- `app_id`
- `app_name`
- `installed`
- `summary`
- `toggles`

Each toggle entry should include:

- `role`
- `toggle_key`
- `display_name`
- `enabled`
- `tools`

Add short curated summaries for each application so a client can explain value, not just names:

- MCP Server: direct database access and diagnostics
- Ontology Forge: ontology maintenance and semantic context tools
- AI Data Liaison: query workflow, approvals, and reusable query collaboration

### 4. Treat current session state as a first-class part of the payload

The resource should include a `session` block with at least:

- `project_id`
- `auth_mode` (`jwt` or `agent`)
- `has_datasource`
- `visible_tool_names`

This keeps the distinction clear:

- the tool list is what the client can call right now
- the resource explains why other tools are not currently listed

### 5. Keep transport thin by pushing catalog logic into services

Do not build catalog logic directly in the MCP resource registration layer.

Preferred structure:

- new service in `pkg/services/mcp_tool_catalog.go`
- thin MCP resource registration in `pkg/mcp/resources/tool_catalog.go`

The service should reuse existing metadata and access helpers rather than duplicating tool/filter logic.

## Resource Shape

Example response shape:

```json
{
  "session": {
    "project_id": "uuid",
    "auth_mode": "jwt",
    "has_datasource": true,
    "visible_tool_names": ["health", "get_context", "list_glossary"]
  },
  "applications": [
    {
      "app_id": "ai-data-liaison",
      "app_name": "AI Data Liaison",
      "installed": false,
      "summary": "Query workflow, approvals, and reusable query collaboration",
      "toggles": [
        {
          "role": "user",
          "toggle_key": "addRequestTools",
          "display_name": "Add Request Tools",
          "enabled": false,
          "tools": ["query", "sample", "validate"]
        }
      ]
    }
  ],
  "tools": [
    {
      "name": "suggest_approved_query",
      "description": "Suggest a reusable parameterized query for approval",
      "visible_in_current_session": false,
      "current_visibility_reason": "AI Data Liaison is not installed for this project",
      "access_paths": [
        {
          "app_id": "ai-data-liaison",
          "app_name": "AI Data Liaison",
          "role": "user",
          "toggle_key": "addRequestTools",
          "toggle_name": "Add Request Tools",
          "app_installed": false,
          "toggle_enabled": false,
          "path_status": "app_not_installed",
          "enablement_hint": "Install AI Data Liaison and enable Add Request Tools"
        }
      ]
    }
  ]
}
```

This should be JSON only in the first implementation. Do not add a parallel markdown rendering path yet.

## Architecture Notes

### Source of truth

The catalog should derive from:

- `AppToggles` for app/toggle provenance
- `AllToolsOrdered` / `GetToolSpec` for canonical descriptions and ordering
- `ToolAccessChecker` for current session visibility
- installed app checks for real project state
- datasource presence checks for session-level gating

Avoid hardcoding second copies of tool descriptions or tool-app relationships inside the resource handler.

### Multi-path tools

Do not assume one tool belongs to one app. The catalog must support multiple access paths per tool.

This is required for tools like `query`, which can be surfaced through different application/toggle combinations.

### Resource access

The resource is informational, but it still needs authenticated project-scoped access.

Implementation should avoid copying tool-access setup code directly into the resource package. Prefer extracting a shared MCP project-context helper if that reduces duplication between:

- `pkg/mcp/tools/access.go`
- new resource read handlers

Unlike tool execution, the resource should not require that a specific tool already be enabled.

## Implementation Plan

### Phase 1: Catalog service and model

1. [ ] Add a new service file `pkg/services/mcp_tool_catalog.go`
2. [ ] Define response models for session state, applications, toggles, tools, and access paths
3. [ ] Add helpers to enumerate all tool access paths from `AppToggles`
4. [ ] Add curated per-app summary text in a service-owned metadata map
5. [ ] Use canonical tool ordering from `mcp_tool_loadouts.go`
6. [ ] Compute current visible tools using the same access logic as the MCP tool filter
7. [ ] Compute deterministic hidden reasons from app installation, toggle state, auth mode, and datasource state

### Phase 2: MCP resource support

8. [ ] Enable resource capabilities in `pkg/mcp/server.go`
9. [ ] Add a new package `pkg/mcp/resources`
10. [ ] Add `pkg/mcp/resources/tool_catalog.go` with registration for `ekaya://mcp/tool-catalog`
11. [ ] Keep the resource handler thin by delegating all catalog assembly to the new service
12. [ ] Ensure the resource is available to authenticated sessions even when the current tool list is minimal

### Phase 3: App wiring

13. [ ] Wire the new catalog service into `internal/app/app.go`
14. [ ] Register the resource alongside existing MCP tool registration
15. [ ] Reuse existing installed-app and project services rather than introducing new data access paths

### Phase 4: Hardening and consistency

16. [ ] Ensure every tool in the catalog has a stable description source
17. [ ] Ensure tools with multiple access paths render all paths
18. [ ] Ensure `health` is represented as always available
19. [ ] Ensure agent-only/current-session differences are reflected as session state, not as missing metadata
20. [ ] Keep the catalog informational only; no action endpoints or mutation links

## Testing Strategy

### Unit tests

- service tests for app grouping and toggle projection
- service tests for multi-path tools such as `query`
- service tests for app-not-installed classification
- service tests for toggle-disabled classification
- service tests for no-datasource classification
- service tests for agent-auth classification
- service tests that preserve canonical tool ordering

### Integration tests

- MCP server `resources/list` includes `ekaya://mcp/tool-catalog`
- `resources/read` returns project-aware JSON content
- a project without AI Data Liaison still advertises its tools as unavailable rather than omitting them
- a project with toggles disabled shows tools as potential but not visible
- a project with no datasource still returns the catalog resource and explains session restrictions

### Consistency tests

- visible tools in the resource match the same underlying access computation used by `tools/list`
- tool descriptions in the resource come from the canonical metadata source rather than duplicated strings

## Open Questions

1. Should the resource include a short top-level `recommended_tools` section, or is the per-app summary sufficient for the first pass?
2. Do we want to expose exact toggle keys in the first version, or only human-friendly toggle names?
3. Should agent sessions see app/toggle paths that are only relevant to JWT-authenticated users, or should that be reduced to the current session plus app summaries?

