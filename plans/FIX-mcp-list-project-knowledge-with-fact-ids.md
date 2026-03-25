# FIX: Add `list_project_knowledge` MCP tool so clients can discover `fact_id`s

**Status:** Completed
**Completed:** 2026-03-25
**Date:** 2026-03-20

## Resolution

Implemented `list_project_knowledge` as a zero-argument ontology-maintenance MCP tool and wired it into the MCP tool catalog, loadouts, and Ontology Forge app toggle.

Also updated the project-knowledge MCP prompts and tests so the supported maintenance workflow is now:

1. create with `update_project_knowledge`
2. discover row IDs with `list_project_knowledge`
3. update or delete by `fact_id`

During implementation review, one claim in this FIX file turned out to be inaccurate: blank or whitespace `fact_id` values were not silently treated as omitted by the runtime. The existing code path rejected them as invalid UUIDs, and the tests were updated to make that behavior explicit.

## Problem

The MCP surface for project knowledge is incomplete.

Today an MCP client can:

- create a project knowledge fact with `update_project_knowledge`
- update an existing fact with `update_project_knowledge` if it already knows the `fact_id`
- delete an existing fact with `delete_project_knowledge` if it already knows the `fact_id`

But there is no MCP tool that lists existing project knowledge rows with their IDs.

The only read path currently available through MCP is `get_context(depth="domain")`, and that path is intentionally summary-oriented. It groups facts by category and returns prompt-friendly `fact` plus optional `context`, but it does not return `fact_id`, timestamps, or provenance fields.

That leaves an MCP-only client with a real management gap:

- it can create facts and receive a `fact_id` in the immediate create response
- it cannot later rediscover existing `fact_id`s through MCP
- it cannot reliably update or delete pre-existing facts that were created in an earlier session, by another client, or through the UI

## Why this matters

This is not just a convenience issue. It breaks the expected workflow for long-lived MCP clients and multi-session tool use.

Current behavior:

1. `update_project_knowledge` creates a new row when `fact_id` is omitted
2. `update_project_knowledge` updates by row ID when `fact_id` is provided
3. `delete_project_knowledge` deletes by row ID and requires `fact_id`
4. `get_context(depth="domain")` exposes only summarized project knowledge
5. there is no MCP discovery/list tool that bridges summarized context to row-level identifiers

This means an MCP client cannot do a full CRUD cycle for project knowledge using MCP tools alone unless it persists IDs from previous calls or gets them from some out-of-band path.

There is also a second, related sharp edge:

- the `update_project_knowledge` tool description currently says facts are upserted by `(category, fact)` pair
- the current implementation does not do that
- the repository create path is a plain insert
- the table does not enforce uniqueness on `(project_id, fact_type, value)`

So omitting `fact_id` does not target an existing row. It creates a new row.

There is a third sharp edge worth capturing because it makes accidental duplication even easier:

- `update_project_knowledge` trims `fact_id` and treats the empty string as "not provided"
- that means a client sending `fact_id: ""` does not get a validation error
- it silently falls through to create mode

That behavior is currently documented in unit tests, but it is easy for LLM clients or UI layers to hit by accident when they serialize optional string fields.

## Observed MCP client failure mode

This was not just inferred from code review. It showed up directly during MCP-only ontology curation against `the_look`.

Observed behavior:

- `get_context(depth="domain")` exposed project knowledge facts grouped by category
- the returned `project_knowledge` entries included `fact` and optional `context`, but no `fact_id`
- that was enough to identify misleading or redundant facts by text
- it was not enough to call `delete_project_knowledge`, which requires `fact_id`
- because `update_project_knowledge` does not actually upsert by `(category, fact)`, there was also no safe MCP-only way to "overwrite in place" without already knowing the row ID

In practice, cleanup had to fall back to manual search/delete outside the MCP tool surface. That is exactly the workflow gap this fix should close.

This fix does not need to redesign that behavior, but it should stop MCP clients from being stranded without a way to discover IDs.

## Existing infrastructure

