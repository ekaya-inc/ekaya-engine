# DESIGN: Data Guardian — SQL Evaluation & Validation

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Security-oriented SQL evaluation applets that analyze, validate, and enforce policies on every SQL query before it touches the database. Built on top of the shared `SQLEvaluator` service (engine infrastructure) with a security rules layer that produces structured findings.

**Note:** The SQL evaluator core is engine infrastructure that serves all Ekaya products (glossary, query management, etc.). The security rules layer described here is the Data Guardian-specific contribution. The core evaluator plans remain in `plans/` as they serve the broader engine.

---

## Applets

### 1. SQL Security Rules Engine

**Type:** Realtime (on every query)
**References:** `plans/PLAN-sql-evaluator-security-rules.md`

Structured security findings attached to every SQL evaluation. The evaluator answers:
- Is this SQL safe enough to run in this mode?
- What sensitive data does it touch?
- What risk signals were detected?
- Should the caller allow, warn, or block?

**Security finding categories:**
- `sql_injection_risk` — Multi-statement detection, suspicious comments, raw SQL patterns
- `sensitive_column_access` — Direct selection of sensitive columns
- `sensitive_content_risk` — Content-based sensitive data detection
- `data_leakage_risk` — Broad raw datasets with sensitive columns
- `unsafe_modification_risk` — Modifying statements in restricted modes

**Finding severity/actions:**
- `allow` — No issues
- `warn` — Flag but permit
- `block` — Prevent execution
- `require_review` — Queue for human approval

---

### 2. Intent-SQL Mismatch Detector

**Type:** Realtime
**References:** `plans/MASTER-PLAN-sql-evaluator.md`

Verify that the SQL actually computes what the stated intent describes. Catches cases where the SQL shape diverges from the requested metric.

---

### 3. Hidden Parameter Detection

**Type:** Realtime
**References:** `plans/MASTER-PLAN-sql-evaluator.md`

Find implicit filters and assumptions in queries — arbitrary time windows, geographic bounds, or business logic that isn't justified by project knowledge.

---

### 4. Degenerate Result Detection

**Type:** Realtime (post-execution)
**References:** `plans/PLAN-sql-evaluator-core.md`

Catch queries that return meaningless results:
- Empty result rejection
- All-null result rejection
- Degenerate zero-value rejection (zero on a populated path)
- Multi-row rejection when single row expected

---

### 5. Query Validation Modes

**Type:** Policy framework
**References:** `plans/PLAN-sql-evaluator-core.md`

Six enforcement levels from permissive to strict:

| Mode | Use Case | Strictness |
|------|----------|------------|
| `glossary_example` | Strict semantic validation | Highest |
| `pending_query_review` | Correctness + security | High |
| `approved_query_review` | Policy enforcement | High |
| `read_query` | Warn on risky patterns | Medium |
| `approved_query_execution` | Reuse prior vetting | Medium |
| `mutation_guard` | Strictest for modifying SQL | Highest |

---

### 6. Vetted Query History

**Type:** Monitoring / Lookup
**References:** `plans/PLAN-sql-evaluator-history-vetting.md`

Persistent evaluation outcomes that enable:
- SQL fingerprinting and reuse keys
- Schema/ontology fingerprints for compatibility checking
- Safe reuse policy (never blindly reuse modifying SQL)
- Compact execution summaries (no raw sensitive data storage)

**Reuse key:** project_id + datasource_id + mode + sql_hash + schema_fingerprint + ontology_fingerprint + policy_version

---

## Mode-Specific Security Enforcement

| Mode | Security Posture |
|------|-----------------|
| `glossary_example` | Block sensitive-column example terms by default |
| `pending_query_review` | Surface findings, require reviewer decision for risky queries |
| `read_query` | Warn on some findings, block clearly unsafe patterns |
| `approved_query_execution` | Block when vetted query no longer satisfies policy |
| `mutation_guard` | Strictest mode for modifying SQL |

---

## Existing Code

The repo already has security-related building blocks:
- `pkg/models/column_metadata.go` — `IsSensitive` for column-level overrides
- `pkg/mcp/tools/sensitive.go` — Pattern-based sensitive column/content detection
- `pkg/services/query.go` — Parameter-level injection detection
- MCP audit and alerting code — Security classification concepts

These are not yet unified behind a single SQL evaluation result. The security rules layer plugs them into the evaluator contract.

---

## Related Plans (Engine Infrastructure)

These plans define the SQL evaluator infrastructure that Guardian's security layer builds on:

- `plans/MASTER-PLAN-sql-evaluator.md` — Master plan for the shared evaluator
- `plans/PLAN-sql-evaluator-core.md` — Core evaluator types, rules, execution checks
- `plans/PLAN-sql-evaluator-security-rules.md` — Security rules layer (most Guardian-relevant)
- `plans/PLAN-sql-evaluator-history-vetting.md` — Evaluation persistence and reuse
- `plans/PLAN-sql-evaluator-client-rollout.md` — Client migration plan
