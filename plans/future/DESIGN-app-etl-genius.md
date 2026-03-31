# DESIGN: ETL Genius App

**Status:** Draft
**App ID:** `etl-genius`
**Category:** Data Engineering Productivity
**Persona:** Admins and Data Engineers
**Date:** 2026-03-02

---

## Overview

ETL Genius is an Ekaya Engine app optimized for building, maintaining, and optimizing ETL pipelines. It gives Data Engineers working inside Claude Code (or other MCP-connected environments) a set of specialized MCP tools and **Advisors** — proactive analysis capabilities that surface problems, optimization opportunities, and best practices during pipeline development.

The core insight: Data Engineers already use Claude Code to write pipelines in Python, dbt, SQL, and other tools. ETL Genius makes the AI agent aware of the full pipeline context — source schemas, destination schemas, transformation logic, performance characteristics, and operational history — so it can actively assist rather than passively respond.

---

## What Makes ETL Genius Different from Base MCP Tools

The base `mcp-server` app provides general-purpose schema introspection, querying, and ontology tools. ETL Genius adds:

1. **Pipeline-Aware Context** — Tools that understand source-to-destination mappings, not just individual schemas
2. **Advisors** — Proactive analysis that runs on demand or continuously, surfacing issues before they become incidents
3. **Cross-Datasource Operations** — Compare schemas, data types, row counts, and freshness across source and destination simultaneously
4. **ETL-Specific Optimization** — Index recommendations, partitioning strategies, incremental load patterns, and CDC configuration tuned for pipeline workloads

---

## Advisors

Advisors are a new concept — proactive analysis capabilities that an ETL Engineer can invoke (or that run automatically) to get actionable recommendations. Each Advisor is an MCP tool that analyzes the current state and returns structured findings with severity, explanation, and suggested fixes.

### 1. Missing Index Advisor

Analyzes query patterns and table access on destination tables to identify missing indexes that would improve pipeline load performance and downstream query speed.

- Scans `pg_stat_user_tables` and `pg_stat_user_indexes` for sequential scan patterns on large tables
- Identifies foreign key columns without indexes (common in ETL-created schemas)
- Detects columns used in WHERE/JOIN clauses of approved queries that lack indexes
- Returns specific `CREATE INDEX` statements with estimated impact
- Warns about over-indexing (too many indexes slowing down INSERT-heavy ETL loads)

### 2. Source-Destination Mismatch Advisor

Compares source and destination schemas to detect drift, type mismatches, and missing columns that will cause pipeline failures.

- Column-level comparison: name matching, data type compatibility (e.g., `VARCHAR(50)` source → `VARCHAR(30)` destination = truncation risk)
- Nullable mismatches: source allows NULLs but destination has NOT NULL constraint
- Missing columns: source has new columns not yet mapped to destination
- Dropped columns: destination references columns removed from source
- Encoding/collation mismatches across databases
- Returns severity-ranked findings with suggested ALTER TABLE or pipeline code fixes

### 3. Data Freshness Advisor

Monitors how current the data is at each stage of the pipeline.

- Compares max timestamp columns between source and destination to calculate lag
- Identifies tables where last load time exceeds configured SLA thresholds
- Detects "stuck" pipelines — tables where destination hasn't been updated despite source changes
- Tracks freshness trends over time to detect degrading pipeline performance
- Returns freshness report with per-table lag, trend direction, and SLA compliance status

### 4. Pipeline Performance Advisor

Analyzes ETL load patterns and suggests optimizations.

- Identifies tables that should use incremental loads instead of full refreshes (based on row count, update patterns, and available timestamp/CDC columns)
- Detects partition candidates — large tables with time-based access patterns
- Recommends batch sizes based on table size and available memory
- Identifies parallel load opportunities — tables with no dependency relationships that could load concurrently
- Flags slow transformations by analyzing query execution plans of pipeline SQL
- Suggests materialized views for expensive, frequently-used aggregations

### 5. Data Quality Gate Advisor

Validates data quality expectations at pipeline boundaries — after extraction, after transformation, and after loading.

- Row count validation: source vs. destination counts (with configurable tolerance for filtered loads)
- Null rate checks: flags unexpected NULL spikes in non-nullable business columns
- Uniqueness validation: confirms primary key and unique constraint integrity post-load
- Referential integrity: validates foreign key relationships survive the ETL process
- Distribution drift: detects when a column's value distribution shifts significantly between loads (e.g., a status column suddenly 80% NULL)
- Returns pass/fail results with specific failing rows or values for debugging

### 6. Cost Advisor

Helps Data Engineers understand and reduce the operational cost of their pipelines.

