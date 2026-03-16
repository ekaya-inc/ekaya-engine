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

The tag system is no longer "LLM-generated tags." The LLM is used only to:
- extract atomic facts from freeform create text
- derive the server-side coarse `fact_type` for create and update flows

Tag assignment is deterministic and code-owned.

### Write-Time Flow: Project Knowledge Writes

There are three distinct write behaviors:

**A. Freeform create for user-facing APIs and MCP**

1. Accept `text` only from HTTP/UI or MCP
2. Run an internal LLM call that rewrites the input into one or more atomic facts plus a coarse `fact_type` for each fact
3. Normalize the emitted `fact_type` values to the accepted model categories
4. Run each atomic fact through deterministic classifiers
5. Assign zero or more tags from a pre-created tag taxonomy
6. Skip any extracted fact whose normalized text already exists in the project
7. Persist the remaining atomic facts, their `fact_type`, and their tags into `engine_project_knowledge`
8. Return the created rows plus the extracted-fact artifact used to create them

**B. Existing project-knowledge fact update by ID**

1. Load the existing fact row by ID
2. Accept edited `text` only from the caller
3. Run an internal LLM call that derives the single updated fact interpretation and server-side `fact_type`
4. Normalize the emitted `fact_type`
5. Re-run deterministic tagging for the updated row
6. Persist the updated row with its revised `fact_type` and tags
7. Return the single updated row plus the extracted-fact artifact used to update it

Updating by ID does **not** fan out into multiple rows. If a caller wants to add more facts, that should happen as new project-knowledge creates, not as a multi-row expansion of an existing fact ID.

Update-by-ID may change the stored `fact_type` for that row. If the edited text would normalize to the same value as another existing row, the update is still allowed.

**C. Structured internal create path**

1. Internal producers that already have structured facts call a server-only structured entry point
2. That path skips freeform LLM extraction, but still uses the same normalization, tagging, duplicate-skipping, provenance, and persistence rules
3. It is used for internal producers such as ontology-question answer processing and overview-derived fact seeding
4. It is not exposed through normal HTTP/UI or MCP create/update contracts

`project_overview` is a special singleton control record and remains outside all normal project-knowledge create/update contracts:
- it is stored as `fact_type = project_overview`
- it is not treated as a normal retrievable project-knowledge fact
- it is created or updated only when ontology extraction starts
- it is not created through normal project-knowledge HTTP create/update endpoints
- it is not created through normal MCP project-knowledge tools
- it is not created through the structured internal create path
- when it changes, previously overview-derived facts are replaced using explicit persisted lineage for those derived rows

Example:

Input:
`Fiscal Year end is June 30th and we do a trailing four quarter average for calculating X`

Potential extracted facts:
- `Fiscal year ends on June 30`
- `X is calculated using a trailing four quarter average`

Then deterministic classifiers tag them, for example:
- `Fiscal year ends on June 30` -> `fiscal`, `temporal`
- `X is calculated using a trailing four quarter average` -> `calculation`, `aggregation`, `fiscal`, `metric`

## Locked Contract Decisions

These decisions are already settled and should not be reopened during implementation.

### External Create and Update Contracts

- HTTP create becomes the canonical freeform create path and accepts `text` only
- HTTP update-by-ID accepts `text` only
- caller-supplied `fact_type`, `category`, and `context` are removed from normal project-knowledge create/update contracts
- the existing `/project-knowledge/parse` endpoint is removed rather than kept as a parallel path
- create responses return all created rows plus the extracted facts and derived `fact_type` values
- update responses return the single updated row plus the single extracted interpretation artifact
- extraction artifacts are response/debug data only and are not persisted
- if create extraction produces no valid facts, the call fails and creates nothing
- if update extraction cannot derive a valid fact and `fact_type`, the call fails and leaves the row unchanged

### MCP Contracts

- replace the current upsert-style MCP write tool with explicit split tools:
  - `create_project_knowledge(text)`
  - `update_project_knowledge(fact_id, text)`
- MCP tools use the same canonical backend service flows as HTTP
- MCP errors from extraction/classification failures should continue using the existing JSON-wrapped MCP error shape so the client LLM receives structured error details

### Duplicate Policy

