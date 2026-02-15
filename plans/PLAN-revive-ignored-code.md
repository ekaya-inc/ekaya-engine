# PLAN: Revive Ignored Test Code

**Status:** REVIEWED — Ready for implementation
**Branch:** TBD
**Context:** 16 test files (~30,856 lines) were excluded via `//go:build ignore`. Each was classified as dead code, duplicated, or missing coverage.

## Summary

| # | File | Lines | Classification | Fix Effort | Action |
|---|------|-------|---------------|------------|--------|
| 1 | `pkg/services/incremental_dag_service_test.go` | 439 | REVIVE | Low | Fix mocks |
| 2 | `pkg/services/ontology_finalization_test.go` | 947 | REVIVE | Low-Med | Fix constructor + 3 tests need ColumnMetadata mock |
| 3 | `pkg/services/table_feature_extraction_test.go` | 832 | REVIVE | Medium | Fix constructor + Metadata→ColumnMetadata |
| 4 | `pkg/services/column_feature_extraction_test.go` | 3,706 | REVIVE | High | 61 tests; constructor + SampleValues + Metadata removal |
| 5 | `pkg/services/column_enrichment_test.go` | 2,654 | REVIVE | Medium | 35 tests; constructor + ColumnMetadata mock |
| 6 | `pkg/services/glossary_service_test.go` | 4,946 | REVIVE | High | 111 tests; constructor + entity repo removal + SQL helpers uncovered |
| 7 | `pkg/services/fk_semantic_evaluation_test.go` | 773 | REVIVE | Low | Constructor matches; remove SampleValues from 1 test |
| 8 | `pkg/services/relationship_candidate_collector_test.go` | 2,620 | REVIVE | Low | Fix mock return value signatures |
| 9 | `pkg/services/relationship_discovery_service_test.go` | 1,061 | REVIVE | Medium | Add columnMetadataRepo param |
| 10 | `pkg/services/schema_integration_test.go` | 609 | REVIVE | Low | Fix constructor + valuable integration tests |
| 11 | `pkg/handlers/glossary_integration_test.go` | 877 | REVIVE | Medium | Full CRUD coverage missing from active tests |
| 12 | `pkg/services/relationship_discovery_service_integration_test.go` | 1,753 | DELETE | — | Entity repos removed from codebase |
| 13 | `pkg/services/dag_relationship_discovery_integration_test.go` | 1,080 | DELETE | — | Entity repos removed from codebase |
| 14 | `pkg/services/pk_match_integration_test.go` | 660 | DELETE | — | Entity repos removed; service deprecated |
| 15 | `pkg/services/deterministic_relationship_service_test.go` | 7,093 | DELETE | — | Service explicitly deprecated; 90% incorrect heuristics replaced by LLM |
| 16 | `pkg/mcp/tools/probe_column_integration_test.go` | 806 | DELETE | — | Duplicated by 24 active unit tests in probe_test.go |

**To delete:** 5 files, 11,392 lines (dead code / duplicated)
**To revive:** 11 files, 19,464 lines

## Common Breakage Pattern

Nearly all REVIVE files share the same root cause: the **column schema refactor** that:
1. Removed `SchemaColumn.Metadata` field (column features moved to `ColumnMetadata` model)
2. Removed `SchemaColumn.SampleValues` field
3. Added `ColumnMetadataRepository` parameter to most service constructors
4. Removed entity concept (`OntologyEntity`, `EntityRelationship` repos deleted)
5. Removed `UpdateEntitySummary` methods from `OntologyRepository`

## Detailed File Analysis

### 1. `pkg/services/incremental_dag_service_test.go` (439 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `incremental_dag_service.go`. No active tests cover this service's logic (only mocked in `change_review_service_test.go` for the other side).

All 8 tests are missing coverage:
- `TestToTitleCase` — `toTitleCase` still exists (line 328), zero coverage
- `TestProcessChange_SkipsWithoutAIConfig` — guard clause, no coverage
- `TestProcessChange_SkipsWithAIConfigNone` — guard clause, no coverage
- `TestProcessChange_SkipsUnknownChangeType` — guard clause, no coverage
- `TestProcessChanges_GroupsByType` — batch nil/empty handling, no coverage
- `TestProcessEnumUpdate_MergesValues` — enum merge logic, no coverage
- `TestProcessEnumUpdate_RespectsExistingValues` — deduplication, no coverage
- `TestProcessEnumUpdate_SkipsDueToPrecedence` — precedence blocking, no coverage

**Fix effort:** Low. Struct fields (`columnMetadataRepo`, `changeReviewSvc`, `aiConfigSvc`) still exist. Mock interfaces may need method signature updates but test logic is structurally sound.

