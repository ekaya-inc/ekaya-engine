-- 017_named_agents.down.sql

DROP TABLE IF EXISTS engine_agent_queries;
DROP TABLE IF EXISTS engine_agents;

ALTER TABLE engine_mcp_config ADD COLUMN IF NOT EXISTS agent_api_key_encrypted text;
CREATE INDEX IF NOT EXISTS idx_engine_mcp_config_agent_key
    ON engine_mcp_config USING btree (agent_api_key_encrypted)
    WHERE (agent_api_key_encrypted IS NOT NULL);
