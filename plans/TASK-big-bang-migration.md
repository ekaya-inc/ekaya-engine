# TASK: Big Bang Migration - Collapse to Clean Schema

## Goal

Collapse 27 migrations into a clean set where each table is fully defined in its original CREATE TABLE statement with zero ALTER TABLE commands (except for `ENABLE ROW LEVEL SECURITY`).

## Prerequisites

- No data to preserve
- No other developers using the database
- Will drop and recreate database after changes

## Current State Analysis

### Migrations that CREATE tables (keep, but modify):
| Migration | Tables Created |
|-----------|----------------|
| 001 | engine_projects, engine_users |
| 002 | engine_datasources |
| 003 | engine_schema_tables, engine_schema_columns, engine_schema_relationships |
| 004 | engine_queries, engine_query_executions |
| 005 | engine_ontologies, engine_ontology_entities, engine_ontology_entity_aliases, engine_ontology_entity_key_columns, engine_entity_relationships |
| 006 | engine_ontology_dag, engine_dag_nodes |
| 007 | engine_ontology_chat_messages, engine_ontology_questions, engine_project_knowledge |
| 008 | engine_llm_conversations |
| 009 | engine_mcp_config |
| 010 | engine_business_glossary, engine_glossary_aliases |
| 011 | engine_ontology_pending_changes |
| 012 | engine_ontology_column_metadata |
| 018 | engine_installed_apps |
| 026 | engine_audit_log |

### Migrations that only ALTER tables (delete after folding):
| Migration | What it does | Fold into |
|-----------|--------------|-----------|
| 013 | Rename provenance values (no-op now) | DELETE |
| 014 | Add `industry_type` to projects | 001 |
| 015 | Add `ontology_id` FK to project_knowledge | 007 |
| 016 | Add `ontology_id` FK to glossary, change unique constraint | 010 |
| 017 | Add `enrichment_status`, `enrichment_error` to glossary | 010 |
| 019 | Add `allows_modification` to queries | 004 |
| 020 | Add audit columns to query_executions | 004 |
| 021 | Add review columns to queries | 004 |
| 022 | Add `email` to users | 001 |
| 023 | Add `confidence`, `is_stale`, `domain` to entities/relationships | 005 |
| 024 | Add `content_hash` to questions | 007 |
| 025 | Add `source_column_id`, `target_column_id` to relationships | 005 |
| 027 | Add composite FKs, project_id to relationships | 005, 007, 010 |

## Implementation Plan

### Step 1: Fold columns into base migrations

#### 001_foundation.up.sql
Add to `engine_projects`:
```sql
industry_type text DEFAULT 'general'
```

Add to `engine_users`:
```sql
email VARCHAR(255)
```

#### 004_queries.up.sql
Add to `engine_queries`:
```sql
allows_modification BOOLEAN NOT NULL DEFAULT FALSE,
reviewed_by VARCHAR(255),
reviewed_at TIMESTAMPTZ,
rejection_reason TEXT,
parent_query_id UUID REFERENCES engine_queries(id)
```

Add to `engine_query_executions`:
```sql
is_modifying BOOLEAN DEFAULT FALSE NOT NULL,
rows_affected BIGINT DEFAULT NULL,
success BOOLEAN DEFAULT TRUE NOT NULL,
error_message TEXT
```

#### 005_ontology_core.up.sql
Add to `engine_ontology_entities`:
```sql
domain character varying(100),
confidence numeric(3,2) DEFAULT 1.0,
is_stale boolean DEFAULT false NOT NULL
```

Add to `engine_entity_relationships`:
```sql
project_id uuid REFERENCES engine_projects(id) ON DELETE CASCADE,
source_column_id uuid REFERENCES engine_schema_columns(id),
target_column_id uuid REFERENCES engine_schema_columns(id),
is_stale boolean DEFAULT false NOT NULL
```

Add composite FK constraints:
```sql
CONSTRAINT engine_ontology_entities_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
CONSTRAINT engine_ontology_entities_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id),
CONSTRAINT engine_entity_relationships_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
CONSTRAINT engine_entity_relationships_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
```

#### 007_ontology_support.up.sql
Add to `engine_ontology_questions`:
```sql
content_hash text
```

Add unique index:
```sql
CREATE UNIQUE INDEX idx_engine_ontology_questions_hash
    ON engine_ontology_questions(ontology_id, content_hash)
    WHERE status = 'pending';
```

Add to `engine_project_knowledge`:
```sql
ontology_id uuid REFERENCES engine_ontologies(id) ON DELETE CASCADE
```

Add composite FK constraints:
```sql
CONSTRAINT engine_project_knowledge_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
CONSTRAINT engine_project_knowledge_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
```