The good news is most of the plumbing already exists. This task should stay narrow.

### Repository support already exists

`pkg/repositories/knowledge_repository.go` already exposes:

- `GetByProject(ctx, projectID)` returning all facts for the project
- `GetByType(ctx, projectID, factType)` if category filtering is ever needed later

No new database tables, migrations, or repository interfaces are required for a v1 list tool if the tool simply returns all facts for the current project.

### Model already has the needed fields

`pkg/models/ontology_chat.go` defines `KnowledgeFact` with:

- `ID`
- `ProjectID`
- `FactType`
- `Value`
- `Context`
- `Source`
- `LastEditSource`
- `CreatedBy`
- `UpdatedBy`
- `CreatedAt`
- `UpdatedAt`

The list tool can project a client-friendly subset of those fields into its own response type.

### HTTP already has a list endpoint

There is already a non-MCP HTTP endpoint for project knowledge:

- `GET /api/projects/{pid}/project-knowledge`

`pkg/handlers/knowledge_handler.go` already returns a list response with raw facts and a count.

That means the gap is specifically at the MCP layer, not at the data layer.

### Tool registration and dependencies are already wired

`internal/app/app.go` already builds `KnowledgeToolDeps` and calls `RegisterKnowledgeTools(...)`.

Because the new list tool belongs in the same knowledge tool family and only needs the existing `KnowledgeRepository`, no new dependency wiring should be needed at app startup beyond adding the registration inside `RegisterKnowledgeTools(...)`.

## Intended fix

Add an explicit read-only MCP tool:

- `list_project_knowledge`

This tool should be the MCP discovery path for row-level project knowledge.

It should:

- return project knowledge facts with `fact_id`
- return enough metadata for an MCP client to choose what to update or delete
- remain separate from `get_context`, which should stay optimized for prompt/context consumption rather than row-level maintenance

## Tool design

### Tool name

- `list_project_knowledge`

### Purpose

List all project knowledge facts for the current project so an MCP client can discover stable `fact_id`s before calling `update_project_knowledge` or `delete_project_knowledge`.

### Scope for v1

Keep v1 narrow:

- zero required parameters
- zero optional filters unless they are trivially reused from existing repository methods
- return all facts for the current project in repository order

The key gap is ID discovery, not advanced filtering.

### Recommended response shape

```json
{
  "facts": [
    {
      "fact_id": "uuid",
      "category": "business_rule",
      "fact": "Platform fees are ~33% of total_amount",
      "context": "Verified from billing rows",
      "source": "mcp",
      "last_edit_source": "manual",
      "created_at": "2026-03-20T09:15:00Z",
      "updated_at": "2026-03-20T09:20:00Z"
    }
  ],
  "count": 1
}
```

Notes:

- Use MCP-friendly field names:
  - `fact_id` instead of raw `id`
  - `category` instead of raw `fact_type`
  - `fact` instead of raw `value`
- Include `source` and `last_edit_source` because they are useful for maintenance decisions
- Include timestamps because they help disambiguate stale or duplicated facts
- Do not include `created_by` or `updated_by` unless there is a concrete need; those actor UUIDs are not required to close the MCP CRUD gap

### Tool annotations

The new tool should be:

- read-only: `true`
- destructive: `false`
- idempotent: `true`
- open-world: `false`

### Access/loadout placement

This should be a developer/ontology-maintenance tool, not a user/query-context tool.

Reason:

- `get_context` already covers the prompt-context use case for users
- `list_project_knowledge` is an ontology-maintenance/admin discovery tool used to support mutation workflows

So `list_project_knowledge` should live alongside:

- `update_project_knowledge`
- `delete_project_knowledge`
- `update_column`
- `update_table`
- `list_pending_changes`
- `list_ontology_questions`

## File-by-file changes

### 1. `pkg/mcp/tools/knowledge.go`

Add a new read-only registration:

