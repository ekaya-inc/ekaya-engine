-- 028_alert_config.up.sql
-- Add alert configuration column to engine_mcp_config for per-project alert settings

ALTER TABLE engine_mcp_config
    ADD COLUMN alert_config JSONB;

COMMENT ON COLUMN engine_mcp_config.alert_config IS 'Per-project alert configuration: master toggle, per-alert-type enable/disable, severity overrides, and thresholds';