#### 010_business_glossary.up.sql
Add to `engine_business_glossary`:
```sql
ontology_id uuid REFERENCES engine_ontologies(id) ON DELETE CASCADE,
enrichment_status text DEFAULT 'pending'::text,
enrichment_error text,
needs_review boolean DEFAULT false,
review_reason text
```

Change unique constraint from `(project_id, term)` to `(ontology_id, term)`.

Add composite FK constraints:
```sql
CONSTRAINT engine_business_glossary_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
CONSTRAINT engine_business_glossary_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
```

#### 012_ontology_provenance.up.sql
Add composite FK constraints:
```sql
CONSTRAINT engine_column_metadata_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
CONSTRAINT engine_column_metadata_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
```

### Step 2: Delete obsolete migrations

Delete these migration files entirely (both .up.sql and .down.sql):
- 013_rename_provenance_values
- 014_project_industry_type
- 015_knowledge_ontology_fk
- 016_glossary_ontology_fk
- 017_glossary_enrichment_status
- 019_query_allows_modification
- 020_query_execution_audit
- 021_add_query_approval_audit
- 022_add_user_email
- 023_ontology_refresh_fields
- 024_question_content_hash
- 025_relationship_column_ids
- 027_provenance_rename

### Step 3: Renumber remaining migrations

After deletion, renumber to be consecutive:
```
001_foundation.up.sql          (unchanged)
002_datasources.up.sql         (unchanged)
003_schema.up.sql              (unchanged)
004_queries.up.sql             (unchanged)
005_ontology_core.up.sql       (unchanged)
006_ontology_dag.up.sql        (unchanged)
007_ontology_support.up.sql    (unchanged)
008_llm_conversations.up.sql   (unchanged)
009_mcp_config.up.sql          (unchanged)
010_business_glossary.up.sql   (unchanged)
011_pending_changes.up.sql     (unchanged)
012_ontology_provenance.up.sql (unchanged)
013_installed_apps.up.sql      (was 018)
014_audit_log.up.sql           (was 026)
```

### Step 4: Update down migrations

Each `.down.sql` file should be the exact inverse of its `.up.sql`:
- DROP tables in reverse order of creation
- No ALTER TABLE statements needed

### Step 5: Reset database

```bash
# Drop and recreate database
psql -c "DROP DATABASE IF EXISTS ekaya_engine;"
psql -c "CREATE DATABASE ekaya_engine;"

# Run migrations
make migrate-up

# Verify
psql -c "\dt engine_*"
```

### Step 6: Verify Go code compatibility

Run full test suite to ensure schema matches Go models:
```bash
make check
```

## Validation Checklist

- [x] All base migrations have zero ALTER TABLE (except ENABLE RLS)
- [x] All provenance columns use 'inferred' (not 'inference')
- [x] All composite FK constraints present in base migrations
- [x] All down migrations are clean DROP TABLE statements
- [x] Migration numbers are consecutive (001-014)
- [x] `make check` passes
- [ ] Manual smoke test of ontology extraction workflow

## Status: COMPLETE

Completed 2025-01-27. All migrations consolidated, tests passing. Awaiting manual smoke test after database DROP/CREATE.

## Files to Modify

1. `migrations/001_foundation.up.sql` - Add industry_type, email
2. `migrations/004_queries.up.sql` - Add all query/execution columns
3. `migrations/005_ontology_core.up.sql` - Add entity/relationship columns + FKs
4. `migrations/007_ontology_support.up.sql` - Add question hash, knowledge ontology_id + FKs
5. `migrations/010_business_glossary.up.sql` - Add ontology_id, enrichment columns + FKs
6. `migrations/012_ontology_provenance.up.sql` - Add composite FKs
7. `migrations/018_installed_apps.up.sql` - Rename to 013
8. `migrations/026_audit_log.up.sql` - Rename to 014

## Files to Delete

- migrations/013_rename_provenance_values.{up,down}.sql
- migrations/014_project_industry_type.{up,down}.sql
- migrations/015_knowledge_ontology_fk.{up,down}.sql
- migrations/016_glossary_ontology_fk.{up,down}.sql
- migrations/017_glossary_enrichment_status.{up,down}.sql
- migrations/019_query_allows_modification.{up,down}.sql
- migrations/020_query_execution_audit.{up,down}.sql
- migrations/021_add_query_approval_audit.{up,down}.sql
- migrations/022_add_user_email.{up,down}.sql
- migrations/023_ontology_refresh_fields.{up,down}.sql
- migrations/024_question_content_hash.{up,down}.sql
- migrations/025_relationship_column_ids.{up,down}.sql
- migrations/027_provenance_rename.{up,down}.sql
