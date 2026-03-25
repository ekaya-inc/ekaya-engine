-- 019_mcp_config_tool_groups_cleanup.up.sql
-- Canonicalize MCP config defaults to the new per-app toggle shape and
-- disable legacy-shaped rows so they do not retain stale tool exposure.

ALTER TABLE engine_mcp_config
    ALTER COLUMN tool_groups SET DEFAULT '{
        "tools": {
            "addDirectDatabaseAccess": true,
            "addOntologyMaintenanceTools": true,
            "addOntologySuggestions": true,
            "addApprovalTools": true,
            "addRequestTools": true
        },
        "agent_tools": {
            "enabled": true
        }
    }'::jsonb;

UPDATE engine_mcp_config
SET tool_groups = '{
    "tools": {
        "addDirectDatabaseAccess": false,
        "addOntologyMaintenanceTools": false,
        "addOntologySuggestions": false,
        "addApprovalTools": false,
        "addRequestTools": false
    },
    "agent_tools": {
        "enabled": false
    }
}'::jsonb
WHERE NOT (tool_groups ? 'tools');
