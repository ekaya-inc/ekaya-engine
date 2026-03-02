-- 012_incremental_ontology.up.sql
-- Add incremental extraction tracking to ontology DAG table.
-- Allows distinguishing between full and incremental extractions
-- and storing a summary of what changed.

ALTER TABLE engine_ontology_dag ADD COLUMN is_incremental boolean NOT NULL DEFAULT false;
ALTER TABLE engine_ontology_dag ADD COLUMN change_summary jsonb;

COMMENT ON COLUMN engine_ontology_dag.is_incremental IS 'Whether this DAG run was an incremental extraction (true) or full extraction (false)';
COMMENT ON COLUMN engine_ontology_dag.change_summary IS 'Summary of schema changes that triggered this incremental extraction: {tables_added, tables_modified, tables_deleted, columns_added, columns_modified, columns_deleted}';
