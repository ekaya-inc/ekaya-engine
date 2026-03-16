# PLAN: SQL-First Example Glossary Generation

**Date:** 2026-03-16
**Status:** TODO
**Priority:** HIGH
**Supersedes:** `plans/PLAN-glossary-iterate-over-generated-sql.md`

## Goal

Replace the current glossary auto-generation flow with a SQL-first pipeline that produces **example glossary terms** only when the system can derive and verify high-quality SQL from the current ontology, schema, and data.

This is not intended to guess the admin's final business glossary. It is intended to generate a small set of reliable example terms that get the admin started without embarrassing SQL failures.

## Problem

The current flow is:

1. generate glossary terms first
2. persist those terms
3. attempt to generate SQL for them afterward

That design fails in the wrong direction:

- it optimizes for term plausibility before SQL correctness
- it persists glossary rows before SQL is proven
- it encourages text-to-SQL to invent formulas for terms that may be underspecified or poorly grounded
- it can produce valid-looking but incorrect SQL, which is worse than producing fewer terms

The new plan must invert that priority:

- SQL quality first
- term/definition writing second

## Product Positioning

Auto-generated glossary terms are **example terms to get the admin going**.

Implications:

- relevance matters, but not more than SQL correctness
- a smaller number of strong examples is better than a larger number of weak ones
- zero generated terms is an acceptable outcome if nothing meets the bar

## Required Outcomes

1. Do not persist any inferred glossary row until its SQL has passed the quality bar.
2. Save only qualified example terms.
3. Target 5-8 terms, but do not force a minimum.
4. If zero candidates qualify, return a successful terminal outcome that clearly tells the user no term met the bar.
5. Never pad the glossary with low-confidence or broken examples just to hit a count.

## Current Code Context

The current implementation lives primarily in:

- `pkg/services/glossary_service.go`
  - `RunAutoGenerate`
  - `DiscoverGlossaryTerms`
  - `EnrichGlossaryTerms`
  - `TestSQL`
- `pkg/handlers/glossary_handler.go`
  - `AutoGenerate`
- `pkg/models/glossary.go`
  - `GlossaryGenerationStatus`
- `ui/src/pages/GlossaryPage.tsx`
  - empty-state generation UX
  - polling and terminal-state handling

The current code path is term-first:

- `RunAutoGenerate` calls discovery, then enrichment
- discovery persists inferred glossary rows before SQL exists
- enrichment later attempts to generate SQL for those rows

The existing behavior that should remain unless explicitly changed in this plan:

- `POST /api/projects/{pid}/glossary/auto-generate` stays asynchronous
- the handler still gates generation on required ontology questions
- the UI still polls glossary list/status after generation starts
- manual and MCP-created glossary terms remain supported and should not be affected by inferred-term replacement

The existing helper that should be reused:

- `TestSQL` remains the execution gate for candidate SQL
- current schema/column validation helpers should be reused where possible
- glossary repository `DeleteBySource` should be reused for inferred-term replacement

## Explicit Implementation Decisions

These decisions are part of the plan of record and should not be re-decided during implementation.

### 1. Rerun Replacement Policy

Auto-generation is generating **example inferred terms** only.

On each auto-generate run:

- do not delete existing inferred terms at the start of the run
- generate new candidates entirely in memory
- if at least one candidate qualifies:
  - delete existing inferred glossary terms for the project
  - persist the newly qualified inferred terms
- if zero candidates qualify:
  - leave existing glossary terms unchanged
  - return the `no_qualified_terms` terminal outcome

Manual and MCP-created glossary terms must never be deleted by auto-generate.

### 2. Candidate Volume Limits

The overview/planning step should request **up to 12 investigation areas**.

Implementation policy:

- process at most the top 12 ranked areas returned by the planner
- each investigation area may produce at most 1 final metric candidate
- persist at most 8 qualified candidates after scoring/sorting

