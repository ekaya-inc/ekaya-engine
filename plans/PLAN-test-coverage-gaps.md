# PLAN: Address Test Coverage Gaps

**Status:** READY
**Branch:** TBD
**Context:** After reviving 19k lines of ignored tests, an audit revealed 35 files with zero test coverage and several components below 50%. This plan prioritizes closing gaps by risk and value.

## Current State

| Metric | Value |
|---|---|
| Overall grade | B- |
| Test functions | ~2,330 |
| Test:Production ratio | 1.84x |
| Zero-coverage files | 35 |
| Weakest areas | DAG nodes, repositories, adapters, MCP tools |

## Prioritization Criteria

- **P0 (Critical):** Code that runs in production, handles user data, or has complex logic with no safety net
- **P1 (Important):** Code with moderate complexity or that has broken before
- **P2 (Nice-to-have):** Simple CRUD, deprecated code, or utility wrappers

---

## P0: Critical Gaps

### 1. Repository Layer — Integration Tests for 9 Untested Repos

The repository layer has 0% coverage in `go test -short` because all tests require Docker. 9 repositories have no tests at all — not even integration tests.

**Files to add tests for:**
- [ ] `column_metadata_repository.go` — Core to ontology pipeline; every service depends on it
- [ ] `query_history_repository.go` — Stores MCP query audit trail
- [ ] `mcp_config_repository.go` — Controls which MCP tools are enabled
- [ ] `mcp_audit_repository.go` — MCP usage tracking
- [ ] `ai_config_repository.go` — Controls LLM provider config per project
- [ ] `alert_repository.go` — Alert storage
- [ ] `alert_trigger_repository.go` — Alert trigger conditions
- [ ] `installed_app_repository.go` — App installation state
- [ ] `audit_page_repository.go` — Audit page queries

**Approach:** Follow existing integration test patterns in `pkg/repositories/*_integration_test.go`. Each repo test uses `testhelpers.GetEngineDB(t)` with a unique project ID. Tag with `//go:build integration`.

**Effort:** Medium. Each repo test is ~100-200 lines of CRUD verification. `column_metadata_repository` is the most important and should be first.

### 2. MCP Tools — Coverage from 19.7% to 50%+

MCP tools are the primary user-facing interface for the product. Current coverage is poor.

**Key untested tools:**
- [ ] `get_context` / `get_ontology` — The main context tools developers use
- [ ] `update_column` / `update_table` — Ontology mutation tools
- [ ] `scan_data_changes` — Data change detection
- [ ] `suggest_approved_query` / `update_approved_query` — Query management
- [ ] `glossary tools` — Business glossary via MCP

**Approach:** Unit tests with mock services. Follow patterns in existing `pkg/mcp/tools/probe_test.go` (24 tests). Each tool needs: valid input, invalid input, error propagation, and JSON error response (not Go error) for validation failures per CLAUDE.md rule #6.

**Effort:** Medium-High. ~30-40 test functions needed across the tool surface.

### 3. Ontology Pipeline Services — Zero Coverage on Core Business Logic

These services implement the ontology extraction and refinement pipeline — the core product differentiator.

- [ ] `ontology_builder.go` — Builds ontology from extracted features
- [ ] `ontology_chat.go` — Ontology refinement chat (user-facing)
- [ ] `ontology_question.go` — Question generation and management
- [ ] `knowledge_parsing.go` — Parses domain knowledge from user input
- [ ] `precedence_service.go` — Controls which provenance can override which

**Approach:** Unit tests with mock repos and LLM factory. These are pure service-layer logic. `precedence_service` is the simplest (pure logic, no I/O). `knowledge_parsing` is also pure parsing. `ontology_question` and `ontology_chat` require LLM mocks.

**Effort:** High. These services have complex orchestration logic. ~200-300 lines per service test file.

---

## P1: Important Gaps

### 4. DAG Node Wrappers — 7 of 11 Untested

DAG nodes are thin wrappers that call services, but they handle error propagation, required field validation, and node state transitions. Bugs here cause silent DAG failures.

- [ ] `column_enrichment_node.go`
- [ ] `column_feature_extraction_node.go`
- [ ] `fk_discovery_node.go`
- [ ] `ontology_finalization_node.go`
- [ ] `pk_match_discovery_node.go`
- [ ] `relationship_discovery_node.go`
- [ ] `node_executor.go`

**Approach:** Each node test verifies: (1) required fields validated, (2) service called with correct args, (3) errors propagated, (4) success path works. Follow patterns in `dag/knowledge_seeding_node_test.go` and `dag/table_feature_extraction_node_test.go`.