### 2. `pkg/services/ontology_finalization_test.go` (947 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `ontology_finalization.go`. `Finalize()` method exists (line 56). The only other test file referencing this service is `ontology_dag_service_test.go`, which only checks interface satisfaction — it doesn't test any finalization logic.

Two categories of breakage:

**Constructor signature change (affects all 12 tests):**
Tests call `NewOntologyFinalizationService` with 6 args. Current constructor takes 7 args: added `columnMetadataRepo` (param 3), `conversationRepo` (param 4), and `getTenantCtx` (param 6). Fix is mechanical — update all constructor calls.

**`SchemaColumn.Metadata` field removed (affects 3 tests):**
Tests at lines 730-947 set `SchemaColumn.Metadata` with nested `column_features` maps. This field no longer exists on `SchemaColumn`. Column features now live in `ColumnMetadata` (separate table, looked up by `ColumnMetadata.GetBySchemaColumnIDs`). These 3 tests need restructuring to mock `ColumnMetadataRepository` instead of setting inline metadata.

**Fix effort:** Low-Medium. 9 tests need only constructor signature update. 3 tests need restructuring to use `ColumnMetadataRepository` mock instead of `SchemaColumn.Metadata`.

### 3. `pkg/services/table_feature_extraction_test.go` (832 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `table_feature_extraction.go`. The DAG node test (`dag/table_feature_extraction_node_test.go`) only tests the DAG node wrapper — it mocks `ExtractTableFeatures` entirely and never exercises the service internals (`buildPrompt`, `parseResponse`, `buildTableContexts`, `writeColumnSummary`). Zero unit-level coverage for these methods.

**Breakage details:**

1. Constructor signature changed: added `columnMetadataRepo` as param 2
2. `SchemaColumn.Metadata` field removed: tests set inline metadata that no longer exists
3. Method signatures changed: `buildTableContexts`, `writeColumnSummary`, `parseResponse` all gained parameters
4. `tableContext` struct now has `MetadataByColumnID` field

**Fix effort:** Medium. All 10 tests need revival. Constructor + Metadata→ColumnMetadata migration.

### 4. `pkg/services/column_feature_extraction_test.go` (3,706 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `column_feature_extraction.go`. All 32+ production methods still exist (phase 1-5 pipeline: data collection, classification, enum analysis, FK resolution, cross-column analysis). Only 1 active test exists (`column_feature_extraction_selected_test.go` with `TestPhase1_OnlyProcessesSelectedColumns`) — 99% of coverage is missing.

61 test functions covering:
- Pattern detection (UUID, Stripe IDs, timestamps, email, URL, currency)
- Classification path routing
- All 5 extraction phases
- Feature storage and merging
- Question generation for uncertain classifications

**Breakage:** Constructor takes 3 params now (added `columnMetadataRepo`), `SchemaColumn.SampleValues` removed, `SchemaColumn.Metadata` removed, `UpdateEntitySummary` methods removed from OntologyRepository.

**Fix effort:** High. 61 tests with pervasive SampleValues + Metadata removal across all test setup code.

### 5. `pkg/services/column_enrichment_test.go` (2,654 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `column_enrichment.go` (used in DAG pipeline). No active unit tests exist — the DAG node test only mocks the interface. 35 test functions covering:
- Core enrichment workflows with retries and error handling
- Parallel processing and chunk failures
- High-confidence column skipping (LLM optimization logic)
- Enum detection, inference, and merging
- Column metadata merging

**Breakage:** Constructor needs `columnMetadataRepo`, `conversationRepo`, `questionService` params added. Method signatures changed to take `map[uuid.UUID]*models.ColumnMetadata`.

**Fix effort:** Medium. 35 tests; add missing repository mocks and update method signatures.

### 6. `pkg/services/glossary_service_test.go` (4,946 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `glossary_service.go`. 111 test functions covering CRUD, term suggestion/discovery/enrichment, SQL validation, and prompt construction. Active tests elsewhere (65 total across handler/MCP/repo/status layers) cover different layers — **none test service-level logic or SQL helper functions**.

Critical: SQL helper functions (`tokenizeSQL`, `extractTableAliases`, `extractColumnReferences`, `validateColumnReferences`, `isUnionInAggregatingSubquery`, `generateTypeComparisonGuidance`) all exist in production with **zero test coverage** outside this ignored file.

**Breakage:** Constructor signature changed (added `knowledgeRepo`, `schemaRepo`; removed `entityRepo`). Entity repo mock is dead. Some tests pass `nil` for now-required params.

**Fix effort:** High. 111 tests with constructor changes and entity repo removal. However, the SQL helper function tests are pure functions that may compile with minimal changes.

