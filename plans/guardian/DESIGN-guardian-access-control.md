# DESIGN: Data Guardian — Access Control

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Role-based access control, data masking, consent enforcement, and permission management applets. These control who can see and do what with the data.

---

## Applets

### 1. RBAC Query Permissions

**Type:** Policy enforcement
**Migrated from:** `PLAN-app-enterprise.md` Module 2

Role-based query execution enforcement. Control which users/roles can view and execute which queries.

**Enforcement points:**
1. Query list endpoint filters by user's permissions
2. Query execute endpoint checks `can_execute`
3. Result formatter applies column masks based on user role
4. MCP tools respect same permissions

---

### 2. Column Masking Rules

**Type:** Policy enforcement
**Migrated from:** `PLAN-app-enterprise.md` Module 2

Hide or obfuscate sensitive columns based on the requesting user's role.

**Masking strategies:**
- `redact` — Replace with `[REDACTED]`
- `partial` — Show last 4 chars (e.g., SSN: `***-**-1234`)
- `hash` — SHA256 hash for referential integrity
- `null` — Replace with NULL

---

### 3. Consent Boundary Enforcement

**Type:** Realtime
**Migrated from:** `PLAN-app-enterprise.md` Module 2 + `BRAINSTORM-ekaya-engine-applications.md` (App #7a)

Cross-reference AI data access against user consent records. If a user consented to "product recommendations" but an AI system reads their support tickets, billing history, or behavioral data beyond the consented scope, flag the boundary violation.

**Key capabilities:**
- Stores consent-to-access mappings and violation history
- Generates evidence that AI data access respects purpose limitation (GDPR Article 5(1)(b))
- Most companies have consent management for web cookies but zero governance over what AI systems access once data is in the database — this closes that gap

---

### 4. Access Review Automator

**Type:** Periodic (scheduled campaigns)
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #3)

Automated periodic permission certification. Pulls current database roles, table-level grants, and RLS policies from the customer's database. Generates review campaigns that route to managers.

**Key capabilities:**
- Tracks approvals, flags stale permissions
- Produces audit evidence packages
- Unknown AI system detection — fingerprints all applications/services querying the database, alerts when unrecognized system appears

Quarterly access reviews via "spreadsheets emailed to managers" are the most commonly cited compliance pain point in mid-market companies.

---

### 5. Sensitive Data Detection & Classification

**Type:** Periodic (scheduled scans)
**Migrated from:** `MASTER-PLAN-sql-evaluator.md` + `BRAINSTORM-ekaya-engine-applications.md` (App #1: PII Radar)

Comprehensive scanning and classification of all data columns.

**Detection approaches:**
- Column name regex patterns (`api_key`, `password`, `ssn`, `credit_card`, etc.)
- Content scanning (JSON keys with sensitive terms, JWT tokens, connection strings, PII formats)
- LLM-assisted classification using column context and sample values
- `ColumnMetadata.IsSensitive` flags from ontology

**Classification hierarchy:**
- `secrets` (API keys, tokens, passwords) — Block by default
- `pii_identity` (SSN, passport) — Block by default
- `pii_contact` (email, phone, address) — Flag for review
- `pii_financial` (credit card, bank account) — Block by default

**Admin approval workflow:**
- Persist allow/block/pending decisions per column
- Decisions survive re-extraction
- Schema UI shows flagged columns with Allow/Block buttons
- Dashboard badge shows count of columns pending review
- MCP tools honor decisions: blocked columns show `[BLOCKED: Admin decision]`

---

## Data Models

### Query Permissions

```sql
CREATE TABLE engine_query_permissions (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    query_id UUID NOT NULL REFERENCES engine_queries(id),
    permission_type VARCHAR(20) NOT NULL,  -- 'role', 'user', 'all'
    role VARCHAR(50),
    user_id VARCHAR(255),
    can_view BOOLEAN DEFAULT true,
    can_execute BOOLEAN DEFAULT true,
    can_edit BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Column Masking Rules

```sql
CREATE TABLE engine_column_masks (
    id UUID PRIMARY KEY,
    project_id UUID NOT NULL,
    datasource_id UUID NOT NULL,
    table_name VARCHAR(255) NOT NULL,
    column_name VARCHAR(255) NOT NULL,
    classification VARCHAR(50) NOT NULL,  -- 'pii', 'financial', 'sensitive'
    mask_type VARCHAR(50) NOT NULL,       -- 'redact', 'partial', 'hash', 'null'
    mask_config JSONB,                    -- {"show_last": 4} for partial SSN
    visible_to_roles TEXT[] DEFAULT '{admin}',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

---

## Related Plans

- `plans/guardian/PLAN-app-enterprise.md` — Enterprise suite Module 2 (Access Control)
