-- Migration 035 rollback: Remove tags support

DROP INDEX IF EXISTS idx_engine_queries_tags;
ALTER TABLE engine_queries DROP COLUMN IF EXISTS tags;
