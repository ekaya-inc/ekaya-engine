# PLAN: Consolidate Migrations for Open Source Release

**Status:** PLANNED
**Branch:** (new branch from main)
**Goal:** Replace 30 incremental migrations (with ALTER TABLEs, destructive refactors, and dead table creation/deletion) with clean, minimal migrations that create the final-state schema directly.

## Context

The current migrations directory has 30 files (001-030, gap at 019) that evolved organically during development. Many migrations create tables that later migrations drop entirely (entity tables), add columns that get removed, or do destructive drop-and-recreate refactors. For a public open-source release, new users should get a clean set of migrations that create the final schema in one pass without wasteful intermediate steps.

**Prerequisite:** Drop and recreate the database after applying. No data migration needed.

## Analysis: What Exists vs. What's Needed

### Tables Deleted by Later Migrations (completely remove)
- `engine_ontology_entities` (created in 005, dropped in 022)
- `engine_ontology_entity_aliases` (created in 005, dropped in 022)
- `engine_ontology_entity_key_columns` (created in 005, dropped in 022)
- `engine_entity_relationships` (created in 005, dropped in 022)
- `engine_table_metadata` (created in 017, dropped in 023 — replaced by `engine_ontology_table_metadata`)

### Columns Removed by Later Migrations
- `engine_ontologies.entity_summaries` (added in 005, dropped in 022)
- `engine_project_knowledge.ontology_id` (added in 007, dropped in 015)
- `engine_project_knowledge.key` (added in 007, dropped in 018)
- `engine_schema_columns.business_name`, `.description`, `.metadata`, `.sample_values` (created in 003, removed in 020)
- `engine_schema_tables.business_name`, `.description`, `.metadata` (created in 003, removed in 023)
- `engine_ontology_column_metadata.table_name`, `.column_name` (created in 012, replaced by `schema_column_id` FK in 021)

### Columns Added by Later Migrations (fold into original CREATE)
- `engine_mcp_config.audit_retention_days` (added in 026)
- `engine_mcp_config.alert_config` (added in 028)
- `engine_business_glossary.defining_sql` needs `DROP NOT NULL` (029 makes it optional)
- `engine_installed_apps.activated_at` (added in 030)

### Tables Dropped and Recreated (use final version only)
- `engine_schema_tables` — final schema from 023
- `engine_schema_columns` — final schema from 023
- `engine_schema_relationships` — final schema from 023
- `engine_ontology_column_metadata` — final schema from 023

### Tables Created Correctly (no changes needed, just re-number)
- `engine_projects` (001) — correct as-is
- `engine_users` (001) — correct as-is
- `engine_datasources` (002) — correct as-is
- `engine_queries` (004) — correct as-is
- `engine_query_executions` (004) — correct as-is
- `engine_ontology_dag` (006) — correct as-is
- `engine_dag_nodes` (006) — correct as-is
- `engine_ontology_chat_messages` (007) — correct as-is
- `engine_ontology_questions` (007) — correct as-is
- `engine_llm_conversations` (008) — correct as-is
- `engine_ontology_pending_changes` (011) — correct as-is
- `engine_audit_log` (014) — correct as-is
- `engine_mcp_audit_log` (024) — correct as-is
- `engine_mcp_query_history` (025) — correct as-is
- `engine_mcp_audit_alerts` (027) — correct as-is
- `engine_glossary_aliases` (010) — correct as-is

## New Migration Structure

Consolidate into **10 logical migration files** grouped by domain, each creating the final-state schema. Every table is created in its final form — no follow-up ALTER TABLEs.

### 001_foundation.up.sql
**Tables:** `engine_projects`, `engine_users`
**Source:** Current 001 (unchanged)
**Down:** Drop both + trigger function

### 002_datasources.up.sql
**Tables:** `engine_datasources`
**Source:** Current 002 (unchanged)
**Down:** Drop table

### 003_schema.up.sql
**Tables:** `engine_schema_tables`, `engine_schema_columns`, `engine_schema_relationships`
**Source:** Take final-state CREATE from 023 (the last destructive refactor that defined these tables)
**Changes from original 003:**
- `engine_schema_tables`: Remove `business_name`, `description`, `metadata` columns
- `engine_schema_columns`: Remove `business_name`, `description`, `metadata`, `sample_values`, `is_sensitive` columns
- `engine_schema_relationships`: Same as 023 version (no entity FK columns)
**Down:** Drop all three tables in reverse dependency order

### 004_queries.up.sql
**Tables:** `engine_queries`, `engine_query_executions`
**Source:** Current 004 (unchanged — it was already clean)
**Down:** Drop both tables

### 005_ontology.up.sql
**Tables:** `engine_ontologies`
**Source:** Current 005 but with modifications:
- Remove `entity_summaries` column (dropped in 022)
- Remove all entity-related tables (dropped in 022): `engine_ontology_entities`, `engine_ontology_entity_aliases`, `engine_ontology_entity_key_columns`, `engine_entity_relationships`
**Down:** Drop table

