-- Add length statistics for text columns to support uniform-length detection
-- Uniform length text columns (min_length = max_length) are likely ID fields (UUIDs, etc.)

ALTER TABLE engine_schema_columns
    ADD COLUMN min_length INTEGER,
    ADD COLUMN max_length INTEGER;

COMMENT ON COLUMN engine_schema_columns.min_length IS 'Minimum string length for text columns (NULL for non-text)';
COMMENT ON COLUMN engine_schema_columns.max_length IS 'Maximum string length for text columns (NULL for non-text)';
