-- Rollback engine_datasources table

DROP TRIGGER IF EXISTS update_engine_datasources_updated_at ON engine_datasources;
DROP POLICY IF EXISTS datasource_access ON engine_datasources;
DROP TABLE IF EXISTS engine_datasources;

-- Note: Keep update_updated_at_column() - may be used by other tables
