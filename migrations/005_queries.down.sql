-- Migration 005 DOWN: Remove queries table

-- Drop trigger first
DROP TRIGGER IF EXISTS update_engine_queries_updated_at ON engine_queries;

-- Drop policy
DROP POLICY IF EXISTS queries_access ON engine_queries;

-- Drop table (indexes are dropped automatically with table)
DROP TABLE IF EXISTS engine_queries;

-- Note: Keep update_updated_at_column() - used by other tables
