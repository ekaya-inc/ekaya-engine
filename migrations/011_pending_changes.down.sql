-- 011_pending_changes.down.sql
-- Rollback pending ontology changes table

DROP TABLE IF EXISTS engine_ontology_pending_changes CASCADE;
