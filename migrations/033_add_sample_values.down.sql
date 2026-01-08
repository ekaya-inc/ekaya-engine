-- Rollback: Remove sample_values column from engine_schema_columns

ALTER TABLE engine_schema_columns
DROP COLUMN IF EXISTS sample_values;
