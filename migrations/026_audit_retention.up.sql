-- 026_audit_retention.up.sql
-- Add admin-configurable retention period for audit and query history data.
-- NULL means use default (90 days). Stored on engine_mcp_config since that's
-- already the per-project MCP/audit configuration table.

ALTER TABLE engine_mcp_config
    ADD COLUMN audit_retention_days INTEGER;

COMMENT ON COLUMN engine_mcp_config.audit_retention_days IS
    'Admin-configurable retention period in days for audit and query history data. NULL = 90 days (default).';
