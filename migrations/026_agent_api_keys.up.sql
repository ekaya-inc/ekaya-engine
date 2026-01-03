-- Migration 026: Agent API Keys for MCP Server
-- Add encrypted API key storage for agent authentication

ALTER TABLE engine_mcp_config
    ADD COLUMN agent_api_key_encrypted TEXT;

-- Index for efficient key lookups (if we add validation queries)
CREATE INDEX IF NOT EXISTS idx_engine_mcp_config_agent_key
    ON engine_mcp_config(agent_api_key_encrypted)
    WHERE agent_api_key_encrypted IS NOT NULL;
