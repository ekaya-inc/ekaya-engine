-- Create engine_datasources table for external data connections
-- Credentials in datasource_config are encrypted with AES-256-GCM at app layer

CREATE TABLE IF NOT EXISTS engine_datasources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    datasource_type VARCHAR(50) NOT NULL,
    datasource_config TEXT NOT NULL,  -- Encrypted blob (base64-encoded ciphertext)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    UNIQUE(project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_engine_datasources_project ON engine_datasources(project_id);
CREATE INDEX IF NOT EXISTS idx_engine_datasources_type ON engine_datasources(datasource_type);

ALTER TABLE engine_datasources ENABLE ROW LEVEL SECURITY;

CREATE POLICY datasource_access ON engine_datasources
  FOR ALL
  USING (
    current_setting('app.current_project_id', true) IS NULL
    OR project_id = current_setting('app.current_project_id', true)::uuid
  );

CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER update_engine_datasources_updated_at
    BEFORE UPDATE ON engine_datasources
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
