-- Migration 009 rollback: Remove is_unique and default_value from engine_schema_columns

ALTER TABLE engine_schema_columns
DROP COLUMN is_unique,
DROP COLUMN default_value;
