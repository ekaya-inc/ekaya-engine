Status: DRAFT
Created: 2026-03-16

# PLAN: Vetted Query History and Evaluation Persistence

Parent: `plans/MASTER-PLAN-sql-evaluator-data-guardian.md`

## Objective

Persist evaluator outcomes so the system can distinguish unvetted SQL from vetted SQL, reuse trusted outcomes when appropriate, and support future external-request vetting and query reuse.

## Why This Is a Separate Plan

Core evaluation and client migration should not be blocked on storage design, but the long-term system needs durable evaluation metadata. This plan introduces that storage model after the evaluator contract exists and before every client is migrated.

## Current Repo Touchpoints

Current history behavior is limited:

- `pkg/models/query_history.go`
  - query history stores successful executions only
  - it stores natural language, SQL, timing, row count, and light classification
- MCP query tools record to query history asynchronously after successful execution
- there is no durable evaluation verdict, no policy version, no SQL fingerprint, and no schema/ontology compatibility marker

That means the repo cannot yet answer:

- Has this SQL already been vetted.
- Was it vetted for this mode.
- Is that vetting still valid after schema or ontology changes.
- Did the query previously pass Data Guardian rules.

## Scope

- Define persistent storage for evaluation outcomes.
- Link those outcomes to query history and query-management flows.
- Define safe reuse rules for past evaluations.

## Non-Goals

- Full query-result caching.
- Full request-routing engine for external clients.
- Long-term analytics over evaluation outcomes beyond what is required for rollout.

## Recommended Design Direction

Create a new evaluation store instead of overloading existing query history with every new concern.

Recommended approach:

- Add a new table such as `engine_sql_evaluations`.
- Keep `engine_query_history` focused on execution history and user feedback.
- Link history entries to evaluation records where useful.

This separation is cleaner because:

- one SQL string can be evaluated multiple times under different modes
- one SQL string can be evaluated before it is executed
- security findings and evaluation evidence are not the same thing as execution history

## Recommended Evaluation Record Shape

Suggested stored fields:

- `id`
- `project_id`
- `datasource_id`
- `query_history_id` nullable
- `query_id` nullable for approved or pending query records
- `source` such as glossary, mcp_query, validate_tool, pending_query_review
- `mode`
- `raw_sql`
- `normalized_sql`
- `sql_hash`
- `verdict`
- `issues_json`
- `warnings_json`
- `security_findings_json`
- `execution_summary_json`
- `policy_version`
- `schema_fingerprint`
- `ontology_fingerprint`
- `created_by`
- `created_at`

The storage shape does not need to be perfect on day one, but it must support:

- exact re-evaluation traceability
- safe reuse checks
- admin review and debugging

## Canonicalization and Fingerprints

This plan must include a clear strategy for when a prior vetted result can be reused.

Recommended minimum reuse key:

- `project_id`
- `datasource_id`
- `mode`
- `sql_hash`
- `schema_fingerprint`
- `ontology_fingerprint`
- `policy_version`

Recommended policy:

- reuse only when all of those match
- never blindly reuse for modifying SQL
- be conservative for security-sensitive modes

## Privacy Constraint

Do not persist raw result rows by default.

Store compact execution summaries instead:

- row count
- output column names and types
- scalar summary when useful
- redacted or omitted sample summaries for sensitive modes

Evaluation persistence should not become a new source of data leakage.

## Relationship to Vetted Historical Queries

This plan is the foundation for the user-facing idea of vetted historical queries.

The eventual system should be able to say:

- this query executed successfully before
- it was evaluated under mode `X`
- it passed or failed policy version `Y`
- it is still valid or needs re-evaluation because the schema or ontology changed

That requires evaluation persistence, not just execution history.

## Relationship to Admin-Managed Approved Queries

Approved and pending queries should eventually be able to reference evaluation outcomes directly. That allows:

- better reviewer context
- clearer approval decisions
- faster triage of failed or risky query suggestions

This plan should make that link easy without forcing all review flows to migrate at once.

## Implementation Tasks

- [ ] Add a new persistence model for SQL evaluations, preferably in a dedicated table.
- [ ] Add repository methods for create, lookup, and fetch-by-history/query linkage.
- [ ] Add canonicalization and SQL-hash strategy.
- [ ] Add schema and ontology fingerprint strategy suitable for reuse decisions.
- [ ] Add service-layer APIs for storing and retrieving evaluation outcomes.
- [ ] Add linkage from query history entries and query-management flows where appropriate.
- [ ] Add unit or repository tests for persistence and reuse-key behavior.

## Completion Criteria

- The repo can persist evaluator outcomes independently of raw execution history.
- A caller can look up whether equivalent SQL has already been vetted for a compatible context.
- Evaluation persistence is safe enough to support future external-request vetting and vetted-query reuse without storing raw sensitive results.
