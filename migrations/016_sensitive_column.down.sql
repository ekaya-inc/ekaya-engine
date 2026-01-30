-- 016_sensitive_column.down.sql
-- Remove is_sensitive flag from columns

ALTER TABLE engine_schema_columns
    DROP COLUMN IF EXISTS is_sensitive;

ALTER TABLE engine_ontology_column_metadata
    DROP COLUMN IF EXISTS is_sensitive;
