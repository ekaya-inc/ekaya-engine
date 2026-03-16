Status: DRAFT
Created: 2026-03-16

# PLAN: SQLEvaluator Core

Parent: `plans/MASTER-PLAN-sql-evaluator.md`

## Objective

Build the shared `SQLEvaluator` service first and test it in isolation before wiring any clients to it. This plan should leave the repo with a stable evaluator API, deterministic rule framework, execution-summary logic, and enough test coverage that later client migrations can proceed with confidence.

## Why This Is First

Without a stable evaluator contract, every downstream migration will invent its own version of:

- how evaluation context is built
- how issues and warnings are represented
- how execution results are summarized
- how callers distinguish reject vs warn vs allow

This plan prevents that drift.

## Current Repo Touchpoints

The new evaluator is expected to replace or absorb behavior currently spread across:

- `pkg/services/glossary_service.go`
  - `TestSQL(...)` executes SQL and checks basic output shape only.
- `pkg/services/glossary_autogen_sql_first.go`
  - glossary-specific deterministic checks exist here today, mixed into glossary generation logic.
- `pkg/services/query.go`
  - saved query validation currently relies on `executor.ValidateQuery(...)`.
- `pkg/mcp/tools/developer.go`
  - MCP `validate` is syntax-only today.

The new evaluator should centralize reusable logic so these callers stop carrying their own validation stacks.

## Scope

- Core evaluator request/response types.
- Evaluation-context loading.
- Deterministic rule engine.
- Execution-summary and degenerate-output inspection.
- Mode-based strictness and behavior.
- Hooks for optional repair logic, but not a full repair implementation.

## Non-Goals

- Full security policy logic.
- Full persistence of evaluation outcomes.
- Full client migration.

## Recommended Package Shape

Recommended home:

- `pkg/services/sql_evaluator`

Recommended top-level components:

- `evaluator.go`
- `types.go`
- `context_builder.go`
- `rules.go`
- `execution_checks.go`
- `modes.go`
- `repair.go` or `repair_hook.go`

The exact file split can change, but the package should have a clear public entry point and private rule implementations.

## Proposed API

Recommended interface:

```go
type SQLEvaluator interface {
    Evaluate(ctx context.Context, req EvaluationRequest) (*EvaluationResult, error)
}
```

Recommended request fields:

- `ProjectID`
- `DatasourceID`
- `UserID`
- `Mode`
- `Intent`
- `SQL`
- `NaturalLanguageContext`
- `AllowExecution`
- `AllowRepair`
- `Parameters` or bound parameter metadata where relevant
- `ExpectedTables` or caller-supplied scoping hints when appropriate

Recommended result fields:

- `Verdict`
- `NormalizedSQL`
- `Issues`
- `Warnings`
- `ExecutionSummary`
- `SecurityFindings`
- `SuggestedFixSQL`
- `Notes`
- `Evidence`

Recommended issue categories for correctness-oriented modes:

- `invalid_enum_literal`
- `missing_table_or_column`
- `invalid_join_path`
- `requires_parameter`
- `unsupported_assumption`
- `intent_sql_mismatch`
- `degenerate_result`

## Evaluation Modes

The evaluator should not behave the same way for every caller. Define modes up front.

Suggested initial modes:

- `glossary_example`
- `pending_query_review`
- `approved_query_review`
- `read_query`
- `approved_query_execution`
- `mutation_guard`

Each mode should define:

- whether non-`SELECT` SQL is allowed
- whether execution is required
- whether repair is allowed
- whether warnings are blocking
- how strict degenerate-result checks should be

## Evaluation Pipeline

Recommended execution order:

1. Build evaluation context.
2. Canonicalize or normalize SQL as needed.
3. Run deterministic pre-execution rules.
4. If still eligible, execute the SQL or a safe test form.
5. Run execution-result checks.
6. If enabled, invoke bounded repair hook.
7. Return structured result.

