-- Migration 020: Add output_columns field to queries
-- Structured description of columns returned by the query for better MCP client matching

ALTER TABLE engine_queries
    ADD COLUMN output_columns JSONB NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN engine_queries.output_columns IS
    'Array of output column metadata (name, type, description) describing what data is returned by the query';

-- Add index for queries that have output columns defined
CREATE INDEX idx_engine_queries_has_output_columns
    ON engine_queries ((output_columns <> '[]'::jsonb))
    WHERE deleted_at IS NULL;