- create-time duplicate detection is based on normalized fact text only, not on `(fact_type, value)`
- initial normalization for duplicate checks is:
  - case-insensitive
  - whitespace-normalized
- create skips duplicates
- structured internal create uses the same duplicate-skipping rule as external create
- update-by-ID allows duplicates
- overview-derived regeneration deletes prior overview-derived rows first, then skips creating a row if it would duplicate an existing non-overview fact

### Provenance Contract

- HTTP/UI creates and updates use manual provenance
- MCP creates and updates use MCP provenance
- internal generated facts such as overview-derived seeded facts use inferred provenance
- implementation must preserve existing provenance semantics using the current source, last-edit, and actor-tracking fields rather than collapsing everything to a single source value

### `project_overview`

- `project_overview` is not a normal project-knowledge fact creation path
- it remains a singleton record written only from ontology extraction start
- it remains excluded from normal retrieval
- overview edits trigger replacement of previously overview-derived facts
- replacement targets only rows explicitly marked as overview-derived and leaves other inferred rows alone
- replacement uses an explicit persisted lineage marker on derived fact rows so cleanup is deterministic

### Read-Time Flow: Eligible LLM Requests

Before any LLM call that analyzes customer schema/content or generates ontology, glossary, SQL, or other business-facing output:

1. Collect the incoming text relevant to the request
2. Run that text through the same deterministic classifiers
3. Produce zero or more retrieval tags
4. Fetch project knowledge facts whose tags overlap those retrieval tags
5. Inject the matching facts into a standard project-knowledge section in the final prompt

The same mechanism should be used for:
- ontology extraction and refinement
- glossary generation and enrichment
- future Text2SQL requests

Control-plane LLM requests that exist only to maintain project knowledge itself, such as fact extraction for project-knowledge create/update calls, are exempt.

The important property is consistency: every eligible LLM request should have a defined project-knowledge retrieval step, even if that step returns no facts.

## Design Principles

1. **Deterministic tag assignment**
   - Tags are assigned by code, not by the LLM
   - The same classifier rules are used at write time and read time

2. **LLM only for fact extraction**
   - The LLM may split or normalize freeform project knowledge input into atomic facts
   - The LLM may derive the coarse `fact_type` for create and update flows
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

6. **Universal request-time integration for product-plane LLM calls**
   - This is infrastructure, not a few ad hoc prompt edits
   - Any LLM code path that analyzes project data or generates ontology/glossary/SQL should be able to invoke the same retrieval mechanism
   - Pure control-plane maintenance calls may opt out

7. **Keep coarse `fact_type`**
   - `fact_type` remains on stored rows as a coarse category
   - tags supplement `fact_type`; they do not replace it
   - the LLM derives `fact_type`, but code normalizes it to accepted values
   - callers do not supply `fact_type` on normal create/update APIs

8. **Special-case `project_overview`**
   - `project_overview` remains a singleton control record
   - it is written only by ontology extraction start
   - it is excluded from normal retrieval unless a caller explicitly asks for it
   - overview-derived facts use explicit persisted lineage so they can be replaced safely when the overview changes

9. **Canonical orchestration path**
   - All normal project-knowledge writes should route through one canonical project-knowledge service flow
   - There are two canonical entry points under that flow: freeform create/update for external callers and structured create for internal producers
   - Handlers, MCP tools, ontology flows, and other callers should not each implement their own extraction/tagging logic
   - Repositories remain focused on metadata-store SQL and persistence

## Current State

Project knowledge is stored in `engine_project_knowledge`, but there is no shared mechanism that guarantees relevant facts are pulled into later prompts.

Today:
- the UI create flow uses `/project-knowledge/parse`, which is actually a freeform create-and-store path
- `POST /project-knowledge` and `PUT /project-knowledge/{id}` are structured row writes that accept caller-supplied fields such as `fact_type`
- the MCP `update_project_knowledge` tool is an upsert-style write path with caller-supplied category and direct repository behavior
- some read paths inject **all** project knowledge into prompts via `GetByProject`
- `project_overview` is created or updated from ontology extraction start, not from normal project-knowledge create/update APIs
- overview-derived seeding currently stops once non-overview knowledge exists, so later overview edits can leave stale overview-derived facts in place
- some knowledge writes bypass a single canonical orchestration path and write directly through the repository

