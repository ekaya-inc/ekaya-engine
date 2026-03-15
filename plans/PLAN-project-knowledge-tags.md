# PLAN: Deterministic Project Knowledge Retrieval for LLM Requests

## Problem Statement

Project Knowledge is critical because users expect that when they tell Ekaya something important about their project, that fact is remembered and used throughout the system.

Today, we do not do that reliably. Project knowledge can be stored, but later LLM requests for ontology and glossary work do not have a consistent mechanism to pull in the relevant facts. The result is that important business context gets lost between the moment it is captured and the moment the system needs it.

We do not currently have pgvector in the metadata store and we are not hosting an embedding model. That means we cannot rely on embedding generation plus vector similarity search to retrieve relevant project knowledge for every LLM request.

We need a simple, deterministic retrieval system that:
- stores project knowledge as reusable atomic facts
- classifies those facts into a pre-created tag taxonomy
- classifies later incoming request text into the same tags
- injects only the matching project knowledge into LLM prompts

This is intentionally a deterministic substitute for vector retrieval. The design must strongly prefer recall over precision because missing a relevant project fact is worse than including a few extra facts.

## Core Design

The tag system is no longer "LLM-generated tags." The LLM is used only to break freeform project knowledge input into individual facts. Tag assignment is deterministic.

### Write-Time Flow: Project Knowledge Upsert

When project knowledge is added or updated:

1. Accept freeform input from the user, MCP tool, or any internal write path
2. Run an internal LLM call that rewrites the input into one or more atomic facts
3. Run each atomic fact through deterministic classifiers
4. Assign zero or more tags from a pre-created tag taxonomy
5. Persist the atomic facts and their tags into `engine_project_knowledge`

Example:

Input:
`Fiscal Year end is June 30th and we do a trailing four quarter average for calculating X`

Potential extracted facts:
- `Fiscal year ends on June 30`
- `X is calculated using a trailing four quarter average`

Then deterministic classifiers tag them, for example:
- `Fiscal year ends on June 30` -> `fiscal`, `temporal`
- `X is calculated using a trailing four quarter average` -> `calculation`, `aggregation`, `fiscal`, `metric`

### Read-Time Flow: Every LLM Request

Before any LLM call:

1. Collect the incoming text relevant to the request
2. Run that text through the same deterministic classifiers
3. Produce zero or more retrieval tags
4. Fetch project knowledge facts whose tags overlap those retrieval tags
5. Inject the matching facts into a standard project-knowledge section in the final prompt

The same mechanism should be used for:
- ontology extraction and refinement
- glossary generation and enrichment
- future Text2SQL requests

The important property is consistency: every LLM request should have a defined project-knowledge retrieval step, even if that step returns no facts.

## Design Principles

1. **Deterministic tag assignment**
   - Tags are assigned by code, not by the LLM
   - The same classifier rules are used at write time and read time

2. **LLM only for fact extraction**
   - The LLM may split or normalize freeform project knowledge input into atomic facts
   - The LLM should not emit internal tags or choose retrieval categories

3. **Internal taxonomy**
   - Tags are an internal retrieval mechanism, not a user-facing concept
   - Users should not need to learn or manage the tag system

4. **Recall-first retrieval**
   - False negatives are deadly
   - The tag taxonomy and classifiers should be broad enough to catch relevant context
   - Mild over-inclusion is acceptable if it avoids missing critical facts

5. **No "send everything" fallback**
   - We should not dump all project knowledge into every prompt
   - Retrieval must remain selective and deterministic

6. **Universal request-time integration**
   - This is infrastructure, not a few ad hoc prompt edits
   - Any code path that calls the LLM should be able to invoke the same retrieval mechanism

## Current State

Project knowledge is stored in `engine_project_knowledge`, but there is no shared mechanism that guarantees relevant facts are pulled into later prompts.

Relevant existing areas:
- `pkg/models/ontology_chat.go`
- `pkg/repositories/knowledge_repository.go`
- `pkg/services/knowledge.go`
- `pkg/services/knowledge_seeding.go`
- `pkg/services/incremental_dag_prompts.go`
- `pkg/services/glossary_service.go`

## Proposed Architecture

### 1. Atomic Fact Extraction

**Goal:** Store project knowledge as reusable facts, not as large undifferentiated blocks of prose.

Requirements:
- Freeform project knowledge input can yield multiple stored facts
- Fact extraction uses an internal LLM call
- The LLM output format should be tightly constrained: an array of fact strings plus optional normalized wording
- Facts should preserve provenance and source context

Non-goal:
- The LLM does not assign tags

### 2. Deterministic Tag Taxonomy

