Status: DRAFT
Created: 2026-03-16

# MASTER PLAN: SQL Evaluator

## Why This Exists

Ekaya currently has multiple SQL validation and execution paths, each with different behavior and different blind spots. That fragmentation already produced a concrete quality failure:

- The glossary generator accepted SQL that used the wrong enum literal even though the ontology enum values were correct.
- The glossary generator accepted SQL that returned semantically useless results like `0` on a populated data path.

The core problem is not just prompt quality. The core problem is that validation is spread across multiple services and tools, with no single place that combines:

- schema validation
- ontology validation
- execution-based validation
- security classification
- reusable evaluation history

This master plan creates a single `SQLEvaluator` service first, tests it in isolation, and then migrates internal clients to it one-by-one until all SQL entry points use the same evaluator.

This initiative also absorbs the remaining valid concerns from prior glossary-only planning and issue documents. Those concerns are no longer treated as glossary-specific; they are treated as evaluator concerns that should apply to any text-to-SQL or SQL-review surface.

## Desired End State

Every internal SQL path should call the same evaluator before SQL is accepted, executed, or recorded as a trusted example. That includes:

- glossary example generation
- approved query validation
- pending query review
- MCP `validate`
- MCP `query`
- MCP `execute`
- approved query execution
- future text-to-SQL and analytics flows

The evaluator must also be extensible enough to support a security rules layer that adds policy findings such as:

- SQL injection risk
- PII or sensitive-column access
- excessive data exposure
- possible data leakage patterns

## Definitions

- `SQLEvaluator`: Shared service that evaluates `(intent, context, SQL, mode)` and returns a structured verdict with evidence.
- `Security Rules`: Security policy layer that plugs into the evaluator and emits risk findings plus allow/warn/block guidance.
- `Vetted Query`: SQL that has passed evaluator checks for a specific mode and context.
- `Vetted Historical Query`: A past query execution with stored evaluator metadata that can be reused or surfaced as trusted prior art.

## Current State in the Repo

Today, the relevant SQL paths are split across several files:

- Glossary SQL execution happens in `pkg/services/glossary_service.go`. `TestSQL` executes the query and checks only basic shape such as single-row output.
- SQL-first glossary qualification happens in `pkg/services/glossary_autogen_sql_first.go`. It does some deterministic checks, but it still does not reject important semantic failures like wrong enum literals or zero-value outputs on populated paths.
- Saved query validation in `pkg/services/query.go` is mostly syntax validation through `executor.ValidateQuery(...)`.
- MCP `validate` in `pkg/mcp/tools/developer.go` is also syntax-only.
- Query history in `pkg/models/query_history.go` records only successful executions and light classification metadata.
- Sensitive-data signals already exist in `pkg/models/column_metadata.go` via `IsSensitive` and in `pkg/mcp/tools/sensitive.go` via pattern-based detection.
- Parameter-level SQL injection checks already exist in `pkg/services/query.go`, but they are not part of a general-purpose evaluator.

This means the repo already contains useful building blocks, but they are not centralized and they are not producing a single reusable verdict.

The motivating failure classes are broader than glossary:

- wrong enum literals even when ontology values are available
- wrong join paths even when documented relationships exist
- hidden or undocumented parameters such as arbitrary time windows
- semantically useless outputs such as zero-value results on populated data paths
- SQL that computes a different metric shape than the user intent describes
- SQL that should have been rejected because the request is underspecified or not answerable from the current schema without inventing business assumptions

## Architectural Direction

### Core Principle

`SQLEvaluator` becomes the single service that owns SQL acceptance logic. Other services and tools should either:

- call it directly, or
- become thin wrappers around it

### Evaluation Model

The evaluator should be mode-aware. Different callers need different strictness.

Examples:

- `glossary_example`: strict on semantic usefulness, strict on degenerate results
- `pending_query_review`: strict on correctness and schema/ontology alignment
- `read_query`: can warn on some findings instead of blocking
- `approved_query_execution`: may reuse prior vetted results if still valid
- `mutation_guard`: strict on security and impact for modifying statements