This keeps the first implementation simple and prevents unbounded exploration.

### 3. Retry Policy

For each investigation area:

- allow up to 3 SQL-generation attempts total
  - attempt 1: initial SQL candidate from investigation evidence
  - attempt 2: retry with validation error feedback
  - attempt 3: final retry with validation error feedback
- if the SQL still does not qualify after attempt 3, reject the area
- do not continue retrying until something merely plausible appears

For glossary writing after SQL qualification:

- allow 1 generation attempt for term/definition/aliases
- if the wording is unusable, retry once
- if wording is still poor, keep the qualified SQL candidate but give it a low presentation score rather than rejecting the SQL

### 4. Execution Model

The first implementation should be **sequential**, not highly parallel.

Rationale:

- it is easier to reason about status reporting
- it avoids concurrent LLM/database state issues during a large behavior change
- performance is secondary to correctness for this feature

Concurrency can be added later if needed.

### 5. Status/API Contract

`GlossaryGenerationStatus.Status` should become:

- `idle`
- `planning`
- `investigating`
- `qualifying`
- `writing`
- `completed`
- `no_qualified_terms`
- `failed`

Contract:

- `completed` means 1 or more inferred terms were qualified and saved
- `no_qualified_terms` means the pipeline ran successfully but saved zero terms
- `failed` means the pipeline itself errored or could not complete

The frontend should treat `planning`, `investigating`, `qualifying`, and `writing` as in-progress states.

### 6. Qualification vs Ranking

Qualification and ranking are separate.

- qualification decides whether a candidate may be persisted
- ranking decides the order among qualified candidates

A candidate must:

- pass every required gate, and
- meet the technical score threshold defined below

Only qualified candidates are ranked against one another.

### 7. Technical Score Model

Use a deterministic technical score with maximum 90 points.

#### Technical score components

- `investigation_priority` 0-20
  - derived from planner rank
  - rank 1 should score highest
- `relationship_confidence` 0-20
  - single-table metric or explicit documented join path scores highest
  - longer but fully documented FK/ontology paths score lower but can still qualify
- `semantic_signal_strength` 0-20
  - based on meaningful measure/status/time columns and ontology metadata
- `data_support` 0-20
  - based on row availability, non-null support, join viability, and whether the metric is backed by real populated data paths
- `execution_stability` 0-10
  - first-attempt success scores highest
  - third-attempt success scores lowest

#### Technical qualification threshold

- minimum technical score to qualify: **60 / 90**

This threshold applies only after all required gates pass.

### 8. Presentation Score Model

After SQL qualification, assign a presentation score with maximum 10 points.

- `term_clarity` 0-5
- `definition_clarity` 0-5

Final ranking score:

- `technical_score + presentation_score`
- maximum 100

Presentation score affects ordering but does not rescue a technically unqualified candidate.

### 9. Degenerate Result Policy

Do not reject a candidate solely because the final aggregate value is `0`.

Reject only when there is no credible supporting data path, for example:

- the source tables are empty
- required measure/status/time columns are effectively unpopulated
- the documented join path produces no joined rows
- the SQL only "works" because every relevant filter collapses to an empty set

A zero result is acceptable if the underlying data path is real and populated.

### 10. Convention Handling

The following conventions are correctness requirements when applicable:

- exact enum values from ontology/project metadata
- soft-delete filters when the project conventions define them for tables used by the metric
- currency normalization when project conventions indicate stored cents or basis points

If a candidate SQL ignores one of these applicable conventions, reject it or force a retry.

### 11. Hidden Parameter Policy

Reject candidates that require undocumented parameters or business assumptions, including:

- arbitrary time windows such as `30 days` unless supported by project knowledge or explicit domain convention
- geographic bounds when no bounds are defined
- custom cohort definitions not backed by ontology/project knowledge
- proxy formulas presented as if they were definitive metrics

If a useful metric requires such parameters, it should not be auto-generated in this change.

