# Data Guardian

**AI-powered database security and compliance — built on the Ekaya Engine.**

Data Guardian connects AI to your database to monitor, protect, and prove compliance automatically. It replaces the manual processes that cost engineering teams weeks per quarter: audit prep, incident detection, access reviews, and quality firefighting.

---

## How It Works

Data Guardian runs as a collection of applets — each focused on a specific security, compliance, or data quality concern. Together they form a unified product with its own UI, dashboards, and workflows.

### Two Deployment Modes

**Stand-alone:** Install Data Guardian as a purpose-built server. It comes configured out of the box — connect a database, and the applets start working. No Ekaya Engine knowledge required. The admin experience is tailored entirely to security and compliance workflows.

**Inside Ekaya Engine:** Admins of an existing Ekaya Engine instance install Data Guardian via the Applications tab, just like any other app. It appears alongside existing tools (AI Data Liaison, Ontology Forge, ETL Genius) and shares the same project infrastructure, datasource connections, and ontology intelligence.

Both modes are 100% compatible. The same applets, the same APIs, the same MCP tools.

### The UI

Data Guardian has its own route: `/guardian/{projectId}`

This is a dedicated UI parallel to the existing `/project/{projectId}` experience. It opens to a dashboard of tiles — one per applet category — each linking to its own screen with dedicated functionality. Think of it as a security operations center for your database.

The **Configuration** page is the existing `/projects/{projectId}` view where backend applets manage their settings, datasource connections, and AI configuration. Data Guardian doesn't reinvent project setup — it builds on top of it.

### MCP Integration

Data Guardian exposes its capabilities as MCP tools. An admin with MCP Server installed gets access to Data Guardian tools directly from Claude Code, Cursor, or any MCP-connected environment. Need to check audit logs, review alerts, or run a compliance report from your IDE? Just ask.

Want to expose that MCP connection externally? Install MCP Tunnel and it works — same tools, same permissions, accessible from anywhere.

---

## Applet Categories

Data Guardian ships with 6-12 applet tiles organized into eight categories. Each category addresses a distinct security or compliance concern.

### 1. Security & Alerting

Real-time threat detection and anomaly monitoring. The nerve center of Data Guardian.

- **Alert Dashboard** — Configurable alerts for SQL injection, unusual query volume, after-hours access, large data exports, sensitive table access, and more (11 alert types defined)
- **Anomaly Detection** — Flag unusual access patterns, volume spikes, and queries against never-before-accessed tables
- **Risk Scoring** — Real-time risk score per AI query based on data sensitivity, volume, identity, and human oversight
- **AI Query Monitor** — Monitors ALL database query activity (not just Ekaya-routed), identifies shadow AI and unmonitored access using `pg_stat_statements` / SQL Server Query Store
- **Data Minimization Alerts** — Flag AI queries that fetch more columns/rows than necessary (GDPR Article 5(1)(c))
- **Regulatory Incident Draft Generator** — Pre-draft incident reports for supervisory authorities when policy violations occur

### 2. Audit & Visibility

Complete audit trail of who accessed what data, when, and how.

- **Query Execution Log** — Every query logged with who/what/when/params/tables/rows/duration
- **User Activity Feed** — Timeline of all actions grouped by session
- **Data Access Tracking** — Track specific tables and columns accessed by each query
- **AI System Registry** — Fingerprint and catalog every AI system querying the database
- **Query Audit Vault** — Compliance-ready access logging mapped to SOC 2, HIPAA, and GDPR
- **Pipeline Access Audit** — Track service account access across ETL pipelines

### 3. Access Control

Who can see what, enforced automatically.

- **RBAC Query Permissions** — Role-based execution enforcement per query
- **Column Masking** — Redact, partial-show, hash, or null sensitive columns by role (e.g., SSN visible to admins only)
- **Consent Boundary Enforcement** — Cross-reference AI data access against user consent records; flag violations in real time
- **Access Review Automator** — Periodic permission certification campaigns with audit evidence
- **Sensitive Data Detection & Classification** — Scan and classify all columns (PII, financial, secrets) with admin approval workflow

### 4. Compliance & Governance

Meet regulatory requirements automatically.

