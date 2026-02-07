-- 024_mcp_audit_log.up.sql
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
CREATE POLICY mcp_audit_log_access ON engine_mcp_audit_log FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
