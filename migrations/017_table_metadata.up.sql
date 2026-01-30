-- 017_table_metadata.up.sql
-- Table-level metadata for storing semantic annotations per table

CREATE TABLE engine_table_metadata (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    datasource_id uuid NOT NULL,
    table_name text NOT NULL,

    -- Semantic information
    description text,                    -- What the table represents
    usage_notes text,                    -- When to use/not use this table
    is_ephemeral boolean DEFAULT false NOT NULL,  -- Transient table not for analytics
    preferred_alternative text,          -- Table to use instead if ephemeral

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,

    PRIMARY KEY (id),
    CONSTRAINT engine_table_metadata_unique UNIQUE (project_id, datasource_id, table_name),
    CONSTRAINT engine_table_metadata_project_id_fkey FOREIGN KEY (project_id)
        REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_table_metadata_datasource_id_fkey FOREIGN KEY (datasource_id)
        REFERENCES engine_datasources(id) ON DELETE CASCADE,
    CONSTRAINT engine_table_metadata_source_check
        CHECK (source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_table_metadata_last_edit_source_check
        CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_table_metadata_created_by_fkey
        FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    CONSTRAINT engine_table_metadata_updated_by_fkey
        FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
);

COMMENT ON TABLE engine_table_metadata IS 'Table-level semantic annotations with provenance tracking';
COMMENT ON COLUMN engine_table_metadata.description IS 'What this table represents and contains';
COMMENT ON COLUMN engine_table_metadata.usage_notes IS 'Guidance on when to use or not use this table';
COMMENT ON COLUMN engine_table_metadata.is_ephemeral IS 'True for transient/temporary tables not suitable for analytics';
COMMENT ON COLUMN engine_table_metadata.preferred_alternative IS 'Table to use instead if this one is ephemeral or deprecated';
COMMENT ON COLUMN engine_table_metadata.source IS 'How this metadata was created: inferred (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_table_metadata.last_edit_source IS 'How this metadata was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_table_metadata.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_table_metadata.updated_by IS 'UUID of user who last updated this metadata';

-- Indexes
CREATE INDEX idx_table_metadata_project ON engine_table_metadata(project_id);
CREATE INDEX idx_table_metadata_datasource ON engine_table_metadata(project_id, datasource_id);

CREATE TRIGGER update_engine_table_metadata_updated_at
    BEFORE UPDATE ON engine_table_metadata
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_table_metadata ENABLE ROW LEVEL SECURITY;
CREATE POLICY table_metadata_access ON engine_table_metadata FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