- Estimates compute cost per pipeline run based on query complexity, data volume, and execution time
- Identifies redundant transformations — multiple pipelines computing the same aggregation
- Flags unused destination tables — tables being loaded but never queried downstream
- Recommends scheduling optimizations — pipelines that could run less frequently without impacting freshness SLAs
- Detects expensive full-table scans that could be replaced with indexed lookups

---

## MCP Tools (Beyond Advisors)

### Pipeline Development Tools

**`etl_compare_schemas`** — Side-by-side schema comparison between any two datasources (or schemas within the same datasource). Returns column-level diff: added, removed, type changed, constraint changed. Essential for designing new pipelines and validating migrations.

**`etl_generate_mapping`** — Given source and destination tables, generates a column mapping with automatic type casting suggestions. Handles common ETL patterns: timestamp timezone conversion, string-to-enum mapping, JSON flattening, and name normalization.

**`etl_validate_sql`** — Validates transformation SQL against both source and destination schemas without executing. Catches column reference errors, type mismatches, and ambiguous joins before the pipeline runs.

**`etl_dry_run`** — Executes the pipeline SQL against a small sample (configurable row limit) and returns the transformation result, execution plan, and estimated full-run duration. Lets engineers validate logic without processing the full dataset.

### Pipeline Operations Tools

**`etl_pipeline_status`** — Returns the operational status of all tracked pipelines: last run time, duration, row counts, success/failure, and next scheduled run.

**`etl_load_history`** — Time-series history of pipeline runs for a specific table or pipeline. Shows trends in duration, row count, error rate, and resource consumption.

**`etl_replay_load`** — Re-runs a specific historical load for a date range. Useful for backfilling after a bug fix or schema change.

**`etl_dependency_graph`** — Returns the DAG of table dependencies based on foreign keys, approved queries, and pipeline definitions. Visualizes which tables must load before others and identifies circular dependencies.

### Source Analysis Tools

**`etl_profile_source`** — Deep profiling of a source table: row count, column statistics (min/max/avg/distinct/null rate), sample values, data type distribution, and estimated size. Provides everything an engineer needs to design the extraction and transformation.

**`etl_detect_cdc_columns`** — Identifies columns suitable for change data capture: `updated_at` timestamps, auto-incrementing IDs, version numbers, and soft-delete flags. Recommends the optimal CDC strategy for each table.

**`etl_estimate_volume`** — Estimates the data volume for an initial load and ongoing incremental loads based on row counts, column widths, and growth rate analysis. Helps engineers plan infrastructure and set expectations.

---

## Brainstorm: What Else Could ETL Genius Do?

The following ideas explore how ETL Genius could expand beyond the initial Advisors and tools to become the comprehensive platform for the Data Engineer persona.

### Pipeline as Code Generation

- **Scaffold entire pipelines** — Given a source schema and desired destination format, generate complete pipeline code in the engineer's framework of choice (dbt models, Python/Airflow DAGs, SQL scripts, Dagster assets)
- **Generate dbt models from ontology** — Use Ekaya's semantic ontology (entity relationships, business rules, column descriptions) to generate dbt staging models, intermediate models, and marts with documentation already written
- **Reverse-engineer existing pipelines** — Point at a set of SQL scripts or dbt project and extract the implicit source-to-destination mappings, dependencies, and transformation logic into a structured representation

### Testing and Validation

- **Auto-generate pipeline tests** — From the column metadata and ontology, generate dbt tests (`not_null`, `unique`, `accepted_values`, `relationships`), Great Expectations suites, or raw SQL assertions
- **Regression testing** — Compare output of a modified pipeline against the previous version's output, highlighting rows that changed and whether changes are expected
- **Contract testing** — Define and enforce data contracts between pipeline stages — if an upstream producer changes their schema, the consumer gets an immediate alert with the specific contract violation

### Documentation and Lineage

- **Auto-document pipelines** — Generate data lineage documentation from pipeline code: which source columns feed which destination columns, through what transformations. Keep documentation synchronized with actual pipeline code (not a separate artifact that goes stale)
- **Impact analysis** — "If I change this source column, what downstream tables, reports, and dashboards are affected?" Uses the dependency graph plus approved query analysis to trace impact through the entire data stack
- **Pipeline changelog** — Track pipeline code changes (dbt model edits, SQL script modifications) alongside schema changes and data quality results. Provides a unified timeline of "what changed and when" for debugging pipeline incidents

### Operational Intelligence

