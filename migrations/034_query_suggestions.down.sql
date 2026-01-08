-- Migration 034 rollback: Remove query suggestion support

DROP INDEX IF EXISTS idx_engine_queries_status;
ALTER TABLE engine_queries DROP COLUMN IF EXISTS suggestion_context;
ALTER TABLE engine_queries DROP COLUMN IF EXISTS suggested_by;
ALTER TABLE engine_queries DROP COLUMN IF EXISTS status;
