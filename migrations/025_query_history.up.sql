-- 025_query_history.up.sql
-- Query history for MCP query learning. Records successful queries with natural language
-- context so the MCP client can learn from past queries and improve suggestions.

CREATE TABLE engine_mcp_query_history (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    user_id varchar(255) NOT NULL,

    -- The query itself
    natural_language text NOT NULL,
    sql text NOT NULL,

    -- Execution details
    executed_at timestamptz NOT NULL DEFAULT now(),
    execution_duration_ms integer,
    row_count integer,

    -- Learning signals
    user_feedback varchar(20),
    feedback_comment text,

    -- Query classification
    query_type varchar(50),
    tables_used text[],
    aggregations_used text[],
    time_filters jsonb,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_user_feedback CHECK (user_feedback IS NULL OR user_feedback IN ('helpful', 'not_helpful'))
);

COMMENT ON TABLE engine_mcp_query_history IS 'Successful query history for MCP query learning. Only successful queries are recorded.';
COMMENT ON COLUMN engine_mcp_query_history.user_id IS 'User ID from JWT claims (text, not UUID FK)';
COMMENT ON COLUMN engine_mcp_query_history.natural_language IS 'Natural language question that prompted this query';
COMMENT ON COLUMN engine_mcp_query_history.sql IS 'The SQL that was actually executed (final form)';
COMMENT ON COLUMN engine_mcp_query_history.user_feedback IS 'User feedback: helpful or not_helpful';
COMMENT ON COLUMN engine_mcp_query_history.query_type IS 'Query classification: aggregation, lookup, report, exploration';
COMMENT ON COLUMN engine_mcp_query_history.tables_used IS 'Tables referenced in the SQL query';
COMMENT ON COLUMN engine_mcp_query_history.aggregations_used IS 'Aggregate functions used: SUM, COUNT, AVG, etc.';
COMMENT ON COLUMN engine_mcp_query_history.time_filters IS 'Time filter metadata if present';

-- Indexes for common query patterns
CREATE INDEX idx_query_history_user ON engine_mcp_query_history(project_id, user_id, created_at DESC);
CREATE INDEX idx_query_history_tables ON engine_mcp_query_history USING GIN(tables_used);

-- RLS
ALTER TABLE engine_mcp_query_history ENABLE ROW LEVEL SECURITY;
CREATE POLICY query_history_access ON engine_mcp_query_history FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
