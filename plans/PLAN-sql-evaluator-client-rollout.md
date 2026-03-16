Status: DRAFT
Created: 2026-03-16

# PLAN: SQLEvaluator Client Rollout

Parent: `plans/MASTER-PLAN-sql-evaluator-data-guardian.md`

## Objective

Migrate each SQL-producing or SQL-validating client to `SQLEvaluator` one-by-one, starting with the highest-value and lowest-risk flows, until all relevant components and exposed tools use the same evaluator.

## Why This Is a Separate Plan

The evaluator should be implemented and tested before client migrations begin. Once the evaluator exists, the highest risk is not missing code. The highest risk is migrating every caller at once and losing control of blast radius.

This plan is intentionally incremental.

## Current Caller Inventory

The repo currently has multiple caller types that should converge on the evaluator:

- glossary generation and glossary SQL testing
- saved query validation endpoints
- pending query suggestion review
- approved query creation and update review
- MCP `validate`
- MCP `query`
- MCP `execute`
- approved query execution

These callers do not all need the same mode or the same enforcement policy.

## Rollout Order

### Stage 1: Glossary Generation

Why first:

- This is the bug that triggered the work.
- It is read-only.
- The success criteria are easy to define.

Target behavior:

- Replace direct `TestSQL(...)` usage and glossary-only rule stacks with evaluator mode `glossary_example`.
- Reject wrong enum literals.
- Reject invalid join paths even when the SQL is syntactically valid.
- Reject hidden parameters and unsupported assumptions such as arbitrary time windows or geographic bounds without supporting project knowledge.
- Reject SQL whose computed metric shape diverges from the requested intent.
- Reject zero-value or empty outputs on populated paths.
- Reject example terms that touch sensitive data unless explicitly allowed by policy.
- Delay inferred-term persistence until candidates pass evaluator checks.
- Preserve manual and MCP-created glossary terms even when inferred examples are replaced.
- Preserve the successful `no_qualified_terms` terminal outcome when nothing meets the bar.

Primary touchpoints:

- `pkg/services/glossary_service.go`
- `pkg/services/glossary_autogen_sql_first.go`

### Stage 2: Query Validation Paths

Why second:

- These are lower blast-radius than execution flows.
- They establish a consistent validation experience before execution is migrated.

Target behavior:

- Saved query validation endpoint delegates to evaluator instead of syntax-only validation.
- MCP `validate` delegates to evaluator and returns structured findings.

Primary touchpoints:

- `pkg/services/query.go`
- `pkg/handlers/queries.go`
- `pkg/mcp/tools/developer.go`

### Stage 3: Pending and Approved Query Review

Why third:

- This is where admin-managed vetting becomes meaningful.
- Evaluator findings become part of approval and rejection context.

Target behavior:

- Pending query suggestions cannot be approved without passing the evaluator for the review mode.
- Approved query creation and update paths record evaluator outcomes.

Primary touchpoints:

- `pkg/services/query.go`
- query suggestion approval paths
- review-related MCP or UI flows as applicable

### Stage 4: Ad-Hoc Read Query Execution

Why fourth:

- This is operationally important, but still lower risk than modifying SQL.
- It benefits from both correctness and Data Guardian findings.

Target behavior:

- MCP `query` and other ad-hoc read-query flows call the evaluator in `read_query` mode before or during execution.
- Successful executions can store evaluator linkage in history.

Primary touchpoints:

- `pkg/mcp/tools/developer.go`
- `pkg/mcp/tools/queries.go`

### Stage 5: Approved Query Execution

Why fifth:

- This flow should benefit from prior vetting and reuse logic.
- It is a natural consumer of evaluation persistence.

Target behavior:

- Approved query execution consults evaluator or prior compatible vetted result.
- High-risk or stale vetted results trigger re-evaluation.

Primary touchpoints:

- `pkg/services/query.go`
- `pkg/mcp/tools/queries.go`

### Stage 6: Modifying Execution and Mutation Guard

Why last:

- Highest blast radius.
- Most security-sensitive.
- Depends heavily on Data Guardian policy and evaluation persistence decisions.

Target behavior:

- Any modifying `execute` flow uses `mutation_guard` mode.
- Unsafe or policy-violating mutations are blocked centrally.

Primary touchpoints:

- MCP `execute`
- any internal modifying SQL flows that currently bypass a unified guard

## Migration Rule

Do not keep parallel validation logic longer than necessary.

During migration, a caller may temporarily:

- call the evaluator first
- fall back to old logic only if required for rollout safety

But once a caller is migrated and tested, the legacy logic in that path should be removed or reduced to a thin wrapper around the evaluator.

## Caller-Specific Result Handling

Different clients will surface evaluator output differently.

Examples:

- glossary generation may use evaluator rejections to discard a candidate and retry
- `validate` endpoints should return issues and suggested fixes
- review flows should attach evaluator findings to approval decisions
- execution tools may block or warn depending on mode

For glossary example generation specifically, evaluator findings should be usable as retry input, for example:

- `invalid_enum_literal` with suggested replacement values
- `invalid_join_path` with referenced-table evidence
- `requires_parameter` for hidden time windows or bounds
- `intent_sql_mismatch` when the SQL computes a different metric than requested

The rollout should preserve one evaluator contract while allowing caller-specific presentation.

## Required Regression Coverage

Each migrated client should gain or update automated tests that prove:

- it calls the evaluator
- evaluator verdicts change behavior correctly
- the old syntax-only or shape-only blind spots are closed

This plan should not rely on manual verification tasks.

## Implementation Tasks

- [ ] Migrate glossary generation and glossary SQL testing to `SQLEvaluator`.
- [ ] Migrate saved query validation endpoint to `SQLEvaluator`.
- [ ] Migrate MCP `validate` to `SQLEvaluator`.
- [ ] Migrate pending query review and approved query management flows to `SQLEvaluator`.
- [ ] Migrate ad-hoc read-query execution flows to `SQLEvaluator`.
- [ ] Migrate approved query execution to evaluator-backed vetting.
- [ ] Migrate modifying execution flows to `mutation_guard`.
- [ ] Remove legacy validation logic that becomes redundant after rollout.
- [ ] Add regression tests for each migrated client path.

## Completion Criteria

- Every relevant SQL caller has an explicit evaluator mode.
- No major SQL path depends on a private one-off validator.
- Admin-managed approved queries and vetted historical queries both use evaluator outcomes as their trust basis.