**Effort:** Low-Medium. Each node is ~50-100 lines of production code. Tests are straightforward mock-and-verify. `node_executor` is the most complex (parallel execution logic).

### 5. Handler Layer — 8 Untested Handlers

Handlers are thin (per architecture rules) but test coverage ensures request parsing, auth checks, and response formatting work correctly.

- [ ] `queries.go` — Approved query CRUD (user-facing API)
- [ ] `users.go` — User management
- [ ] `mcp_config.go` — MCP tool configuration toggle
- [ ] `ontology_enrichment_handler.go` — Ontology refinement endpoints
- [ ] `config_project.go` — Project-level settings
- [ ] `installed_app.go` — App installation endpoints
- [ ] `audit_page_handler.go` — Audit page API
- [ ] `response.go` — Response utility functions (pure, easy to test)

**Approach:** Follow patterns in `pkg/handlers/glossary_handler_test.go` — mock the service interface, verify HTTP status codes and response shapes. `response.go` tests are pure function tests.

**Effort:** Medium. ~100-150 lines per handler test file.

### 6. LLM Package — Coverage from 36.7% to 60%+

The LLM package wraps provider APIs and handles retries, token counting, and conversation logging.

- [ ] Error handling paths (rate limits, context length exceeded, provider down)
- [ ] Conversation logging (verify logs are written correctly)
- [ ] Token counting edge cases
- [ ] Streaming client paths

**Approach:** Unit tests with HTTP test server mocking provider responses.

**Effort:** Medium. Most untested code is in error paths and edge cases.

---

## P2: Nice-to-Have

### 7. Adapter Layer (postgres/mssql)

Currently at 6%/0.1%. These are complex integration-heavy packages that require real database connections.

- [ ] Schema discovery edge cases (unusual data types, large schemas)
- [ ] Query executor (parameterized queries, explain plans)
- [ ] Connection management (pool lifecycle, timeout handling)

**Approach:** Integration tests using the test container. Tag with `//go:build integration && (postgres || all_adapters)`.

**Effort:** High. Adapter tests are inherently integration-heavy and slow. Lower priority because the adapters are exercised indirectly through service and handler integration tests.

### 8. Central Client — 0% Coverage

`pkg/central/client.go` wraps HTTP calls to ekaya-central.

- [ ] Request construction (auth headers, URLs)
- [ ] Error handling (non-200 responses, network errors)
- [ ] Response parsing

**Approach:** Unit tests with `httptest.Server`. Relatively straightforward.

**Effort:** Low. ~50-100 lines of tests.

### 9. Remaining Services

- [ ] `query.go` — Query service (approved query execution)
- [ ] `mcp_audit_service.go` — MCP audit logging
- [ ] `audit_page_service.go` — Audit page queries
- [ ] `tenant_context.go` — Tenant context setup (mostly infrastructure)
- [ ] `incremental_dag_prompts.go` — Prompt templates for incremental enrichment
- [ ] `deterministic_relationship_service.go` — Deprecated; delete instead of test

**Approach:** Unit tests. `deterministic_relationship_service.go` should be deleted (already marked deprecated, tests were deleted in the revive plan).

---

## Implementation Order

| Phase | Items | Est. Test Functions | Target Coverage Impact |
|---|---|---|---|
| **Phase 1** | P0 #1 (column_metadata_repo) + P0 #3 (precedence, knowledge_parsing) + P2 delete deprecated | ~30 | Quick wins on critical gaps |
| **Phase 2** | P0 #2 (MCP tools) + P1 #4 (DAG nodes) | ~70 | MCP tools 19%→50%, DAG 41%→70% |
| **Phase 3** | P0 #1 (remaining repos) + P0 #3 (ontology_builder, chat, question) | ~50 | Services 60%→70% |
| **Phase 4** | P1 #5 (handlers) + P1 #6 (LLM) | ~50 | Handlers 30%→50%, LLM 36%→60% |
| **Phase 5** | P2 (adapters, central, remaining) | ~30 | Diminishing returns cleanup |

## Notes

- Each phase should pass `make check` before moving to the next
- Deprecated `deterministic_relationship_service.go` should be deleted (production code + ignore tag already removed)
- Repository tests are integration-only by design (they test SQL against real Postgres) — this is intentional, not a gap in unit test philosophy
- `pkg/auth` at 49.3% is acceptable — OAuth flows are inherently hard to unit test and are covered by manual testing
