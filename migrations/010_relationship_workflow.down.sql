-- Migration 010 rollback: Remove relationship detection workflow support

-- Drop indexes
DROP INDEX IF EXISTS idx_engine_ontology_workflows_datasource;

-- Drop columns
ALTER TABLE engine_ontology_workflows DROP COLUMN IF EXISTS datasource_id;
ALTER TABLE engine_ontology_workflows DROP COLUMN IF EXISTS phase;
