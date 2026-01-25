-- Add allows_modification flag to engine_queries
-- Default false to preserve existing behavior (SELECT-only)
ALTER TABLE engine_queries
ADD COLUMN allows_modification BOOLEAN NOT NULL DEFAULT FALSE;

-- Add comment for documentation
COMMENT ON COLUMN engine_queries.allows_modification IS
    'When true, this query can execute INSERT/UPDATE/DELETE/CALL statements. When false, only SELECT is allowed.';
