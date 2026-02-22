# Ekaya Engine Application Brainstorm

## First Three Apps: The AI Data Guardian Suite

**Status:** Selected for first implementation after WASM platform foundation is built.
**Design docs:** `DESIGN-wasm-application-platform.md` (runtime), `DESIGN-engine-app-marketplace.md` (engine integration)
**Distribution:** Built in this repository (`wt-ekaya-engine-applications`), pre-compiled to WASM, downloaded to customer engine instances via the marketplace funnel (`ekaya-central/plans/DESIGN-app-marketplace-funnel.md`).

All three apps are **AI-automated** — they require LLM access (via `llm_generate` host function) and use it to eliminate manual work that competitors require. LLM access uses the project's existing AI configuration (BYOK, community, embedded, or on-prem fine-tuned model). This is what differentiates them from tools like Monte Carlo, Vanta, or Collibra: the AI does the work, not the human.

### 1. AI Drift Monitor

**App ID:** `ai-drift-monitor`
**Category:** Data reliability
**Problem:** Schema drift causes pipeline failures. Detection takes 4+ hours on average. Teams discover problems after stakeholders complain.

**What it does:**
- Runs on a configurable schedule (default: every 6 hours)
- Takes schema snapshots via the `SchemaDiscoverer` interface (supports PostgreSQL + MSSQL)
- Compares against previous snapshot using `SchemaChangeDetectionService` patterns (5 change types: new table, dropped table, new column, dropped column, modified column)
- **AI generates natural-language impact analysis:** "The `orders.discount_code` column was dropped. This will break 3 approved queries that reference it. The column had 45,000 non-null values."
- **AI classifies severity:** breaking change vs. additive change vs. cosmetic
- Stores historical timeline of all schema changes in app-isolated Postgres schema (`app_ai_drift_monitor`)
- Alerts via webhook when breaking changes detected

**Existing engine code it leverages:**
- `pkg/adapters/datasource/postgres/schema.go` — `DiscoverTables()`, `DiscoverColumns()`, `DiscoverForeignKeys()`
- `pkg/adapters/datasource/mssql/schema.go` — Same interfaces for SQL Server
- `pkg/services/schema_change_detection.go` — Change type detection, suggested actions, `PendingChange` model
- `pkg/models/column_metadata.go` — Column classification, sensitivity flags (provides context for impact analysis)

**Host functions used:** `datasource_query`, `db_query`, `llm_generate`, `get_auth_context`, `log`

**Why it's the first app:** Lowest complexity. Schema introspection queries and change detection logic already exist in the engine. The WASM app orchestrates them on a schedule and adds AI-generated analysis. Clear I/O pattern (read schema → compare → generate report). Proves the WASM runtime works end-to-end.

---

### 2. AI Data Guardian

**App ID:** `ai-data-guardian`
**Category:** Data quality
**Problem:** 57% of data professionals cite poor data quality as their #1 challenge. Companies maintain ~290 manually-written tests. Enterprise tools (Monte Carlo) cost six figures.