- **Anomaly detection on pipeline metrics** — Learn normal patterns for pipeline duration, row counts, and error rates. Alert when a pipeline deviates significantly from its baseline — before downstream consumers notice stale data
- **Smart scheduling** — Analyze pipeline dependencies, SLA requirements, and resource usage patterns to suggest optimal scheduling. Detect scheduling conflicts (two heavy pipelines running simultaneously and competing for resources)
- **Failure prediction** — Based on trends in execution time, data volume growth, and resource utilization, predict when a pipeline will start failing (e.g., "At current growth rate, this table will exceed the 30-minute load window in 6 weeks")
- **Incident correlation** — When a pipeline fails, automatically check: Did the source schema change? Did data volume spike? Did a dependent pipeline fail first? Did the database run out of connections? Surface the probable root cause, not just the error message

### Multi-Environment Management

- **Environment promotion** — Track pipeline definitions across dev/staging/prod environments. Validate that a pipeline change works in staging before promoting to production
- **Environment drift detection** — Compare pipeline definitions and configurations across environments to ensure they haven't diverged
- **Seed data management** — Generate and maintain realistic test data for development environments that mirrors production schemas and data distributions without exposing real data

### Collaboration Features

- **Pipeline review assistant** — When a Data Engineer submits a pipeline change for review, analyze the change and generate a review summary: what changed, what's the blast radius, what tests cover the change, what risks exist
- **Runbook generation** — From pipeline operational history and incident patterns, generate runbooks for common failure scenarios: "When pipeline X fails with error Y, here are the steps that resolved it in the past"
- **Knowledge capture** — When an engineer debugs a tricky pipeline issue, capture the diagnosis and solution as a reusable pattern. Next time a similar issue occurs, surface the previous solution

### Data Transformation Patterns

- **Slowly Changing Dimension (SCD) wizard** — Detect dimension tables and recommend SCD type (1, 2, or 3) based on column characteristics and business requirements. Generate the merge/upsert SQL for the chosen strategy
- **Deduplication assistant** — Analyze source data for duplicate patterns, recommend deduplication strategies (exact match, fuzzy match, window-based), and generate the deduplication SQL with configurable match rules
- **JSON flattening advisor** — For JSONB source columns, analyze the actual JSON structure across rows, recommend a flattening strategy, and generate the extraction SQL. Handle nested objects, arrays, and schema-on-read variability
- **Type coercion library** — Common cross-database type mappings (MSSQL → PostgreSQL, MySQL → PostgreSQL) with edge case handling: timezone-naive datetimes, precision loss in numeric conversions, character encoding issues

### Security and Compliance for Pipelines

**Migrated to:** `plans/guardian/DESIGN-guardian-data-protection.md` — PII Flow Tracking, Data Masking Advisor
**Migrated to:** `plans/guardian/DESIGN-guardian-audit-visibility.md` — Pipeline Access Audit

---

## Architecture Notes

### App Gating

Following the existing pattern (`ai-data-liaison` gates `DataLiaisonTools`), ETL Genius tools would be gated by the `etl-genius` app installation. In `access.go`, a new check would verify `etl-genius` is installed before allowing access to any tool in the `ETLGeniusTools` set.

### Advisor Implementation

Advisors are MCP tools that return structured JSON results. Each Advisor:
- Accepts optional configuration parameters (thresholds, scope filters, severity levels)
- Queries schema metadata, statistics, and operational data
- Applies analysis logic (statistical comparison, heuristic rules, pattern matching)
- Returns a list of findings, each with: severity (critical/warning/info), description, affected objects, and suggested fix (SQL or configuration change)

Advisors could optionally use LLM (`llm_generate`) for natural-language explanations and contextual recommendations, but the core analysis logic should be deterministic and reliable without LLM dependency.

### Multi-Datasource Support

ETL Genius inherently works across datasources — comparing a source (e.g., MSSQL production database) against a destination (e.g., PostgreSQL warehouse). Tools like `etl_compare_schemas` need access to multiple connected datasources within the same project. This leverages the existing `QueryExecutor` interface that supports PostgreSQL and MSSQL adapters.

---

## Relationship to Other Apps

| App | Relationship |
|-----|-------------|
| **AI Data Guardian** (App #11) | Data Guardian monitors quality in destination tables. ETL Genius monitors quality *during* the pipeline process. They complement each other — Guardian catches issues at rest, Genius catches issues in motion. |
| **AI Drift Monitor** (App #6) | Drift Monitor detects schema changes. ETL Genius's Source-Destination Mismatch Advisor acts on those changes by evaluating pipeline impact. Drift Monitor could trigger ETL Genius advisors automatically. |
| **Smart Ingestion** (App #27) | Smart Ingestion handles OAuth-based third-party data connectors (inbound). ETL Genius optimizes the transformation and loading that happens after ingestion. They form a natural pipeline: Smart Ingestion → ETL Genius → destination. |
| **AI Data Liaison** | Data Liaison helps business users query data. ETL Genius helps engineers build the pipelines that produce that data. The ontology and approved queries created by Data Liaison inform ETL Genius about downstream usage patterns. |
