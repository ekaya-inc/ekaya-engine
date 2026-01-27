-- 010_business_glossary.up.sql
-- Business glossary with metric definitions and aliases

CREATE TABLE engine_business_glossary (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    ontology_id uuid,
    term text NOT NULL,
    definition text NOT NULL,
    defining_sql text NOT NULL,
    base_table text,
    output_columns jsonb,
    enrichment_status text DEFAULT 'pending'::text,
    enrichment_error text,

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_business_glossary_source_check CHECK (source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_business_glossary_last_edit_source_check CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_business_glossary_enrichment_status_check CHECK (enrichment_status IS NULL OR enrichment_status = ANY (ARRAY['pending'::text, 'success'::text, 'failed'::text])),
    CONSTRAINT engine_business_glossary_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_business_glossary_ontology_id_fkey FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    CONSTRAINT engine_business_glossary_created_by_fkey FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    CONSTRAINT engine_business_glossary_updated_by_fkey FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
);

-- Unique constraint on (project_id, ontology_id, term) to allow same term across ontologies
CREATE UNIQUE INDEX engine_business_glossary_project_ontology_term_unique ON engine_business_glossary USING btree (project_id, ontology_id, term);

COMMENT ON TABLE engine_business_glossary IS 'Business terms with definitive SQL definitions for MCP query composition';
COMMENT ON COLUMN engine_business_glossary.term IS 'Business term name (e.g., "Active Users", "Monthly Recurring Revenue")';
COMMENT ON COLUMN engine_business_glossary.definition IS 'Human-readable description of what this term means';
COMMENT ON COLUMN engine_business_glossary.defining_sql IS 'Complete executable SQL that defines this metric (SELECT statement)';
COMMENT ON COLUMN engine_business_glossary.base_table IS 'Primary table being queried (for quick reference)';
COMMENT ON COLUMN engine_business_glossary.output_columns IS 'Array of output columns with name, type, and optional description';
COMMENT ON COLUMN engine_business_glossary.ontology_id IS 'Optional link to specific ontology version for CASCADE delete';
COMMENT ON COLUMN engine_business_glossary.enrichment_status IS 'Status of SQL enrichment: pending, success, or failed';
COMMENT ON COLUMN engine_business_glossary.enrichment_error IS 'Error message if enrichment failed, NULL otherwise';
COMMENT ON COLUMN engine_business_glossary.source IS 'How this term was created: inferred (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_business_glossary.last_edit_source IS 'How this term was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_business_glossary.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_business_glossary.updated_by IS 'UUID of user who last updated this term';

CREATE INDEX idx_business_glossary_project ON engine_business_glossary USING btree (project_id);
CREATE INDEX idx_business_glossary_ontology ON engine_business_glossary USING btree (ontology_id);
CREATE INDEX idx_business_glossary_base_table ON engine_business_glossary USING btree (base_table);
CREATE INDEX idx_business_glossary_source ON engine_business_glossary USING btree (source);
CREATE INDEX idx_business_glossary_enrichment_status ON engine_business_glossary USING btree (enrichment_status);

CREATE TRIGGER update_business_glossary_updated_at
    BEFORE UPDATE ON engine_business_glossary
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Glossary term aliases (alternative names for terms)
CREATE TABLE engine_glossary_aliases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    glossary_id uuid NOT NULL,
    alias text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_glossary_aliases_unique UNIQUE (glossary_id, alias),
    CONSTRAINT engine_glossary_aliases_glossary_id_fkey FOREIGN KEY (glossary_id) REFERENCES engine_business_glossary(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_glossary_aliases IS 'Alternative names for glossary terms (e.g., MAU = Monthly Active Users)';

CREATE INDEX idx_glossary_aliases_glossary ON engine_glossary_aliases USING btree (glossary_id);
CREATE INDEX idx_glossary_aliases_alias ON engine_glossary_aliases USING btree (alias);

-- RLS
ALTER TABLE engine_business_glossary ENABLE ROW LEVEL SECURITY;
CREATE POLICY business_glossary_access ON engine_business_glossary FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

ALTER TABLE engine_glossary_aliases ENABLE ROW LEVEL SECURITY;
CREATE POLICY glossary_aliases_access ON engine_glossary_aliases FOR ALL
    USING (glossary_id IN (
        SELECT id FROM engine_business_glossary
        WHERE current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid
    ));