We need a finite, pre-created set of tags that classifier functions can assign consistently.

The taxonomy should start broad and practical rather than perfect. Candidate initial tags:

**Business semantics**
- `terminology`
- `business_rule`
- `calculation`
- `aggregation`
- `metric`

**Time**
- `temporal`
- `fiscal`
- `lifecycle`

**Financial**
- `money`
- `billing`
- `accounting`
- `percentage`

**Identity and entities**
- `user`
- `organization`
- `product`
- `identifier`

**Structure and relationships**
- `hierarchy`
- `cardinality`
- `status`
- `enumeration`

**Data shape**
- `format`
- `measurement`
- `geography`

This taxonomy is internal and can evolve, but only through code changes and tests. It should not be open-ended or user-defined.

### 3. Deterministic Classifiers

We need classifier functions that map text to zero or more tags using deterministic rules.

Classifier inputs:
- atomic fact text at write time
- request text or prompt input text at read time
- optional request kind, which can supply default tags

Classifier techniques should be deterministic:
- case-normalized phrase matching
- synonym dictionaries
- exact business keyword lists
- regex for common patterns
- request-kind defaults
- explicit caller-provided tags when deterministic code already knows context

The system should use the same tag definitions and core matching logic for both stored facts and incoming request text.

Example classifier rules:
- `fiscal`: phrases like `fiscal year`, `year end`, `quarter`, `Q1`, `Q2`, `Q3`, `Q4`, `trailing four quarter`, `TTM`
- `calculation`: phrases like `calculated`, `formula`, `average`, `sum`, `ratio`, `derived`, `minus`, `divided by`
- `money`: phrases like `revenue`, `expense`, `amount`, `price`, `cost`, currency symbols, currency codes
- `business_rule`: phrases like `must`, `should`, `cannot`, `only`, `required`, `never`
- `terminology`: phrases like `means`, `refers to`, `we call`, `is called`, `aka`

### 4. Shared Retrieval Contract for LLM Calls

Any LLM call path should be able to invoke the same project-knowledge retrieval layer.

Example shape:

```go
type KnowledgeInjectionRequest struct {
    ProjectID    uuid.UUID
    RequestKind  string   // e.g. ontology_entity_discovery, glossary_enrichment, text2sql_generation
    InputTexts   []string // text that should be classified for retrieval
    ExplicitTags []string // optional deterministic caller-provided tags
}
```

Expected behavior:
- classify `InputTexts` into retrieval tags
- union those tags with request-kind defaults and `ExplicitTags`
- fetch matching project knowledge facts
- format them into a consistent `## Relevant Project Knowledge` section
- return empty content when no facts match

This mechanism should be invoked before the final prompt is sent to the LLM.

### 5. Retrieval Budgeting and Ranking

Tag overlap alone may return too many facts for broad tags.

We need deterministic selection rules so we do not send all matching project knowledge:
- prefer facts matching multiple tags over single-tag matches
- prefer exact phrase hits when available
- prefer facts matching request-kind defaults plus text-derived tags
- impose a per-request token or fact-count budget
- keep ordering stable and explainable

The ranking can stay simple, but it must be deterministic.

### 6. Observability for Misses

Because false negatives are dangerous, this system needs feedback loops:
- log facts that receive zero tags at write time
- log LLM requests that resolve to zero retrieval tags
- log high-volume tag matches that exceed the prompt budget
- build a regression corpus from real examples that were missed or over-included

Zero-tag facts may be allowed initially, but they should be treated as coverage gaps in the taxonomy/classifier system, not as a healthy steady state.

## Integration Scenarios

### 1. Ontology

Use project-knowledge retrieval for all ontology LLM requests that need business context, including:
- column feature extraction
- entity discovery
- relationship enrichment
- ontology refinement or clarification prompts
- any later ontology-question-related prompt that benefits from business rules or terminology

Examples:
- `Fiscal year ends June 30` should influence ontology prompts that reason about fiscal periods or quarter boundaries
- `Revenue excludes refunds` should influence ontology prompts that classify revenue-like columns

### 2. Glossary

Use the same retrieval mechanism for glossary work:
- glossary term discovery
- glossary term enrichment
- SQL definition generation for glossary terms
- future glossary suggestion flows

Examples:
- project terminology should bias term naming
- stored calculations and business rules should influence generated definitions

### 3. Future Text2SQL

Future Text2SQL should reuse this exact infrastructure rather than inventing a parallel memory or retrieval path.

Use cases:
- ambiguity resolution
- business-rule-aware SQL generation
- terminology normalization
- fiscal/metric interpretation

