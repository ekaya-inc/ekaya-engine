-- 028_alert_config.down.sql
ALTER TABLE engine_mcp_config DROP COLUMN IF EXISTS alert_config;