### Deterministic First, Assisted Second

The evaluator should run in this order:

1. deterministic validation
2. execution-based validation
3. optional bounded repair loop

The repair loop is important, but it must not be the foundation. The foundation must be deterministic rules that are testable and reusable across all callers.

### Security Rules as a Layer, Not a Side System

Security rules should not be a separate SQL engine. They should be a rule set that plugs into the evaluator and returns structured findings. That keeps:

- one request contract
- one result contract
- one audit trail
- one rollout path

## High-Level Architecture

```text
Caller
  -> SQLEvaluator.Evaluate(ctx, request)
       -> Build evaluation context
       -> Run deterministic rules
       -> Optionally execute SQL and inspect results
       -> Run security rules
       -> Optionally attempt bounded repair
       -> Return verdict + evidence + suggested fix + security findings
       -> Optionally persist evaluation metadata
```

## Recommended Result Shape

The exact structs can be refined in implementation, but the evaluator should return the following classes of information:

- verdict: allow, warn, reject
- normalized SQL
- issues: correctness problems that should block or materially degrade trust
- warnings: useful but non-blocking concerns
- execution summary: row count, sample row summary, output columns, execution duration
- security findings: security rules output
- suggested fix SQL: optional
- notes: compact explanation suitable for LLM or UI consumption
- evidence: enum values, referenced tables, policy hits, result signals

For intent-aware modes, the issue model should be rich enough to represent evaluator outcomes such as:

- `invalid_enum_literal`
- `invalid_join_path`
- `requires_parameter`
- `unsupported_assumption`
- `intent_sql_mismatch`
- `degenerate_result`

## Rollout Strategy

The system should be implemented in this order:

1. Build and test `SQLEvaluator` core in isolation.
2. Add security rule findings to the evaluator, still with no major client migrations.
3. Add evaluation persistence so vetted outcomes can be reused and audited.
4. Migrate internal clients one-by-one until legacy validators are removed.

This order matters because it keeps the early work testable and low-risk. It also avoids coupling client migrations to unfinished storage or policy design.

## What This Plan Intentionally Does Not Do

- It does not build a free-form autonomous SQL agent.
- It does not require a huge context window.
- It does not overload the first implementation with every security policy.
- It does not add legacy fallback paths by default.

## Design Constraints

- Keep handler logic thin. The evaluator belongs in the service layer.
- Keep metadata-store SQL in repositories, not in handlers or MCP tools.
- Respect RLS and tenant scoping for every evaluation.
- Preserve provenance and auditability when queries or glossary terms are accepted because of evaluator results.
- Do not assume a single caller or a single datasource mode forever.

## Key Risks

- If the evaluator request/result contract is too narrow, later clients will bypass it.
- If the evaluator stores too much raw execution output, it can create new privacy problems.
- If policy rules are mixed into client code instead of the evaluator, the fragmentation problem will return.
- If repair logic is added before deterministic rules are solid, debugging will become difficult.

## Recommended Sub-Plan Order

1. `plans/PLAN-sql-evaluator-core.md`
2. `plans/PLAN-sql-evaluator-security-rules.md`
3. `plans/PLAN-sql-evaluator-history-vetting.md`
4. `plans/PLAN-sql-evaluator-client-rollout.md`

## Exit Criteria

- All SQL entry points route through `SQLEvaluator`.
- Glossary generation rejects wrong enum literals, degenerate outputs, and semantically useless example SQL.
- Security rule findings are available to callers and can block high-risk flows.
- Approved queries and vetted historical queries can be distinguished from unvetted SQL.
- Query history and evaluation storage are rich enough to support future query reuse, admin review, and external-request vetting.

## Open Questions to Resolve During Implementation

- Should `engine_sql_evaluations` be a new table, or should evaluation metadata extend an existing history table.
- How strict should each mode be about zero-row or zero-value results.
- Which modifying operations should ever be eligible for prior-vetted reuse.
- Whether the first repair loop should live inside the evaluator package or behind an interface injected by callers.
