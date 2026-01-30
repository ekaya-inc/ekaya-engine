-- 017_table_metadata.down.sql
-- Rollback table-level metadata

DROP POLICY IF EXISTS table_metadata_access ON engine_table_metadata;
DROP TRIGGER IF EXISTS update_engine_table_metadata_updated_at ON engine_table_metadata;
DROP TABLE IF EXISTS engine_table_metadata;
