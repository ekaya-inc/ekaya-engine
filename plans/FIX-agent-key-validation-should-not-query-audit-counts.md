# FIX: Agent API key validation should not query audit-count aggregates

**Status:** Open
**Date:** 2026-03-14

## Problem

Every MCP request authenticated with an AI Agent API key currently does more database work than necessary.

The auth path validates the provided key on every request. That validation only needs the agent rows and their encrypted API key material, but the repository path currently also computes MCP usage counts from `engine_mcp_audit_log` for every agent in the project.

This means agent-authenticated MCP request latency grows with:

- number of agents in the project
- size of the project's MCP audit history

This is avoidable work on a hot authentication path.

## Why this matters

The current behavior is on the request path for every agent-authenticated MCP call:

1. MCP auth middleware extracts the API key and project ID
2. middleware calls `AgentService.ValidateKey(...)`
3. service calls `AgentRepository.FindByAPIKey(...)`
4. repository currently delegates `FindByAPIKey(...)` to `ListByProject(...)`
5. `ListByProject(...)` runs a lateral `COUNT(*)` against `engine_mcp_audit_log` for each agent row

So a request that should only be doing key lookup/decryption work also performs usage aggregation work intended for UI display.

## Current code path

### MCP auth hot path

- `pkg/mcp/auth/middleware.go`
  - `handleAgentKeyAuth(...)`
  - calls `m.agentService.ValidateKey(tenantCtx, projectID, apiKey)`

### Service validation path

- `pkg/services/agent_service.go`
  - `ValidateKey(...)`
  - calls `s.repo.FindByAPIKey(ctx, projectID)`
  - iterates through returned agents and decrypts `APIKeyEncrypted` for constant-time comparison

### Repository path causing extra work

- `pkg/repositories/agent_repository.go`
  - `FindByAPIKey(...)` currently returns `r.ListByProject(ctx, projectID)`
  - `ListByProject(...)` selects agent rows plus:
    - `LEFT JOIN LATERAL (...)`
    - `COUNT(*)::bigint AS total_mcp_calls`
    - from `engine_mcp_audit_log`

That lateral audit-count query exists for agent listing UX, not for auth.

## Root cause

`FindByAPIKey(...)` was implemented as a thin delegation to `ListByProject(...)`.

That was safe before agent listing started computing usage counts from the audit log. After that change, the auth path inherited the heavier query unintentionally.

The regression is architectural:

- one repository method now serves two distinct use cases
- UI listing needs aggregated `MCPCallCount`
- auth validation only needs base agent records, especially `id`, `project_id`, `name`, and `api_key_encrypted`

## Existing database indexes

There is already an audit index on:

- `engine_mcp_audit_log(project_id, user_id, created_at DESC)`

from `migrations/010_mcp_audit.up.sql`.

That helps the count query, but it does not make the current design appropriate for auth:

- the auth path still performs one aggregate lookup per agent row
- the work is still proportional to agent count and audit-log volume
- none of that aggregation is required to validate the API key

## Intended fix

Split the repository read paths so auth does not reuse the stats-enriched listing query.

### Repository changes

In `pkg/repositories/agent_repository.go`:

- [ ] Replace `FindByAPIKey(...)` delegation with a dedicated query that loads only base agent columns for the project
- [ ] Do **not** join `engine_mcp_audit_log` in that query
- [ ] Do **not** compute `MCPCallCount` for auth validation reads

The dedicated auth query should return enough data for `ValidateKey(...)` to work:

- `id`
- `project_id`
- `name`
- `api_key_encrypted`
- `created_at`
- `updated_at`
- `last_access_at`

### Service behavior

In `pkg/services/agent_service.go`:

- [ ] Keep `ValidateKey(...)` behavior unchanged apart from benefiting from the thinner repository query
- [ ] Continue decrypting and constant-time comparing keys exactly as today

### Tests

Add or update tests so the auth path is protected from regressing back to stats-heavy queries:

- [ ] Repository test proving `FindByAPIKey(...)` returns agents without requiring audit-count aggregation
- [ ] Service test for `ValidateKey(...)` still succeeding with the dedicated repository path
- [ ] If useful, repository test that seeds audit rows and asserts `FindByAPIKey(...)` does not populate UI usage counts

## Non-goals

This fix should stay narrow.

Do **not** in this task:

- add hashed API key lookup or fingerprint columns
- redesign agent key storage
- change UI-facing agent list/count behavior
- remove `MCPCallCount` from `ListByProject(...)` or `GetByID(...)`

Those may be future optimizations, but they are separate from removing unnecessary audit aggregation from the current auth path.

## Expected outcome

After this fix:

- agent-authenticated MCP requests no longer run audit-count aggregation during key validation
- auth cost still scales with the number of agents whose encrypted keys must be checked, but not with audit-history aggregation work
- UI list/detail views keep their current `MCPCallCount` behavior
