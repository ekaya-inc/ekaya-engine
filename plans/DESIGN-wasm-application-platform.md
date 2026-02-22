# DESIGN: WASM Application Platform for Ekaya Engine

**Status:** EXPLORATORY RESEARCH
**Created:** 2026-02-21
**Updated:** 2026-02-21
**Branch:** wasm-application-platform

## Vision

Build a platform for downloadable, AI-automated WASM applications that run sandboxed inside Ekaya Engine with controlled access to databases, MCP tools, datasources, and LLM capabilities. Applications are distributed through a self-service marketplace (see companion DESIGN files) where users discover, trial, and install apps with a click.

The first three applications — **AI Drift Monitor**, **AI Data Guardian**, and **AI Compliance Manager** — form the **AI Data Guardian suite**, proving the platform with high-impact, AI-automated data reliability and compliance tools.

---

## Architecture Overview

```
User installs app from ekaya-central marketplace
  → Engine syncs app state from central
    → Engine downloads WASM package (verified by hash)
      → WASM runs in Extism sandbox with host functions
        → Host functions provide DB/datasource/MCP/LLM/HTTP access
          → App runs on schedule or on-demand, AI-automated
```

### Runtime Stack

```
WASM App (JavaScript via QuickJS-in-WASM, or compiled Rust/Go)
    │
    │ calls imported host functions
    ▼
Extism Host SDK (wraps wazero, pure Go, no CGO)
    │
    │ routes to ekaya-engine services
    ▼
├── db_query(sql, params)                → app's isolated Postgres schema
├── datasource_query(sql, params)        → read-only queries on project datasource
├── datasource_execute(query_id, params) → pre-approved queries on project datasource
├── mcp_tool_invoke(tool_name, params)   → ToolRegistry
├── llm_generate(prompt, options)        → project's LLM config (BYOK/community/embedded/on-prem)
├── http_request(method, url, ...)       → allowlisted external APIs
├── get_auth_context()                   → project_id, roles, user_id
└── log(level, message)
```

---

## Key Technology Choices

### WASM Runtime: wazero (via Extism)

