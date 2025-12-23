-- Migration 008 DOWN: Remove MCP config table

-- Drop trigger first
DROP TRIGGER IF EXISTS update_engine_mcp_config_updated_at ON engine_mcp_config;

-- Drop policy
DROP POLICY IF EXISTS mcp_config_access ON engine_mcp_config;

-- Drop table
DROP TABLE IF EXISTS engine_mcp_config;
