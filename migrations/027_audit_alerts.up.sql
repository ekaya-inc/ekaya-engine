-- 027_audit_alerts.up.sql
-- Alerts table for security and governance notifications on MCP audit activity

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
CREATE POLICY audit_alerts_access ON engine_mcp_audit_alerts FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
