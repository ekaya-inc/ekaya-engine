-- 020_add_query_approval_audit.down.sql
-- Remove audit trail fields for query approval workflow

DROP INDEX IF EXISTS idx_queries_parent;
DROP INDEX IF EXISTS idx_queries_pending_review;

ALTER TABLE engine_queries
DROP COLUMN IF EXISTS parent_query_id,
DROP COLUMN IF EXISTS rejection_reason,
DROP COLUMN IF EXISTS reviewed_at,
DROP COLUMN IF EXISTS reviewed_by;