## New Generation Model

### Phase 1: Overview / Investigation Planning

Make one overview LLM call that receives:

- project knowledge
- domain summary and conventions
- ontology-backed schema structure
- key measures, dimensions, identifiers, roles
- high-level table inventory and relationship cues

It must return **ranked investigation areas**, not glossary terms.

Examples of acceptable investigation areas:

- revenue and payout path
- completed transaction funnel
- engagement duration path
- user participation path
- inventory movement path

Examples of unacceptable output:

- finalized glossary terms with definitions
- free-form SQL
- vague generic business suggestions disconnected from the schema

The planner response should be typed and constrained.

Recommended response shape:

```json
{
  "areas": [
    {
      "rank": 1,
      "area": "revenue and payout path",
      "business_rationale": "Revenue and payout quality are central to this business model.",
      "tables": ["billing_transactions", "billing_payouts"],
      "focus_columns": ["amount_cents", "status", "created_at"]
    }
  ]
}
```

Rules:

- `area` is a business investigation label, not a glossary term
- `tables` and `focus_columns` are hints, not authority
- ranking is ordinal and should be preserved by the service

### Phase 2: Investigation

For each ranked area, gather deterministic evidence before asking for metric SQL:

- relevant tables
- available join paths
- candidate measure columns
- candidate grouping and status columns
- time columns
- enum values and role semantics
- lightweight data evidence needed to understand whether the area is populated

This phase should prefer actual schema and ontology evidence over LLM intuition.

The investigation phase should produce a typed in-memory structure. Suggested fields:

- area label and planner rank
- candidate tables
- allowed relationship edges / join paths
- candidate measure columns
- candidate status columns
- candidate time columns
- applicable conventions
- enum/sample information
- per-table row counts
- per-column non-null support for candidate columns
- join viability counts for the preferred path

### Deterministic Data Probes

Use the datasource executor directly from the service layer. Do not route this through MCP tools.

The first implementation should use simple dialect-neutral probes only:

- `COUNT(*)` for table row counts
- `COUNT(column)` for non-null support
- grouped counts for status/enum-like columns
- limited sample queries without embedding dialect-specific `LIMIT`/`TOP`
  - rely on the executor's row-limit handling when possible

Avoid dialect-specific probe SQL in the first implementation.

The investigation phase does not need deep profiling. It needs enough evidence to answer:

- is this area populated?
- are there meaningful measure/time/status columns?
- does the preferred join path produce rows?
- are the enum values/conventions known?

### Phase 3: Verified Metric Construction

For each investigation area, generate candidate SQL and iterate until it either:

- reaches the quality bar, or
- is rejected as unqualified

The loop may use LLM assistance, but qualification must be mostly deterministic.

The SQL-generation prompt should be grounded in:

- one investigation area
- the allowed tables and join paths
- exact column names/types
- enum values
- applicable conventions
- data support findings

The prompt should not ask for free-form ideation beyond that area.

### Allowed Join Policy

The first implementation should use a strict deterministic policy:

- every join in candidate SQL must be explainable by:
  - an explicit schema FK relationship, or
  - an ontology relationship already present in metadata
- if the SQL joins tables on columns that do not match a documented relationship edge, reject the candidate
- if multiple plausible relationship chains could connect the same tables and the SQL does not clearly follow one documented chain, reject the candidate

This is intentionally strict. It is acceptable to reject some otherwise interesting candidates in favor of reliability.

### Phase 4: Glossary Writing from Verified SQL

Only after a candidate SQL query is qualified should the system generate:

- term
- definition
- aliases

That writing step should be grounded in:

- the verified SQL
- the source tables and columns
- the observed result shape
- the evidence gathered during investigation

Recommended response shape:

```json
{
  "term": "Net Revenue",
  "definition": "Total revenue captured from completed billing transactions after applying the documented currency normalization and status filters.",
  "aliases": ["Revenue", "Total Revenue"]
}
```

