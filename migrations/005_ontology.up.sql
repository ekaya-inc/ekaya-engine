-- 005_ontology.up.sql
-- Ontology core: tiered ontology storage (entities removed)

CREATE TABLE engine_ontologies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    domain_summary jsonb,
    column_details jsonb,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontologies_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_engine_ontologies_project ON engine_ontologies USING btree (project_id);
CREATE UNIQUE INDEX idx_engine_ontologies_unique ON engine_ontologies USING btree (project_id, version);
CREATE UNIQUE INDEX idx_engine_ontologies_single_active ON engine_ontologies USING btree (project_id) WHERE (is_active = true);

CREATE TRIGGER update_engine_ontologies_updated_at
    BEFORE UPDATE ON engine_ontologies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_ontologies ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_ontologies FORCE ROW LEVEL SECURITY;
CREATE POLICY ontologies_access ON engine_ontologies FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
