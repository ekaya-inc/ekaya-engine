-- Down migration for 026: Agent API Keys
DROP INDEX IF EXISTS idx_engine_mcp_config_agent_key;
ALTER TABLE engine_mcp_config DROP COLUMN IF EXISTS agent_api_key_encrypted;
