-- Add enum_values column to engine_schema_columns for Postgres enum type definitions.
-- Stores the ordered list of enum values discovered from pg_enum during schema discovery.
-- This allows the extraction pipeline to use definitive values instead of LLM guessing.
ALTER TABLE engine_schema_columns ADD COLUMN IF NOT EXISTS enum_values JSONB;
