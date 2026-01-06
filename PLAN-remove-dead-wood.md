# PLAN: Remove Dead Code from Ontology DAG Migration

## Context

After migrating to the DAG-based ontology extraction workflow, several pieces of code are no longer used. This plan tracks their removal to reduce maintenance burden and prevent confusion.

## Tasks

### 1. Delete `pkg/models/ontology_workflow.go` ✅

- [x] Verify no imports reference this file
- [x] Delete the entire file (~260 lines)
- [x] Run `make check` to confirm no build errors

**Risk:** Low - grep shows zero references outside the file itself.

**Completed:** Successfully removed `pkg/models/ontology_workflow.go` (261 lines) and its only import point in `pkg/services/workflow/infra.go` (323 lines). The `TenantContextFunc` type alias that was referenced from `workflow` package was moved inline to `pkg/services/tenant_context.go` to maintain the public API. All tests pass. Total: 584 lines removed.

### 2. Remove sample questions dead code from `pkg/services/ontology_finalization.go` ✅

- [x] Delete `generateSampleQuestions()` method (lines 275-312)
- [x] Delete `buildSampleQuestionsPrompt()` method (lines 314-358)
- [x] Delete `sampleQuestionsResponse` struct (lines 360-362)
- [x] Delete `parseSampleQuestionsResponse()` method (lines 364-370)
- [x] Remove the commented-out call and TODO at lines 133-140
- [x] Add test to verify sample questions are empty
- [x] Run `make check`

**Risk:** Low - code is already commented out and disabled.

**Completed:** Successfully removed all sample questions dead code from `pkg/services/ontology_finalization.go` (~110 lines removed total):
- Removed 4 methods: `generateSampleQuestions()`, `buildSampleQuestionsPrompt()`, `parseSampleQuestionsResponse()`, and the `sampleQuestionsResponse` struct
- Removed commented-out call with TODO explaining it was disabled pending relationship algorithm improvements
- Left `var sampleQuestions []string // Empty for now` in place to maintain the domain summary structure
- Added test `TestOntologyFinalization_SampleQuestionsAreEmpty` to verify behavior
- All tests pass (`make check`)

### 3. Clean up unused repository methods in `pkg/repositories/ontology_repository.go` ✅

- [x] Remove `GetByVersion()` from interface and implementation
- [x] Remove `SetActive()` from interface and implementation
- [x] Remove `DeactivateAll()` from interface and implementation
- [x] Remove `UpdateMetadata()` from interface and implementation
- [x] Remove `WriteCleanOntology()` no-op from interface and implementation
- [x] Update any test files that reference these methods
- [x] Run `make check`

**Risk:** Medium - need to verify no tests depend on these methods.

**Completed:** Successfully removed 5 unused repository methods from the `OntologyRepository` interface and implementation:
- Removed methods: `GetByVersion()`, `SetActive()`, `DeactivateAll()`, `UpdateMetadata()`, and `WriteCleanOntology()`
- **pkg/repositories/ontology_repository.go:** Removed ~150 lines (method implementations + no-op comment)
- **pkg/repositories/ontology_repository_test.go:** Removed 8 dedicated test functions for deleted methods, updated remaining tests to use `GetActive()` instead of `GetByVersion()`, removed references from tenant scope tests (~160 lines removed)
- **Service layer test mocks:** Updated 7 service test files (column_enrichment_test.go, datasource_test.go, deterministic_relationship_service_test.go, entity_service_test.go, glossary_service_test.go, ontology_context_test.go, ontology_finalization_test.go) to remove mock implementations of deleted methods
- All tests pass (`make check`)
- **Total impact:** ~310 lines removed across repository and test files

**Context for next session:** The ontology repository now only contains actively used methods. The single-active-version model is enforced at the database level (unique constraint), so versioning methods are no longer needed at the repository layer. Service layer tests only mock the methods they actually use.

### 4. Remove deprecated LLM tools from `pkg/llm/tool_executor.go`

- [ ] Determine if any persisted DAGs still reference `answer_question` or `get_pending_questions` tools
- [ ] If safe, remove `answerQuestion()` method (lines 509-530)
- [ ] If safe, remove `getPendingQuestions()` method (lines 536-558)
- [ ] Remove tool registrations
- [ ] Run `make check`

**Risk:** Medium - old DAGs in the database might reference these tools. May need a database query to verify.

### 5. Document `DAGNodeRelationshipDiscovery` deprecation timeline

- [ ] Add comment with target removal version/date
- [ ] No code removal yet - kept for backward compatibility

**Risk:** None - documentation only.

## Verification

After all changes:
- [ ] `make check` passes
- [ ] Manual test: trigger ontology extraction on a test project
- [ ] Verify DAG completes successfully

## Estimated Impact

~450 lines of dead code removed.
