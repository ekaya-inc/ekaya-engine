# Plan: Go Unit Test Coverage

**Status: PENDING**

## Objective

Increase Go unit test coverage from 35.4% to improve confidence in edge cases and error paths that integration tests don't easily reach. Focus on complementing (not duplicating) existing integration tests.

## Philosophy

Integration tests cover the happy paths through real databases. Unit tests here target:
- Error handling branches (malformed input, nil values, unexpected states)
- Pure functions and data transformations with no DB dependency
- Validation logic and edge cases
- Interface boundaries where mocks are natural

## Environment

- Working directory: `/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine`
- Run tests: `go test ./... -short -count=1`
- Test framework: `stretchr/testify` (assert/require)
- Mocks: Hand-rolled (see existing patterns in `pkg/llm/mock.go`, `pkg/services/dag/*_test.go`)
- Existing test helpers: `pkg/testhelpers/` (integration DB containers)

## Existing Test Patterns to Follow

Read these files before writing any tests to understand conventions:

- `pkg/llm/errors_test.go` — Pure unit test with table-driven tests
- `pkg/llm/client_test.go` — httptest server for HTTP client testing
- `pkg/services/dag/glossary_discovery_node_test.go` — Hand-rolled interface mocks, DAG node testing pattern
- `pkg/services/precedence_service.go` — Small pure-logic service ideal for unit tests
- `pkg/services/column_filter_test.go` — Service unit tests with mock repos

### Key patterns

Hand-rolled mocks follow a consistent pattern:
```go
type mockFoo struct {
    barFunc func(ctx context.Context, id uuid.UUID) (string, error)
}
func (m *mockFoo) Bar(ctx context.Context, id uuid.UUID) (string, error) {
    if m.barFunc != nil {
        return m.barFunc(ctx, id)
    }
    return "", nil
}
```

Table-driven tests:
```go
tests := []struct {
    name    string
    input   SomeType
    want    SomeType
    wantErr bool
}{...}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) { ... })
}
```

## Implementation Checklist

### Phase 1 — Pure Functions & Small Utilities (quick wins)

#### `pkg/jsonutil/flexible.go` (26 lines, 0 tests)

File to create: `pkg/jsonutil/flexible_test.go`

- [x] Add table-driven tests for `FlexibleStringValue`: string input `"hello"`, integer `42`, float `3.14`, boolean `true`/`false`, null, empty `json.RawMessage`, large integer (precision edge), nested object/array fallback to raw string
- [x] Run `go test ./pkg/jsonutil/... -count=1` and confirm all tests pass

#### `pkg/services/precedence_service.go` (65 lines, 0 tests)

File to create: `pkg/services/precedence_service_test.go`

- [x] Read `pkg/services/precedence_service.go` and `pkg/models/provenance.go` to confirm provenance constants
- [x] Add tests for `GetPrecedenceLevel`: verify Manual=3, MCP=2, Inference=1, unknown=0
- [x] Add tests for `GetEffectiveSource`: updatedBy set vs nil, empty string updatedBy
- [x] Add tests for `CanModify`: Manual can modify all, MCP can modify MCP+Inference but not Manual, Inference can only modify Inference, unknown source behavior
- [x] Run `go test ./pkg/services/... -run TestPrecedence -count=1` and confirm all tests pass

#### `pkg/handlers/response.go` (26 lines, 0 tests)

File to create: `pkg/handlers/response_test.go`

- [x] Add test for `ErrorResponse`: verify Content-Type header, status code, JSON body shape (`error` and `message` keys)
- [x] Add test for `WriteJSON`: verify Content-Type, status 200 does not call WriteHeader explicitly, non-200 does, JSON body matches input struct
- [x] Add test for `WriteJSON` with unencodable data (e.g., `chan int`) — verify error is returned
- [x] Run `go test ./pkg/handlers/... -run TestResponse -count=1` and confirm all tests pass

### Phase 2 — LLM Package Edge Cases (streaming, tool execution, factory)

#### `pkg/llm/streaming.go` — Pure helper functions (494 lines, 0 tests)

File to create: `pkg/llm/streaming_test.go`

The streaming client's `streamIteration` and `StreamWithTools` require a real OpenAI stream so skip those. Focus on the pure helper functions that are unit-testable:

