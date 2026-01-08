-- Migration 034: Add query suggestion support
-- Adds fields for AI agents to suggest queries for human approval

-- Add status column (default 'approved' for backward compatibility)
ALTER TABLE engine_queries ADD COLUMN IF NOT EXISTS status VARCHAR NOT NULL DEFAULT 'approved';
COMMENT ON COLUMN engine_queries.status IS 'Query lifecycle: pending, approved, rejected';

-- Add suggested_by column to track origin
ALTER TABLE engine_queries ADD COLUMN IF NOT EXISTS suggested_by VARCHAR;
COMMENT ON COLUMN engine_queries.suggested_by IS 'Origin: user, agent, admin';

-- Add suggestion_context column for metadata
ALTER TABLE engine_queries ADD COLUMN IF NOT EXISTS suggestion_context JSONB;
COMMENT ON COLUMN engine_queries.suggestion_context IS 'Stores example usage, validation results, etc.';

-- Index for filtering by status
CREATE INDEX IF NOT EXISTS idx_engine_queries_status
    ON engine_queries(project_id, datasource_id, status)
    WHERE deleted_at IS NULL;