Project knowledge retrieval should happen before SQL generation so that relevant facts are already in the prompt context.

### 4. Project Knowledge Creation and Editing

All project-knowledge write paths must use the same fact-extraction and deterministic-tagging flow:
- manual UI edits
- MCP tool upserts
- internal seeding flows
- any future API write path

No write path should be able to create a new project-knowledge fact without routing through the canonical extraction/tagging pipeline.

## Implementation Plan

### Phase 1: Tag Taxonomy and Classifier Design

1. [ ] Finalize the initial internal tag taxonomy
2. [ ] Define deterministic classifier rules for each tag
3. [ ] Document representative positive examples and tricky edge cases for each classifier
4. [ ] Bias taxonomy/rules toward recall, not minimalism
5. [ ] Decide how request-kind defaults participate in retrieval

### Phase 2: Storage Model and Write Path

6. [ ] Add `tags` storage to `engine_project_knowledge`
7. [ ] Update the project knowledge model to store tags
8. [ ] Introduce a canonical project-knowledge upsert flow in `pkg/services/knowledge.go`
9. [ ] Add the internal LLM-based fact extraction step for freeform project-knowledge input
10. [ ] Run each extracted fact through deterministic classifiers before persistence
11. [ ] Persist extracted facts plus tags on every write path
12. [ ] Ensure MCP/manual/project-overview ingestion all use the canonical flow

### Phase 3: Deterministic Retrieval Infrastructure

13. [ ] Add a shared classifier package or service for classifying arbitrary text into project-knowledge tags
14. [ ] Add a shared retrieval function for "tags -> matching facts"
15. [ ] Add a shared `KnowledgeInjectionRequest` contract for LLM call sites
16. [ ] Add deterministic ranking and prompt-budgeting rules for matched facts
17. [ ] Standardize the injected prompt section format

### Phase 4: Ontology Integration

18. [ ] Integrate retrieval into ontology column feature extraction
19. [ ] Integrate retrieval into ontology entity discovery
20. [ ] Integrate retrieval into ontology relationship enrichment
21. [ ] Audit remaining ontology LLM call sites and route them through the shared retrieval mechanism

### Phase 5: Glossary Integration

22. [ ] Integrate retrieval into glossary term discovery
23. [ ] Integrate retrieval into glossary enrichment
24. [ ] Integrate retrieval into glossary SQL-definition generation

### Phase 6: Future Text2SQL Readiness

25. [ ] Define the request kinds Text2SQL will use with this retrieval system
26. [ ] Ensure Text2SQL planning assumes reuse of this infrastructure rather than a separate project-knowledge retrieval layer

### Phase 7: MCP and Tooling

27. [ ] Update `update_project_knowledge` MCP behavior so writes route through the canonical extraction/tagging path
28. [ ] Keep internal tags hidden from normal users unless there is a deliberate debugging/admin reason to expose them
29. [ ] Add debugging support to inspect extracted facts and assigned tags when needed

### Phase 8: Observability and Hardening

30. [ ] Log zero-tag facts after write-time classification
31. [ ] Log zero-tag request classifications before LLM calls
32. [ ] Add regression fixtures for known high-value project-knowledge examples
33. [ ] Add regression fixtures for request-time retrieval examples across ontology and glossary
34. [ ] Tune taxonomy and classifiers based on misses before expanding scope

## Testing Strategy

### Unit Tests

- classifier tests for every tag
- normalization tests
- phrase, synonym, and regex matching tests
- ranking/budgeting tests for matched fact selection
- fact extraction response parsing tests

### Integration Tests

- project-knowledge upsert: freeform input -> extracted facts -> deterministic tags -> stored rows
- ontology LLM request: request text -> retrieval tags -> matched facts -> injected prompt section
- glossary LLM request: request text -> retrieval tags -> matched facts -> injected prompt section
- MCP write path uses the same extraction/tagging flow

### Recall-Focused Regression Tests

Add real examples where missing context would be unacceptable, such as:
- `Fiscal year ends June 30`
- `Revenue excludes refunds`
- `X uses a trailing four quarter average`
- domain-specific terminology definitions

The goal is not only "does classification run" but "does relevant knowledge get pulled into the right requests."

## Open Questions

1. Should zero-tag facts be stored with warnings, or rejected until classifier coverage improves?
2. Do we need a tiny set of "always include" facts for universally important project-wide context?
3. What is the initial fact-count or token budget for injected project knowledge per request kind?
4. How do we version and regression-test taxonomy changes so classifier improvements do not break prior behavior?
5. Do we want deterministic exact-term matching in addition to tags for especially important domain terminology?
