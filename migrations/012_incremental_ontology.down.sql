-- 012_incremental_ontology.down.sql
-- Revert incremental extraction tracking from ontology DAG table.

ALTER TABLE engine_ontology_dag DROP COLUMN IF EXISTS change_summary;
ALTER TABLE engine_ontology_dag DROP COLUMN IF EXISTS is_incremental;
