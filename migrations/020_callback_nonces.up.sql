-- 020_callback_nonces.up.sql
-- Shared callback nonce storage for multi-instance lifecycle redirects

CREATE TABLE engine_nonces (
    nonce text PRIMARY KEY,
    project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    action text NOT NULL,
    app_id text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);

CREATE INDEX idx_engine_nonces_expires_at
    ON engine_nonces (expires_at);

CREATE INDEX idx_engine_nonces_project_expires_at
    ON engine_nonces (project_id, expires_at);

COMMENT ON TABLE engine_nonces IS 'Single-use callback nonces for central redirect flows';
COMMENT ON COLUMN engine_nonces.app_id IS 'Installed app ID, or the sentinel value ''project'' for project deletion callbacks';

ALTER TABLE engine_nonces ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_nonces FORCE ROW LEVEL SECURITY;

CREATE POLICY nonce_access ON engine_nonces FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());
