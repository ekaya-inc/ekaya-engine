# PLAN: Generate and Consolidate Ontology Questions Throughout the DAG

**Status:** DRAFT
**Date:** 2026-03-15

## Context

Ontology questions are now a first-class system with their own table and service layer. They are no longer tied to the old workflow-state model.

The current ontology DAG has seven stages:
1. `KnowledgeSeeding`
2. `ColumnFeatureExtraction`
3. `FKDiscovery`
4. `TableFeatureExtraction`
5. `PKMatchDiscovery` / relationship discovery
6. `ColumnEnrichment`
7. `OntologyFinalization`

Today, question generation exists in only part of that pipeline:
- `pkg/services/column_feature_extraction.go` creates questions for uncertain classifications
- `pkg/services/column_enrichment.go` stores questions returned by the LLM

There is already exact duplicate suppression in the question model/repository:
- `pkg/models/ontology_question.go` computes `ContentHash`
- `pkg/repositories/ontology_question_repository.go` inserts with `ON CONFLICT (project_id, content_hash) DO NOTHING`

That exact deduplication is useful, but it is not the whole solution. We still need:
- a consistent way for every ontology stage to emit questions
- a shared dependency/interface instead of stage-specific direct writes
- an end-of-pipeline consolidation pass that removes remaining duplicates and can group related questions for review

This plan supersedes the stale "Question Generation During Extraction" notes that were previously embedded in `PLAN-ontology-next.md`.

## Problem

The ontology pipeline can surface uncertainty at many stages, but only some stages can currently turn that uncertainty into stored questions.

This creates three gaps:

1. **Coverage gap**
   - Important ontology uncertainty in knowledge seeding, relationship discovery, table analysis, and finalization may never reach the user as a question

2. **Integration gap**
   - Question creation is not handled through a shared injected dependency across stages
   - Stages that should be able to emit questions are structurally inconsistent

3. **Review gap**
   - Exact hash dedup catches identical text, but not broader overlap
   - Users may still see multiple questions that are effectively the same or should be reviewed together

## Goals

- Every ontology DAG stage can emit zero or more ontology questions
- Question generation/emission is injected as a shared dependency, not hand-coded differently in each stage
- Exact duplicate suppression remains in place
- At the end of ontology processing, the system consolidates pending questions by removing duplicates and optionally grouping related questions together
- The final question list is easier for the user to review without losing important context

## Non-Goals

- This plan does not redesign the answer-processing workflow
- This plan does not move glossary generation or change glossary question gating
- This plan does not replace the existing exact `content_hash` deduplication; it builds on top of it

## Proposed Design

### 1. Shared Stage Dependency

Introduce a shared dependency that ontology stages can use to emit candidate questions.

Recommended shape:

```go
type OntologyQuestionEmitter interface {
    Emit(ctx context.Context, projectID uuid.UUID, workflowID *uuid.UUID, stage models.DAGNodeName, questions []*models.OntologyQuestion) error
}
```

Requirements:
- Every ontology stage should accept this dependency in its service or adapter wiring
- A stage may emit zero questions on a given run, but no stage should be structurally unable to emit questions
- Stages should not call repository methods directly for question creation
- Existing `OntologyQuestionService.CreateQuestions(...)` should remain the canonical persistence path underneath the emitter

### 2. Stage-by-Stage Question Coverage

Each ontology stage should have a defined way to produce questions when uncertainty or missing business context is detected.

#### Knowledge Seeding

Generate questions when project overview parsing reveals:
- ambiguous business terminology
- conflicting conventions
- missing definitions for high-impact business concepts

#### Column Feature Extraction

Keep and expand the existing uncertain-classification questions:
- unclear terminology
- enum meaning uncertainty
- suspicious status/type columns
- ambiguous identifier semantics

#### FK Discovery

Generate questions for:
- suspicious or incomplete foreign key signals
- columns that appear relational but lack a clear target
- database FK structures that conflict with observed data patterns

#### Table Feature Extraction

Generate questions for:
- unclear table purpose
- likely central entities with weak semantic labeling
- orphan or ambiguous tables
- tables whose business role cannot be inferred confidently

#### PK Match / Relationship Discovery

Generate questions for:
- ambiguous relationships
- unclear cardinality
- multiple plausible targets
- business-role ambiguity between related tables

#### Column Enrichment

Keep the existing LLM-generated question support:
- terminology
- temporal meaning
- business-rule clarification
- data quality questions

#### Ontology Finalization

Generate questions for unresolved end-state gaps:
- domain summary contradictions
- missing top-level concepts
- conflicting business conventions
- unresolved semantic holes that should block confidence in the ontology

### 3. Stage Metadata on Questions

To support consolidation and debugging, generated questions should carry stage/source metadata.

Recommended additions:
- `SourceStage` or equivalent metadata showing which DAG stage emitted the question
- preserve existing `WorkflowID`
- keep `ParentQuestionID` for follow-up chains only; do not repurpose it for grouping unrelated sibling questions