Relevant existing areas:
- `pkg/handlers/knowledge_handler.go`
- `pkg/handlers/ontology_dag_handler.go`
- `pkg/models/ontology_chat.go`
- `pkg/repositories/knowledge_repository.go`
- `pkg/services/knowledge.go`
- `pkg/services/knowledge_parsing.go`
- `pkg/services/knowledge_seeding.go`
- `pkg/services/ontology_dag_service.go`
- `pkg/services/ontology_question.go`
- `pkg/services/ontology_builder.go`
- `pkg/services/column_feature_extraction.go`
- `pkg/services/table_feature_extraction.go`
- `pkg/services/relationship_validator.go`
- `pkg/services/glossary_service.go`
- `pkg/mcp/tools/knowledge.go`
- `ui/src/components/ProjectKnowledgeEditor.tsx`
- `ui/src/services/engineApi.ts`

## Proposed Architecture

### 1. Atomic Fact Extraction

**Goal:** Store project knowledge as reusable facts, not as large undifferentiated blocks of prose.

Requirements:
- freeform project knowledge create can yield multiple stored facts
- fact extraction uses an internal LLM call
- the LLM output format should be tightly constrained to extracted facts plus coarse `fact_type`
- the emitted `fact_type` is coarse and must be normalized to accepted model values before persistence
- facts should preserve provenance and actor/source context
- updating an existing fact by ID edits one stored fact row, does not fan out into multiple rows, and re-derives its `fact_type` from the edited text
- create and update responses should surface the extracted-fact artifact used by the server
- extraction artifacts are not persisted
- `project_overview` is exempt and remains a singleton special record outside this flow

Non-goal:
- the LLM does not assign tags

### 2. Canonical Write Contracts

We need a single service-level orchestration layer with explicit entry points rather than a mix of parse/upsert/direct-repository paths.

Requirements:
- external freeform create accepts `text` only
- external update-by-ID accepts `fact_id` plus `text` only
- internal structured create accepts already-structured facts for server-only producers
- caller-supplied `fact_type`, `category`, and `context` are removed from normal external contracts
- freeform create uses LLM extraction and may create multiple rows
- update-by-ID uses LLM interpretation for one row and may change the stored `fact_type`
- `project_overview` is blocked from these normal contracts and remains owned by ontology extraction start
- HTTP and MCP responses expose the extraction artifact shape needed for debugging and client follow-up behavior
- HTTP and MCP share the same service implementation rather than parallel business logic

### 3. Keep `fact_type` and Tags Alongside It

We should keep the existing `fact_type` column because it already provides coarse categorization and supports special control semantics such as `project_overview`.

Requirements:
- `fact_type` remains stored on each normal knowledge row
- tags are additional internal retrieval metadata, not a replacement for `fact_type`
- prompt formatting, APIs, and debugging tools may continue to group by `fact_type` when useful
- `project_overview` remains a special `fact_type` outside normal atomic retrieval
- external callers do not set `fact_type`; the server does

### 4. Deterministic Tag Taxonomy

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

### 5. Deterministic Classifiers

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

### 6. Shared Retrieval Contract for LLM Calls

Any eligible LLM call path should be able to invoke the same project-knowledge retrieval layer.

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
- exclude special control records such as `project_overview` by default
- format them into a consistent `## Relevant Project Knowledge` section
- return empty content when no facts match

This mechanism should be invoked before the final prompt is sent to the LLM.

### 7. Retrieval Budgeting and Ranking

Tag overlap alone may return too many facts for broad tags.

We need deterministic selection rules so we do not send all matching project knowledge:
- prefer facts matching multiple tags over single-tag matches
- prefer exact phrase hits when available
- prefer facts matching request-kind defaults plus text-derived tags
- impose a per-request token or fact-count budget
- keep ordering stable and explainable

The ranking can stay simple, but it must be deterministic.

### 8. Overview-Derived Fact Lifecycle

Overview-derived facts need deterministic replacement semantics, not today's "seed once and stop" behavior.

