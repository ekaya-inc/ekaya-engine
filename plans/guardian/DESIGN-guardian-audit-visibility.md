# DESIGN: Data Guardian — Audit & Visibility

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Complete audit logging and visibility into who is accessing what data, when, and how. These applets produce the evidence trail that compliance teams, auditors, and security reviewers need.

---

## Applets

### 1. Query Execution Log

**Type:** Monitoring (continuous)
**Migrated from:** `PLAN-app-enterprise.md` Module 1

Complete log of every query executed: who ran it, what SQL, when, with what parameters, what tables/columns were accessed, how many rows returned, and how long it took.

**Key features:**
- Full audit log page with filters (user, date range, event type, security level)
- Event detail modal showing full context
- Export to CSV/JSON for compliance
- Configurable retention policy

---

### 2. User Activity Feed

**Type:** Monitoring
**Migrated from:** `PLAN-app-enterprise.md` Module 1

Timeline of all user actions in the project — query execution, configuration changes, ontology edits, app installations. Groups activity by session/conversation.

---

### 3. Data Access Tracking

**Type:** Monitoring
**Migrated from:** `PLAN-app-enterprise.md` Module 1

Track which specific tables and columns were accessed by each query. Enables answering: "Who accessed the `users.email` column in the last 30 days?"

---

### 4. AI System Registry

**Type:** Periodic
**Migrated from:** `PLAN-app-enterprise.md` Module 1

Fingerprint and catalog every AI system/service querying the database. Maintains a registry of known systems and alerts on unrecognized ones.

---

### 5. Query Audit Vault

**Type:** Monitoring (continuous)
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #2)

Compliance-ready data access logging that captures every query touching sensitive tables through Ekaya's data access layer. Enriches logs with:
- User identity and role
- Sensitivity classification of accessed data
- Risk score per query
- Session context (which AI agent, which conversation)

Generates audit-ready reports mapped to:
- SOC 2 (CC6.1-CC6.3)
- HIPAA (§164.312)
- GDPR requirements

Uses pgAudit patterns and TimescaleDB-style partitioning for efficient long-term retention.

---

### 6. Pipeline Access Audit

**Type:** Periodic
**Migrated from:** `DESIGN-app-etl-genius.md` brainstorm

Track which service accounts and users access pipeline source and destination data. Ensure least-privilege access across the ETL layer.

---

## Data Model

### Audit Events Table

```sql
CREATE TABLE engine_audit_events (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,

    -- Who
    user_id VARCHAR(255) NOT NULL,
    user_email VARCHAR(255),
    user_role VARCHAR(50),
    session_id VARCHAR(255),

    -- What
    event_type VARCHAR(50) NOT NULL,  -- 'query_executed', 'query_created', 'config_changed', etc.
    resource_type VARCHAR(50),        -- 'query', 'datasource', 'ontology', 'mcp_config'
    resource_id UUID,

    -- Details
    action VARCHAR(50),               -- 'execute', 'create', 'update', 'delete', 'view'
    request_data JSONB,               -- Sanitized input (params, etc.)
    response_summary JSONB,           -- {row_count, tables_accessed, duration_ms}

    -- Context
    natural_language TEXT,             -- If query came from NL prompt
    sql_executed TEXT,                 -- The actual SQL (truncated if huge)
    tables_accessed TEXT[],            -- ['users', 'orders']

    -- Outcome
    was_successful BOOLEAN DEFAULT true,
    error_message TEXT,
    duration_ms INTEGER,
    rows_returned INTEGER,

    -- Classification
    security_level VARCHAR(20) DEFAULT 'normal',  -- 'normal', 'warning', 'alert'
    data_classifications TEXT[],      -- ['pii', 'financial', 'sensitive']

    created_at TIMESTAMPTZ DEFAULT NOW(),
    partition_date DATE DEFAULT CURRENT_DATE
) PARTITION BY RANGE (partition_date);
```

---

## UI Design

### Audit Log Page

```
/guardian/{pid}/audit

Filters: [Last 7 days] [All Users] [All Events] [Search...]  [Export CSV]

Today
  10:45 AM  john@acme.com  executed  "Monthly Sales Report"    145ms
            -> 847 rows from [orders, customers]
  10:42 AM  john@acme.com  executed  "Customer Lookup"          23ms
            -> 1 row from [customers]  params: {customer_id: "abc123"}
  10:15 AM  jane@acme.com  blocked   "Direct SQL"               (!)
            -> Attempted access to restricted table [salaries]
```

### Dashboard Tile

```
Title: "Audit & Visibility"
Description: "Complete query and data access audit trail"
Stats: 1,234 events (7d) | 12 users | 3 blocked
```

---

## Related Plans

- `plans/guardian/PLAN-app-enterprise.md` — Enterprise suite Module 1 (Audit & Visibility)
