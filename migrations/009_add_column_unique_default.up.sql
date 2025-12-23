-- Migration 009: Add is_unique and default_value to engine_schema_columns
-- Required for MCP developer tools schema endpoint

ALTER TABLE engine_schema_columns
ADD COLUMN is_unique BOOLEAN NOT NULL DEFAULT false,
ADD COLUMN default_value TEXT;