- [x] Read `pkg/llm/streaming.go` fully to identify all pure functions
- [x] Add tests for `parseTextToolCalls`: valid XML tool call, multiple tool calls, malformed JSON inside XML, no matches, nested braces in arguments
- [x] Add tests for `cleanModelOutput`: remove `<think>` blocks, remove `<tool_call>` blocks, collapse triple+ newlines, content with no markup passes through unchanged
- [x] Add tests for `buildOpenAIMessages`: empty messages, system prompt + messages, messages with tool calls, messages with tool call IDs (tool role)
- [x] Add tests for `buildOpenAITools`: empty tools returns nil, single tool, tool with nested parameters JSON
- [x] Run `go test ./pkg/llm/... -run TestStreaming -count=1` and confirm all tests pass

#### `pkg/llm/tool_executor.go` — Tool dispatch and validation (401 lines, 0 tests)

File to create: `pkg/llm/tool_executor_test.go`

The tool executor calls repository interfaces — mock them to test dispatch logic and argument validation without a DB.

- [x] Read `pkg/llm/tool_executor.go` fully to identify all tool functions and their argument structs
- [x] Create mock implementations for `OntologyRepository`, `KnowledgeRepository`, `SchemaRepository`, and `datasource.QueryExecutor` (only the methods actually called)
- [x] Add test for `ExecuteTool` dispatch: known tool names route correctly, unknown tool returns error
- [x] Add tests for `queryColumnValues`: missing table_name, missing column_name, limit defaults to 10, limit capped at 100, nil queryExecutor returns error JSON, query execution error returns error JSON (not Go error)
- [x] Add tests for `querySchemaMetadata`: valid table filter, empty table name returns all tables, repo error propagates
- [x] Add tests for `storeKnowledge`: missing fact_type, missing value, invalid fact_type rejected, valid fact types accepted (all 5)
- [x] Add tests for `updateColumn`: missing table/column, invalid semantic_type rejected, valid semantic types, update existing column vs create new, businessName adds to synonyms
- [x] Run `go test ./pkg/llm/... -run TestOntologyToolExecutor -count=1` and confirm all tests pass

#### `pkg/llm/factory.go` — Client factory (124 lines, 0 tests)

File to create: `pkg/llm/factory_test.go`

- [x] Create a mock `AIConfigProvider` that returns configurable `*models.AIConfig`
- [x] Add test for `CreateForProject`: config provider error propagates, valid config creates client, recorder wraps client when set
- [x] Add test for `CreateEmbeddingClient`: uses effective embedding URL/key fallback
- [x] Add test for `CreateStreamingClient`: config provider error propagates, valid config creates streaming client
- [x] Add test for `SetRecorder`: nil recorder disables wrapping
- [x] Run `go test ./pkg/llm/... -run TestClientFactory -count=1` and confirm all tests pass

### Phase 3 — DAG Nodes (complete the set)

All 4 existing node tests follow the same pattern from `glossary_discovery_node_test.go`. Each untested node should get the same treatment: mock the service interface + dagRepo, test Execute success, Execute with service error, Execute with nil ontology ID (where applicable), and progress reporting.

#### Untested DAG nodes (7 files, ~500 lines total)

File to create: `pkg/services/dag/column_enrichment_node_test.go`

- [x] Mock `ColumnEnrichmentMethods` interface
- [x] Add test: Execute success — service returns enriched tables, progress reported
- [x] Add test: Execute with service error — error propagates
- [x] Add test: progress callback is invoked by the service mock
- [x] Run `go test ./pkg/services/dag/... -run TestColumnEnrichment -count=1`

File to create: `pkg/services/dag/column_feature_extraction_node_test.go`

- [x] Read `pkg/services/dag/column_feature_extraction_node.go` to identify its service interface
- [x] Mock the interface and add success/error/progress tests following the same pattern
- [x] Run `go test ./pkg/services/dag/... -run TestColumnFeature -count=1`

File to create: `pkg/services/dag/fk_discovery_node_test.go`

- [x] Read `pkg/services/dag/fk_discovery_node.go` to identify its service interface
- [x] Mock the interface and add success/error tests
- [x] Run `go test ./pkg/services/dag/... -run TestFKDiscovery -count=1`

File to create: `pkg/services/dag/pk_match_discovery_node_test.go`

- [x] Read `pkg/services/dag/pk_match_discovery_node.go` to identify its service interface
- [x] Mock the interface and add success/error tests
- [x] Run `go test ./pkg/services/dag/... -run TestPKMatch -count=1`

File to create: `pkg/services/dag/relationship_discovery_node_test.go`

- [x] Read `pkg/services/dag/relationship_discovery_node.go` to identify its service interface
- [x] Mock the interface and add success/error tests
- [x] Run `go test ./pkg/services/dag/... -run TestRelationshipDiscovery -count=1`