Requirements:
- add a persisted lineage marker on derived fact rows that identifies they were produced from `project_overview`
- when ontology extraction starts with a changed overview, delete prior overview-derived rows before regenerating
- regeneration creates fresh overview-derived rows from the new overview
- regeneration skips creating a row if it duplicates an existing non-overview fact
- overview-derived facts use inferred provenance
- the `project_overview` singleton itself remains manual user-provided context written during ontology extraction start

### 9. Observability for Misses

Because false negatives are dangerous, this system needs feedback loops:
- log facts that receive zero tags at write time
- log LLM requests that resolve to zero retrieval tags
- log high-volume tag matches that exceed the prompt budget
- build a regression corpus from real examples that were missed or over-included

Zero-tag facts should be stored with warnings rather than rejected initially. They should be treated as coverage gaps in the taxonomy/classifier system, not as a healthy steady state.

## Integration Scenarios

### 1. Ontology

Use project-knowledge retrieval for all ontology LLM requests that need business context, including:
- column feature extraction
- entity discovery
- relationship enrichment
- relationship-validation and relationship-discovery prompts
- ontology refinement or clarification prompts
- ontology question-answer processing when stored business context is relevant
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

All project-knowledge write paths must use the same canonical service behavior with the correct entry point:
- manual UI create and edit via HTTP create/update
- MCP `create_project_knowledge` and `update_project_knowledge`
- internal structured create for overview-derived seeding
- internal structured create for ontology-question answer processing
- any future API or service write path

`project_overview` is the one explicit exception: it remains a singleton special record owned by ontology extraction start and does not use the normal create/update contracts.

No write path should be able to create a new non-`project_overview` project-knowledge fact without routing through the canonical normalization/tagging/persistence pipeline.

## Implementation Plan

### Phase 1: Tag Taxonomy and Classifier Design

1. [ ] Finalize the accepted coarse `fact_type` set that remains on stored rows
2. [ ] Define special handling for `project_overview` and overview-derived lineage
3. [ ] Finalize the initial internal tag taxonomy
4. [ ] Define deterministic classifier rules for each tag
5. [ ] Document representative positive examples and tricky edge cases for each classifier
6. [ ] Bias taxonomy/rules toward recall, not minimalism
7. [ ] Define how request-kind defaults and control-plane exemptions participate in retrieval

### Phase 2: Storage Model and Write Path

8. [ ] Add a migration for tag storage plus efficient deterministic tag -> fact lookup
9. [ ] Add persisted lineage support for overview-derived rows
10. [ ] Introduce a canonical project-knowledge orchestration layer in `pkg/services/knowledge.go` or an adjacent shared service
11. [ ] Add a freeform create entry point that accepts `text` only and runs LLM extraction
12. [ ] Add an update-by-ID entry point that accepts edited `text` only and runs single-row LLM interpretation
13. [ ] Add a server-only structured create entry point for internal producers that already have structured facts
14. [ ] Normalize derived `fact_type` values before persistence
15. [ ] Run each persisted fact through deterministic classifiers before persistence
16. [ ] Implement create-time duplicate skipping using normalized text only
17. [ ] Implement update-by-ID duplicate allowance
18. [ ] Preserve source, last-edit source, and actor provenance semantics across all entry points
19. [ ] Return extracted-fact artifacts in external create/update responses without persisting them
20. [ ] Block `project_overview` from normal create/update entry points

### Phase 3: HTTP and MCP Contract Cleanup

21. [ ] Make `POST /project-knowledge` the canonical freeform create endpoint
22. [ ] Remove `/project-knowledge/parse` rather than keeping a second create path
23. [ ] Change HTTP update-by-ID request/response contracts to text-only input plus extraction artifact output
24. [ ] Remove caller-supplied `fact_type` and `context` from the UI and HTTP request types
25. [ ] Replace MCP upsert behavior with `create_project_knowledge(text)` and `update_project_knowledge(fact_id, text)`
26. [ ] Remove caller-supplied `category` and `context` from MCP contracts
27. [ ] Ensure HTTP and MCP error handling surfaces extraction failures clearly, including the existing JSON-wrapped MCP error behavior

### Phase 4: Overview Lifecycle Correction

