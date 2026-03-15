# ISSUE: AI Agents can select disabled approved queries, then fail with "Agent not found"

**Status:** Open
**Pages:** AI Agents (`/projects/{pid}/ai-agents`), Pre-Approved Queries (`/projects/{pid}/queries`)
**Date:** 2026-03-14

## Context

AI Agents are assigned access to pre-approved queries. The current UI and backend do not agree on which approved queries are eligible for assignment.

Today:

- the AI Agents page loads the normal query list for the project's datasource and keeps anything with `status === 'approved'`
- the backend only allows agent access to queries that are all of:
  - `status = 'approved'`
  - `is_enabled = true`
  - `deleted_at IS NULL`
- when the backend rejects an ineligible query, the handler maps the resulting `ErrNotFound` to `404 Agent not found`

This creates a bad user-facing failure mode: the UI can offer a disabled approved query as selectable, but saving the agent fails with an error that implies the agent is missing.

## Observed behavior

If an approved query is disabled and then selected from the AI Agents page:

- the query still appears in the AI Agents add/edit dialog
- saving the agent fails
- the returned error is `404 not_found` with message `Agent not found`

The message is misleading because the real problem is that the selected query is no longer eligible for agent access.

## Human-reproducible scenario

1. Create a project with one datasource.
2. Create and approve a pre-approved query `Q1`.
3. Navigate to AI Agents and confirm `Q1` appears in the query selection UI.
4. Navigate to Pre-Approved Queries.
5. Open `Q1` and disable it using the enable/disable toggle for approved queries.
6. Return to AI Agents and refresh the page.
7. Open `Add Agent`, or edit an existing agent.
8. Select `Q1` and save.

## Actual result

- `Q1` is still visible and selectable on the AI Agents page
- the save fails with an error equivalent to `Agent not found`

## Why this happens

### UI behavior

The AI Agents page fetches the full datasource query list and only filters by query status:

- `ui/src/pages/AIAgentsPage.tsx`
  - calls `engineApi.listQueries(...)`
  - filters with `query.status === 'approved'`

That means approved-but-disabled queries remain selectable.

### Query management behavior

The Pre-Approved Queries page allows an approved query to be disabled through the normal UI:

- `ui/src/components/QueriesView.tsx`
  - `handleToggleEnabled(...)` updates `is_enabled`

This is a normal supported user action, not an artificial test condition.

### Backend eligibility behavior

Agent query assignment only succeeds if the query is approved, enabled, and not deleted:

- `pkg/repositories/agent_repository.go`
  - `setQueryAccessTx(...)`
  - requires:
    - `q.is_enabled = true`
    - `q.status = 'approved'`
    - `q.deleted_at IS NULL`
  - otherwise returns `ErrNotFound`

### Handler error mapping

The agent handler maps all `ErrNotFound` cases to the same response:

- `pkg/handlers/agent_handler.go`
  - `handleServiceError(...)`
  - returns `404 Agent not found`

So a missing/ineligible query and a missing agent collapse into the same user-visible error.

## Research notes

This issue is not about multi-datasource support. The project currently enforces a single-datasource-per-project policy:

- `pkg/handlers/datasources.go`
- `pkg/repositories/datasource_repository.go`
- `pkg/handlers/datasources_integration_test.go`

The scenario above is reproducible within the currently supported product shape.

## Policy decision required

Before implementing a fix, decide the intended policy:

### Option A: AI Agents should honor disabled queries

Interpretation:

- disabling an approved query means it should not be assignable to AI Agents

Implications:

- AI Agents UI should stop showing approved-but-disabled queries
- save errors should clearly state that one or more selected queries are not eligible
- backend eligibility rules can stay as they are

### Option B: AI Agents may still use disabled approved queries

Interpretation:

- `is_enabled` controls interactive query use elsewhere, but does not block agent assignment/execution

Implications:

- repository eligibility rules for agent query access need to be relaxed
- any downstream execution/auth logic for agents must be reviewed for consistency
- UI may be correct to keep showing disabled approved queries, but the label/copy may still need clarification

## Open question

Should AI Agents adhere to disabled approved queries, or are disabled queries still valid for agent assignment and execution?

This is a product/policy decision, not just an implementation detail. The correct fix depends on that answer.
