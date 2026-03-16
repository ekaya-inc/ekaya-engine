Status: DRAFT
Created: 2026-03-16

# PLAN: Data Guardian Rules for SQLEvaluator

Parent: `plans/MASTER-PLAN-sql-evaluator-data-guardian.md`

## Objective

Add a security-oriented rule layer to `SQLEvaluator` that produces structured findings about risk, while keeping the initial implementation practical and incremental. This plan is about making the evaluator security-aware without turning it into a giant policy engine on day one.

## Why This Is a Separate Plan

Correctness and security are related, but they are not the same concern.

- Core evaluator work should establish the shared evaluation contract first.
- Data Guardian should plug into that contract cleanly.

Keeping this in a separate plan avoids blocking the core evaluator on every future security policy detail while still forcing the architecture to support those policies.

## Current Repo Touchpoints

The repo already has several security-related building blocks:

- `pkg/models/column_metadata.go`
  - `IsSensitive` supports column-level sensitive-data overrides.
- `pkg/mcp/tools/sensitive.go`
  - pattern-based sensitive column and sensitive content detection already exists.
- `pkg/services/query.go`
  - parameter-level injection detection exists for parameterized query execution.
- MCP audit and alerting code already has concepts for sensitive access and security classification.

These are useful, but they are not currently unified behind a single SQL evaluation result.

## Scope

- Define Data Guardian finding types.
- Add Data Guardian rules to the evaluator pipeline.
- Add enforcement policy by mode.
- Return security findings in a way callers can consume directly.

## Non-Goals

- Full policy authoring UI.
- Full row-level data-loss-prevention engine.
- Full legal/compliance classification matrix.

## Design Goal

The evaluator should be able to answer:

- Is this SQL safe enough to run in this mode.
- What sensitive data does it touch.
- What risk signals were detected.
- Should the caller allow, warn, or block.

## Recommended Security Finding Model

Use structured findings instead of string-only warnings.

Recommended fields:

- `Category`
- `Severity`
- `Action`
- `Summary`
- `Evidence`
- `AffectedTables`
- `AffectedColumns`
- `RuleID`

Suggested categories:

- `sql_injection_risk`
- `sensitive_column_access`
- `sensitive_content_risk`
- `data_leakage_risk`
- `unsafe_modification_risk`

Suggested actions:

- `allow`
- `warn`
- `block`
- `require_review`

## Recommended Rule Sources

Data Guardian should use both metadata-backed and heuristic signals.

Metadata-backed signals:

- `ColumnMetadata.IsSensitive`
- ontology or schema-based table classification once available
- evaluator mode and caller identity

Heuristic signals:

- sensitive column name patterns from the existing sensitive detector
- suspicious SQL structure
- broad column access patterns like `SELECT *`
- large or unbounded result shapes in sensitive contexts

## Phase 1 Rules

Implement a minimal but useful first set.

### Injection-Oriented Rules

- multi-statement detection
- suspicious comment usage
- raw SQL patterns that should never appear in safe read modes
- reuse existing parameter-injection findings where applicable

These rules should not pretend to solve all injection risks. The initial goal is to unify existing safeguards and expose findings through the evaluator.

### Sensitive Access Rules

- flag direct selection of sensitive columns
- flag `SELECT *` when sensitive columns are present
- flag modifying statements touching sensitive columns or tables in strict modes

### Data Leakage Heuristics

- warn or block on queries that can return broad raw datasets when sensitive columns are included
- use mode-specific policy, because an internal admin flow and an external request flow should not behave identically

## Mode-Specific Enforcement

Data Guardian should not hard-code one enforcement level for every caller.

Suggested initial posture:

- `glossary_example`
  - block sensitive-column example terms by default
- `pending_query_review`
  - surface findings and require explicit reviewer decision for risky queries
- `read_query`
  - warn on some findings, block clearly unsafe patterns
- `approved_query_execution`
  - block when a vetted query no longer satisfies policy
- `mutation_guard`
  - strictest mode, especially for modifying SQL

## Important Implementation Constraint

Security findings should be attached to `EvaluationResult`, not emitted through side channels only. Auditing can mirror those findings, but the evaluator result is the contract that all callers should consume.

## Audit Integration

This plan should leave the codebase ready to connect evaluation findings to existing audit streams. The first implementation can keep that integration light, but the result should already include enough information to:

- write audit records
- drive alerts
- explain blocked actions

## Risks

- If Data Guardian rules are embedded directly inside clients, the repo will repeat the same fragmentation problem.
- If findings are too coarse, clients will ignore them.
- If findings are too chatty, developers will immediately add bypasses.

The result model needs to be structured and mode-aware so callers can make defensible decisions.

## Implementation Tasks

- [ ] Add structured Data Guardian finding types to the evaluator result model.
- [ ] Integrate existing sensitive-column and sensitive-content detection into evaluator rule execution.
- [ ] Integrate existing parameter-injection detection where parameterized execution paths exist.
- [ ] Add basic raw-SQL risk rules for multi-statement and unsafe read-mode patterns.
- [ ] Add sensitive-access and `SELECT *` heuristics.
- [ ] Add mode-based allow/warn/block policy mapping.
- [ ] Add focused unit tests for each rule and policy outcome.

## Completion Criteria

- The evaluator can return structured security findings for both raw SQL and parameterized SQL flows.
- Callers can distinguish between security warnings and blocking security issues.
- The initial Data Guardian rule set reuses existing repo signals instead of duplicating them in new client code.