**What it does:**
- Profiles data using aggregate queries on customer datasources (null rates, cardinality, distributions, freshness timestamps)
- **AI auto-generates quality expectations** from profiling results: "This column has 0.3% nulls historically. Alert if null rate exceeds 2%." — eliminates the cold-start problem where teams must manually define every check
- Runs scheduled checks against expectations using statistical methods (z-score, IQR, threshold comparisons)
- **AI explains anomalies** when checks fail: "The `users.email` column null rate jumped from 0.3% to 15.2% in the last 24 hours. This correlates with a new column `users.sso_id` appearing yesterday — likely a migration that made email optional for SSO users."
- **AI suggests SQL fixes** with human approval before execution (borrowed from App #24 concept): "UPDATE users SET status = 'active' WHERE status IS NULL AND created_at > '2026-02-01'" — detection without remediation is table stakes; detection with AI-suggested fixes is the moat
- Stores expectations and check results in app-isolated Postgres schema (`app_ai_data_guardian`)

**Existing engine code it leverages:**
- `pkg/adapters/datasource/interfaces.go` — `QueryExecutor` for aggregate queries against customer databases
- `pkg/services/column_feature_extraction.go` — Feature extraction patterns (null rates, cardinality, classification confidence, `TimestampFeatures`, `BooleanFeatures`, `EnumFeatures`, `IdentifierFeatures`, `MonetaryFeatures`)
- `pkg/adapters/datasource/postgres/schema.go` — `AnalyzeColumnStats()` (cardinality, distinctness, min/max), `GetDistinctValues()`, `GetEnumValueDistribution()`

**Host functions used:** `datasource_query`, `db_query`, `llm_generate`, `get_auth_context`, `log`

**Why it's the second app:** Builds on Drift Monitor's scheduling + LLM patterns. Adds statistical computation (simple math) and the expectation auto-generation capability. Higher value density — the AI-generated expectations eliminate the biggest barrier to adoption (manual rule writing).

---

### 3. AI Compliance Manager

**App ID:** `ai-compliance-manager`
**Category:** Compliance & governance
**Problem:** SOC 2 audits cost $50K-$100K+. Companies scramble for weeks collecting screenshots and documentation. Vanta and Drata cover infrastructure but miss the data layer entirely.

**What it does:**
- Reads the engine's own audit infrastructure — no new data collection needed:
  - MCP audit events (`engine_mcp_audit_log`): 10 event types including `query_executed`, `sql_injection_attempt`, `unauthorized_table_access`, `sensitive_data_access`, with security classification (normal/warning/critical)
  - General audit log (`engine_audit_log`): entity CRUD tracking with provenance (source: inferred/MCP/manual + actor)
  - Column sensitivity metadata: `IsSensitive` flag, classification path, semantic type
  - Auth system: JWT claims with project_id, roles, user_id — provides access control evidence
  - Retention service: scheduled data pruning with configurable windows — provides data lifecycle compliance
- **AI maps evidence to compliance frameworks:** "MCP audit events showing `query_blocked` with `security_level=critical` map to SOC 2 CC6.1 (Logical and Physical Access Controls)"
- **AI generates compliance narratives:** "During the reporting period, 47 unauthorized table access attempts were detected and blocked. All 23 sensitive columns are classified and access is logged with user identity."
- Generates audit-ready evidence packages on demand (JSON + human-readable report)
- Stores compliance reports and evidence snapshots in app-isolated Postgres schema (`app_ai_compliance_manager`)

**Existing engine code it leverages:**
- `pkg/models/mcp_audit.go` — `MCPAuditEvent` with 10 event types, security levels, security flags
- `pkg/services/audit_service.go` — `AuditService` with `LogCreate()`, `LogUpdate()`, `LogDelete()`, entity tracking with `ChangedFields`
- `pkg/models/provenance.go` — `ProvenanceContext` with source + actor tracking
- `pkg/models/column_metadata.go` — `IsSensitive` flag, classification pipeline
- `pkg/auth/claims.go` — Role and access information
- `pkg/services/retention_service.go` — Data lifecycle compliance evidence

**Host functions used:** `db_query` (reads engine audit tables via app schema access), `llm_generate`, `get_auth_context`, `log`

**Why it's the third app:** Zero new data collection — aggregates and formats what the engine already captures. Demonstrates multi-source data aggregation. Highest dollar-value impact (directly reduces $50K-$100K audit costs). Proves the platform can safely read engine internal data.

---

### Suite economics

These three apps form a natural upsell chain:
1. **AI Drift Monitor** (entry point) — "Your schema changed and it's going to break things" → hooks the data team
2. **AI Data Guardian** (expansion) — "Here are all the quality issues, and here's how to fix them" → becomes indispensable
3. **AI Compliance Manager** (lock-in) — "Here's your audit evidence package, generated automatically" → justifies the platform to the CFO

Together they replace manual processes that cost mid-market companies 2-4 engineering weeks per quarter in audit prep, incident detection, and quality firefighting.

---

---

# Application Ideas: Full Catalog

**The following is a catalog of 29 application ideas** evaluated for the Ekaya Engine WASM platform. The first three (AI Data Guardian suite above) have been selected for initial implementation. The rest are prioritized below for future waves.

**Ekaya Engine sits at the intersection of three converging enterprise pain points: data infrastructure sprawl costing mid-market companies millions annually, a compliance gap at the data layer that existing tools ignore, and the collapse of the "modern data stack" into consolidated platforms.** The ideas below exploit Ekaya's unique combination of WASM-sandboxed execution, direct customer database access, and a managed PostgreSQL runtime — targeting the exact problems CTOs solve today with spreadsheets, scripts, and duct tape.

The core insight is this: **57% of data professionals** cite poor data quality as their top challenge, mid-market companies spend **up to 80% more per employee** on regulatory compliance than large enterprises, and **35% of engineering teams** have already replaced at least one SaaS tool with custom builds. Ekaya's architecture — apps that run on a customer's own data without moving it, in a secure sandbox, with PostgreSQL superpowers underneath — is purpose-built to capture this demand.

---

## The market context: why these apps, why now

Three forces make this the right moment for a WASM-based data app platform. First, **platform consolidation is accelerating**. Fivetran acquired dbt Labs in October 2025 for a combined ~$600M ARR entity, Snowflake absorbed Datavolo, and Databricks acquired Arcion — all signaling that the 8+ vendor "modern data stack" is collapsing. CTOs at 50–500 employee companies are desperate to reduce their tool count. A typical mid-market company uses **~185 apps** and spends across vendors with incompatible pricing models, creating budget unpredictability that finance teams reject.

Second, **compliance is becoming existential for mid-market**. Twenty US states will have comprehensive privacy laws by January 2026. The EU AI Act's prohibited-practices provisions took effect in February 2025. Over one-third of midmarket organizations already face unexpected compliance fines. Yet existing compliance automation tools like Vanta ($2.45B valuation, 15,000+ customers) and Drata operate almost entirely at the *infrastructure* layer — checking whether MFA is on, not whether PII is being queried without authorization. The data layer remains a manual process of cron jobs, spreadsheets, and SQL scripts.

Third, **WASM is production-ready for enterprise app delivery**. Shopify Functions runs untrusted merchant code as WASM modules with a 5ms execution budget and handles 100K modules per minute. Fermyon (acquired by Akamai) delivers sub-millisecond cold starts at 75 million requests per second. Extism's `pg_extism` brings WASM UDFs directly into PostgreSQL. The pattern of sandboxed, downloadable, lightweight applications executing against live data is proven.

---

## Apps 1–7: Data governance and compliance (the biggest gap)

These apps target the **data-layer compliance gap** — the space between infrastructure compliance tools (Vanta/Drata) and enterprise data governance platforms (Collibra at $100K+/year). Mid-market companies handle all of these processes manually today.

**1. PII Radar — Sensitive data discovery and classification.** Scans customer database columns using regex patterns, NLP heuristics, and embedding-based classifiers (pgvector) to identify and tag PII, PHI, and financial data. Generates GDPR Article 30 records of processing and CCPA data inventories automatically. Today, companies document PII column-by-column in spreadsheets — a process that becomes stale immediately. PII Radar runs on pg_cron schedules, stores classification results in JSONB, and flags new sensitive columns as schemas evolve. **The DSPM market is projected to reach $10B by 2033**, but current tools (Cyera, Varonis, BigID) cost $50K+ annually and target enterprises. A lightweight, data-platform-native scanner fills an obvious mid-market void.

**2. Query Audit Vault — Compliance-ready data access logging.** Captures every query that touches sensitive tables through Ekaya's data access layer, enriches logs with user identity and sensitivity classification, and generates audit-ready reports mapped to SOC 2 (CC6.1–CC6.3), HIPAA (§164.312), and GDPR requirements. Uses pgAudit patterns on the managed PostgreSQL instance and TimescaleDB-style partitioning for efficient long-term retention. The duct-tape alternative is exporting query logs to spreadsheets and manually searching for anomalies before audits — a process that takes engineers **weeks** per audit cycle.

**3. Access Review Automator — Periodic permission certification.** Pulls current database roles, table-level grants, and row-level security policies from the customer's database. Generates review campaigns that route to the correct managers via configurable workflows. Tracks approvals, flags stale permissions, and produces audit evidence packages. **Quarterly access reviews done via "spreadsheets emailed to managers"** are the most commonly cited compliance pain point in mid-market companies. Veza ($100M+ funding) targets this space but at enterprise price points. A WASM app that reads directly from `pg_roles` and `information_schema` and stores review state in managed PostgreSQL would deliver this at a fraction of the cost.

**4. Data Retention Enforcer — Policy-based lifecycle management.** Define retention rules per schema, table, or data classification tier. pg_cron monitors compliance continuously. When data ages past its retention window, the app auto-archives to cold partitions (using pg_partman), queues for deletion, or flags for human review — all with a complete audit trail in JSONB. Companies write retention policies in Word documents but **rarely enforce them programmatically**. This app makes policies executable, which is exactly what auditors and the emerging "compliance as code" movement (adopted by only 46% of CISOs so far) demand.

**5. DSAR Fulfillment Agent — Automated data subject requests.** Given a subject identifier (email, customer ID), scans all connected tables for matching records using the customer's database schema, assembles a complete data package, and optionally executes verified deletion with cryptographic proof. GDPR DSARs increased dramatically after €2.3B in fines were issued in 2025 alone. Most mid-market companies handle DSARs with engineers manually querying multiple systems — tools like DataGrail handle SaaS apps but **cannot execute deletions in databases** or prove completeness. This app leverages Ekaya's direct database access to solve the hardest part of DSAR compliance.

**6. Schema Drift Detective — Change detection and impact analysis.** Continuously monitors customer database schemas, detects additions, removals, type changes, and constraint modifications. Stores schema snapshots as JSONB, computes diffs, and alerts downstream consumers before pipelines break. **Schema drift** is one of the top causes of data pipeline failures, and Monte Carlo found that data incident detection takes **4+ hours on average** — often because stakeholders discover problems first. This app uses pg_cron to snapshot `information_schema` at configurable intervals and provides a queryable history of every schema change.

**7. Compliance Evidence Collector — Auto-generated audit packages.** Aggregates evidence from the platform itself — encryption status, backup configurations, access logs, role assignments, query audit summaries, data classification reports, retention policy compliance — and maps each artifact to specific SOC 2 controls, ISO 27001 Annex A controls, or HIPAA safeguards. Exports audit-ready packages on demand. Currently, companies scramble for weeks before audits to collect screenshots and documentation. The typical SOC 2 audit costs **$50K–$100K+** including consultant and auditor fees; reducing evidence collection time directly reduces that cost.

---

## Apps 8–13: Data infrastructure that replaces duct tape

These apps address the core data engineering bottlenecks: **77% of data engineering teams say workloads are getting heavier**, and organizations spend **30% of their time** on non-value-add data management tasks.

**8. Auto Data Catalog — Self-discovering, searchable data dictionary.** Uses Foreign Data Wrappers to introspect the customer's databases, extracts table/column metadata, infers descriptions using LLM analysis of column names and sample data, and stores everything in managed PostgreSQL with pgvector embeddings for semantic search. Teams can search "where do we store customer emails?" and get ranked results across all connected data sources. Mid-market companies rarely have formal data catalogs — enterprise tools like Atlan (Forrester Wave Leader, 2025) and Alation start at **$50K+/year**. An auto-discovering catalog running as a WASM app on Ekaya fills the gap between "nothing" and "six-figure contracts."

**9. Semantic Metrics Store — Define once, query everywhere.** A lightweight semantic layer where teams define canonical business metrics (MRR, churn rate, customer LTV) as SQL expressions with dimensions, filters, and access controls. Serves consistent metric values via API to any downstream tool — dashboards, notebooks, AI agents, Slack bots. Gartner's 2025 guidance elevated semantic layers **from optional to foundational**, and Snowflake's internal testing showed **85% accuracy for natural language queries with semantic context versus 40% without**. This directly enables the existing AI Data Liaison app to give dramatically better answers while solving the "everyone has different numbers" problem that plagues every growing company.

**10. Reverse ETL Sync — Push warehouse data to operational tools.** Configurable syncs that push data from the customer's warehouse to SaaS tools (Salesforce, HubSpot, Intercom, Zendesk) on pg_cron schedules. Transformation logic runs in WASM for safety and customizability. Stores sync state, change detection, and error logs in managed PostgreSQL. Census (acquired by Fivetran, May 2025) and Hightouch dominate this category, but their usage-based pricing creates budget anxiety. A platform-native reverse ETL app with predictable pricing and no data leaving the customer's infrastructure addresses the **38% of data engineers** who cite integration complexity as their top challenge.

**11. Data Quality Sentinel — Automated anomaly detection and freshness monitoring.** Defines expectations (volume thresholds, null rates, distribution bounds, freshness SLAs) per table or column. pg_cron runs checks on schedule, stores results in time-series partitions, and alerts via webhook or email when anomalies are detected. Uses statistical methods (z-score, IQR) with optional LLM-generated explanations of what went wrong. Monte Carlo ($1.5B+ market, growing at 8.7% CAGR) charges six figures annually. Bigeye and Soda are cheaper but still require separate infrastructure. **Companies currently maintain ~290 manually-written tests on average** — a WASM-based sentinel that auto-generates tests from data profiling would eliminate most of that toil.

**12. Change Data Capture Hub — Stream database changes downstream.** Captures INSERT/UPDATE/DELETE events from the customer's database (via PostgreSQL logical replication or LISTEN/NOTIFY patterns), stores them as an immutable event log in PGMQ, and routes changes to configurable destinations — webhooks, other databases, message queues, or downstream WASM apps on the platform. This replaces custom Debezium setups and Python CDC scripts that **20% of data teams** run with no orchestration at all.

**13. Customer 360 Builder — Entity resolution and profile unification.** Ingests customer records from multiple source tables (CRM, billing, support, product analytics), applies fuzzy matching (pg_trgm for string similarity, pgvector for semantic matching), and builds unified customer profiles stored in JSONB. Exposes a single API for "give me everything about this customer." **75% of companies struggle with siloed data** — this app attacks the problem at the entity level, producing the unified view that sales, support, and product teams all need but that typically requires a $200K+ CDP implementation.

---

## Apps 14–17: AI-native tools leveraging MCP and agent patterns

With **17,000+ MCP servers** indexed, the AI agent ecosystem is exploding, but production deployments remain rare — only **95 out of 1,837 respondents** in Cleanlab's 2025 survey had agents live in production. These apps productize proven patterns.

**14. MCP Gateway & Registry — Centralized AI tool access governance.** A managed MCP gateway that brokers all AI agent interactions with data sources through Ekaya. Provides OAuth 2.1 authentication, role-based tool access (which agents can query which tables), rate limiting, complete audit logging, and a visual registry of available MCP servers. MCP gateways are an emerging category (MintMCP, Bifrost, Docker MCP Catalog) but none are data-platform-native. This app turns Ekaya into the **control plane for enterprise AI data access** — directly addressing the concern that **74% of leaders** view AI agents as a new attack vector (Gartner 2025). It also upgrades the existing MCP Tunnel app from a simple endpoint into a governed, auditable gateway.

**15. AI Agent Workflow Builder — Visual multi-step agent orchestration.** A visual workflow designer where users define multi-step agent tasks: "Every Monday, query last week's support tickets, classify by urgency using AI, update the CRM, and post a summary to Slack." Each step executes as a WASM function with configurable AI model calls, database queries, and API actions. Stores workflow definitions in JSONB, execution history in time-series tables, and uses PGMQ for step-by-step async execution. This productizes the "deterministic backbone + intelligent steps" architecture that CrewAI identified from processing **1.7 billion agentic workflows** — the pattern where structured flows handle core logic while individual steps use varying levels of AI agency.

**16. AI Knowledge Sync — Automated RAG pipeline manager.** Monitors source data (documents, database tables, wiki pages) for changes, automatically re-chunks, re-embeds, and updates the pgvector knowledge base used by AI apps. Manages embedding model versioning, chunk overlap strategies, and hybrid search indexes (combining pgvector with ParadeDB BM25 scoring). This upgrades the existing AI Memory and Document Store from a static RAG setup to a **living knowledge base** that stays current without manual intervention. Hybrid search with BM25 + vector similarity improves recall by **15–30%** over pure vector search — a significant quality improvement for the AI Data Liaison.

**17. AI Cost & Usage Dashboard — LLM spend observability.** Tracks every LLM API call made through the platform — model, tokens consumed, latency, cost, calling agent, source query. Stores metrics in TimescaleDB-pattern hypertables for efficient time-series aggregation. Surfaces cost trends, identifies expensive queries, and enforces per-team or per-agent budget caps. As companies scale AI agent deployments, LLM costs become unpredictable — this is the "Datadog for AI spend" that every CTO running multiple agents needs. It complements the existing AI Agents and Automation app by adding the financial visibility layer.

---

## Apps 18–20: Internal tools that CTOs build every quarter

The internal tools market is projected to reach **$58.2B by 2029** (Gartner), and Retool alone generates ~$120M ARR. But **35% of teams are already replacing SaaS tools with custom builds** — signaling that packaged, data-connected apps on a platform like Ekaya can capture demand that's currently flowing to build-from-scratch projects.

**18. Cron Job Control Center — Visual scheduled task management.** A dashboard for managing all pg_cron jobs and scheduled WASM tasks across the platform: create, edit, monitor, pause, and inspect execution history with logs. Provides alerting on failures, execution duration trends, and dependency visualization. Every growing company accumulates scattered cron jobs — on servers, in CI/CD, in random AWS Lambda functions — that nobody fully understands. Centralizing them on the data platform with a visual interface, audit trail, and WASM-sandboxed execution eliminates the "what does this cron job do and who owns it?" problem that causes **incident detection delays of 4+ hours**.

**19. Approval Flow Engine — Configurable multi-step approval workflows.** A generic workflow engine for purchase approvals, access requests, content publishing, data change requests, and vendor onboarding. Define approval chains, conditional routing rules, escalation timeouts, and notification channels. Stores workflow definitions in JSONB, execution state in PostgreSQL, and provides a complete audit trail. **Approval workflows are the #3 most-built internal tool category** after admin panels and dashboards. Every company builds these — usually in Slack + Google Sheets + email — and rebuilds them when the process changes. A data-platform-native version can reference actual database state in approval conditions ("approve this discount only if customer LTV exceeds $50K").

**20. Geo-Intelligence Dashboard — Location analytics powered by PostGIS.** Visualizes business data on maps: customer distribution, delivery zones, store coverage areas, fleet positions, risk exposure by region. Combines PostGIS spatial queries with the customer's operational data to answer questions like "how many customers are within 30 minutes of each warehouse?" or "which zip codes have the highest support ticket density?" **PostGIS is described as one of the most underrated PostgreSQL extensions.** Combined with pgRouting for actual road-network calculations, this app replaces the cobbled-together Google Maps + spreadsheet solutions that operations and logistics teams at mid-market companies rely on — particularly valuable for companies in delivery, field services, insurance, and real estate.

---

## How these 20 apps map to Ekaya's technical differentiators

Each app exploits a specific combination of Ekaya's architectural advantages. The table below shows why these apps are better delivered on Ekaya than as standalone SaaS products.

| App | Customer DB access | Managed PostgreSQL | WASM sandboxing |
|-----|:-:|:-:|:-:|
| PII Radar | Scans live schemas | Stores classifications in JSONB | Safe execution of classification logic |
| Query Audit Vault | Monitors query patterns | pgAudit + partitioned storage | Tamper-proof logging in sandbox |
| Access Review Automator | Reads `pg_roles` directly | Stores review workflows | Isolated execution per tenant |
| Data Retention Enforcer | Enforces on source tables | pg_cron scheduling | Safe deletion scripts |
| DSAR Fulfillment Agent | Scans all connected tables | Assembles data packages | Sandboxed PII handling |
| Schema Drift Detective | Monitors `information_schema` | JSONB schema snapshots | Isolated diff computation |
| Compliance Evidence Collector | Reads platform state | Maps to compliance controls | Secure evidence assembly |
| Auto Data Catalog | FDWs for multi-source introspection | pgvector semantic search | Safe metadata extraction |
| Semantic Metrics Store | Queries customer warehouse | Caches computed metrics | Custom metric logic |
| Reverse ETL Sync | Reads source data | Stores sync state/errors | Safe transformation execution |
| Data Quality Sentinel | Profiles source tables | Time-series anomaly storage | Sandboxed test execution |
| CDC Hub | Captures source changes | PGMQ event storage | Safe routing logic |
| Customer 360 Builder | Reads from all source systems | pg_trgm + pgvector matching | Entity resolution logic |
| MCP Gateway & Registry | Brokers data access | Stores policies/audit logs | Sandboxed tool execution |
| AI Agent Workflow Builder | Agents query live data | PGMQ for step execution | Each step in sandbox |
| AI Knowledge Sync | Monitors source changes | pgvector + ParadeDB indexes | Safe embedding generation |
| AI Cost Dashboard | Tracks all AI API calls | TimescaleDB-pattern metrics | Isolated billing logic |
| Cron Job Control Center | Manages jobs on source DB | pg_cron orchestration | Sandboxed job execution |
| Approval Flow Engine | References live business data | JSONB workflow state | Isolated workflow execution |
| Geo-Intelligence Dashboard | Reads location data | PostGIS + pgRouting | Safe spatial computation |

---

## Prioritization: where to start for maximum market impact

Not all 20 apps have equal pull. Based on market urgency, competitive landscape, and Ekaya's unique positioning, three clusters emerge in priority order.

**Highest immediate demand** centers on the compliance cluster (Apps 1–7). Over one-third of mid-market companies face unexpected compliance fines, existing tools explicitly miss the data layer, and the alternative is manual work that costs engineering weeks per audit cycle. PII Radar, Query Audit Vault, and Access Review Automator form a "compliance trio" that no other data platform offers natively and that would immediately differentiate Ekaya from every competitor.

**Strongest platform lock-in** comes from the data infrastructure cluster (Apps 8–13). Auto Data Catalog and Semantic Metrics Store become the connective tissue that every other app depends on — once a company's metric definitions and data dictionary live on Ekaya, switching costs rise dramatically. These apps also directly amplify the existing AI Data Liaison by giving it semantic context, which Snowflake proved improves natural language query accuracy from **40% to 85%**.

**Largest TAM expansion** comes from the AI-native cluster (Apps 14–17). MCP Gateway & Registry positions Ekaya as the control plane for enterprise AI data access — a category that barely exists today but that **Gartner predicts will include integrated agents in 40% of enterprise apps by 2026**. Moving first here establishes Ekaya as the default governance layer between AI agents and company data.

---

## Apps 21–26: Earlier explorations (consolidated from ekaya-app-* repos)

These ideas were originally explored as separate repositories (Dec 2025) before the WASM application platform was conceived. The original repos have been archived and the unique concepts consolidated here.

**21. Data Chat — Conversational data interface.** A "chat with your data" frontend deployed within the customer's data boundary. Originally built as a Docker appliance (LibreChat fork + MongoDB + Redis + Meilisearch) connecting to ekaya-engine via MCP SSE. The concept: business users ask questions in plain English, the system generates SQL and returns visualized results (EChart artifacts). This is essentially a UI layer on top of the AI Data Liaison — worth revisiting as a lightweight WASM app that renders a chat interface within the engine UI rather than requiring a separate Docker deployment. The original implementation exists as a reference at `ekaya-app-data-chat` (archived).

**22. Data Factory — Continuously-fresh demo data generation.** Generates realistic, temporally-shifting demo data for prospect demos and testing. Day 1: generates 90-day historical data. Day 2+: shifts timestamps and adds new "today" records so demo data never goes stale. Uses LLM for realistic value generation (names, addresses, plausible business metrics) and preserves foreign key relationships across tables. Solves the universal "stale demo" problem — every SaaS company has demo data from 2023 that undermines credibility. A WASM app with `db_query` + `llm_generate` host functions could run this entirely within the engine.

**23. Data Model Factory — AI-powered "vibe coding" for schemas.** Describe what you want in natural language ("a calendar application with Google Calendar integration") and Ekaya generates the complete schema (tables, columns, constraints), ontology (entities, relationships, business rules), API endpoints, and integration stubs. Iterative refinement via natural language feedback. This is the ultimate expression of the "vibe-code data applications" vision from the WASM platform design — but focused on the data model layer rather than application logic. Could be a powerful onboarding tool: new users describe their domain and get a working schema instantly.

**24. Data Quality Agents — Ontology-aware quality analysis with LLM fix suggestions.** Detects issues (unexpected nulls, type mismatches, duplicate records, broken foreign keys, range violations, pattern mismatches) by analyzing data against the ontology. Unique differentiator: LLM generates SQL fix suggestions (e.g., "UPDATE users SET email = LOWER(email) WHERE email != LOWER(email)") that require human approval before execution. This overlaps significantly with App #11 (Data Quality Sentinel / AI Data Guardian) but adds the **fix suggestion + approval workflow** which is a distinct capability. The approval pattern should be incorporated into the AI Data Guardian design.

**25. Ontology Templates — Pre-built, one-click schema bootstrapping.** Library of curated ontology templates (3-5 tables each) for common business domains: digital marketing (campaigns, results, creatives), B2B sales pipeline, e-commerce analytics, mobile app analytics, subscription SaaS metrics, campaign optimization. Each template includes schema definitions, semantic ontology (descriptions, business rules, relationships), sample queries, and visual previews. Solves the cold-start problem for new projects — instead of connecting a database and waiting for AI discovery, start with a template and customize. Could be embedded in the engine binary or downloaded from the marketplace.

**26. Product Kit — Embedded text-to-SQL for SaaS products.** A standalone, read-only inference server that SaaS companies deploy into their production backend to give end-users AI-powered data querying. The server has **no access to the product database** — it generates safe, validated SQL that the product's own backend executes against the schema it was designed for (e.g., a denormalized table used for end-user analytics in the product UI).

**How it works:**
- Ekaya Engine exports a static config bundle (SQLite file) containing schema metadata, ontology rules (business entities, relationships), and optionally vector embeddings for semantic search. This is a point-in-time snapshot of the Ekaya intelligence — the Product Kit server loads it into memory at startup and runs independently.
- The product's backend sends a natural language query with a JWT containing tenant context (tenant_id, user_id, role, RLS fields). The Product Kit validates the JWT (HMAC), generates SQL using an LLM, validates the SQL for injection/leakage/RLS compliance, and returns the SQL + parameters. The product backend executes it.
- The server is stateless — all instances load the same config bundle, scale independently, no database connection required at runtime. Hot reload via SIGHUP when a new config bundle is deployed.

**View-based data access:** Customers expose data to AI through PostgreSQL views (not direct table access). Views define the AI-accessible schema — only columns explicitly included are queryable. Base table structure and sensitive columns remain hidden from the AI.

**Tenant isolation (two approaches):**
- **RLS on base tables (recommended):** Database RLS policies enforce isolation automatically. Product server sets session context (`SET app.tenant_id = $1`) before executing returned SQL. Even if the AI generates SQL without a tenant filter, RLS ensures isolation at execution time. Defense in depth.
- **WHERE clause validation (fallback):** Product Kit validates that generated SQL contains `WHERE tenant_id = $1` before returning it. Fails closed — SQL not returned if RLS fields missing. Use when RLS cannot be enabled on base tables.

**SQL validation pipeline:**
- Injection detection: pattern matching for DROP, TRUNCATE, DELETE, UPDATE, stacked queries, UNION injection, pg_sleep
- RLS enforcement: validates required WHERE clauses (Approach B) or skips (Approach A, database handles it)
- Data leakage detection: flags SELECT * on wide tables, deep JOINs, missing LIMIT clauses
- Automatic LIMIT injection: appends configurable max_rows (default 1000) if no LIMIT present
- PII column flagging: warns when queries select sensitive columns

**LLM integration:** The server requires LLM access for text-to-SQL generation. Two models:
1. **Product Kit hosts its own LLM client:** Calls the customer's LLM endpoint (AWS Bedrock, GCP Vertex AI, Azure AI, or any OpenAI-compatible API). Multi-provider abstraction behind a common interface. The product team configures which provider/model to use.
2. **Product's LLM intercepts the call:** The product already has an LLM (e.g., for a chat UI). The text-to-SQL request becomes a tool call that the product's LLM hosting code intercepts — it calls the Product Kit's generate-sql endpoint, executes the returned SQL, and incorporates the results into its response. This is the more likely integration pattern for products that already have AI features.

**API surface:**
- `POST /api/v1/generate-sql` — NL query in, validated SQL + parameters + security metadata out
- `GET /api/v1/schema` — cached schema info (for debugging)
- `GET /health` — health check with component status (config loaded, LLM available, vector index ready)

**Config bundle contents (SQLite):**
- `config_version` — version, timestamp, schema hash, isolation model, required RLS columns
- `schema_tables` — view names and descriptions (views only, not base tables)
- `schema_columns` — column names, types, nullability, PII flags, descriptions
- `ontology_entities` — business rules as JSON arrays, entity relationships
- `vector_embeddings` — optional, for semantic search (binary float32 vectors)
- Schema safety validation on load: refuses to start if views are missing required tenant isolation columns

**Deployment targets:** <50MB Docker image, <5s cold start, stateless horizontal scaling. Can run as Cloud Run service, Kubernetes deployment, ECS task, or sidecar container.

**Key insight:** Ekaya's intelligence becomes an embeddable component. The go-to-market is not "install Ekaya" but "embed Ekaya's intelligence in your product." This is a B2B2C play — different buyer (SaaS CTOs), different pricing (embedded licensing), different distribution (developer docs, SDKs).

**27. Smart Ingestion — OAuth-based third-party data connector.** Admin-facing data ingestion engine for pulling data from external sources (Google Analytics 4, Salesforce, HubSpot, Stripe, custom APIs) into Ekaya-managed ontology. OAuth-first authentication, encrypted credential storage, automatic token refresh, and incremental sync with CDC state management. Source data maps to semantic ontology rather than raw schema. This is the inbound counterpart to App #10 (Reverse ETL Sync) — together they form a complete data integration layer. The connector framework pattern (interface-based, pluggable) aligns well with WASM: each connector could be a separate WASM module.

---

## Updated prioritization with consolidated apps

The earlier explorations add three new priority considerations:

**Immediate GTM amplifier:** App #25 (Ontology Templates) has the highest impact on reducing time-to-value for new users. A prospect who can start with a "digital marketing" template and see results in 5 minutes is far more likely to convert than one who needs to connect a database and wait for ontology discovery. This should be considered for the first wave alongside the AI Data Guardian suite.

**New market segment:** App #26 (Product Kit) opens an entirely different buyer — SaaS CTOs who want to add AI data features to their own products. This is a B2B2C play with different pricing (embedded licensing) and different distribution (developer docs, SDKs). Worth tracking as a separate initiative.

**Fix suggestions as differentiator:** App #24's LLM-generated fix suggestions with human approval should be incorporated into the AI Data Guardian (App #11) design. Detection without remediation is table stakes — detection with AI-suggested fixes and one-click approval is the moat.

**28. Agent Memory — Persistent AI memory with vector search and cross-session communication.** Transforms Postgres into a long-term memory system for AI coding agents (Claude Code, Claude Chat, etc.). Stores structured memory types: facts with confidence/source tracking, people & entity profiles, decision logs with rationale, action audit trails, pending question queues, and temporal facts with validity periods. Uses pgvector embeddings for semantic retrieval ("what do I know about monthly costs?" finds WillDom billing facts without exact keyword matching). Key differentiator: **cross-session communication** via asynchronous, thread-based messaging between Claude instances — Claude Code implements a feature while Claude Chat explores design, coordinating via shared threads and handoff snapshots. Includes bug reporting between sessions and session continuity ("what was I working on yesterday?"). This is distinct from App #16 (AI Knowledge Sync) which focuses on document/data RAG pipelines — Agent Memory is purpose-built for AI agent workflows and inter-agent coordination.

**29. Public Address — Zero-config public HTTPS for ekaya-engine.** An app that gives any ekaya-engine instance a public HTTPS URL without TLS certificate configuration. Engine opens a secure tunnel to a managed service running at `us.[dev.]ekaya.ai`. The managed service authenticates and proxies calls from the public address back to the user's engine instance. Eliminates the entire HTTPS setup problem for users who don't need custom domains/certs — which is most users during initial setup and evaluation. Similar to ngrok or Cloudflare Tunnel but integrated into the platform. This directly supports the marketplace funnel: a prospect who installs via `try.ekaya.ai` shouldn't need to configure TLS before trying the product.

---

## Notes: PII Radar (App #1) implementation inputs

App #1 (PII Radar) should incorporate the patterns from the sensitive data detection security plan (previously `SECURITY-sensitive-data-detection.md`). Key implementation details to preserve:

**Detection patterns** (column name regex + content scanning):
- Column names: `api_key`, `password`, `secret_key`, `access_token`, `private_key`, `credential`, `ssn`, `credit_card`
- Content patterns: JSON keys containing sensitive terms, JWT tokens, connection strings, PII formats (email, phone, SSN)

**Classification categories with default actions:**
- `secrets` (API keys, tokens, passwords) → Block by default
- `pii_identity` (SSN, passport) → Block by default
- `pii_contact` (email, phone, address) → Flag for review
- `pii_financial` (credit card, bank account) → Block by default

**Admin approval workflow:**
- Persist decisions in database (allow/block/pending per column)
- Decisions survive re-extraction — don't re-flag what's already been reviewed
- Schema UI shows flagged columns with Allow/Block buttons
- Dashboard badge shows count of columns pending review
- MCP tools honor decisions: blocked columns show `[BLOCKED: Admin decision]` instead of sample values

**Origin:** This was identified from a real security finding — `get_context` with `include: ["sample_values"]` exposed LiveKit API keys from a `users.agent_data` JSONB column.