### 006_ontology_dag.up.sql
**Tables:** `engine_ontology_dag`, `engine_dag_nodes`
**Source:** Current 006 (unchanged)
**Down:** Drop both tables

### 007_ontology_support.up.sql
**Tables:** `engine_ontology_chat_messages`, `engine_ontology_questions`, `engine_project_knowledge`
**Source:** Current 007 but with modifications to `engine_project_knowledge`:
- Remove `ontology_id` column and its FK constraint (dropped in 015)
- Remove `key` column (dropped in 018)
- Use `value` as the main fact content column
- Use index `idx_engine_project_knowledge_project_type` from 018 instead of the original unique index
**Down:** Drop all three tables

### 008_ontology_metadata.up.sql
**Tables:** `engine_ontology_column_metadata`, `engine_ontology_table_metadata`
**Source:** Take final-state CREATE from 023 for both tables (which already has the refactored `schema_column_id` FK and `schema_table_id` FK)
**Note:** This replaces old 012 (original column_metadata), 017 (old table_metadata), 021 (column_metadata refactor), and part of 023 (table_metadata with new name)
**Down:** Drop both tables

### 009_llm_and_config.up.sql
**Tables:** `engine_llm_conversations`, `engine_mcp_config`, `engine_business_glossary`, `engine_glossary_aliases`, `engine_ontology_pending_changes`, `engine_installed_apps`, `engine_audit_log`
**Source:** Merge current 008, 009, 010, 011, 013, 014 with modifications:
- `engine_mcp_config`: Add `audit_retention_days INTEGER` and `alert_config JSONB` columns inline (from 026, 028)
- `engine_business_glossary`: Make `defining_sql` nullable (no NOT NULL) with DEFAULT '' (from 029)
- `engine_installed_apps`: Add `activated_at TIMESTAMPTZ` column (from 030)
**Down:** Drop all tables in reverse dependency order

### 010_mcp_audit.up.sql
**Tables:** `engine_mcp_audit_log`, `engine_mcp_query_history`, `engine_mcp_audit_alerts`
**Source:** Current 024, 025, 027 (unchanged — these were already clean)
**Down:** Drop all three tables

## Implementation Checklist

- [ ] Create new branch `ddanieli/consolidate-migrations`
- [ ] Delete all 60 existing migration files (30 up + 30 down)
- [ ] Write `001_foundation.up.sql` and `.down.sql` — copy from current 001
- [ ] Write `002_datasources.up.sql` and `.down.sql` — copy from current 002
- [ ] Write `003_schema.up.sql` and `.down.sql` — use final schema from 023
- [ ] Write `004_queries.up.sql` and `.down.sql` — copy from current 004
- [ ] Write `005_ontology.up.sql` and `.down.sql` — modified 005 (no entities, no entity_summaries)
- [ ] Write `006_ontology_dag.up.sql` and `.down.sql` — copy from current 006
- [ ] Write `007_ontology_support.up.sql` and `.down.sql` — modified 007 (no ontology_id/key on project_knowledge)
- [ ] Write `008_ontology_metadata.up.sql` and `.down.sql` — from final 023 (column_metadata + table_metadata)
- [ ] Write `009_llm_and_config.up.sql` and `.down.sql` — merge 008/009/010/011/013/014 with folded-in ALTER changes
- [ ] Write `010_mcp_audit.up.sql` and `.down.sql` — merge 024/025/027
- [ ] Verify `embed.go` still works (glob pattern `*.sql` unchanged)
- [ ] Run `make check` to confirm all tests pass with the new migration set
- [ ] Manually test: drop database, restart server, verify migrations apply cleanly

## Files Changed

### Deleted (60 files)
All files in `migrations/` except `embed.go`:
- `001_foundation.{up,down}.sql` through `030_app_activation.{up,down}.sql`

### Created (20 files)
- `migrations/001_foundation.{up,down}.sql`
- `migrations/002_datasources.{up,down}.sql`
- `migrations/003_schema.{up,down}.sql`
- `migrations/004_queries.{up,down}.sql`
- `migrations/005_ontology.{up,down}.sql`
- `migrations/006_ontology_dag.{up,down}.sql`
- `migrations/007_ontology_support.{up,down}.sql`
- `migrations/008_ontology_metadata.{up,down}.sql`
- `migrations/009_llm_and_config.{up,down}.sql`
- `migrations/010_mcp_audit.{up,down}.sql`

### Unchanged
- `migrations/embed.go` — no changes needed
- No Go code changes — all table names, column names, and constraints remain identical to the current final state

## Risks & Mitigations

1. **Missing column/constraint:** The final-state SQL must exactly match what the current 30 migrations produce. Mitigation: compare `pg_dump` output before and after.
2. **Index name collisions:** Some indexes were renamed across migrations. Mitigation: use the index names from the latest migration that defined each table.
3. **Comment drift:** COMMENTs on columns may reference outdated context. Mitigation: review all COMMENTs during consolidation.
4. **Test database:** Integration tests use the same migration runner. Mitigation: `make check` validates this.