### 7. `pkg/services/fk_semantic_evaluation_test.go` (773 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `fk_semantic_evaluation.go`. 15 test functions covering `EvaluateCandidates`, `EvaluateCandidate`, `FKCandidateFromAnalysis`, LLM error handling, domain knowledge integration. Zero active tests cover this service.

**Breakage:** Constructor signature already matches current production (6 params). Only issue: one test sets `SampleValues` on `SchemaColumn` (line 509) which no longer exists — just remove the line.

**Fix effort:** Low. Remove build tags, delete `SampleValues` line from one test.

### 8. `pkg/services/relationship_candidate_collector_test.go` (2,620 lines)

**Classification: REVIVE — Missing coverage**

Production code is active. Tests cover `shouldExcludeFromFKSources`, `isQualifiedFKSource`, `identifyFKSources`, `identifyFKTargets`, `CollectCandidates`. Partial active coverage exists in `relationship_candidate_selected_test.go` but doesn't cover core collection logic.

**Breakage:** Mock return value signatures and mock struct mismatches with current repository interfaces.

**Fix effort:** Low. Fix mock signatures and struct fields.

### 9. `pkg/services/relationship_discovery_service_test.go` (1,061 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `relationship_discovery_service.go`. Tests cover `NewLLMRelationshipDiscoveryService`, `buildExistingSchemaRelationshipSet`, FK preservation, UUID text validation, LLM call tracking.

**Breakage:** Constructor signature expanded to include `columnMetadataRepo` parameter (7 params, tests pass 6).

**Fix effort:** Medium. Update all constructor calls and add ColumnMetadataRepository mock.

### 10. `pkg/services/schema_integration_test.go` (609 lines)

**Classification: REVIVE — Missing coverage**

Production code is active. 8 integration tests covering `SaveSelections`, `GetSelectedDatasourceSchema`, `GetDatasourceSchemaForPrompt` with real database. Active unit tests exist (mocks only) but these integration tests exercise the full stack.

**Breakage:** Constructor signature changed. One test explicitly skipped (`UpdateColumnMetadata` — needs metadata table approach).

**Fix effort:** Low. Update constructor calls; skip or rewrite the metadata test.

### 11. `pkg/handlers/glossary_integration_test.go` (877 lines)

**Classification: REVIVE — Missing coverage**

Production code is active in `glossary_handler.go`. 17 integration tests covering full CRUD (`Create`, `Update`, `Delete`, `Get`, `List`), SQL validation (`TestSQL`), term suggestion (`Suggest`), and error handling. Active handler tests (6 tests) only cover `List` and `AutoGenerate` — **Create, Update, Delete, Get, TestSQL, Suggest are uncovered**.

**Breakage:** No explicit TODO. Likely disabled during service refactor. Needs constructor/setup verification.

**Fix effort:** Medium. Verify service setup matches current signatures; test logic may need minor updates.

### 12-16. FILES TO DELETE

**`relationship_discovery_service_integration_test.go`** (1,753 lines) — DELETE. References `OntologyEntityRepository` and `EntityRelationshipRepository` which no longer exist. Entity concept fully removed from codebase.

**`dag_relationship_discovery_integration_test.go`** (1,080 lines) — DELETE. Same entity repo references. Also references deprecated `DeterministicRelationshipService`.

**`pk_match_integration_test.go`** (660 lines) — DELETE. Same entity repo references. Service explicitly deprecated.

**`deterministic_relationship_service_test.go`** (7,093 lines) — DELETE. Service explicitly deprecated (line 3: "scheduled for removal", "~90% incorrect relationship inferences"). 55 tests all use removed entity model. Replacement: `LLMRelationshipDiscoveryService` with active test coverage.

**`probe_column_integration_test.go`** (806 lines) — DELETE. Fully duplicated by 24 active unit tests in `probe_test.go` covering metadata fallback, enum values, merge logic, error handling.

## Implementation Order (suggested)

Priority by effort and value:

1. **Low effort, quick wins:**
   - [ ] `fk_semantic_evaluation_test.go` — nearly compiles as-is
   - [ ] `incremental_dag_service_test.go` — mock signature updates only
   - [ ] `schema_integration_test.go` — constructor fix
   - [ ] `relationship_candidate_collector_test.go` — mock fixes

2. **Medium effort:**
   - [ ] `ontology_finalization_test.go` — constructor + 3 Metadata tests
   - [ ] `table_feature_extraction_test.go` — constructor + Metadata migration
   - [ ] `column_enrichment_test.go` — constructor + new repo mocks
   - [ ] `relationship_discovery_service_test.go` — constructor + mock
   - [ ] `glossary_integration_test.go` (handler) — setup verification

3. **High effort:**
   - [ ] `column_feature_extraction_test.go` — 61 tests, pervasive SampleValues removal
   - [ ] `glossary_service_test.go` — 111 tests, constructor + entity removal
