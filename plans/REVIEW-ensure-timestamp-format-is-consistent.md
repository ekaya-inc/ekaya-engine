# REVIEW PLAN: Ensure Timestamp Format Is Consistent

**Status:** NOT STARTED
**Branch:** ddanieli/add-ai-agents

## Objective

Review the project end to end to verify that timestamps exposed by APIs, UI-facing service types, logs intended for machine consumption, and related tests use a consistent and intentional format.

This is a review and verification plan only. It does not authorize implementation changes by itself.

## Scope

In scope:

- Go HTTP handlers and JSON response shaping
- Go services and repositories where timestamps are converted to strings
- Frontend API clients, types, and UI assumptions about timestamp strings
- Tests that encode timestamp expectations
- Documentation that implies a timestamp contract

Out of scope:

- Raw database storage types such as `timestamptz`
- Human-only freeform log messages unless they define a programmatic contract
- External vendor payloads we only pass through unchanged

## Context

The codebase already shows multiple timestamp serialization patterns, including:

- `http.TimeFormat` in [agent_handler.go](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/pkg/handlers/agent_handler.go#L353)
- `time.RFC3339` in handlers and repositories such as [ai_config.go](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/pkg/handlers/ai_config.go#L115)
- Literal RFC3339 layouts like `"2006-01-02T15:04:05Z07:00"` in handlers such as [queries.go](/Users/damondanieli/go/src/github.com/ekaya-inc/ekaya-engine/pkg/handlers/queries.go#L1039)

That raises several review risks:

- Two endpoints may return the same conceptual field in different formats
- Frontend code may parse some timestamps successfully and mis-handle others
- Tests may normalize inconsistencies instead of detecting them
- New endpoints may copy the wrong precedent
- API consumers may treat timestamp format as unstable

## Review Questions

1. What timestamp string formats are currently emitted by each external API surface?
2. Are semantically equivalent fields such as `created_at`, `updated_at`, `reviewed_at`, `last_used_at`, and `completed_at` consistent across endpoints?
3. Is there an explicit project convention for machine-readable timestamps?
4. Do frontend types and display code assume one format while some endpoints emit another?
5. Do tests assert a consistent contract, or do they merely accept mixed formats?
6. Are there any justified exceptions where a different format should remain?

## Deliverables

The review should produce:

- A matrix of timestamp-producing code paths and their current formats
- A recommended project-wide timestamp standard for external contracts
- A list of justified exceptions, if any
- Findings grouped by severity
- A remediation plan for code, tests, and documentation if inconsistencies exist

## Method

### Phase 1: Inventory Timestamp Producers

Create a complete inventory of project code that converts `time.Time` values to strings for externally consumed outputs.

Search targets:

- `time.RFC3339`
- `http.TimeFormat`
- literal layouts such as `"2006-01-02T15:04:05Z07:00"`
- `.Format(...)` calls in handlers, API adapters, and machine-readable logs
- custom formatter helpers, if any

For each occurrence, record:

- File and function
- Field names involved
- Output surface: REST API, MCP payload, frontend service object, audit export, or log
- Format used
- Whether the field is user-visible, machine-consumed, or both

Expected output:

- A repo-wide timestamp-format inventory

### Phase 2: Group by External Contract

Group the inventory by API/resource family rather than by file.

Examples:

- Queries endpoints
- Datasource endpoints
- Ontology and glossary endpoints
- MCP/audit endpoints
- AI config endpoints
- AI agent endpoints

For each family, answer:

- Which timestamp fields are returned?
- Are they all in the same format?
- Does the format match similar endpoints elsewhere in the product?

### Phase 3: Review Frontend Assumptions

Inspect frontend types and consumers to determine how timestamp strings are interpreted.

Checks:

- Type definitions that treat timestamps as `string`
- UI code that passes timestamps into `new Date(...)`
- Sorting, filtering, and display logic that assumes ISO-like parseability
- Tests and fixtures that imply a preferred format

Special attention:

- Pages rendering dates from multiple endpoints
- Any code that depends on locale parsing or browser-specific behavior

### Phase 4: Review Existing Conventions and Implicit Standards

Determine whether the codebase already has a de facto standard.

Evidence sources:

- The majority format used in REST handlers
- Existing tests and snapshots
- Any docs or comments that describe timestamp fields
- Existing frontend expectations

The review should explicitly decide whether the project standard should be:

- RFC3339 via `time.RFC3339`
- RFC3339Nano
- another precise layout

The likely default for machine-readable JSON should be ISO/RFC3339 unless a documented reason says otherwise, but the review must verify rather than assume.

### Phase 5: Identify Inconsistencies and Their Impact

For each mismatch, record:

- Exact location
- Current format
- Expected or recommended format
- Whether the issue is cosmetic, a compatibility hazard, or a functional bug

Examples of high-signal mismatches:

- One endpoint uses RFC1123 while similar endpoints use RFC3339
- A timestamp string is emitted without timezone information
- A frontend parser depends on ISO but receives a non-ISO format
- Tests encode contradictory expectations for the same kind of field

### Phase 6: Review Test Coverage

Audit tests to see whether timestamp formats are actually asserted.

Checks:

- Handler tests that validate serialized JSON timestamps
- Frontend tests that parse or display timestamps
- Integration tests covering representative resources

Determine whether the suite would catch:

- An accidental switch from RFC3339 to RFC1123
- Missing timezone offsets
- Mixed timestamp formats within a single endpoint family

### Phase 7: Define the Remediation Plan

If inconsistencies are found, the follow-up plan should specify:

- Which format becomes the standard
- Which files must change
- Which tests must be added or updated
- Whether any API compatibility note is required
- Whether a shared helper should be introduced to prevent future drift

This phase should not execute changes. It should only define the fix strategy.

## Acceptance Criteria

This review is complete when all of the following are true:

- Every externally visible timestamp formatter has been inventoried
- Timestamp outputs are grouped by endpoint/resource family
- Frontend assumptions about timestamp parseability are documented
- The project’s intended standard format is explicitly identified
- Any justified exceptions are documented
- Inconsistencies and their impact are recorded
- Test coverage gaps are documented
- A concrete remediation plan is ready if changes are needed

## Expected Output Files After Review

The review itself should eventually produce one or more of:

- `plans/ISSUE-*.md` for timestamp-contract bugs
- `plans/FIX-*.md` for narrow formatting remediations
- `plans/PLAN-*.md` for broader API consistency work

This file is only the review plan.
