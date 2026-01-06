# Plan: Big Bang Migration Reset

## Context

We have 30 incremental migrations that have accumulated column additions, renames, dropped tables, and constraint fixes. Since we're pre-launch with no production data, we can consolidate these into clean, logical migration files that represent the current final state of each table.

## Tables to Create (Final State)

| Table | Purpose | RLS |
|-------|---------|-----|
| `engine_projects` | Core projects | No |
| `engine_users` | Project access control | No |
| `engine_datasources` | External data connections | Yes |
| `engine_schema_tables` | Discovered tables | Yes |
| `engine_schema_columns` | Discovered columns with stats | Yes |
| `engine_schema_relationships` | FK/inferred column relationships | Yes |
| `engine_queries` | Saved SQL queries with parameters | Yes |
| `engine_ontologies` | Tiered ontology storage | Yes |
| `engine_ontology_entities` | Domain entities | Yes |
| `engine_ontology_entity_aliases` | Entity alternative names | Yes |
| `engine_ontology_entity_key_columns` | Important business columns | Yes |
| `engine_entity_relationships` | Entity-to-entity relationships | Yes |
| `engine_ontology_chat_messages` | Refinement chat history | Yes |
| `engine_ontology_questions` | Questions for clarification | Yes |
| `engine_project_knowledge` | Project-level facts | Yes |
| `engine_ontology_dag` | DAG execution state | Yes |
| `engine_dag_nodes` | Per-node DAG state | Yes |
| `engine_llm_conversations` | LLM request/response logs | Yes |
| `engine_mcp_config` | MCP server configuration | Yes |
| `engine_business_glossary` | Metric definitions | Yes |

## Migration Grouping

### 001_foundation.up.sql
- `engine_projects` table
- `engine_users` table
- `update_updated_at_column()` trigger function
- Indexes for both tables

### 002_datasources.up.sql
- `engine_datasources` table (with encrypted config)
- RLS policy
- Trigger

### 003_schema.up.sql
- `engine_schema_tables` table
- `engine_schema_columns` table (includes is_unique, default_value, min_length, max_length)
- `engine_schema_relationships` table
- All RLS policies and triggers
- Partial unique indexes for soft delete

### 004_queries.up.sql
- `engine_queries` table (includes parameters, output_columns, constraints)
- RLS policy
- Trigger
- Parameter indexes

### 005_ontology_core.up.sql
- `engine_ontologies` table
- `engine_ontology_entities` table (includes domain, is_deleted, deletion_reason)
- `engine_ontology_entity_aliases` table
- `engine_ontology_entity_key_columns` table
- `engine_entity_relationships` table (includes description, association, full unique constraint)
- All RLS policies and triggers
- Comments

### 006_ontology_dag.up.sql
- `engine_ontology_dag` table
- `engine_dag_nodes` table
- All constraints, indexes, RLS policies
- Comments

### 007_ontology_support.up.sql
- `engine_ontology_chat_messages` table
- `engine_ontology_questions` table
- `engine_project_knowledge` table
- All RLS policies and triggers
- Priority and status constraints

### 008_llm_conversations.up.sql
- `engine_llm_conversations` table
- All indexes (including GIN on context)
- RLS policy
- Comments

### 009_mcp_config.up.sql
- `engine_mcp_config` table (includes agent_api_key_encrypted)
- RLS policy
- Trigger

### 010_business_glossary.up.sql
- `engine_business_glossary` table
- RLS policy (WITH CHECK clause)
- Trigger
- Comments

## Steps to Execute

1. Delete all files in `migrations/` directory
2. Create the 10 new migration files (up and down)
3. Drop and recreate the local database
4. Run `make migrate-up`
5. Verify with `make check`

## Notes

- Down migrations should drop in reverse dependency order
- All RLS policies use the same pattern: `current_setting('app.current_project_id', true)`
- All tables except `engine_projects` and `engine_users` get RLS
- Trigger function `update_updated_at_column()` is created once in 001 and reused
- Comments preserved for semantic columns and tables
