-- Migration 038: Create query executions history table
-- Tracks executed queries to enable get_query_history tool for AI agent context

CREATE TABLE engine_query_executions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    query_id UUID NOT NULL REFERENCES engine_queries(id) ON DELETE CASCADE,
    sql TEXT NOT NULL,  -- Resolved SQL with parameters substituted
    executed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    row_count INTEGER NOT NULL,
    execution_time_ms INTEGER NOT NULL,
    parameters JSONB,  -- Parameter values used for this execution
    user_id TEXT,      -- User ID from JWT claims (if available)
    source TEXT NOT NULL DEFAULT 'mcp',  -- 'mcp', 'api', 'ui'
    CONSTRAINT valid_execution_time CHECK (execution_time_ms >= 0),
    CONSTRAINT valid_row_count CHECK (row_count >= 0)
);

-- Index for get_query_history: recent queries by project
CREATE INDEX idx_query_executions_project_time
    ON engine_query_executions(project_id, executed_at DESC);

-- Index for query-specific history
CREATE INDEX idx_query_executions_query
    ON engine_query_executions(query_id, executed_at DESC);

-- RLS policy for tenant isolation
ALTER TABLE engine_query_executions ENABLE ROW LEVEL SECURITY;

CREATE POLICY query_executions_access ON engine_query_executions
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Comments for documentation
COMMENT ON TABLE engine_query_executions IS 'Execution history for approved queries. Used by get_query_history MCP tool to provide AI agents with context about recent queries.';
COMMENT ON COLUMN engine_query_executions.sql IS 'Resolved SQL with actual parameter values (for debugging/reference)';
COMMENT ON COLUMN engine_query_executions.parameters IS 'Parameter values used (key-value pairs)';
COMMENT ON COLUMN engine_query_executions.source IS 'Execution source: mcp (MCP tools), api (direct API), ui (web interface)';
