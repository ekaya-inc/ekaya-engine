-- Add audit fields to engine_query_executions for enhanced modifying query logging
-- These fields capture success/failure status, modification type, and rows affected

-- Add is_modifying to indicate if the query modifies data
ALTER TABLE engine_query_executions
ADD COLUMN is_modifying BOOLEAN DEFAULT FALSE NOT NULL;

-- Add rows_affected for modifying queries (separate from row_count which is for returned rows)
ALTER TABLE engine_query_executions
ADD COLUMN rows_affected BIGINT DEFAULT NULL;

-- Add success indicator for audit trail
ALTER TABLE engine_query_executions
ADD COLUMN success BOOLEAN DEFAULT TRUE NOT NULL;

-- Add error_message for failed executions
ALTER TABLE engine_query_executions
ADD COLUMN error_message TEXT;

-- Add comments for documentation
COMMENT ON COLUMN engine_query_executions.is_modifying IS
    'True if this was a data-modifying query (INSERT/UPDATE/DELETE/CALL)';

COMMENT ON COLUMN engine_query_executions.rows_affected IS
    'Number of rows affected by modifying queries (from database command tag, not RETURNING clause)';

COMMENT ON COLUMN engine_query_executions.success IS
    'True if query executed successfully, false if it failed';

COMMENT ON COLUMN engine_query_executions.error_message IS
    'Error message if the query execution failed';

-- Create index for filtering by modification type (useful for security audits)
CREATE INDEX idx_query_executions_modifying ON engine_query_executions USING btree (project_id, is_modifying, executed_at DESC)
    WHERE is_modifying = TRUE;
