-- 017_named_agents.up.sql
-- Named AI agents with per-agent API keys and scoped query access

DROP INDEX IF EXISTS idx_engine_mcp_config_agent_key;
ALTER TABLE engine_mcp_config DROP COLUMN IF EXISTS agent_api_key_encrypted;

CREATE TABLE engine_agents (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    name varchar(255) NOT NULL,
    api_key_encrypted text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    last_access_at timestamptz,
    CONSTRAINT engine_agents_project_name_unique UNIQUE (project_id, name)
);

CREATE TABLE engine_agent_queries (
    agent_id uuid NOT NULL REFERENCES engine_agents(id) ON DELETE CASCADE,
    query_id uuid NOT NULL REFERENCES engine_queries(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, query_id)
);

CREATE INDEX idx_engine_agents_project ON engine_agents(project_id);
CREATE INDEX idx_engine_agent_queries_agent ON engine_agent_queries(agent_id);

CREATE TRIGGER update_engine_agents_updated_at
    BEFORE UPDATE ON engine_agents
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE engine_agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_agents FORCE ROW LEVEL SECURITY;
CREATE POLICY engine_agents_access ON engine_agents FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

ALTER TABLE engine_agent_queries ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_agent_queries FORCE ROW LEVEL SECURITY;
CREATE POLICY engine_agent_queries_access ON engine_agent_queries FOR ALL
    USING (
        rls_tenant_id() IS NULL OR EXISTS (
            SELECT 1
            FROM engine_agents
            WHERE engine_agents.id = engine_agent_queries.agent_id
              AND engine_agents.project_id = rls_tenant_id()
        )
    )
    WITH CHECK (
        rls_tenant_id() IS NULL OR EXISTS (
            SELECT 1
            FROM engine_agents
            WHERE engine_agents.id = engine_agent_queries.agent_id
              AND engine_agents.project_id = rls_tenant_id()
        )
    );
