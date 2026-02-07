-- 026_audit_retention.down.sql
ALTER TABLE engine_mcp_config DROP COLUMN IF EXISTS audit_retention_days;
