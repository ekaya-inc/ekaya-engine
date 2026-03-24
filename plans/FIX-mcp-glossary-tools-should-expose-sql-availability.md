# FIX: MCP glossary tools should expose whether a term actually has SQL

**Status:** Open
**Date:** 2026-03-24

## Problem

The MCP glossary surface mixes two valid kinds of glossary terms:

- SQL-backed terms that can be composed into queries
- definition-only terms that are useful business vocabulary but do not have a defining SQL statement

That distinction is not explicit in the MCP read contract today.

Observed current behavior:

- `create_glossary_term` explicitly allows SQL to be omitted
- `update_glossary_term` also allows terms to exist without SQL
- `GlossaryService.TestSQL(...)` validates glossary SQL as a single-row query, so many dimension-style concepts are definition-only by design
- `list_glossary` returns term, definition, aliases, and enrichment status, but not whether the term has SQL
- `get_glossary_sql` is described as returning "the SQL definition for a specific business term", but the response shape allows `defining_sql` to be empty without any explicit explanation

During MCP work on `the_look`, this showed up directly with `Acquisition Channel`.

That term is a valid glossary concept for the tutorial, but its natural representation is a dimension (`users.traffic_source`), not a single-row metric query. The right move was to create it as a definition-only term. The current tool surface made that awkward:

- `list_glossary` gave no signal that the term was definition-only
- `get_glossary_sql` would have returned a successful response with empty `defining_sql`
- the tool descriptions still lean heavily toward "business metric" phrasing, which nudges MCP clients toward assuming every glossary term must be SQL-backed

## Why this matters

This is primarily an MCP-client usability problem.

An LLM client using only tool contracts sees mixed signals:

- the write tools say SQL is optional
- the read tool is named `get_glossary_sql`
- the read tool description implies every term has SQL
- the list tool does not say which terms are composable vs definition-only

That creates several bad outcomes:

- agents may treat valid definition-only terms as broken or incomplete
- agents may keep retrying SQL generation for terms that should not have SQL
- agents cannot reliably pre-filter glossary terms for query-composition use
- demos/tutorials that intentionally include business vocabulary terms become harder to explain with the current surface

## Existing implementation context

### `pkg/mcp/tools/glossary.go`

Current MCP response shapes:

- `listGlossaryResponse` includes:
  - `term`
  - `definition`
  - `aliases`
  - enrichment fields
- `getGlossarySQLResponse` includes:
  - `term`
  - `definition`
  - `defining_sql`
  - `base_table`
  - `output_columns`
  - aliases and enrichment fields

Neither response includes an explicit `has_sql` or similar flag.

### `pkg/services/glossary_service.go`

`TestSQL(...)` explicitly enforces single-row output for glossary SQL:

- glossary SQL is treated as a metric-like definition
- multi-row dimension-style queries are rejected

So definition-only terms are not just allowed accidentally. They are a natural outcome of the current glossary model.

## Intended fix

Keep this narrow. Do not redesign the glossary system.

Add an explicit SQL-availability signal to the MCP glossary read surface.

### Recommended contract change

Add `has_sql` to:

- `list_glossary`
- `get_glossary_sql`

Recommended semantics:

- `has_sql = true` when `defining_sql` is non-empty
- `has_sql = false` when the term is definition-only or enrichment has not produced SQL

For `get_glossary_sql`, keep the successful lookup behavior, but make the empty-SQL case explicit:

```json
{
  "term": "Acquisition Channel",
  "definition": "Original acquisition source recorded on users.traffic_source",
  "has_sql": false,
  "defining_sql": ""
}
```

That is much easier for MCP clients to reason about than an unexplained empty string.

### Description cleanup

Update tool descriptions so they no longer imply every glossary term is a metric:

- `list_glossary`: mention that some terms are definition-only
- `get_glossary_sql`: mention that some valid terms may not have a SQL definition and that the response will indicate `has_sql`
- `create_glossary_term`: replace metric-only phrasing with "business term" wording and note that SQL-backed terms should normally be single-row metric-style definitions
- `update_glossary_term`: same clarification

The descriptions should reflect the real model instead of pushing clients toward a stricter assumption than the code enforces.

## File-by-file changes

### 1. `pkg/mcp/tools/glossary.go`

- [ ] Add `HasSQL bool \`json:"has_sql"\`` to `listGlossaryResponse`
- [ ] Add `HasSQL bool \`json:"has_sql"\`` to `getGlossarySQLResponse`
- [ ] Populate `HasSQL` from `term.DefiningSQL != ""`
- [ ] Update glossary tool descriptions to reflect definition-only terms

### 2. `pkg/services/mcp_tools_registry.go`

- [ ] Tighten the one-line descriptions for:
  - `list_glossary`
  - `get_glossary_sql`
  - `create_glossary_term`
  - `update_glossary_term`

The registry descriptions should stay aligned with the MCP tool descriptions.

### 3. `pkg/services/mcp_tool_loadouts.go`

- [ ] Update the matching tool descriptions in `AllToolsOrdered`

No loadout membership changes are needed.

## Tests to update

### `pkg/mcp/tools/glossary_test.go`

- [ ] Update `list_glossary` response-shape tests to expect `has_sql`
- [ ] Update `get_glossary_sql` response-shape tests to expect `has_sql`
- [ ] Add explicit coverage for a definition-only term returning `has_sql = false`

### `pkg/mcp/tools/glossary_integration_test.go`

- [ ] Add an integration test for a glossary term created without SQL
- [ ] Verify `list_glossary` marks it as `has_sql = false`
- [ ] Verify `get_glossary_sql` returns success with `has_sql = false` and empty `defining_sql`

## Non-goals

- [ ] Do not redesign glossary terms into separate metric and dimension entity types in this task
- [ ] Do not change the underlying single-row SQL validation rule in this task
- [ ] Do not rename `get_glossary_sql` in this task
- [ ] Do not add SQL auto-generation for definition-only terms in this task

## Expected outcome

After this fix:

- MCP clients can distinguish SQL-backed glossary terms from definition-only terms
- definition-only terms stop looking like broken glossary entries
- glossary tool descriptions match actual behavior
- agents can use glossary terms more reliably for both vocabulary grounding and query composition