- [ ] Add `registerListProjectKnowledgeTool(...)`
- [ ] Register it from `RegisterKnowledgeTools(...)`
- [ ] Reuse `AcquireToolAccess(...)` for access checks, matching the existing knowledge tools
- [ ] Load facts with `deps.KnowledgeRepository.GetByProject(tenantCtx, projectID)`
- [ ] Convert repository models into a dedicated response type for MCP output
- [ ] Return `{ facts: [...], count: N }`

Recommended implementation shape:

- define a `listProjectKnowledgeResponse` struct
- define a `projectKnowledgeListItem` struct
- add a small mapper function from `*models.KnowledgeFact` to the response item
- preserve repository ordering rather than inventing new sorting logic in the tool

Also tighten the mutation tool descriptions while in this file:

- [ ] Update `update_project_knowledge` description so it no longer claims a natural-key upsert unless the implementation is changed in the same task
- [ ] Mention `list_project_knowledge` in the descriptions for `update_project_knowledge` and `delete_project_knowledge` as the discovery step for finding existing `fact_id`s

This description cleanup is part of making the tool family coherent for MCP clients.

### 2. `pkg/services/mcp_tools_registry.go`

Add a registry entry for:

- [ ] `list_project_knowledge`

Place it with the other developer/ontology-maintenance tools.

Recommended description:

- `List all project knowledge facts with fact IDs for discovery and maintenance`

### 3. `pkg/services/mcp_tool_loadouts.go`

Add the tool to the canonical tool catalog and ontology-maintenance loadout.

- [ ] Add `list_project_knowledge` to `AllToolsOrdered`
- [ ] Add `list_project_knowledge` to `LoadoutOntologyMaintenance`

Suggested ordering:

- place it immediately before `update_project_knowledge` / `delete_project_knowledge`

That keeps discovery next to mutation in the catalog.

### 4. `pkg/services/mcp_app_toggles.go`

Add the tool to the Ontology Forge developer toggle.

- [ ] Include `list_project_knowledge` in `AddOntologyMaintenanceTools`

Without this, the tool may exist in code but not show up in project config-derived tool listings.

### 5. `pkg/mcp/tools/strict_client_schema_test.go`

If `list_project_knowledge` is implemented as a zero-arg tool, update the strict-client compatibility test so it expects an explicit empty `properties` object for this tool too.

- [ ] Add `list_project_knowledge` to the zero-arg tool assertions

If the implementation adds optional parameters, then keep the test focused on schema validity instead of empty properties.

Prefer the zero-arg version for v1 unless there is a strong reason not to.

### 6. `tests/claude-mcp/prompts/240-project-knowledge.md` and `tests/claude-mcp/prompts/340-project-knowledge-delete.md`

These MCP prompt fixtures currently encode the wrong contract:

- they still refer to `key`, `value`, and `fact_type`
- they assume "update existing fact by key" behavior
- the delete prompt still assumes deletion by key instead of by `fact_id`

- [ ] Update the project-knowledge prompt fixtures to use the real MCP parameters: `fact`, `category`, optional `context`, and `fact_id`
- [ ] Rewrite the update test flow so it creates a fact, captures returned `fact_id`, and updates by that ID
- [ ] Rewrite the delete test flow so it first discovers or captures `fact_id` and deletes by that ID

These prompt files are not the root bug, but leaving them stale will keep reinforcing the wrong mental model for future MCP testing.

## Tests to update

This repo has a wide MCP test surface. A new tool usually needs more than one handler test.

### Core tool unit tests

`pkg/mcp/tools/knowledge_test.go`

- [ ] Update `TestRegisterKnowledgeTools` to assert `list_project_knowledge` is registered
- [ ] Add response-structure coverage for the new list tool
- [ ] Add success test for listing multiple facts
- [ ] Add success test for empty list
- [ ] Add repository error-path test

The existing mock repository already implements `GetByProject(...)`, so unit coverage should be straightforward.

- [ ] Add coverage for `fact_id: ""` / whitespace-only handling so the intended behavior is explicit

Decide in implementation whether the tool should keep silently treating blank `fact_id` as omitted or tighten validation. Either way, the behavior should be intentional and tested.

### Core tool integration tests

