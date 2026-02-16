-- 010_mcp_audit.up.sql
-- MCP audit tables: audit log, query history, audit alerts

-- MCP audit log for tracking tool invocations, session events, and security activity
CREATE TABLE engine_mcp_audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- Who
    user_id varchar(255) NOT NULL,
    user_email varchar(255),
    session_id varchar(255),

    -- What
    event_type varchar(50) NOT NULL,
    tool_name varchar(100),

    -- Request details
    request_params jsonb,
    natural_language text,
    sql_query text,

    -- Response details
    was_successful boolean NOT NULL DEFAULT true,
    error_message text,
    result_summary jsonb,

    -- Performance
    duration_ms integer,

    -- Security classification
    security_level varchar(20) NOT NULL DEFAULT 'normal',
    security_flags text[],

    -- Context
    client_info jsonb,
    created_at timestamp with time zone NOT NULL DEFAULT now()
);

COMMENT ON TABLE engine_mcp_audit_log IS 'Audit trail of MCP tool invocations, session events, and security activity';
COMMENT ON COLUMN engine_mcp_audit_log.event_type IS 'Event category: tool_call, tool_success, tool_error, mcp_session_start, mcp_session_end, mcp_auth_failure, query_blocked, sql_injection_attempt, rate_limit_hit';
COMMENT ON COLUMN engine_mcp_audit_log.security_level IS 'Security classification: normal, warning, critical';

-- Indexes for common query patterns
CREATE INDEX idx_mcp_audit_project_time ON engine_mcp_audit_log(project_id, created_at DESC);
CREATE INDEX idx_mcp_audit_user ON engine_mcp_audit_log(project_id, user_id, created_at DESC);
CREATE INDEX idx_mcp_audit_event_type ON engine_mcp_audit_log(project_id, event_type, created_at DESC);
CREATE INDEX idx_mcp_audit_security ON engine_mcp_audit_log(project_id, security_level, created_at DESC)
    WHERE security_level != 'normal';

-- RLS
ALTER TABLE engine_mcp_audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_mcp_audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY mcp_audit_log_access ON engine_mcp_audit_log FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- Query history for MCP query learning
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
ALTER TABLE engine_mcp_query_history FORCE ROW LEVEL SECURITY;
CREATE POLICY query_history_access ON engine_mcp_query_history FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- Alerts for security and governance notifications on MCP audit activity
CREATE TABLE engine_mcp_audit_alerts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    alert_type varchar(50) NOT NULL,
    severity varchar(20) NOT NULL,

    title varchar(255) NOT NULL,
    description text,
    affected_user_id varchar(255),
    related_audit_ids uuid[],

    status varchar(20) NOT NULL DEFAULT 'open',
    resolved_by varchar(255),
    resolved_at timestamp with time zone,
    resolution_notes text,

    created_at timestamp with time zone NOT NULL DEFAULT now(),
    updated_at timestamp with time zone NOT NULL DEFAULT now()
);

COMMENT ON TABLE engine_mcp_audit_alerts IS 'Security and governance alerts generated from MCP audit activity';
COMMENT ON COLUMN engine_mcp_audit_alerts.alert_type IS 'Alert category: sql_injection_detected, unusual_query_volume, sensitive_table_access, large_data_export, after_hours_access, new_user_high_volume, repeated_errors';
COMMENT ON COLUMN engine_mcp_audit_alerts.severity IS 'Alert severity: critical, warning, info';
COMMENT ON COLUMN engine_mcp_audit_alerts.status IS 'Alert status: open, resolved, dismissed';

-- Indexes for common query patterns
CREATE INDEX idx_audit_alerts_project_status ON engine_mcp_audit_alerts(project_id, status, created_at DESC);
CREATE INDEX idx_audit_alerts_project_severity ON engine_mcp_audit_alerts(project_id, severity, created_at DESC);
CREATE INDEX idx_audit_alerts_project_time ON engine_mcp_audit_alerts(project_id, created_at DESC);

-- RLS
ALTER TABLE engine_mcp_audit_alerts ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_mcp_audit_alerts FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_alerts_access ON engine_mcp_audit_alerts FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());