This metadata will make it easier to:
- debug noisy stages
- explain why a question exists
- consolidate questions without losing provenance

### 4. Keep Exact Deduplication

The existing exact-dedup behavior should remain:
- `ContentHash` is computed from category + text
- repository insert uses `ON CONFLICT ... DO NOTHING`

This should stay as the first line of defense against duplicates across stages.

### 5. End-of-Pipeline Consolidation

After ontology processing completes, run a consolidation pass over pending questions for the current workflow/project.

This consolidation is responsible for:
- removing remaining duplicates that exact hash dedup did not catch
- merging or suppressing near-identical pending questions
- optionally grouping related questions together for review

Recommended implementation:
- add a dedicated `OntologyQuestionConsolidationService`
- invoke it as the final step of ontology processing
- preferred orchestration: a dedicated terminal DAG step or a clearly isolated post-finalization pipeline action

Consolidation rules should be deterministic first:
- normalized text comparison
- category match
- overlap in affected tables/columns
- same source stage or compatible source stages
- similarity in detected pattern / semantic target

The consolidation pass should operate on non-terminal questions only.
It must not rewrite or collapse already answered or dismissed questions.

### 6. Grouping Related Questions

Grouping is desirable, but it should be built on top of consolidation rather than replacing it.

Examples of groupable questions:
- multiple questions about the same table's business role
- several column questions that all point to one domain concept
- several relationship questions that belong to the same join path ambiguity

Recommended grouping behavior:
- assign a `GroupKey` / `GroupLabel` or equivalent grouping metadata
- keep individual questions addressable
- allow UI/API consumers to present grouped review without losing the underlying question records

Do not overload `ParentQuestionID` for this purpose unless we explicitly decide to change its meaning.

## Architecture Notes

### Why Inject an Interface Into Each Stage

This keeps question generation aligned with the DAG architecture:
- stages surface uncertainty where it actually appears
- services remain responsible for business logic local to that stage
- persistence and dedup stay centralized

It also avoids a weaker design where only one final pass tries to infer all missing questions after the fact.

### Why Consolidate at the End

Question generation should happen as close as possible to the originating uncertainty.
Question consolidation should happen only after all stages have had a chance to contribute.

That gives us:
- better coverage during extraction
- cleaner final review output
- less pressure for each stage to solve global dedup/grouping on its own

## Implementation Plan

### Phase 1: Shared Infrastructure

1. [ ] Introduce a shared ontology-question emitter interface
2. [ ] Route emitter writes through the existing `OntologyQuestionService`
3. [ ] Add source-stage metadata to generated questions
4. [ ] Update question model/repository as needed for stage metadata

### Phase 2: Wire Every Ontology Stage

5. [ ] Inject the emitter dependency into all ontology DAG stage services/adapters
6. [ ] Keep existing column feature extraction question generation but route it through the shared emitter
7. [ ] Keep existing column enrichment question generation but route it through the shared emitter
8. [ ] Add question generation hooks to knowledge seeding
9. [ ] Add question generation hooks to FK discovery
10. [ ] Add question generation hooks to table feature extraction
11. [ ] Add question generation hooks to relationship discovery
12. [ ] Add question generation hooks to ontology finalization

### Phase 3: Consolidation Pass

13. [ ] Add a dedicated ontology-question consolidation service
14. [ ] Add repository methods needed to list/update pending questions for consolidation
15. [ ] Define deterministic near-duplicate suppression rules beyond exact `content_hash`
16. [ ] Ensure consolidation only affects non-terminal questions
17. [ ] Invoke consolidation at the end of ontology processing

### Phase 4: Grouping

18. [ ] Define grouping metadata for related questions
19. [ ] Add deterministic grouping rules based on category + affected entities + semantics
20. [ ] Persist grouping metadata without losing individual question identity
21. [ ] Expose grouping through existing question read paths if grouping is enabled

### Phase 5: Testing and Hardening

22. [ ] Add unit tests for stage-specific question emitters/generators
23. [ ] Add unit tests for consolidation rules
24. [ ] Add unit tests for grouping rules
25. [ ] Add integration tests covering a full DAG run with questions from multiple stages
26. [ ] Add regression cases proving that answered/dismissed questions are not incorrectly consolidated

## Testing Strategy

### Unit Tests

- stage-specific question generation tests
- emitter wiring tests
- exact dedup compatibility tests
- near-duplicate consolidation tests
- grouping metadata tests

### Integration Tests

- full ontology DAG run creates questions from multiple stages
- end-of-pipeline consolidation reduces noisy overlap
- grouped questions remain individually answerable

### Regression Focus

Prioritize cases where multiple stages might ask overlapping questions about:
- the same domain term
- the same status/enum column
- the same relationship ambiguity
- the same table role

## Open Questions

1. Should end-of-pipeline consolidation be a dedicated DAG node or a post-finalization service call?
2. Do we want grouping in the first implementation, or should we land stage coverage + consolidation first and add grouping second?
3. What stage metadata belongs on the question model versus being derived at read time?
4. Should some categories be marked "never group" because they must remain standalone for review?