- **Regulatory Report Templates** — One-click reports for GDPR Article 30, EU AI Act, SOC 2, HIPAA, ISO 27001, NIST AI RMF
- **AI Compliance Manager** — AI reads your own audit data and generates compliance narratives and evidence packages
- **DPIA Management** — Auto-trigger Data Protection Impact Assessments when AI access patterns change
- **Human Override Tracker** — Verify humans are actually reviewing AI-driven decisions (EU AI Act Article 14)
- **Query Approval Workflow** — Suggested queries require admin approval with full change history
- **Data Retention Enforcer** — Policy-based lifecycle management with audit trail
- **DSAR Fulfillment Agent** — Automated Data Subject Access Requests with verified deletion and erasure proof
- **Compliance Evidence Collector** — Aggregate platform evidence mapped to specific compliance controls

### 5. Data Quality & Drift

Catch problems before downstream consumers notice.

- **AI Drift Monitor** — Scheduled schema snapshots with AI-generated impact analysis ("This column drop will break 3 queries")
- **AI Data Quality Monitor** — AI auto-generates quality expectations, runs scheduled checks, explains anomalies
- **Data Quality Gate Advisor** — Row count, null rate, uniqueness, referential integrity, and distribution drift validation
- **Data Freshness Advisor** — Pipeline lag monitoring, SLA compliance, stuck pipeline detection

### 6. Data Protection & Privacy

Discover, classify, track, and protect sensitive data.

- **PII Radar** — Comprehensive sensitive data scanning (regex + NLP + LLM), auto-classification, admin approval workflow
- **PII Flow Tracking** — Trace PII columns through pipeline stages, alert on non-compliant destinations
- **Data Masking Advisor** — Recommend and generate masking SQL for non-production environments
- **Bias & Representativeness Monitor** — Monitor AI training/inference data for demographic fairness (EU AI Act Article 10)

### 7. SQL Security

Every query evaluated for safety before it touches the database.

- **SQL Security Rules Engine** — Structured findings for injection risk, sensitive access, data leakage, unsafe modifications
- **Intent-SQL Mismatch Detection** — Verify SQL computes what the stated intent describes
- **Hidden Parameter Detection** — Find implicit filters and unjustified assumptions in queries
- **Validation Modes** — Six enforcement levels from permissive to strict, tailored per use case

### 8. Integrations

Connect Data Guardian to your existing security and monitoring stack.

- **SIEM Export** — Stream to Splunk, Datadog, Sumo Logic
- **Grafana/Prometheus Metrics** — Export Guardian metrics for dashboards
- **Slack/Teams Notifications** — Route alerts to channels by severity
- **Webhooks** — Custom integration endpoints with delivery tracking
- **API Key Management** — Named keys with rotation, expiration, scoping, and usage tracking
- **Rate Limiting** — Per-user, per-query, and per-role configurable limits

---

## What Makes Data Guardian Different

**AI does the work.** Competitors (Monte Carlo, Vanta, Collibra) require humans to write tests, collect evidence, and investigate anomalies. Data Guardian's applets use AI to auto-generate quality expectations, explain anomalies in plain English, map audit evidence to compliance frameworks, and draft incident reports. The AI uses the project's existing LLM configuration (BYOK, community, embedded, or on-prem).

**Data never leaves.** Data Guardian runs on-premise or in the customer's cloud, connected directly to their database. No data is shipped to a third-party SaaS. This is critical for contracts, financial data, and healthcare — and it's a requirement for EU AI Act compliance.

**One platform, not eight tools.** Security monitoring, audit logging, access control, compliance reporting, data quality, and privacy protection — all in one product, sharing the same ontology intelligence and datasource connections.

---

## Detailed Design Files

Each category has a detailed DESIGN file with applet specifications, data models, and API designs:

| Category | Design File |
|----------|-------------|
| Security & Alerting | `DESIGN-guardian-security-alerting.md` |
| Audit & Visibility | `DESIGN-guardian-audit-visibility.md` |
| Access Control | `DESIGN-guardian-access-control.md` |
| Compliance & Governance | `DESIGN-guardian-compliance.md` |
| Data Quality & Drift | `DESIGN-guardian-data-quality.md` |
| Data Protection & Privacy | `DESIGN-guardian-data-protection.md` |
| SQL Security | `DESIGN-guardian-sql-evaluation.md` |
| Integrations | `DESIGN-guardian-integrations.md` |