This order keeps the evaluator explainable and testable.

## Evaluation Context

The evaluator should load and cache only the information required for the current request. It should not require the caller to manually assemble deep project context.

Recommended context contents:

- project and datasource info
- schema tables and columns
- column metadata
- ontology enum values and conventions
- project knowledge summary
- glossary summary when useful
- query executor

Recommended rule:

- Context should be compact and mode-aware so future repair loops do not depend on a large context window.

## Deterministic Rules to Implement in Phase 1

Minimum initial rules:

- syntax validation
- statement-class validation
- table existence
- column existence
- enum literal validation against ontology
- join/reference policy hooks
- convention checks hook
- hidden-parameter and unsupported-assumption checks
- intent/metric-shape validation hook for strict intent-aware modes

Enum validation is important enough to call out explicitly. This is the class of failure that produced `status = 'Completed'` when the ontology already knew the enum value was `Complete`.

The rule set should also absorb other failure classes that were previously observed in glossary generation:

- arbitrary hidden parameters such as `30 days` when no project knowledge justifies that window
- relationship-path mismatches when documented joins exist
- SQL that computes a different shape than the requested metric, such as a completeness percentage instead of a density metric
- SQL that should be refused because the request is underspecified without new business assumptions

## Execution Checks to Implement in Phase 1

Execution checks should be mode-aware and return summaries, not raw unbounded results.

Minimum execution outputs:

- output columns
- row count
- one-row sample summary for aggregate-like queries
- execution duration
- lightweight typed value summary where useful

Minimum execution-result rules:

- empty result rejection where the mode expects a meaningful example
- all-null result rejection
- degenerate zero-value rejection for strict modes such as `glossary_example`
- multi-row rejection when the mode expects one row

The evaluator should not assume `0` is always wrong. It should let the mode define whether zero on a populated path is acceptable.

For strict example-generation modes, execution checks should be able to distinguish:

- a legitimate zero over a credible populated path
- a degenerate zero caused by a broken join, dead filter, or unpopulated source path

## Repair Hook

This plan should define an interface for bounded repair, but not require a full implementation yet.

Recommended shape:

```go
type RepairProvider interface {
    AttemptRepair(ctx context.Context, req EvaluationRequest, prior *EvaluationResult) (*RepairAttemptResult, error)
}
```

The evaluator should be able to run without any repair provider.

## Tests Required in This Plan

This plan should leave strong unit coverage behind because every later client migration will rely on it.

Required test categories:

- syntax failure vs syntax success
- enum mismatch detection
- missing table and missing column detection
- invalid join-path detection
- hidden-parameter or unsupported-assumption detection
- intent/metric-shape mismatch detection in strict modes
- mode-specific non-`SELECT` behavior
- single-row vs multi-row behavior
- all-null and zero-value degenerate-result detection
- context-loading behavior with mock repositories and mock executors

## Expected Integration Seams After This Plan

At the end of this plan, the evaluator should exist, but existing clients can still be using their old paths. That is intentional. The output of this plan is the evaluator service itself, not the rollout.

## Implementation Tasks

- [ ] Add a new `pkg/services/sql_evaluator` package with public evaluator interface and core types.
- [ ] Implement mode definitions and mode-specific policy defaults.
- [ ] Implement evaluation-context loading using existing schema, metadata, and knowledge services/repositories.
- [ ] Implement deterministic rule registry and base rule set.
- [ ] Implement execution-summary and degenerate-result checks.
- [ ] Add optional repair hook interface without requiring a repair implementation.
- [ ] Add focused unit tests for rule behavior and execution-result handling.

## Completion Criteria

- `SQLEvaluator.Evaluate(...)` exists and is stable enough for client rollout work.
- Core deterministic rules are no longer glossary-specific.
- The evaluator can already catch the enum-literal and degenerate-result failures that motivated this work.
- The evaluator package has unit tests that isolate rule and execution behavior without needing client integration tests.
