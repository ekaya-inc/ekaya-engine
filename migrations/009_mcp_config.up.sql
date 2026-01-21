-- 009_mcp_config.up.sql
-- MCP server configuration

CREATE TABLE engine_mcp_config (
    project_id uuid NOT NULL,
    tool_groups jsonb DEFAULT '{"developer": {"enabled": false}}'::jsonb NOT NULL,
    agent_api_key_encrypted text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (project_id),
    CONSTRAINT engine_mcp_config_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_engine_mcp_config_agent_key ON engine_mcp_config USING btree (agent_api_key_encrypted) WHERE (agent_api_key_encrypted IS NOT NULL);

CREATE TRIGGER update_engine_mcp_config_updated_at
    BEFORE UPDATE ON engine_mcp_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_mcp_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY mcp_config_access ON engine_mcp_config FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
