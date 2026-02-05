-- 021_column_metadata_refactor.down.sql
-- Rollback: restore engine_ontology_column_metadata with original schema (table_name/column_name)
--
-- WARNING: This is a destructive rollback. All column metadata will be lost.

DROP TABLE IF EXISTS engine_ontology_column_metadata CASCADE;

-- Recreate original engine_ontology_column_metadata with table_name/column_name
CREATE TABLE engine_ontology_column_metadata (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    table_name text NOT NULL,
    column_name text NOT NULL,

    -- Semantic information
    description text,
    entity text,           -- Entity this column belongs to (e.g., 'User', 'Account')
    role text,             -- Semantic role: 'dimension', 'measure', 'identifier', 'attribute'
    enum_values jsonb,     -- Array of enum values with descriptions
    is_sensitive boolean,  -- Added in migration 016

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,

    PRIMARY KEY (id),
    CONSTRAINT engine_column_metadata_unique UNIQUE (project_id, table_name, column_name),
    CONSTRAINT engine_column_metadata_project_id_fkey FOREIGN KEY (project_id)
        REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_column_metadata_source_check
        CHECK (source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_column_metadata_last_edit_source_check
        CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_column_metadata_role_check
        CHECK (role IS NULL OR role IN ('dimension', 'measure', 'identifier', 'attribute')),
    CONSTRAINT engine_column_metadata_created_by_fkey
        FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    CONSTRAINT engine_column_metadata_updated_by_fkey
        FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
);

COMMENT ON TABLE engine_ontology_column_metadata IS 'Column-level semantic annotations with provenance tracking';
COMMENT ON COLUMN engine_ontology_column_metadata.entity IS 'Entity this column belongs to (e.g., User, Account)';
COMMENT ON COLUMN engine_ontology_column_metadata.role IS 'Semantic role: dimension (group by), measure (aggregate), identifier (PK/FK), attribute (other)';
COMMENT ON COLUMN engine_ontology_column_metadata.enum_values IS 'Array of enum values with descriptions, e.g., ["ACTIVE - Normal account", "SUSPENDED - Temp hold"]';
COMMENT ON COLUMN engine_ontology_column_metadata.is_sensitive IS 'Manual sensitive data override: NULL=auto-detect, TRUE=always sensitive, FALSE=never sensitive';
COMMENT ON COLUMN engine_ontology_column_metadata.source IS 'How this metadata was created: inferred (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_ontology_column_metadata.last_edit_source IS 'How this metadata was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_ontology_column_metadata.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_ontology_column_metadata.updated_by IS 'UUID of user who last updated this metadata';

-- Indexes
CREATE INDEX idx_column_metadata_project ON engine_ontology_column_metadata(project_id);
CREATE INDEX idx_column_metadata_table ON engine_ontology_column_metadata(project_id, table_name);

CREATE TRIGGER update_engine_column_metadata_updated_at
    BEFORE UPDATE ON engine_ontology_column_metadata
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_ontology_column_metadata ENABLE ROW LEVEL SECURITY;
CREATE POLICY column_metadata_access ON engine_ontology_column_metadata FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
