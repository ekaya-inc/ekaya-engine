-- ETL load history: tracks file load operations for ETL applets.

CREATE TABLE engine_etl_load_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    app_id VARCHAR(50) NOT NULL,
    file_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    rows_attempted INTEGER NOT NULL DEFAULT 0,
    rows_loaded INTEGER NOT NULL DEFAULT 0,
    rows_skipped INTEGER NOT NULL DEFAULT 0,
    errors JSONB DEFAULT '[]'::jsonb,
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    status VARCHAR(20) NOT NULL DEFAULT 'pending'
);

ALTER TABLE engine_etl_load_history ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_etl_load_history FORCE ROW LEVEL SECURITY;

CREATE POLICY etl_load_history_access ON engine_etl_load_history FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

CREATE INDEX idx_etl_load_history_project ON engine_etl_load_history(project_id);
CREATE INDEX idx_etl_load_history_project_app ON engine_etl_load_history(project_id, app_id);
CREATE INDEX idx_etl_load_history_status ON engine_etl_load_history(status);
