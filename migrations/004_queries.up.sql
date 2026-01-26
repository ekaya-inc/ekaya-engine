-- 004_queries.up.sql
-- Saved SQL queries with parameters and execution history

CREATE TABLE engine_queries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    datasource_id uuid NOT NULL,
    natural_language_prompt text NOT NULL,
    additional_context text,
    sql_query text NOT NULL,
    dialect text NOT NULL,
    is_enabled boolean DEFAULT true NOT NULL,
    usage_count integer DEFAULT 0 NOT NULL,
    last_used_at timestamp with time zone,
    parameters jsonb DEFAULT '[]'::jsonb NOT NULL,
    output_columns jsonb DEFAULT '[]'::jsonb NOT NULL,
    constraints text,
    status character varying DEFAULT 'approved'::character varying NOT NULL,
    suggested_by character varying,
    suggestion_context jsonb,
    tags text[] DEFAULT '{}'::text[],
    allows_modification boolean NOT NULL DEFAULT false,
    reviewed_by character varying(255),
    reviewed_at timestamp with time zone,
    rejection_reason text,
    parent_query_id uuid REFERENCES engine_queries(id),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    PRIMARY KEY (id),
    CONSTRAINT engine_queries_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_queries_datasource_id_fkey FOREIGN KEY (datasource_id) REFERENCES engine_datasources(id) ON DELETE CASCADE
);

COMMENT ON COLUMN engine_queries.parameters IS 'Array of parameter definitions with name, type, description, required flag, and optional default value. Supported types: string, integer, decimal, boolean, date, timestamp, uuid, string[], integer[]';
COMMENT ON COLUMN engine_queries.output_columns IS 'Array of output column metadata (name, type, description) describing what data is returned by the query';
COMMENT ON COLUMN engine_queries.constraints IS 'Explicit limitations and assumptions of the query (e.g., "Only includes completed orders", "Excludes refunded amounts")';
COMMENT ON COLUMN engine_queries.status IS 'Query lifecycle: pending, approved, rejected';
COMMENT ON COLUMN engine_queries.suggested_by IS 'Origin: user, agent, admin';
COMMENT ON COLUMN engine_queries.suggestion_context IS 'Stores example usage, validation results, etc.';
COMMENT ON COLUMN engine_queries.tags IS 'Tags for organizing queries (e.g., "billing", "reporting", "category:analytics")';
COMMENT ON COLUMN engine_queries.allows_modification IS 'When true, this query can execute INSERT/UPDATE/DELETE/CALL statements. When false, only SELECT is allowed.';
COMMENT ON COLUMN engine_queries.reviewed_by IS 'User/admin who reviewed the pending query';
COMMENT ON COLUMN engine_queries.reviewed_at IS 'When the query was approved or rejected';
COMMENT ON COLUMN engine_queries.rejection_reason IS 'Explanation provided when query was rejected';
COMMENT ON COLUMN engine_queries.parent_query_id IS 'For update suggestions: references the original query being updated';

CREATE INDEX idx_engine_queries_project ON engine_queries USING btree (project_id);
CREATE INDEX idx_engine_queries_datasource ON engine_queries USING btree (project_id, datasource_id);
CREATE INDEX idx_engine_queries_enabled ON engine_queries USING btree (project_id, datasource_id, is_enabled) WHERE (deleted_at IS NULL);
CREATE INDEX idx_engine_queries_status ON engine_queries USING btree (project_id, datasource_id, status) WHERE (deleted_at IS NULL);
CREATE INDEX idx_engine_queries_has_parameters ON engine_queries USING btree ((parameters <> '[]'::jsonb)) WHERE (deleted_at IS NULL);
CREATE INDEX idx_engine_queries_has_output_columns ON engine_queries USING btree ((output_columns <> '[]'::jsonb)) WHERE (deleted_at IS NULL);
CREATE INDEX idx_engine_queries_tags ON engine_queries USING gin (tags) WHERE (deleted_at IS NULL);
CREATE INDEX idx_queries_pending_review ON engine_queries(project_id, datasource_id) WHERE status = 'pending' AND deleted_at IS NULL;
CREATE INDEX idx_queries_parent ON engine_queries(parent_query_id) WHERE parent_query_id IS NOT NULL AND deleted_at IS NULL;

CREATE TRIGGER update_engine_queries_updated_at
    BEFORE UPDATE ON engine_queries
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Query execution history
CREATE TABLE engine_query_executions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    query_id uuid NOT NULL,
    sql text NOT NULL,
    executed_at timestamp with time zone DEFAULT now() NOT NULL,
    row_count integer NOT NULL,
    execution_time_ms integer NOT NULL,
    parameters jsonb,
    user_id text,
    source text DEFAULT 'mcp'::text NOT NULL,
    is_modifying boolean DEFAULT false NOT NULL,
    rows_affected bigint DEFAULT NULL,
    success boolean DEFAULT true NOT NULL,
    error_message text,
    PRIMARY KEY (id),
    CONSTRAINT valid_row_count CHECK (row_count >= 0),
    CONSTRAINT valid_execution_time CHECK (execution_time_ms >= 0),
    CONSTRAINT engine_query_executions_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_query_executions_query_id_fkey FOREIGN KEY (query_id) REFERENCES engine_queries(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_query_executions IS 'Execution history for approved queries. Used by get_query_history MCP tool to provide AI agents with context about recent queries.';
COMMENT ON COLUMN engine_query_executions.sql IS 'Resolved SQL with actual parameter values (for debugging/reference)';
COMMENT ON COLUMN engine_query_executions.parameters IS 'Parameter values used (key-value pairs)';
COMMENT ON COLUMN engine_query_executions.source IS 'Execution source: mcp (MCP tools), api (direct API), ui (web interface)';
COMMENT ON COLUMN engine_query_executions.is_modifying IS 'True if this was a data-modifying query (INSERT/UPDATE/DELETE/CALL)';
COMMENT ON COLUMN engine_query_executions.rows_affected IS 'Number of rows affected by modifying queries (from database command tag, not RETURNING clause)';
COMMENT ON COLUMN engine_query_executions.success IS 'True if query executed successfully, false if it failed';
COMMENT ON COLUMN engine_query_executions.error_message IS 'Error message if the query execution failed';

CREATE INDEX idx_query_executions_project_time ON engine_query_executions USING btree (project_id, executed_at DESC);
CREATE INDEX idx_query_executions_query ON engine_query_executions USING btree (query_id, executed_at DESC);
CREATE INDEX idx_query_executions_modifying ON engine_query_executions USING btree (project_id, is_modifying, executed_at DESC) WHERE is_modifying = TRUE;

-- RLS
ALTER TABLE engine_queries ENABLE ROW LEVEL SECURITY;
CREATE POLICY queries_access ON engine_queries FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

ALTER TABLE engine_query_executions ENABLE ROW LEVEL SECURITY;
CREATE POLICY query_executions_access ON engine_query_executions FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
