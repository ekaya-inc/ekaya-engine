# DESIGN: Data Guardian — Data Protection & Privacy

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Applets focused on discovering, classifying, and protecting sensitive data. These ensure PII and other sensitive information is identified, tracked through pipelines, and properly masked or blocked.

---

## Applets

### 1. PII Radar

**Type:** Periodic (scheduled scans)
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #1)

Comprehensive sensitive data discovery and classification. Scans customer database columns using regex patterns, NLP heuristics, and embedding-based classifiers.

**Detection patterns (column name regex + content scanning):**
- Column names: `api_key`, `password`, `secret_key`, `access_token`, `private_key`, `credential`, `ssn`, `credit_card`
- Content patterns: JSON keys containing sensitive terms, JWT tokens, connection strings, PII formats (email, phone, SSN)

**Classification categories with default actions:**
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

**Data minimization monitoring:**
- Detects when AI queries fetch more columns/rows than necessary
- Supports GDPR Article 5(1)(c)

**Auto-generates:**
- GDPR Article 30 records of processing
- CCPA data inventories

**Origin:** Identified from a real security finding — `get_context` with `include: ["sample_values"]` exposed LiveKit API keys from a `users.agent_data` JSONB column.

---

### 2. PII Flow Tracking

**Type:** Periodic
**Migrated from:** `DESIGN-app-etl-genius.md` brainstorm (Security & Compliance for Pipelines)

Trace PII columns through the entire pipeline to ensure they're properly masked, hashed, or excluded at each stage.

**Key capabilities:**
- Map PII columns from source through transformation to destination
- Alert if raw PII flows to a destination that shouldn't have it
- Verify PII masking/hashing/exclusion at each pipeline stage
- Generate PII flow reports for compliance

---

### 3. Data Masking Advisor

**Type:** On-demand
**Migrated from:** `DESIGN-app-etl-genius.md` brainstorm (Security & Compliance for Pipelines)

Recommend and generate masking strategies for sensitive columns in non-production environments.

**Key capabilities:**
- Recommend masking strategies based on column type and classification
- Generate masking SQL: hash emails, randomize phone numbers, anonymize names
- Preserve referential integrity while masking (hashed FK columns match across tables)
- Support for multiple masking strategies per column type

---

### 4. Bias & Representativeness Monitor

**Type:** Periodic
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #7b)

For companies using AI in high-risk domains (medical, credit scoring, hiring, insurance), monitors whether data fed to AI models is demographically representative.

**Key capabilities:**
- Profiles data distributions across protected characteristics
- Detects skew and underrepresentation
- Alerts when training or inference data deviates from configured fairness thresholds
- Generates audit-ready representativeness reports

Required by EU AI Act Article 10: training, validation, and testing datasets must be "sufficiently representative" and free of bias for high-risk AI systems.

---

## Dashboard Tile

```
Title: "Data Protection"
Description: "PII detection, classification, masking, and privacy monitoring"
Stats: 47 sensitive columns | 3 pending review | 12 masked
```

---

## Related Plans

- `plans/guardian/DESIGN-guardian-access-control.md` — Column masking rules enforcement
- `plans/guardian/DESIGN-guardian-compliance.md` — GDPR/regulatory requirements
