-- 017_installed_apps.up.sql
-- Track installed applications per project

CREATE TABLE engine_installed_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    app_id VARCHAR(50) NOT NULL,  -- e.g., 'ai-data-liaison', 'mcp-server'
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    installed_by VARCHAR(255),  -- User email/ID who installed
    settings JSONB DEFAULT '{}'::jsonb,

    CONSTRAINT unique_project_app UNIQUE (project_id, app_id)
);

-- RLS for tenant isolation
ALTER TABLE engine_installed_apps ENABLE ROW LEVEL SECURITY;
CREATE POLICY installed_apps_access ON engine_installed_apps FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid);

-- Index for listing apps by project
CREATE INDEX idx_installed_apps_project ON engine_installed_apps(project_id);
