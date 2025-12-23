-- Migration 008: MCP Server Configuration
-- Stores MCP tool group configuration per project

CREATE TABLE engine_mcp_config (
    project_id UUID PRIMARY KEY REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- Tool groups configuration (JSONB for flexibility)
    -- Structure: {"developer": {"enabled": false}, ...}
    tool_groups JSONB NOT NULL DEFAULT '{"developer": {"enabled": false}}',

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- RLS Policy
ALTER TABLE engine_mcp_config ENABLE ROW LEVEL SECURITY;
CREATE POLICY mcp_config_access ON engine_mcp_config
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Auto-update timestamp trigger (reuses existing function from migration 003)
CREATE TRIGGER update_engine_mcp_config_updated_at
    BEFORE UPDATE ON engine_mcp_config
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
