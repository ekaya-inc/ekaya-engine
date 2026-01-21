-- 002_datasources.up.sql
-- External data connections (credentials encrypted)

CREATE TABLE engine_datasources (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    datasource_type character varying(50) NOT NULL,
    datasource_config text NOT NULL,
    provider character varying(50),
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    PRIMARY KEY (id),
    CONSTRAINT engine_datasources_project_id_name_key UNIQUE (project_id, name),
    CONSTRAINT engine_datasources_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

COMMENT ON COLUMN engine_datasources.provider IS 'Optional provider identifier for adapter variants (e.g., supabase, neon for postgres adapter)';

CREATE INDEX idx_engine_datasources_project ON engine_datasources USING btree (project_id);
CREATE INDEX idx_engine_datasources_type ON engine_datasources USING btree (datasource_type);

CREATE TRIGGER update_engine_datasources_updated_at
    BEFORE UPDATE ON engine_datasources
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_datasources ENABLE ROW LEVEL SECURITY;
CREATE POLICY datasource_access ON engine_datasources FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
