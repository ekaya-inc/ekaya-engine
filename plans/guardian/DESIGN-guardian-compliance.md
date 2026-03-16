# DESIGN: Data Guardian — Compliance & Governance

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Regulatory compliance automation, governance workflows, and evidence collection applets. These help enterprises meet regulatory requirements (GDPR, EU AI Act, SOC 2, HIPAA) and enforce change management processes.

---

## Applets

### 1. Regulatory Report Templates

**Type:** On-demand / Periodic
**Migrated from:** `PLAN-app-enterprise.md` Module 1

Pre-built report templates mapped to specific regulatory frameworks:
- **GDPR Article 30** — Records of processing activities
- **EU AI Act** — AI system evidence and risk documentation
- **SOC 2** — Formatted compliance exports (CC6.1-CC6.3)
- **HIPAA** — §164.312 technical safeguard evidence
- **ISO 27001** — Annex A control evidence
- **NIST AI RMF** — AI risk management framework mapping
- **ISO 42001** — AI management system evidence

One-click generation of auditor-ready reports.

---

### 2. AI Compliance Manager

**Type:** On-demand + Periodic
**Migrated from:** `DESIGN-wasm-application-platform.md` (AI Data Guardian Suite App #3)

AI-automated compliance evidence assembly and narrative generation.

**Key capabilities:**
- Reads engine's own audit infrastructure (MCP audit events, general audit log, column sensitivity, role assignments)
- AI maps evidence to compliance frameworks: "MCP audit events showing query_blocked events with security_level=critical map to SOC 2 CC6.1 (Logical and Physical Access Controls)"
- AI generates compliance narratives: "During the reporting period, 47 unauthorized table access attempts were detected and blocked. All sensitive columns are classified and access is logged."
- Generates audit-ready evidence packages on demand
- Stores compliance reports and evidence snapshots

**Existing code leveraged:**
- `pkg/models/mcp_audit.go` — 10 event types with security classification
- `pkg/services/audit_service.go` — Entity CRUD tracking with provenance
- `pkg/models/column_metadata.go` — `IsSensitive` flag
- `pkg/auth/claims.go` — Role and access information
- `pkg/services/retention_service.go` — Data lifecycle compliance

---

### 3. DPIA Trigger & Management

**Type:** Realtime trigger + Workflow
**Migrated from:** `PLAN-app-enterprise.md` Module 3

Auto-detect when AI access patterns change enough to require a Data Protection Impact Assessment. Required by GDPR Article 35 and EU AI Act Article 9 for high-risk processing changes.

**Key capabilities:**
- Triggers when: new data types accessed, new user segments, significant volume increase
- Pre-populates DPIA with observed data access patterns
- Tracks human review status

---

### 4. Human Override Tracker

**Type:** Monitoring
**Migrated from:** `PLAN-app-enterprise.md` Module 3

Verify that humans are actually in the loop for AI-driven decisions. EU AI Act Article 14 requires human oversight capability for high-risk AI systems — this verifies it's actually happening in practice.

**Key capabilities:**
- Detects AI-driven decisions that had no human review
- Logs when AI decisions are reviewed by humans vs. auto-approved
- Generates evidence reports for regulators

---

### 5. Query Approval Workflow

**Type:** Workflow
**Migrated from:** `PLAN-app-enterprise.md` Module 3

Suggested queries require admin approval before becoming available. Full change management with audit trail.

**Key capabilities:**
- Suggestion → review → approve/reject workflow
- Change audit trail (who modified what, when)
- Version history with rollback capability
- Status tracking: pending, approved, rejected

---

### 6. Data Retention Enforcer

**Type:** Periodic (scheduled)
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #4)

Policy-based data lifecycle management. Define retention rules per schema, table, or classification tier. Monitor compliance continuously.

**Key capabilities:**
- When data ages past retention window: auto-archive to cold partitions, queue for deletion, or flag for human review
- Complete audit trail of all retention actions
- Makes policies executable — what auditors and "compliance as code" demand

---

### 7. DSAR Fulfillment Agent

**Type:** On-demand
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #5)

Automated Data Subject Access Requests. Given a subject identifier (email, customer ID):
- Scans all connected tables for matching records
- Assembles a complete data package
- Optionally executes verified deletion with cryptographic proof

**Extended capabilities:**
- Consent withdrawal propagation verification (Right to be Forgotten)
- Post-deletion checks to confirm no AI pipeline still references the deleted user
- Provides proof of complete erasure across all AI data access paths

---

### 8. Compliance Evidence Collector

**Type:** On-demand + Periodic
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #7)

Aggregates evidence from the platform itself and maps each artifact to specific compliance controls:
- Encryption status, backup configurations
- Access logs, role assignments
- Query audit summaries, data classification reports
- Retention policy compliance

**Extended compliance workflows:**
- AI transparency disclosure generation (EU AI Act Article 4)
- Post-market monitoring reports (EU AI Act Article 72)
- Framework mapping beyond SOC 2 to EU AI Act, GDPR, NIST AI RMF, ISO 42001

---

## Data Models

### Query Versions

```sql
CREATE TABLE engine_query_versions (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    query_id UUID NOT NULL REFERENCES engine_queries(id),
    version_number INTEGER NOT NULL,
    natural_language_prompt TEXT,
    sql_query TEXT,
    parameters JSONB,
    created_by VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    change_reason TEXT
);
```

### Query Suggestions

```sql
CREATE TABLE engine_query_suggestions (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    datasource_id UUID NOT NULL,
    suggested_by VARCHAR(255) NOT NULL,
    suggested_at TIMESTAMPTZ DEFAULT NOW(),
    natural_language_prompt TEXT NOT NULL,
    suggested_sql TEXT,
    status VARCHAR(20) DEFAULT 'pending',  -- 'pending', 'approved', 'rejected'
    reviewed_by VARCHAR(255),
    reviewed_at TIMESTAMPTZ,
    review_notes TEXT,
    approved_query_id UUID REFERENCES engine_queries(id)
);
```

---

## Related Plans

- `plans/guardian/PLAN-app-enterprise.md` — Enterprise suite Module 3 (Governance & Workflow)
