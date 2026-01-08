-- Migration 035: Add tags/categories support for queries
-- Allows organizing queries by business domain, use case, or custom categories

-- Add tags column (TEXT array for flexible tagging)
ALTER TABLE engine_queries ADD COLUMN IF NOT EXISTS tags TEXT[] DEFAULT '{}';
COMMENT ON COLUMN engine_queries.tags IS 'Tags for organizing queries (e.g., "billing", "reporting", "category:analytics")';

-- Index for tag-based queries using GIN for array containment
CREATE INDEX IF NOT EXISTS idx_engine_queries_tags
    ON engine_queries USING GIN (tags)
    WHERE deleted_at IS NULL;
