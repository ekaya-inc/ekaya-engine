# Ekaya Engine Application Brainstorm

## First Four Apps: The AI Data Guardian Suite

**Migrated to:** `plans/guardian/` — The Data Guardian suite and all governance/compliance apps have been reorganized into dedicated DESIGN files:
- `plans/guardian/DESIGN-guardian-security-alerting.md` — Security & Alerting (including AI Query Monitor)
- `plans/guardian/DESIGN-guardian-audit-visibility.md` — Audit & Visibility
- `plans/guardian/DESIGN-guardian-access-control.md` — Access Control & Data Protection
- `plans/guardian/DESIGN-guardian-compliance.md` — Compliance & Governance
- `plans/guardian/DESIGN-guardian-data-quality.md` — Data Quality & Drift (including AI Drift Monitor, AI Data Quality Monitor)
- `plans/guardian/DESIGN-guardian-data-protection.md` — Data Protection & Privacy (including PII Radar, Bias Monitor)
- `plans/guardian/DESIGN-guardian-sql-evaluation.md` — SQL Evaluation Security Layer
- `plans/guardian/DESIGN-guardian-integrations.md` — Integrations & Infrastructure

**Original context preserved below for non-Guardian apps (Apps 8+).**

<!-- Data Guardian Suite apps (1-4), suite economics, market context, Apps 1-7b, and PII Radar notes
     have been migrated to plans/guardian/DESIGN-guardian-*.md files. See migration pointers above. -->

<!-- BEGIN REMOVED SECTION (migrated to plans/guardian/) -->
<!-- Apps 1-4 (Data Guardian Suite), suite economics, market context, Apps 1-7b (governance/compliance) removed.
     Content now lives in plans/guardian/DESIGN-guardian-*.md files. -->
<!-- END REMOVED SECTION -->

## Apps 8–13: Data infrastructure that replaces duct tape

These apps address the core data engineering bottlenecks: **77% of data engineering teams say workloads are getting heavier**, and organizations spend **30% of their time** on non-value-add data management tasks.

**8. Auto Data Catalog** — *Migrated to `plans/edge/DESIGN-edge-discovery.md`*

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
| *Apps 1-7 (Data Guardian)* | *Migrated to `plans/guardian/`* | | |
| *App 8 (Auto Data Catalog)* | *Migrated to `plans/edge/`* | | |
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

**Highest immediate demand** centers on the compliance cluster (Apps 1–7, now migrated to `plans/guardian/`). See `plans/guardian/DESIGN-guardian-*.md` for the full Data Guardian product design.

**Strongest platform lock-in** comes from the data infrastructure cluster (Apps 9–13; App 8 Auto Data Catalog migrated to `plans/edge/`). Semantic Metrics Store becomes connective tissue that every other app depends on — once a company's metric definitions live on Ekaya, switching costs rise dramatically. These apps also directly amplify the existing AI Data Liaison by giving it semantic context, which Snowflake proved improves natural language query accuracy from **40% to 85%**.

**Largest TAM expansion** comes from the AI-native cluster (Apps 14–17). MCP Gateway & Registry positions Ekaya as the control plane for enterprise AI data access — a category that barely exists today but that **Gartner predicts will include integrated agents in 40% of enterprise apps by 2026**. Moving first here establishes Ekaya as the default governance layer between AI agents and company data.

---

## Apps 21–26: Earlier explorations (consolidated from ekaya-app-* repos)

These ideas were originally explored as separate repositories (Dec 2025) before the WASM application platform was conceived. The original repos have been archived and the unique concepts consolidated here.

**21. Data Chat** — *Migrated to `plans/edge/DESIGN-edge-chat.md`*

**22. Data Factory — Continuously-fresh demo data generation.** Generates realistic, temporally-shifting demo data for prospect demos and testing. Day 1: generates 90-day historical data. Day 2+: shifts timestamps and adds new "today" records so demo data never goes stale. Uses LLM for realistic value generation (names, addresses, plausible business metrics) and preserves foreign key relationships across tables. Solves the universal "stale demo" problem — every SaaS company has demo data from 2023 that undermines credibility. A WASM app with `db_query` + `llm_generate` host functions could run this entirely within the engine.

**23. Data Model Factory** — *Migrated to `plans/edge/DESIGN-edge-discovery.md`*

**24. Data Quality Agents — Ontology-aware quality analysis with LLM fix suggestions.** Detects issues (unexpected nulls, type mismatches, duplicate records, broken foreign keys, range violations, pattern mismatches) by analyzing data against the ontology. Unique differentiator: LLM generates SQL fix suggestions (e.g., "UPDATE users SET email = LOWER(email) WHERE email != LOWER(email)") that require human approval before execution. This overlaps significantly with App #11 (Data Quality Sentinel / AI Data Guardian) but adds the **fix suggestion + approval workflow** which is a distinct capability. The approval pattern should be incorporated into the AI Data Guardian design.

**25. Ontology Templates** — *Migrated to `plans/edge/DESIGN-edge-discovery.md`*

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

**Immediate GTM amplifier:** App #25 (Ontology Templates, migrated to `plans/edge/DESIGN-edge-discovery.md`) has the highest impact on reducing time-to-value for new users. Key feature of Ekaya Edge.

**New market segment:** App #26 (Product Kit) opens an entirely different buyer — SaaS CTOs who want to add AI data features to their own products. This is a B2B2C play with different pricing (embedded licensing) and different distribution (developer docs, SDKs). Worth tracking as a separate initiative.

**Fix suggestions as differentiator:** App #24's LLM-generated fix suggestions with human approval should be incorporated into the AI Data Guardian design (`plans/guardian/DESIGN-guardian-data-quality.md`). Detection without remediation is table stakes — detection with AI-suggested fixes and one-click approval is the moat.

**28. Agent Memory — Persistent AI memory with vector search and cross-session communication.** Transforms Postgres into a long-term memory system for AI coding agents (Claude Code, Claude Chat, etc.). Stores structured memory types: facts with confidence/source tracking, people & entity profiles, decision logs with rationale, action audit trails, pending question queues, and temporal facts with validity periods. Uses pgvector embeddings for semantic retrieval ("what do I know about monthly costs?" finds WillDom billing facts without exact keyword matching). Key differentiator: **cross-session communication** via asynchronous, thread-based messaging between Claude instances — Claude Code implements a feature while Claude Chat explores design, coordinating via shared threads and handoff snapshots. Includes bug reporting between sessions and session continuity ("what was I working on yesterday?"). This is distinct from App #16 (AI Knowledge Sync) which focuses on document/data RAG pipelines — Agent Memory is purpose-built for AI agent workflows and inter-agent coordination.

**29. Public Address — Zero-config public HTTPS for ekaya-engine.** An app that gives any ekaya-engine instance a public HTTPS URL without TLS certificate configuration. Engine opens a secure tunnel to a managed service running at `us.[dev.]ekaya.ai`. The managed service authenticates and proxies calls from the public address back to the user's engine instance. Eliminates the entire HTTPS setup problem for users who don't need custom domains/certs — which is most users during initial setup and evaluation. Similar to ngrok or Cloudflare Tunnel but integrated into the platform. This directly supports the marketplace funnel: a prospect who installs via `try.ekaya.ai` shouldn't need to configure TLS before trying the product.

---

## Notes: PII Radar (App #1) implementation inputs

**Migrated to:** `plans/guardian/DESIGN-guardian-data-protection.md` — PII Radar applet with full detection patterns, classification categories, and admin approval workflow.