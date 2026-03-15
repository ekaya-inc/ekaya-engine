# Project AGENTS

## Local Environment

- `psql` access to the server metadata store is available via the existing `PG*` environment variables in the shell.
- The metadata database is `ekaya_engine` on `localhost:5432` with user `ekaya` and `PGSSLMODE=disable`.
- Use the existing `PG*` environment variables for database commands instead of hardcoding credentials.

## MCP Test Environment

- For MCP/UI sync work, the user manages the dev servers. Do not start or stop them unless explicitly asked.
- Expected local dev commands are `make dev-ui` for the frontend on `http://localhost:5173` and `make dev-server` for the backend on `http://localhost:3443`.
- The shared test project ID is `2b5b014f-191a-41b4-b207-85f7d5c3b04b`.
- MCP testing uses `mcp__test_data__*` tools. Browser verification uses `mcp__claude-in-chrome__*` tools.
- Common project page paths are `/projects/2b5b014f-191a-41b4-b207-85f7d5c3b04b/`, `/projects/2b5b014f-191a-41b4-b207-85f7d5c3b04b/glossary`, and `/projects/2b5b014f-191a-41b4-b207-85f7d5c3b04b/schema`.
- Typical Chrome automation flow is `tabs_context_mcp` -> `tabs_create_mcp` -> `navigate`, then `screenshot`, `find`, `read_page`, `get_page_text`, `left_click`, `type`, and `scroll_to` as needed.
- Use two sync patterns when testing:
  - MCP to UI: make the change via MCP, navigate to the relevant page, and verify it in the UI.
  - UI to MCP: make the change in the UI, then verify the persisted state via MCP.
- In the glossary UI, the Create Term button stays disabled until SQL has been tested successfully.

## MCP Tool Families

- Read tools include `health`, `echo`, `get_schema`, `get_ontology`, `get_context`, `get_column_metadata`, `probe_column`, `probe_columns`, `search_schema`, `list_glossary`, `get_glossary_sql`, `list_pending_changes`, `list_ontology_questions`, `list_approved_queries`, `get_query_history`, `query`, `sample`, `validate`, and `explain_query`.
- Write tools include `update_column`, `delete_column_metadata`, `update_table`, `delete_table_metadata`, `create_glossary_term`, `update_glossary_term`, `delete_glossary_term`, `update_project_knowledge`, `delete_project_knowledge`, `approve_change`, `approve_all_changes`, `reject_change`, `refresh_schema`, `scan_data_changes`, `execute`, and `execute_approved_query`.
- Ontology-question tools include `resolve_ontology_question`, `skip_ontology_question`, `dismiss_ontology_question`, and `escalate_ontology_question`.
- If the AI Data Liaison app is installed, extra tools include `suggest_approved_query`, `suggest_query_update`, `list_query_suggestions`, `approve_query_suggestion`, `reject_query_suggestion`, `create_approved_query`, `update_approved_query`, and `delete_approved_query`.

## Metadata Store Investigation

- There are two databases during testing:
  - The datasource database is accessed through `mcp__test_data__*` tools and contains the business data being queried.
  - The metadata store is `ekaya_engine`, accessed through `psql`, and contains ontology, glossary, project-knowledge, and LLM conversation data.
- The MCP server reads the current active ontology. Use `psql` against `ekaya_engine` to inspect underlying ontology state or stale metadata directly.

## Ontology Tables

- `engine_ontology_dag`: DAG workflow state, status, and current node.
- `engine_dag_nodes`: individual DAG node states.
- `engine_ontologies`: tiered ontology storage such as domain summaries, entity summaries, and column details.
- `engine_ontology_entities`: discovered domain entities and descriptions.
- `engine_ontology_entity_occurrences`: where entities appear across the schema with role semantics.
- `engine_ontology_entity_aliases`: alternate entity names used for matching.
- `engine_ontology_entity_key_columns`: important business columns per entity with synonyms.
- `engine_entity_relationships`: entity-to-entity relationships from FK constraints or inference.
- `engine_ontology_questions`: analysis questions that need clarification.
- `engine_ontology_chat_messages`: ontology refinement chat history.
- `engine_llm_conversations`: verbatim LLM request/response logs.
- `engine_project_knowledge`: project-level facts learned during refinement.
- `engine_business_glossary`: glossary terms and defining SQL.

## Stale Ontology Data

- Check the current ontology first:

```sql
psql -d ekaya_engine -c "
  SELECT id, version, is_active, created_at
  FROM engine_ontologies
  WHERE project_id = '2b5b014f-191a-41b4-b207-85f7d5c3b04b'
  ORDER BY created_at"
```

- When a datasource changes and an old ontology is deleted, tables keyed by `ontology_id` are cleaned up automatically. That includes `engine_ontology_entities`, `engine_entity_relationships`, `engine_ontology_questions`, `engine_ontology_chat_messages`, and `engine_ontology_dag`.
- Tables keyed only by `project_id` can retain stale rows from a prior ontology. That includes `engine_project_knowledge` and `engine_business_glossary`.
- For RLS-protected tables, set `app.current_project_id` before querying them manually:

```sql
psql -d ekaya_engine -c "
  SELECT set_config('app.current_project_id', '2b5b014f-191a-41b4-b207-85f7d5c3b04b', false);
  SELECT fact_type, key, created_at
  FROM engine_project_knowledge
  ORDER BY created_at"

psql -d ekaya_engine -c "
  SELECT set_config('app.current_project_id', '2b5b014f-191a-41b4-b207-85f7d5c3b04b', false);
  SELECT term, created_at
  FROM engine_business_glossary
  ORDER BY created_at"
```

- To detect stale metadata, compare `mcp__test_data__get_context(depth="domain")` with the raw metadata-store queries above. If `psql` shows older project-scoped rows than MCP, the older rows are stale leftovers.
- Only delete stale `engine_project_knowledge` or `engine_business_glossary` rows after verifying their `created_at` values against the active ontology timestamp.

## Test Cleanup

- Remove temporary glossary terms with `mcp__test_data__delete_glossary_term`.
- Remove temporary column metadata with `mcp__test_data__delete_column_metadata`.
- Remove temporary table metadata with `mcp__test_data__delete_table_metadata`.

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
