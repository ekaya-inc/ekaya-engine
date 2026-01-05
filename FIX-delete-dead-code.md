# FIX: Delete Dead Code from Ontology Extraction

## Background

When we moved everything to the Ontology DAG workflow, dead code was left behind. All ontology, entity, and relationship extraction now goes through the DAG (`pkg/services/ontology_dag_service.go`). This plan removes the orphaned code.

## Files to Delete Entirely

### 1. `ui/src/services/realOntologyService.ts` (DELETED)

**Why:** Near-identical duplicate of `ontologyService.ts`. Never imported or used anywhere.

**Verification before deleting:**
```bash
grep -r "realOntologyService" --include="*.ts" --include="*.tsx" ui/src/
```
Expected: No hits.

✅ **Completed:** File deleted. UI build verified successful.

---

## Files to Refactor

### 3. `pkg/services/ontology_builder.go` - Extract `ProcessAnswer` Only

✅ **Completed:** Refactored to keep only ProcessAnswer and helpers. Deleted ~1,500 lines of dead code, removed unused repository dependencies, deleted test files that tested removed methods. Build and tests verified.

**Why:** ~1,800 line file where ~75% is dead code. Only `ProcessAnswer()` is still actively used.

**What calls `ProcessAnswer()`:**
- `pkg/services/ontology_question.go:133` - `OntologyQuestionService.AnswerQuestion()` calls `s.builder.ProcessAnswer()`

**Struct fields used by ProcessAnswer:**
- `logger` - YES
- `llmFactory` - YES
- `ontologyRepo` - NO (can remove)
- `schemaRepo` - NO (can remove)
- `knowledgeRepo` - NO (can remove)
- `ontologyEntityRepo` - NO (can remove)
- `entityRelRepo` - NO (can remove)

**Methods to KEEP (used by ProcessAnswer):**
- `ProcessAnswer()` (line ~1331)
- `answerProcessingSystemMessage()` (line ~1375)
- `buildAnswerProcessingPrompt()` (line ~1392)
- `parseAnswerProcessingResponse()` (line ~1450)

**Interface methods to DELETE (deprecated stubs that return errors):**
- `BuildTieredOntology()` - returns error, never called
- `ProcessProjectDescription()` - returns error, never called

**Private methods to DELETE (only called by deprecated methods):**
- `buildEntitySummariesFromDomainEntities()`
- `buildDomainSummaryFromEntities()`
- `loadTablesWithColumns()`
- `BuildEntitySummaries()`
- `buildEntitySummariesWithContext()`
- `buildEntityBatch()`
- `buildEntityBatchWithContext()`
- `tier1SystemMessage()`
- `buildTier1Prompt()`
- `buildTier1PromptWithContext()`
- `parseTier1Response()`
- `BuildDomainSummary()`
- `buildDomainSummaryWithContext()`
- `tier0SystemMessage()`
- `buildTier0Prompt()`
- `buildTier0PromptWithContext()`
- `parseTier0Response()`
- `entityAnalysisSystemMessage()`
- `buildEntityAnalysisPrompt()`
- `parseEntityAnalysisResponse()`
- `questionGenerationSystemMessage()`
- `buildSchemaContext()`
- `buildQuestionGenerationPrompt()`
- `parseQuestionsResponse()`
- `sortAndLimitQuestions()`
- `descriptionProcessingSystemMessage()`
- `buildSchemaSummaryForDescription()`
- `buildDescriptionProcessingPrompt()`
- `parseDescriptionProcessingResponse()`
- `loadRelationships()`
- `buildRelationshipMap()`
- `uniqueStrings()`

**Constants to DELETE:**
- `MaxTablesPerBatch` (only used by dead methods)

**Test files to clean up:**
- `pkg/services/ontology_builder_test.go` - tests `buildSchemaSummaryForDescription` (dead)
- `pkg/services/ontology_builder_integration_test.go` - tests dead methods:
  - `TestLoadTablesWithColumns_Integration`
  - `TestBuildSchemaSummaryForDescription_IncludesColumns`
  - `TestBuildSchemaSummaryForDescription_EmptyColumns`
  - `TestBuildTier1PromptWithContext_IncludesColumns`
  - `TestLoadTablesWithColumns_MultipleTablesAndColumns_Integration`

**Approach:**
1. Delete the two full-file deletions first (entity_discovery_task, realOntologyService)
2. Rewrite `ontology_builder.go` to keep only ProcessAnswer and its helpers
3. Simplify the interface to only have `ProcessAnswer()`
4. Simplify the struct to only have `logger` and `llmFactory`
5. Update constructor and `main.go` accordingly
6. Delete or update test files
7. Run `make check` to verify

---

## Verification Steps After Cleanup

```bash
# Build should pass
go build ./...

# Tests should pass
make test

# Lint should pass
make check

# Verify OntologyBuilderService is still wired up in main.go
grep -A5 "NewOntologyBuilderService" main.go
```

---

## DONE: Already Cleaned Up

### `pkg/services/entity_discovery_task.go` + test (DELETED)

Cleaned up in this session. The following were deleted:
- `pkg/services/entity_discovery_task.go` (16,302 bytes)
- `pkg/services/entity_discovery_task_test.go` (6,553 bytes)

These were part of an old task-queue-based workflow system and were never instantiated anywhere in the codebase. Build verification passed after deletion.

### `pkg/services/relationship_discovery.go` (DELETED)

This was cleaned up in a previous session. The following were deleted:
- `pkg/services/relationship_discovery.go` (807 lines)
- `pkg/services/relationship_discovery_test.go` (730 lines)
- `pkg/services/relationship_discovery_integration_test.go` (813 lines)

References in `pkg/handlers/schema.go` and `main.go` were also cleaned up.
