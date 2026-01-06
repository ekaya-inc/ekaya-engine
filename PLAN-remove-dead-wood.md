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

### 2. Remove sample questions dead code from `pkg/services/ontology_finalization.go`

- [ ] Delete `generateSampleQuestions()` method (lines 275-312)
- [ ] Delete `buildSampleQuestionsPrompt()` method (lines 314-358)
- [ ] Delete `sampleQuestionsResponse` struct (lines 360-362)
- [ ] Delete `parseSampleQuestionsResponse()` method (lines 364-370)
- [ ] Remove the commented-out call and TODO at lines 133-140
- [ ] Run `make check`

**Risk:** Low - code is already commented out and disabled.

### 3. Clean up unused repository methods in `pkg/repositories/ontology_repository.go`

- [ ] Remove `GetByVersion()` from interface and implementation
- [ ] Remove `SetActive()` from interface and implementation
- [ ] Remove `DeactivateAll()` from interface and implementation
- [ ] Remove `UpdateMetadata()` from interface and implementation
- [ ] Remove `WriteCleanOntology()` no-op from interface and implementation
- [ ] Update any test files that reference these methods
- [ ] Run `make check`

**Risk:** Medium - need to verify no tests depend on these methods.

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
