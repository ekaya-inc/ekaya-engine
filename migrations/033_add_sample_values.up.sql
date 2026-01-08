-- Add sample_values column to engine_schema_columns to persist distinct values
-- discovered during ontology extraction for low-cardinality columns (≤50 distinct values).
-- This enables probe_column and get_context tools to return sample values without
-- on-demand database queries.

ALTER TABLE engine_schema_columns
ADD COLUMN sample_values TEXT[];
