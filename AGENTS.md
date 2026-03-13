# Project AGENTS

## Local Environment

- `psql` access to the server metadata store is available via the existing `PG*` environment variables in the shell.
- The metadata database is `ekaya_engine` on `localhost:5432` with user `ekaya` and `PGSSLMODE=disable`.
- Use the existing `PG*` environment variables for database commands instead of hardcoding credentials.

## Architecture Guidelines

- Preserve the layer boundaries described in [CLAUDE.md](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/CLAUDE.md#L1): handlers own transport and HTTP concerns, services own business rules and orchestration, repositories own metadata-store SQL, and models stay as data shapes rather than behavior-heavy objects.
- Keep metadata-store SQL close to the repository layer by default. If a service, MCP tool, or infrastructure component executes SQL directly, that should be rare, justified, and easy to point to in review.
- Treat PostgreSQL RLS as a real security boundary. Metadata-store access should normally use tenant-scoped connections, with `project_id` as the default tenant key. Any other scoping key or parent-join policy must be explicit and documented.
- `database.WithoutTenant(...)` and other non-tenant access paths are exceptions. Use them only for clearly global/bootstrap/admin flows, and make the reason obvious in code or nearby docs.
- Do not add backward-compatibility fallbacks, legacy code paths, transitional storage, or automatic migrations from old behavior unless Damond explicitly asks for them.
- Keep ontology provenance intact. Changes to ontology-backed entities should preserve and correctly update fields such as `source`, `last_edit_source`, and actor-tracking fields when applicable.
- Respect the pending-changes model. Automatically detected schema or data changes should flow through review/approval unless the design explicitly says otherwise.
- Keep DAG behavior in the DAG/service layers. Handlers, MCP tools, and UI-facing code should orchestrate through services rather than reimplementing extraction-pipeline logic.
- MCP tools should stay thin. They should prefer existing service/repository behavior, existing access checks, and existing audit/provenance mechanisms over custom parallel logic.
- Keep external contracts consistent across the project. Timestamp formats, JSON field naming, error shapes, identifier handling, and similar wire-level conventions should not drift endpoint by endpoint.
- Prefer explicit exceptions over silent drift. If code intentionally violates a guideline, leave a short comment, note, or plan entry that explains why.

## Reviewer Protocol

- When a future review finds a likely architectural inconsistency, do not assume it is a defect immediately. Raise it to Damond first, because the deviation may be intentional or tied to prior context not obvious from the code.
- If Damond confirms the deviation is intentional, document the rationale and decide whether the guideline should be clarified rather than forcing the code back into the default pattern.
