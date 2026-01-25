-- Remove audit fields from engine_query_executions

DROP INDEX IF EXISTS idx_query_executions_modifying;

ALTER TABLE engine_query_executions DROP COLUMN IF EXISTS error_message;
ALTER TABLE engine_query_executions DROP COLUMN IF EXISTS success;
ALTER TABLE engine_query_executions DROP COLUMN IF EXISTS rows_affected;
ALTER TABLE engine_query_executions DROP COLUMN IF EXISTS is_modifying;