File to create: `pkg/services/dag/ontology_finalization_node_test.go`

- [x] Read `pkg/services/dag/ontology_finalization_node.go` to identify its service interface
- [x] Mock the interface and add success/error tests
- [x] Run `go test ./pkg/services/dag/... -run TestOntologyFinalization -count=1`

File to create: `pkg/services/dag/node_executor_test.go`

- [x] Add tests for `BaseNode.ReportProgress`: with nil nodeID (uuid.Nil) returns nil, with valid nodeID calls dagRepo
- [x] Add tests for `NewExecutionContext`: with nil OntologyID, with valid OntologyID
- [x] Run `go test ./pkg/services/dag/... -run TestBaseNode -count=1`

- [x] Run `go test ./pkg/services/dag/... -count=1` and confirm ALL dag tests pass

### Phase 4 — Service Spot Checks (targeted edge cases in large packages)

These are services that have integration tests or partial coverage. Add unit tests specifically for edge cases and error paths.

#### `pkg/services/tenant_context.go` (67 lines, 0 tests)

File to create: `pkg/services/tenant_context_test.go`

- [x] Add test for `WithInferredProvenanceWrapper`: wraps getTenantCtx, adds provenance, cleanup function is passed through, error from inner function propagates
- [x] Run `go test ./pkg/services/... -run TestTenantContext -count=1`

#### `pkg/services/knowledge_parsing.go` — Input validation (0 tests)

File to create: `pkg/services/knowledge_parsing_test.go`

- [ ] Add test for `ParseAndStore` with empty/whitespace-only text — verify error without hitting LLM
- [ ] Add test for `ParseAndStore` with LLM factory error — verify error propagates
- [ ] Mock `LLMClientFactory` to return a `MockLLMClient` that returns a known JSON response, verify facts are created via mock `KnowledgeService`
- [ ] Run `go test ./pkg/services/... -run TestKnowledgeParsing -count=1`

#### `pkg/services/ontology_question.go` — Orchestration edge cases (0 tests)

- [ ] Read `pkg/services/ontology_question.go` to understand its interface and dependencies
- [ ] If it contains pure validation or transformation logic, add targeted unit tests
- [ ] If it's purely orchestration (calls other services), skip — integration tests cover this

#### `pkg/services/audit_page_service.go` — Pagination edge cases (0 tests)

- [ ] Read `pkg/services/audit_page_service.go` to understand its interface
- [ ] If it contains pagination logic (offset/limit calculation, boundary checks), add unit tests for those pure functions
- [ ] If it's a thin wrapper over the repository, skip

#### `pkg/services/mcp_audit_service.go` — (0 tests)

- [ ] Read `pkg/services/mcp_audit_service.go`
- [ ] If it contains validation or transformation beyond simple repo passthrough, add targeted tests
- [ ] If it's a thin wrapper, skip

### Phase 5 — Handler Error Paths

Handlers have 47.4% coverage. The untested handlers are mostly CRUD wrappers, but they contain request parsing and error response logic worth spot-checking.

#### `pkg/handlers/response.go` — Already covered in Phase 1

#### Spot-check: Request parsing edge cases in existing handlers

Pick 2-3 handlers that have test files but may not test error paths:

- [ ] Read `pkg/handlers/datasources_test.go` and identify if UUID parse errors (invalid `:id` path param) are tested
- [ ] Read `pkg/handlers/glossary_test.go` (if exists) and identify if malformed JSON body errors are tested
- [ ] For any untested error paths found, add targeted tests to the existing test files (do not create new files)
- [ ] Run `go test ./pkg/handlers/... -count=1` and confirm all tests pass

### Phase 6 — Central Client Error Handling

#### `pkg/central/client.go` — HTTP client error paths

- [ ] Read `pkg/central/client.go` and existing `client_test.go` fully
- [ ] Identify untested error paths: non-200 status codes, malformed JSON responses, timeout handling, URL construction edge cases
- [ ] Add tests to existing `pkg/central/client_test.go` for: HTTP 500 error response handling, HTTP 404 handling, malformed JSON body, empty response body
- [ ] Use `httptest.NewServer` to mock the central API (follow existing test patterns)
- [ ] Run `go test ./pkg/central/... -count=1` and confirm all tests pass

## Completion Criteria

- All new test files created and passing
- `go test ./... -short -count=1` exits with 0
- No existing tests broken
- Do not create documentation, README, or architecture files
