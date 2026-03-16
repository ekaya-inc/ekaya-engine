# DESIGN: Data Guardian — Security & Alerting

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Real-time and near-real-time security monitoring applets that detect threats, anomalies, and policy violations as they occur. These form the "nerve center" of Data Guardian — the first thing a security team sees.

---

## Applets

### 1. Alert Dashboard

**Type:** Realtime monitoring
**Migrated from:** `PLAN-alerts-tile-and-screen.md` (moved to `plans/guardian/`)

Real-time alert system that triggers on security events detected during database access.

**Alert Types (11 defined, 7 with triggers implemented):**

| Alert Type | Description | Trigger Status |
|------------|-------------|----------------|
| `sql_injection_detected` | Triggers on security_level=critical or injection flag | Implemented |
| `unusual_query_volume` | User exceeds threshold_multiplier * 10 queries/hour | Implemented |
| `sensitive_table_access` | Query touches sensitive tables | NOT implemented |
| `large_data_export` | Query returns > row_threshold rows | Implemented |
| `after_hours_access` | Access outside business hours | Implemented |
| `new_user_high_volume` | New user runs > query_threshold queries in first 24h | Implemented |
| `repeated_errors` | Same error repeats > error_count times in window_minutes | Implemented |
| `unknown_ai_system` | Unrecognized application/service starts querying | NOT implemented |
| `consent_boundary_violation` | AI query accesses data outside consented scope | NOT implemented |
| `data_minimization_violation` | AI query fetches more columns/rows than necessary | NOT implemented |
| `dpia_threshold_exceeded` | AI access patterns warrant a DPIA | NOT implemented |

**Backend status:** Nearly complete. Models, handlers (4 endpoints), AlertService, AlertTriggerService (6/7 triggers), repositories, migrations, DI wiring, and ~1,400 lines of tests all exist.

**Frontend status:** Alert list + resolution UI exists but is embedded inside AuditPage (tab 6 of 6). Alert config editor exists but is commented out in SettingsPage. No standalone route or dashboard tile.

**Detailed implementation plan:** See `plans/guardian/PLAN-alerts-tile-and-screen.md`

---

### 2. SQL Injection Detection

**Type:** Realtime
**Migrated from:** `PLAN-app-enterprise.md` Module 4

Detect injection attempts in query parameters. Reuses existing parameter-level injection checks from `pkg/services/query.go` and integrates with the SQL evaluator security rules layer.

**Existing code:**
- `pkg/services/query.go` — parameter-level injection detection
- `pkg/mcp/tools/sensitive.go` — pattern-based detection

---

### 3. Anomaly Detection

**Type:** Realtime
**Migrated from:** `PLAN-app-enterprise.md` Module 4

Flag unusual access patterns — sudden volume spikes, access from new locations, queries against tables a user has never accessed before.

---

### 4. Risk Scoring

**Type:** Realtime
**Migrated from:** `PLAN-app-enterprise.md` Module 4

Real-time risk score per AI query based on:
- Data sensitivity classification of accessed columns
- Query volume and data volume
- User/system identity and trust level
- Presence or absence of human oversight

Risk scores aggregate at the system level for compliance dashboards and trend analysis.

---

### 5. AI Query Monitor

**Type:** Periodic (scheduled)
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #4)

Monitors ALL database query activity (not just Ekaya-routed queries) by enabling database-native query tracking:
- **PostgreSQL:** `pg_stat_statements` extension
- **SQL Server:** Query Store (`sys.query_store_runtime_stats`)

**Key capabilities:**
- Correlates database-level queries against Ekaya's MCP audit log — queries through Ekaya are tagged; everything else is flagged as "unmonitored access"
- AI classifies unmonitored queries: "This query pattern (`SELECT email, phone FROM users WHERE ...`) appears 340 times/day from a connection not routed through Ekaya. It accesses PII columns and likely originates from an AI tool or script."
- AI identifies shadow AI fingerprints — patterns characteristic of AI-generated SQL vs. human-written or ORM-generated queries
- Alerts when new unmonitored access patterns appear or when sensitive table access is detected outside Ekaya

**Host functions used:** `datasource_query`, `db_query`, `llm_generate`, `get_auth_context`, `log`

---

### 6. Data Minimization Alerts

**Type:** Realtime
**Migrated from:** `PLAN-app-enterprise.md` Module 1

Flag AI queries that fetch more columns/rows than necessary for their stated purpose. If a recommendation engine runs `SELECT *` but only needs `product_id` and `category`, the alert fires.

Supports GDPR Article 5(1)(c) data minimization requirements.

---

### 7. Regulatory Incident Draft Generator

**Type:** On-demand
**Migrated from:** `PLAN-app-enterprise.md` Module 4

When AI access patterns trigger policy violations, pre-draft incident reports formatted for submission to supervisory authorities. EU AI Act Article 62 requires providers of high-risk AI to report serious incidents.

---

## Data Models

### Security Alerts Table

```sql
CREATE TABLE engine_security_alerts (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    alert_type VARCHAR(50) NOT NULL,     -- 'injection', 'anomaly', 'rate_limit', 'unauthorized'
    severity VARCHAR(20) NOT NULL,       -- 'info', 'warning', 'critical'
    title VARCHAR(255) NOT NULL,
    description TEXT,
    affected_user_id VARCHAR(255),
    related_event_ids UUID[],            -- Links to audit_events
    evidence JSONB,                      -- {pattern_matched, threshold_exceeded, etc.}
    status VARCHAR(20) DEFAULT 'open',   -- 'open', 'acknowledged', 'resolved', 'dismissed'
    resolved_by VARCHAR(255),
    resolved_at TIMESTAMPTZ,
    resolution_notes TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Alert Config

Alert config is stored as JSONB with per-type enable/disable, severity, and threshold settings. Currently in `engine_mcp_config.alert_config` column — needs decoupling per the alerts plan.

---

## Dashboard Tile

```
Title: "Security & Alerting"
Description: "Real-time threat detection, anomaly alerts, and security monitoring"
Icon: Shield
Color: red/amber
Badge: Open alert count by severity
```

---

## Related Plans

- `plans/guardian/PLAN-alerts-tile-and-screen.md` — Detailed implementation plan for alert tile and page
- `plans/guardian/PLAN-app-enterprise.md` — Enterprise suite Module 4 (Security & Threat Detection)
- `plans/PLAN-sql-evaluator-security-rules.md` — SQL evaluator security rules layer
