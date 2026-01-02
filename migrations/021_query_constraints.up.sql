-- Migration 021: Add constraints field to queries
-- Structured description of limitations and assumptions of the query for better MCP client matching

ALTER TABLE engine_queries
    ADD COLUMN constraints TEXT;

COMMENT ON COLUMN engine_queries.constraints IS
    'Explicit limitations and assumptions of the query (e.g., "Only includes completed orders", "Excludes refunded amounts")';
