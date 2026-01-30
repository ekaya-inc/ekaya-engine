-- 016_sensitive_column.up.sql
-- Add is_sensitive flag to columns for manual override of automatic sensitive detection

-- Add is_sensitive to engine_schema_columns (where sample_values live)
-- NULL = use automatic detection, TRUE = always sensitive, FALSE = never sensitive
ALTER TABLE engine_schema_columns
    ADD COLUMN is_sensitive BOOLEAN DEFAULT NULL;

COMMENT ON COLUMN engine_schema_columns.is_sensitive IS 'Manual sensitive data override: NULL=auto-detect, TRUE=always sensitive, FALSE=never sensitive';

-- Add is_sensitive to engine_ontology_column_metadata (for provenance tracking)
ALTER TABLE engine_ontology_column_metadata
    ADD COLUMN is_sensitive BOOLEAN DEFAULT NULL;

COMMENT ON COLUMN engine_ontology_column_metadata.is_sensitive IS 'Manual sensitive data override: NULL=auto-detect, TRUE=always sensitive, FALSE=never sensitive';
