-- 019_mcp_config_tool_groups_cleanup.down.sql
-- Revert the schema default only. The row rewrite in the up migration is
-- intentionally irreversible because the legacy shape cannot be reconstructed.

ALTER TABLE engine_mcp_config
    ALTER COLUMN tool_groups SET DEFAULT '{"developer": {"enabled": false}}'::jsonb;