The writing prompt must explicitly state:

- do not claim semantics not present in the SQL
- do not introduce time windows or other assumptions absent from the SQL
- keep the wording aligned to what the SQL actually computes

### Phase 5: Ranking and Persistence

Score qualified candidates, sort by quality, and persist the best set.

Rules:

- persist only qualified candidates
- sort by score descending
- keep up to 8
- fewer than 5 is acceptable
- zero is acceptable

## Quality Bar

Qualification should not be mostly LLM judgment.

### Required Gates

A candidate must satisfy all of the following:

- SQL executes successfully against the datasource
- SQL returns exactly one row
- SQL references only valid schema objects
- SQL does not depend on an ambiguous or obviously wrong join path
- SQL does not require hidden parameters or undocumented business assumptions
- SQL is tied to a ranked investigation area

If any required gate fails, reject the candidate.

Additional required gate details:

- the SQL must be a single self-contained `SELECT`
- the SQL must produce a stable, non-error result on the current datasource
- every join must satisfy the allowed join policy
- applicable project conventions must be honored
- the metric must be implementable without undocumented parameters

### Positive Quality Signals

These increase score but are not individually required:

- uses semantically meaningful measure/dimension/status/time columns
- uses ontology/project-knowledge-backed conventions correctly
- has populated supporting data
- produces a non-degenerate result
- required little or no retrying
- investigation area ranked highly
- resulting term/definition are clear and business-appropriate

### Rejection Cases

Reject, do not persist, and do not soften the rules when:

- the SQL only works by making undocumented business assumptions
- the metric requires missing parameters to be meaningful
- the SQL is syntactically valid but semantically weak or clearly proxy-based
- the datasource has no credible data path for the candidate
- the generated SQL repeatedly fails and only reaches a "plausible" but untrusted form

## Zero-Qualified Outcome

If no candidate reaches the quality bar, that is a valid generation outcome.

This must not be treated as a pipeline failure.

Add a distinct terminal status for this case, for example:

- `no_qualified_terms`

Expected UX behavior:

- show a clear explanatory message
- offer `Try Again`
- offer `Add Term`
- do not show a generic failure state

Suggested user message:

> No example glossary terms met the quality bar for this project. The current ontology and data did not produce any verified example metrics with reliable SQL. You can try auto-generate again or add a term manually.

If this outcome occurs during a rerun and existing inferred terms are already present:

- keep those inferred terms unchanged
- surface the `no_qualified_terms` message anyway
- do not clear the page into an empty state

The message in that case can be adjusted slightly to clarify that existing example terms were preserved.

## Architecture Notes

- Keep the orchestration in `GlossaryService`.
- Reuse existing datasource execution and validation behavior where possible.
- Keep glossary persistence delayed until qualification is complete.
- Do not introduce fallback persistence of partially qualified terms.
- Do not keep the old term-first and new SQL-first pipelines alive together.

Recommended service-level flow:

1. resolve tenant context and inferred provenance
2. set status `planning`
3. plan investigation areas
4. for each area:
   - set status `investigating`
   - gather deterministic evidence
   - set status `qualifying`
   - generate and validate SQL up to 3 attempts
   - if qualified, set status `writing`
   - write glossary text and score candidate
5. sort qualified candidates
6. if any qualified:
   - replace existing inferred terms
   - persist new inferred terms
   - set status `completed`
7. if none qualified:
   - persist nothing
   - set status `no_qualified_terms`
8. on unexpected pipeline error:
   - set status `failed`

## Suggested In-Memory Structures

The implementation does not need these exact names, but it should have equivalent typed structures:

- `InvestigationArea`
  - planner rank
  - area label
  - business rationale
  - hinted tables
  - hinted focus columns
- `InvestigationEvidence`
  - area metadata
  - allowed tables
  - allowed relationship edges
  - candidate measure/status/time columns
  - enum values
  - applicable conventions
  - row-count/non-null/join-support findings
