# REVIEW PLAN: Ensure Architectural Principles Are Used

**Status:** NOT STARTED
**Branch:** ddanieli/add-ai-agents

## Objective

Review the codebase against the architectural principles defined in [CLAUDE.md](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/CLAUDE.md#L94) and the expanded local guidance in [AGENTS.md](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/AGENTS.md#L1), and determine where the current implementation follows, bends, or breaks those principles.

This is a review and verification plan only. It does not authorize implementation changes by itself.

## Important Reviewer Instruction

When a future reviewer finds a likely inconsistency with these guidelines, they must run it by Damond before treating it as a defect or cleanup task. Some deviations may be intentional, historical, or the result of a tradeoff that is not obvious from the code alone.

The reviewer should classify suspicious cases as:

- likely defect
- likely intentional exception
- needs Damond context before classification

No architectural inconsistency should be treated as settled until that context check happens.

## Scope

In scope:

- Backend layer boundaries across handlers, services, repositories, database utilities, MCP tools, and DAG code
- Metadata-store database access patterns and tenant-scoping behavior
- Provenance and pending-changes architectural rules
- API and wire-contract consistency where architecture implies shared conventions
- Places where the code intentionally bypasses the default pattern

Out of scope:

- Pure style disputes that do not affect architecture
- Customer datasource SQL patterns except where they interact with engine architecture boundaries
- Immediate remediation work

## Architectural Baseline

The review should treat the following as the default project architecture:

1. Handlers own transport concerns, request parsing, auth/routing integration, and response shaping.
2. Services own business rules, orchestration, and policy decisions.
3. Repositories own metadata-store persistence and SQL by default.
4. Tenant-scoped metadata access should rely on tenant-scoped connections and RLS.
5. `project_id` is the default tenant boundary unless another boundary is explicitly justified.
6. Ontology provenance is a first-class concern and should not be dropped by convenience shortcuts.
7. Pending changes are part of the product architecture and should not be bypassed casually.
8. DAG logic belongs in DAG/service layers, not in transport or UI-facing orchestration.
9. MCP tools should be thin adapters over core services and access-control machinery.
10. External contracts should be consistent instead of drifting per endpoint.
11. Exceptions are allowed, but they should be explicit and explainable.
12. Legacy fallback behavior should not be added unless Damond explicitly requests it.

## Review Questions

1. Do package boundaries match the intended responsibilities in `CLAUDE.md`?
2. Are handlers thin, or are business rules leaking into transport code?
3. Are services orchestrating behavior cleanly, or are they accumulating repository/HTTP/UI concerns?
4. Is metadata-store SQL concentrated in repositories, with direct SQL outside repositories only where justified?
5. Are tenant scoping and RLS consistently used for metadata-store access?
6. Are provenance rules preserved across ontology mutations and MCP writes?
7. Does the pending-changes architecture remain intact, or are there paths that bypass it without an explicit design reason?
8. Is DAG logic staying in DAG/service layers?
9. Are MCP tools reusing service logic rather than forking behavior?
10. Are external API contracts consistent enough to feel like one system?
11. Are any deviations intentional, and if so, are they documented well enough?

## Deliverables

The review should produce:

- An architectural findings list grouped by severity
- An exceptions register of intentional deviations
- A list of places that need Damond context before classification
- A follow-up remediation outline for confirmed problems
- Suggested updates to `AGENTS.md` if the current guidelines need refinement after review

## Method

### Phase 1: Establish the Review Baseline

Read:

- [CLAUDE.md](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/CLAUDE.md#L1)
- [AGENTS.md](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/AGENTS.md#L1)

Turn the architectural guidance into a review checklist that can be applied consistently.

Required outcome:

- A stable checklist of principles, anti-patterns, and allowed exceptions

### Phase 2: Audit Layer Boundaries

Review representative modules across:

- `pkg/handlers`
- `pkg/services`
- `pkg/repositories`
- `pkg/mcp`
- `pkg/database`
- `pkg/services/dag`

Checks:

- Handlers are not taking on deep business logic
- Services are not taking on transport-layer response shaping
- Repositories are not leaking HTTP or UI concerns
- MCP tools are not duplicating service logic unnecessarily
- DAG nodes are not leaking into unrelated layers

High-interest patterns:

- Validation logic split between handler and service
- Response shaping done in services
- SQL executed outside repositories
- Cross-layer imports that suggest responsibility drift

### Phase 3: Audit Metadata-Store Access Architecture

Review whether database access matches the intended repository and tenant-scope architecture.

Checks:

- Metadata-store SQL is mostly in repositories
- Direct SQL in services or MCP infrastructure is explicitly justified
- `database.GetTenantScope(ctx)` is the default metadata-store access pattern
- `database.WithoutTenant(...)` usage is narrow, documented, and justified
- RLS-based access patterns align with the separate review in `plans/REVIEW-rls-access-is-enforced.md`

High-interest files include:

- `pkg/services/projects.go`
- `pkg/services/retention_service.go`
- `pkg/mcp/audit.go`
- `pkg/mcp/tools/*`
- any service or handler that uses DB connections directly

### Phase 4: Audit Provenance and Pending-Changes Discipline

Review whether the architecture around ontology provenance and pending changes is preserved.

Checks:

- Ontology mutations preserve `source`, `last_edit_source`, and relevant actor fields
- MCP write paths use the same provenance rules as UI/admin paths
- Pending changes remain the default mechanism where the product expects review before application
- Any bypass of pending changes is intentional, narrow, and explainable

### Phase 5: Audit DAG Boundary Discipline

Review the ontology extraction pipeline architecture.

Checks:

- Node behavior lives in `pkg/services/dag` or closely related services
- Request handlers and MCP tools call service-level abstractions instead of embedding DAG logic
- Recovery, orchestration, and execution responsibilities are not scattered arbitrarily

Special attention:

- Startup/recovery flows
- Incremental processing logic
- Background execution paths

### Phase 6: Audit MCP Tool Architecture

Review whether MCP tools follow the project architecture rather than becoming a parallel backend.

Checks:

- Tools are thin adapters over services, repositories, and access-control helpers
- Tool-specific logic does not fork business rules already owned elsewhere
- Audit, provenance, and access checks are reused consistently
- Tool responses follow shared contract conventions where applicable

### Phase 7: Audit External Contract Consistency

Review cross-cutting API conventions that affect architectural coherence.

Examples:

- Timestamp formatting
- Error shape and error-code conventions
- UUID handling
- JSON naming and response envelopes

This phase should use related review plans such as:

- [REVIEW-ensure-timestamp-format-is-consistent.md](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/plans/REVIEW-ensure-timestamp-format-is-consistent.md#L1)
- [REVIEW-rls-access-is-enforced.md](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/plans/REVIEW-rls-access-is-enforced.md#L1)

### Phase 8: Record Exceptions and Ask Damond

For every questionable deviation, record:

- file and function
- guideline it appears to violate
- why it looks suspicious
- likely impact
- whether it appears intentional

Then explicitly bring the item to Damond before deciding whether it should become:

- a confirmed finding
- an intentional exception
- a guideline clarification

This phase is mandatory. It is part of the review, not optional follow-up.

### Phase 9: Produce Findings and Follow-Up Plans

Summarize results as:

- Critical architectural defects
- Important inconsistencies
- Intentional exceptions
- Items awaiting Damond context

For confirmed issues, propose the correct follow-up document type:

- `plans/ISSUE-*.md` for narrow defects
- `plans/FIX-*.md` for focused remediations
- `plans/PLAN-*.md` for larger architectural corrections

## Acceptance Criteria

This review is complete when all of the following are true:

- The project’s architectural baseline is explicitly documented from `CLAUDE.md` and `AGENTS.md`
- Layer boundaries have been reviewed across handlers, services, repositories, MCP tools, database helpers, and DAG code
- Metadata-store access patterns and tenant-scoping exceptions have been inventoried
- Provenance and pending-changes rules have been reviewed
- Cross-cutting contract consistency has been reviewed at an architectural level
- Every suspicious deviation has been logged
- Every suspicious deviation has either been confirmed by Damond as intentional or classified as a real finding after that check
- Follow-up plans are ready for confirmed issues

## Expected Output Files After Review

The review itself should eventually produce one or more of:

- `plans/ISSUE-*.md`
- `plans/FIX-*.md`
- `plans/PLAN-*.md`

This file is only the review plan.
