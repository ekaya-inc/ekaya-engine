# DESIGN: Data Guardian — Data Quality & Drift

**Status:** DRAFT
**Product:** Data Guardian
**Created:** 2026-03-16

## Overview

Proactive data quality monitoring and schema drift detection applets. These catch problems before downstream consumers notice — replacing manual monitoring and Monte Carlo-like tools at a fraction of the cost.

---

## Applets

### 1. AI Drift Monitor

**Type:** Periodic (scheduled, default every 6 hours)
**Migrated from:** `DESIGN-wasm-application-platform.md` + `BRAINSTORM-ekaya-engine-applications.md` (App #1 of suite)

AI-automated schema drift detection and impact analysis.

**5 change types detected:**
- New table
- Dropped table
- New column
- Dropped column
- Modified column

**Key capabilities:**
- Scheduled schema snapshots via `SchemaDiscoverer` interface (PostgreSQL + MSSQL)
- Change detection via `SchemaChangeDetectionService` patterns
- AI generates natural-language impact analysis: "The `orders.discount_code` column was dropped. This will break 3 approved queries that reference it. The column had 45,000 non-null values."
- AI classifies severity: breaking change vs. additive vs. cosmetic
- Stores historical timeline of all schema changes
- Alerts via webhook when breaking changes detected

**Existing code leveraged:**
- `pkg/adapters/datasource/postgres/schema.go` — Table/column/FK discovery
- `pkg/adapters/datasource/mssql/schema.go` — Same interfaces
- `pkg/services/schema_change_detection.go` — Change type detection
- `pkg/models/column_metadata.go` — Column classification context

**Host functions used:** `datasource_query`, `db_query`, `llm_generate`, `get_auth_context`, `log`

---

### 2. AI Data Quality Monitor

**Type:** Periodic (scheduled)
**Migrated from:** `DESIGN-wasm-application-platform.md` (App #2 of suite)

AI-automated data quality expectations and anomaly detection.

**Key capabilities:**
- Profiles data using aggregate queries (null rates, cardinality, distributions, freshness)
- AI auto-generates quality expectations from profiling: "This column has 0.3% nulls historically. Alert if null rate exceeds 2%."
- Scheduled checks against expectations
- AI explains anomalies when checks fail: "The `users.email` null rate jumped from 0.3% to 15.2% in the last 24 hours. This correlates with a new column `users.sso_id` appearing yesterday — likely a migration that made email optional for SSO users."
- Uses statistical methods (z-score, IQR) with LLM-generated explanations
- Stores expectations and check results in app-isolated schema

**Existing code leveraged:**
- `pkg/adapters/datasource/interfaces.go` — `QueryExecutor` for aggregate queries
- `pkg/services/column_feature_extraction.go` — Feature extraction patterns
- `pkg/adapters/datasource/postgres/schema.go` — `AnalyzeColumnStats()`, `GetDistinctValues()`

---

### 3. Data Quality Gate Advisor

**Type:** Periodic (at pipeline boundaries)
**Migrated from:** `DESIGN-app-etl-genius.md` Advisor #5

Validates data quality expectations after extraction, transformation, and loading.

**Checks:**
- **Row count validation** — Source vs. destination counts (configurable tolerance for filtered loads)
- **Null rate checks** — Flags unexpected NULL spikes in non-nullable business columns
- **Uniqueness validation** — Primary key and unique constraint integrity post-load
- **Referential integrity** — Foreign key relationships survive ETL process
- **Distribution drift** — Column value distribution shifts significantly between loads

Returns pass/fail with specific failing rows or values for debugging.

---

### 4. Data Freshness Advisor

**Type:** Periodic
**Migrated from:** `DESIGN-app-etl-genius.md` Advisor #3

Monitors how current the data is at each stage.

**Key capabilities:**
- Compares max timestamp columns between source and destination to calculate lag
- Identifies tables where last load time exceeds configured SLA thresholds
- Detects "stuck" pipelines — tables where destination hasn't been updated despite source changes
- Tracks freshness trends over time
- Returns freshness report with per-table lag, trend direction, and SLA compliance status

---

### 5. Schema Drift Detective

**Type:** Periodic
**Migrated from:** `BRAINSTORM-ekaya-engine-applications.md` (App #6)

Continuously monitors customer database schemas, detects additions, removals, type changes, and constraint modifications. Stores schema snapshots as JSONB, computes diffs, and alerts downstream consumers before pipelines break.

Overlaps with AI Drift Monitor — this brainstorm entry represents the standalone app concept that predates the AI-automated version.

---

## Dashboard Tile

```
Title: "Data Quality & Drift"
Description: "Schema drift detection, quality monitoring, and freshness tracking"
Stats: 3 schema changes (7d) | 2 quality alerts | All SLAs met
```

---

## Related Plans

- `plans/BRAINSTORM-ekaya-engine-applications.md` — Apps #1-4 of the AI Data Guardian Suite