28. [ ] Keep `project_overview` owned by ontology extraction start
29. [ ] Detect overview changes and delete prior overview-derived rows before reseeding
30. [ ] Regenerate overview-derived facts from the new overview
31. [ ] Skip regenerated rows that duplicate existing non-overview facts

### Phase 5: Deterministic Retrieval Infrastructure

32. [ ] Add a shared classifier package or service for classifying arbitrary text into project-knowledge tags
33. [ ] Add a shared retrieval function for "tags -> matching facts"
34. [ ] Exclude special control records such as `project_overview` from normal retrieval by default
35. [ ] Add a shared `KnowledgeInjectionRequest` contract for LLM call sites
36. [ ] Add deterministic ranking and prompt-budgeting rules for matched facts
37. [ ] Standardize the injected prompt section format

### Phase 6: Ontology Integration

38. [ ] Integrate retrieval into ontology column feature extraction
39. [ ] Integrate retrieval into ontology entity discovery
40. [ ] Integrate retrieval into ontology relationship enrichment and related relationship-validation prompts
41. [ ] Integrate retrieval into ontology question-answer processing where business context is relevant
42. [ ] Audit remaining eligible ontology/data-analysis LLM call sites and route them through the shared retrieval mechanism

### Phase 7: Glossary Integration

43. [ ] Integrate retrieval into glossary term discovery
44. [ ] Integrate retrieval into glossary enrichment
45. [ ] Integrate retrieval into glossary SQL-definition generation

### Phase 8: Future Text2SQL Readiness

46. [ ] Define the request kinds Text2SQL will use with this retrieval system
47. [ ] Ensure Text2SQL planning assumes reuse of this infrastructure rather than a separate project-knowledge retrieval layer

### Phase 9: Observability and Hardening

48. [ ] Log zero-tag facts after write-time classification
49. [ ] Log zero-tag request classifications before eligible LLM calls
50. [ ] Add regression fixtures for known high-value project-knowledge examples
51. [ ] Add regression fixtures for request-time retrieval examples across ontology, glossary, and relationship-validation flows
52. [ ] Add regression coverage for create dedupe, update duplicate allowance, and `project_overview` lifecycle behavior
53. [ ] Tune taxonomy and classifiers based on misses before expanding scope

## Testing Strategy

### Unit Tests

- classifier tests for every tag
- `fact_type` normalization tests
- normalization tests for duplicate detection
- phrase, synonym, and regex matching tests
- ranking/budgeting tests for matched fact selection
- fact extraction response parsing tests
- freeform create tests that verify multiple extracted facts, duplicate skipping, and returned extraction artifacts
- update-by-ID tests that verify a fact update stays a single row
- update-by-ID tests that verify `fact_type` can change and duplicates are still allowed
- structured internal create tests for normalization/tagging/provenance
- `project_overview` tests that verify it remains outside normal tagging/retrieval and normal create/update APIs
- overview-derived lineage tests for targeted cleanup

### Integration Tests

- project-knowledge create: freeform text -> extracted facts -> deterministic tags -> stored rows
- project-knowledge create duplicate behavior: duplicate extracted fact -> skipped row
- project-knowledge update by ID: one stored row -> edited row -> reclassified tags on the same row
- project-knowledge update duplicate behavior: edited row collides with another row -> duplicate allowed
- structured internal create: structured fact -> deterministic tags -> stored row without second extraction
- `project_overview`: singleton write/read path remains separate from normal project-knowledge retrieval and normal create/update contracts
- overview replacement: changed overview -> delete prior overview-derived rows -> reseed new overview-derived rows
- ontology LLM request: request text -> retrieval tags -> matched facts -> injected prompt section
- glossary LLM request: request text -> retrieval tags -> matched facts -> injected prompt section
- relationship-validation request: request text -> retrieval tags -> matched facts -> injected prompt section
- HTTP and MCP freeform writes use the same canonical extraction/tagging flow
- internal structured writes use the same normalization/tagging/provenance flow

### Recall-Focused Regression Tests

Add real examples where missing context would be unacceptable, such as:
- `Fiscal year ends June 30`
- `Revenue excludes refunds`
- `X uses a trailing four quarter average`
- domain-specific terminology definitions

The goal is not only "does classification run" but "does relevant knowledge get pulled into the right requests."
