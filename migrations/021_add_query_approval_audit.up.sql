-- 020_add_query_approval_audit.up.sql
-- Add audit trail fields for query approval workflow

-- Add audit columns for tracking who reviewed and when
ALTER TABLE engine_queries
ADD COLUMN reviewed_by VARCHAR(255),
ADD COLUMN reviewed_at TIMESTAMPTZ,
ADD COLUMN rejection_reason TEXT,
ADD COLUMN parent_query_id UUID REFERENCES engine_queries(id);

COMMENT ON COLUMN engine_queries.reviewed_by IS 'User/admin who reviewed the pending query';
COMMENT ON COLUMN engine_queries.reviewed_at IS 'When the query was approved or rejected';
COMMENT ON COLUMN engine_queries.rejection_reason IS 'Explanation provided when query was rejected';
COMMENT ON COLUMN engine_queries.parent_query_id IS 'For update suggestions: references the original query being updated';

-- Partial index for efficient lookup of pending queries awaiting review
CREATE INDEX idx_queries_pending_review
ON engine_queries(project_id, datasource_id)
WHERE status = 'pending' AND deleted_at IS NULL;

-- Partial index for finding pending updates for a given parent query
CREATE INDEX idx_queries_parent
ON engine_queries(parent_query_id)
WHERE parent_query_id IS NOT NULL AND deleted_at IS NULL;