- **wazero** — Pure Go WASM runtime, zero dependencies, no CGO
  - Core WASM 1.0/2.0 compliant
  - Does NOT support Component Model (maintainers explicitly rejected it — see https://github.com/tetratelabs/wazero/issues/2200)
  - WASI Preview 1 support
  - Custom host functions via Go API
  - JIT compilation to native assembly
  - Homepage: https://wazero.io/
  - Source: https://github.com/tetratelabs/wazero

- **Extism** — Higher-level plugin framework wrapping wazero
  - Pure Go, no CGO
  - Host function binding with simpler API than raw wazero
  - Persistent memory / module-scoped variables
  - Host-controlled HTTP (no WASI needed)
  - Runtime limiters and timers
  - Go SDK: https://github.com/extism/go-sdk
  - Framework: https://github.com/extism/extism

### App Language Strategy

**For Ekaya-built apps (first-party):** Use any language that compiles to WASM. Rust, Go (TinyGo), or C/C++ are all options since we control the build toolchain in this repository. The WASM binary is pre-compiled and distributed as a package — no compiler needed on the customer's engine.

**For user/AI-generated apps (future vibe-coding):** JavaScript via QuickJS-in-WASM. No compiler/toolchain needed on the server. Zero compilation step. AI generates JavaScript far more reliably than any other WASM-targeting language.

This is a key distinction from the original design: first-party apps are pre-compiled WASM packages distributed through the marketplace. The QuickJS runtime is for the future vibe-coding use case where users generate apps in the engine UI.

### Why NOT Other Languages for User-Generated Code

| Language | Problem |
|----------|---------|
| **Rust, Go, C/C++** | Requires compiler toolchain installed on server. Cannot compile at runtime. |
| **AssemblyScript** | Looks like TypeScript but has significant semantic differences (no closures over GC objects, no union types, strict memory model). AI will constantly generate broken code because it writes TypeScript patterns that don't compile. Small community = less AI training data. |
| **TinyGo** | Requires Go toolchain on server. Subset of Go with limitations. |

### Performance Trade-off

For first-party compiled WASM apps: near-native performance. For interpreted JS inside WASM: slower, but for data applications the hot path is the database query, not the app logic. These apps are I/O-bound, not CPU-bound.

---

## Reference Architecture: IronClaw

IronClaw (https://github.com/nearai/ironclaw) is a production-proven WASM sandbox (v0.9.0) that provides a strong reference for security and architecture patterns, even though it uses a different runtime (Wasmtime/Rust).

### Patterns to Borrow from IronClaw

**Security Model:**
- Capability-based permissions declared in a capabilities file per app
- Credential injection at host boundary (WASM never sees raw secrets)
- Leak detection scanning all outputs for secret exfiltration
- Fresh instance per execution (no shared state between runs)
- Allowlisted HTTP endpoints only

**Resource Limiting:**
- Memory limits (10MB default, configurable per app)
- CPU fuel metering (instruction count limits)
- Timeout enforcement via epoch interruption
- Rate limiting per tool
- Max request/response body sizes

**Architecture:**
- Compile-once, instantiate-fresh execution pattern
- Tool aliasing (indirection layer preventing direct access)
- Capabilities declaration file per app (`.capabilities.json`)
- BLAKE3 hash verification on module load
- Structured logging with rate limiting

### IronClaw Host Functions (for reference)

| Function | Purpose |
|----------|---------|
| `log(level, message)` | Structured logging (1000 entries max, 4KB per message) |
| `now-millis()` | Current Unix timestamp |
| `workspace-read(path)` | Read files from agent workspace (read-only) |
| `http-request(method, url, headers, body, timeout)` | Allowlisted HTTP requests |
| `tool-invoke(alias, params)` | Call other tools by alias |
| `secret-exists(name)` | Check if secret exists (cannot read value) |

### IronClaw Security Threat Model

| Threat | Mitigation |
|--------|-----------|
| CPU exhaustion | Fuel metering |
| Memory exhaustion | ResourceLimiter (10MB default) |
| Infinite loops | Epoch interruption + timeout |
| Filesystem access | No WASI FS, only host workspace_read |
| Network access | Allowlisted endpoints only |
| Credential exposure | Injection at host boundary only |
| Secret exfiltration | Leak detector scans all outputs |
| Log spam | Max 1000 entries, 4KB per message |
| Path traversal | Validate paths (no `..`, no `/` prefix) |
| Side channels | Fresh instance per execution |
| Rate abuse | Per-tool rate limiting |
| WASM tampering | Hash verification on load |

---

## App Distribution Model

### Package Format

Each app is a **package** — not a bare WASM binary. A package contains:

```
ai-drift-monitor/
├── manifest.json          # App metadata, version, capabilities, host function requirements
├── capabilities.json      # Security: allowed host functions, HTTP endpoints, resource limits
├── app.wasm               # Compiled WASM binary (pre-compiled for first-party apps)
├── schema.sql             # Initial database schema for app's isolated Postgres schema (optional)
└── ui.json                # Output schema: what the app returns and how to render it (optional)
```

### manifest.json

```json
{
  "id": "ai-drift-monitor",
  "name": "AI Drift Monitor",
  "version": "1.0.0",
  "description": "AI-automated schema drift detection and impact analysis",
  "author": "Ekaya Inc",
  "category": "data-quality",
  "requiresAI": true,
  "minEngineVersion": "1.5.0",
  "execution": {
    "modes": ["scheduled", "on-demand"],
    "defaultSchedule": "0 */6 * * *",
    "timeout": "300s",
    "memoryLimit": "32MB"
  },
  "hostFunctions": [
    "db_query",
    "datasource_query",
    "llm_generate",
    "get_auth_context",
    "log"
  ],
  "hash": "blake3:abc123..."
}
```

### Distribution

**Phase 1 (First-party apps):** WASM packages are built in this repository (`wt-ekaya-engine-applications`), published to GitHub Releases or a GCS bucket. Engine downloads on install via URL from ekaya-central's app catalog.

**Phase 2 (Marketplace):** ekaya-central hosts an app catalog with metadata, screenshots, pricing. WASM binaries stored in cloud storage. Engine downloads on install, verifies hash, caches locally.

**Phase 3 (Third-party):** Developers submit apps to the marketplace. Review process validates capabilities, security, and quality. Published apps available to all Ekaya users.

---

## Ekaya Engine Integration Points

### Existing Infrastructure to Leverage

1. **Application Model** — `InstalledAppService` + `engine_installed_apps` table already manages app lifecycle (install, activate, uninstall) with billing via ekaya-central callbacks.

2. **Tool Registry** — Single `ToolRegistry` with 49 tool definitions, role-based filtering, and app-installation gating. WASM apps could expose new tools or consume existing ones.

3. **Datasource Adapters** — `DatasourceAdapterFactory` with PostgreSQL and MSSQL adapters, connection pooling, and RLS tenant isolation. Host functions would wrap these.

4. **Schema Introspection** — `SchemaDiscoverer` interface with `DiscoverTables()`, `DiscoverColumns()`, `DiscoverForeignKeys()`, `AnalyzeColumnStats()` for both PostgreSQL and MSSQL.

5. **Schema Change Detection** — `SchemaChangeDetectionService` detects 5 change types: new table, dropped table, new column, dropped column, modified column.

6. **Column Classification** — `ColumnFeatureExtractionService` extracts semantic features, classifies columns (timestamp, boolean, enum, identifier, etc.), tracks sensitivity flags.

7. **Auth/Role System** — JWT claims with role-based access. Host functions would inject auth context into WASM apps.

8. **Audit Logging** — Dual audit system: general audit log (entity CRUD + provenance) and MCP audit log (tool calls + security classification with 10 event types). Both available for compliance reporting.

9. **LLM Infrastructure** — `LLMClientFactory` with BYOK, community, and embedded modes. Streaming support. Conversation recording. Per-project configuration.

10. **Work Queue** — In-memory task queue with concurrency control, retry logic (exponential backoff), and state tracking. Can schedule app executions.

### Host Functions to Implement

| Host Function | Maps To | Notes |
|---------------|---------|-------|
| `db_query(sql, params)` | App-isolated Postgres schema | Each app gets its own schema. RLS or schema-based isolation. |
| `datasource_query(sql, params)` | `QueryExecutor.Query()` | Read-only queries on project datasource. For apps like Drift Monitor that need schema introspection. |
| `datasource_execute(query_id, params)` | `QueryService.Execute()` | Pre-approved parameterized queries only. App cannot run arbitrary SQL against project datasource. |
| `mcp_tool_invoke(tool_name, params)` | `ToolRegistry` | Subject to same role/app-installation checks. |
| `llm_generate(prompt, options)` | `LLMClientFactory.CreateForProject()` | Uses project's AI config. Rate-limited. Token usage tracked. |
| `http_request(method, url, headers, body, timeout)` | Direct HTTP | Allowlisted endpoints declared in app capabilities. |
| `get_auth_context()` | `auth.Claims` | Returns project_id, user roles, user_id. Read-only. |
| `log(level, message)` | Structured logger | Rate-limited, size-limited. |

### Database Isolation Strategy

Each WASM app gets its own Postgres schema:

```sql
CREATE SCHEMA app_{app_id};
GRANT ALL ON SCHEMA app_{app_id} TO ekaya;
-- App's db_query() host function prefixes all queries with SET search_path
```

This provides:
- Data isolation between apps
- Apps can create their own tables, indexes
- No access to engine internal tables (engine uses `public` schema)
- No access to project datasources except via `datasource_query()` / `datasource_execute()`
- Easy cleanup on app uninstall (DROP SCHEMA CASCADE)

---

## First Three Apps: AI Data Guardian Suite

All three apps are **AI-automated** — they require LLM access and use it to eliminate manual work. They form a cohesive data reliability and compliance suite.

### 1. AI Drift Monitor

**Problem:** Schema drift causes pipeline failures. Detection takes 4+ hours on average. Teams discover problems after stakeholders complain.

**What it does (AI-automated):**
- Scheduled schema snapshots via `SchemaDiscoverer` (existing interface, PostgreSQL + MSSQL)
- Change detection via existing `SchemaChangeDetectionService` patterns
- **AI generates natural-language impact analysis:** "The `orders.discount_code` column was dropped. This will break 3 approved queries that reference it: [list]. The column had 45,000 non-null values."
- **AI classifies severity:** breaking change vs. additive change vs. cosmetic
- Stores historical timeline of all schema changes in app-isolated schema
- Alerts via webhook when breaking changes detected

**Existing code leveraged:**
- `pkg/adapters/datasource/postgres/schema.go` — Table/column/FK discovery
- `pkg/adapters/datasource/mssql/schema.go` — Same interfaces
- `pkg/services/schema_change_detection.go` — Change type detection
- `pkg/models/column_metadata.go` — Column classification context

**Why it's a good first app:**
- Lowest complexity — schema introspection queries already exist
- Clear I/O pattern — read schema, compare, generate report
- High value with minimal computation
- Demonstrates scheduled execution + LLM integration

### 2. AI Data Guardian

**Problem:** 57% of data professionals cite poor data quality as their #1 challenge. Companies maintain ~290 manually-written tests. Monte Carlo charges six figures.

**What it does (AI-automated):**
- Profiles data using aggregate queries (null rates, cardinality, distributions, freshness)
- **AI auto-generates quality expectations** from profiling results: "This column has 0.3% nulls historically. Alert if null rate exceeds 2%."
- Scheduled checks against expectations using `QueryExecutor`
- **AI explains anomalies** when checks fail: "The `users.email` column null rate jumped from 0.3% to 15.2% in the last 24 hours. This correlates with a new column `users.sso_id` appearing yesterday — likely a migration that made email optional for SSO users."
- Stores expectations and check results in app-isolated schema

**Existing code leveraged:**
- `pkg/adapters/datasource/interfaces.go` — `QueryExecutor` for aggregate queries
- `pkg/services/column_feature_extraction.go` — Feature extraction patterns
- `pkg/adapters/datasource/postgres/schema.go` — `AnalyzeColumnStats()`, `GetDistinctValues()`

**Why it's a good second app:**
- Builds on Drift Monitor's scheduling + LLM patterns
- Adds statistical computation (z-score, IQR — simple math)
- Higher value density — the AI-generated expectations eliminate the "cold start" problem

### 3. AI Compliance Manager

**Problem:** SOC 2 audits cost $50K-$100K+. Companies scramble for weeks collecting screenshots and documentation. Existing tools (Vanta, Drata) cover infrastructure but miss the data layer.

**What it does (AI-automated):**
- Reads engine's own audit infrastructure (MCP audit events, general audit log, column sensitivity, role assignments)
- **AI maps evidence to compliance frameworks:** "MCP audit events showing query_blocked events with security_level=critical map to SOC 2 CC6.1 (Logical and Physical Access Controls)"
- **AI generates compliance narratives:** "During the reporting period, 47 unauthorized table access attempts were detected and blocked. All sensitive columns are classified and access is logged."
- Generates audit-ready evidence packages on demand
- Stores compliance reports and evidence snapshots in app-isolated schema

**Existing code leveraged:**
- `pkg/models/mcp_audit.go` — 10 event types with security classification
- `pkg/services/audit_service.go` — Entity CRUD tracking with provenance
- `pkg/models/column_metadata.go` — `IsSensitive` flag, classification
- `pkg/auth/claims.go` — Role and access information
- `pkg/services/retention_service.go` — Data lifecycle compliance

**Why it's a good third app:**
- Zero new data collection — aggregates existing audit data
- Demonstrates multi-source data aggregation pattern
- Highest dollar-value impact (directly reduces audit costs)
- Proves the platform can read engine internal data safely

---

## Effort Estimates

### WASM Platform Foundation (~3-4 weeks)

| Component | Effort | Notes |
|-----------|--------|-------|
| Extism integration + plugin loader | 3-4 days | Well-documented Go SDK |
| Host function: isolated Postgres schema (`db_query`) | 1 week | Schema-per-app + connection routing |
| Host function: datasource queries (`datasource_query`) | 3-5 days | Wrap SchemaDiscoverer + QueryExecutor |
| Host function: LLM access (`llm_generate`) | 2-3 days | Wrap LLMClientFactory |
| Host function: MCP tool invocation | 2-3 days | Route through ToolRegistry |
| Host function: HTTP (allowlisted) | 2-3 days | Extism has built-in HTTP, add allowlist |
| App package loader + hash verification | 2-3 days | Download, verify, cache WASM packages |
| App scheduler (cron-style execution) | 2-3 days | Extend work queue for scheduled apps |
| Security: capability declarations, resource limits | 3-4 days | Borrow IronClaw's capability.json pattern |
| App sync with central | 2-3 days | Reconcile on auth (see companion DESIGN) |

### First Three Apps (~2-3 weeks each)

| App | Effort | Notes |
|-----|--------|-------|
| AI Drift Monitor | 2 weeks | Schema queries exist; new: scheduling, diffing, LLM analysis, output |
| AI Data Guardian | 2-3 weeks | Stats queries exist; new: expectation model, anomaly detection, LLM explanations |
| AI Compliance Manager | 2 weeks | Audit data exists; new: framework mapping, evidence assembly, LLM narratives |

### UI for App Management (additional ~1-2 weeks)

| Component | Effort | Notes |
|-----------|--------|-------|
| App list in sidebar | 2-3 days | Show installed apps |
| App output page (generic JSON renderer) | 3-5 days | Tables, charts, status indicators |
| App settings panel | 2-3 days | Schedule, notifications, configuration |
| App activity log | 2-3 days | Recent executions, results, errors |

---

## Open Questions (Updated)

### Answered

1. **App distribution model** — Package format with manifest + capabilities + WASM binary + optional schema. First-party apps pre-compiled in this repo, distributed via cloud storage. *(Answered above)*

2. **Marketplace** — Yes. ekaya-central hosts the app catalog. Self-service install with trial periods. First-party apps first, third-party later. *(See companion DESIGN files)*

3. **AI generation prompt** — Deferred. Vibe-coding is Phase 2+. First-party apps are pre-compiled. *(Clarified above)*

### Still Open

4. **App UI rendering** — Three progressive levels:
   - **Phase 1: Structured data** — App returns JSON. Engine UI has a generic renderer: tables, charts, key-value pairs, status indicators. App manifest declares output schema. **Start here** — safe, consistent, sufficient for all three first apps.
   - **Phase 2: Rich components** — App returns a component descriptor (React component tree as JSON). Engine UI renders using pre-built components: alert lists, timeline views, diff viewers, metric cards.
   - **Phase 3: Custom HTML (maybe never)** — App returns HTML rendered in an iframe sandbox. Maximum flexibility but security and UX concerns.

   Each installed app gets: a nav item in the engine sidebar (under "Apps" section), an app-specific page that renders output, a settings panel (schedule, notifications, config), and an activity log (recent executions, results, errors).

   Remaining question: How rich does the Phase 1 renderer need to be? Diff viewer for Drift Monitor? Chart for Data Guardian trends?

5. **App lifecycle** — Scheduled (cron) is the primary mode for all three first apps. On-demand is secondary (user clicks "Run now"). Event-driven (on schema change) is future.

6. **Versioning** — How are app updates handled? Engine downloads new WASM binary, verifies hash, hot-swaps? Or requires restart? Rollback mechanism?

7. **Testing** — How do users test apps before deploying? Preview mode against sample data? Or just rely on trial period?

8. **Inter-app communication** — Can apps call each other, or are they fully isolated? e.g., can Compliance Manager read Drift Monitor's data? Recommendation: fully isolated initially, shared read access later.

9. **Notification delivery** — How do apps send alerts? Webhook is simplest. Email requires new engine capability. In-app notifications require UI infrastructure.

10. **Multi-datasource** — Can an app monitor multiple datasources in one project? The adapter infrastructure supports it, but the host function interface needs to handle datasource selection.