`pkg/mcp/tools/knowledge_integration_test.go`

- [ ] Add integration test that seeds multiple project knowledge facts and verifies the list tool returns `fact_id`, `category`, `fact`, `source`, and timestamps
- [ ] Add integration test for an empty project returning `facts: []` and `count: 0`
- [ ] Add a lifecycle integration test that proves the gap is closed:
  - create a fact
  - list facts and capture returned `fact_id`
  - update that fact using the returned `fact_id`
  - delete that fact using the returned `fact_id`
  - list again and confirm it is gone

That lifecycle test is the most important regression test for this fix.

### Tool annotation tests

`pkg/mcp/tools/annotation_policy_test.go`

- [ ] Assert `list_project_knowledge` is marked read-only
- [ ] Assert `list_project_knowledge` is non-destructive
- [ ] Assert `list_project_knowledge` is idempotent

### Tool catalog and access tests

At minimum, review and update the tests that assert enabled tool sets, loadout contents, or tool ordering:

- [ ] `pkg/services/mcp_tool_loadouts_test.go`
- [ ] `pkg/services/tool_access_test.go`
- [ ] `pkg/services/mcp_tools_registry_test.go`

### MCP app ownership/config tests

Because the tool should belong to Ontology Forge developer tooling:

- [ ] `pkg/services/mcp_config_test.go`

Update expectations so `list_project_knowledge` resolves to `models.AppIDOntologyForge` for developer tools.

### MCP server catalog / scenario tests

This repo also has broader MCP catalog and scenario coverage. Review these for hard-coded tool lists or expected counts:

- [ ] `pkg/mcp/tools/mcp_tools_integration_test.go`
- [ ] `pkg/mcp/tools/mcp_tools_scenario_test.go`
- [ ] `pkg/mcp/tools/access_test.go`
- [ ] `pkg/mcp/tools/developer_filter_test.go`
- [ ] `pkg/mcp/tools/strict_client_schema_test.go`

If any of these enumerate tool names explicitly, add `list_project_knowledge` where appropriate.

## Implementation notes

### Keep `get_context` unchanged

Do not add `fact_id`s to `get_context`.

`get_context(depth="domain")` is explicitly designed as prompt-friendly, summarized context. Adding row IDs and maintenance metadata there would blur the boundary between:

- query/context consumption
- ontology/project-knowledge maintenance

This task should preserve that separation.

### Do not require new repository or service layers

For the narrow v1 feature, the MCP tool should call the existing repository directly, just like the current knowledge tools do.

That means:

- no new service layer is required
- no new database query is required if `GetByProject(...)` is reused
- no migration is required

### Response naming should match MCP expectations, not raw DB naming

The HTTP handler can continue returning raw-ish `KnowledgeFact` records.

For MCP, prefer a small purpose-built response shape with explicit client-facing names:

- `fact_id`
- `category`
- `fact`

This makes the tool easier for LLM clients to use correctly.

## Non-goals

Do not expand this task into a larger redesign.

- [ ] Do not change `get_context` to include `fact_id`s
- [ ] Do not redesign `update_project_knowledge` into a true natural-key upsert by `(category, fact)` in this task
- [ ] Do not add database uniqueness constraints or migrations for project knowledge in this task
- [ ] Do not change the HTTP project-knowledge endpoints
- [ ] Do not add agent-auth access to this tool unless there is a separate product decision to widen the agent loadout
- [ ] Do not add pagination, source filters, or category filters unless implementation discovers that returning all facts is insufficient

If later work wants richer browsing or auditing, that can be a follow-up after the basic MCP CRUD gap is closed.

## Expected outcome

After this fix:

- MCP clients can enumerate project knowledge facts and discover stable `fact_id`s
- `update_project_knowledge` and `delete_project_knowledge` become usable for pre-existing facts in MCP-only workflows
- `get_context(depth="domain")` remains a summarized context tool, not a row-level maintenance tool
- the project-knowledge tool family becomes consistent with the rest of the MCP surface, where discovery/list tools exist before ID-based mutation tools