- `MetricCandidate`
  - source area
  - SQL
  - base table or root table
  - output columns
  - technical score
  - presentation score
  - final score
  - qualification result
  - rejection reason if not qualified
- `GlossaryWriteResult`
  - term
  - definition
  - aliases

## Implementation Tasks

## Task 1: Replace term-first generation orchestration

- Remove the current assumption that discovery creates glossary rows before SQL exists.
- Replace the `discover -> enrich` flow with `plan investigations -> investigate -> qualify SQL -> write glossary -> persist`.
- Update status reporting to reflect the new pipeline phases and terminal outcomes.

## Task 2: Add investigation-area planning

- Introduce a planning step that produces ranked investigation areas from ontology/schema/project knowledge.
- Define a typed response for investigation areas and ranking metadata.
- Reject low-value or generic investigation areas early.

## Task 3: Add deterministic investigation gathering

- Build helpers that collect the evidence needed to attempt strong metric SQL:
  - tables
  - columns
  - relationships
  - enums
  - semantic roles
  - lightweight data support
- Keep this grounded in actual schema and available metadata rather than term prose.
- Use simple dialect-neutral datasource probes in the first implementation.

## Task 4: Add SQL qualification loop

- Generate SQL candidates per investigation area.
- Validate each candidate with existing SQL execution checks.
- Add deterministic qualification checks beyond "query runs":
  - valid references
  - single-row aggregate
  - credible join path
  - no hidden-parameter dependency
  - no obvious semantic degradation
- Track retry attempts and final qualification outcome per candidate.
- Enforce the technical score threshold before a candidate may qualify.

## Task 5: Add glossary writing from verified SQL

- Generate term/definition/aliases only after SQL is qualified.
- Ground the writing prompt in verified SQL and investigation evidence.
- Ensure the final glossary text does not claim semantics that exceed what the SQL actually computes.

## Task 6: Add scoring, sorting, and persistence rules

- Add a candidate score model with deterministic gates and weighted positive signals.
- Sort qualified candidates by score.
- Persist at most 8 terms.
- Allow fewer than 5.
- Persist zero when none qualify.
- Replace inferred terms only after at least one new qualified candidate exists.

## Task 7: Add zero-qualified UX/state handling

- Add a distinct terminal status for successful runs with zero qualified terms.
- Update the glossary page to render a dedicated message and actions for that case.
- Ensure polling and toast behavior handle this as a successful completion, not a failure.
- Preserve any existing inferred terms if a rerun ends in `no_qualified_terms`.

## Task 8: Update tests

- Replace tests that assume term-first discovery followed by enrichment.
- Add service tests for:
  - zero qualified terms
  - partial success with fewer than 5 terms
  - qualification rejecting otherwise plausible candidates
  - persistence only after qualification
  - rerun preserving prior inferred terms when zero new candidates qualify
  - rerun replacing prior inferred terms when new candidates do qualify
  - new status progression through `planning`, `investigating`, `qualifying`, `writing`
- Add handler/UI tests for the `no_qualified_terms` terminal state.

## Non-Goals

- No attempt to infer the admin's definitive business glossary
- No fallback path that persists unverified inferred terms
- No forced minimum term count
- No manual-review workflow for generated terms in this change
- No broad rewrite of manual glossary creation/editing

## Files Likely Affected

- `pkg/services/glossary_service.go`
- `pkg/models/glossary.go`
- `pkg/handlers/glossary_handler.go`
- `ui/src/pages/GlossaryPage.tsx`
- glossary service, handler, and UI tests

## Success Criteria

- Auto-generated glossary entries only appear when backed by verified SQL.
- The system may save 1-8 example terms, not necessarily 5-8.
- Zero qualified terms is surfaced clearly as a successful but empty outcome.
- The product no longer presents broken or weak inferred glossary SQL as a completed result.
