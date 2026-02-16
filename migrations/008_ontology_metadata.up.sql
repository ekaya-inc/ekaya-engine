-- 008_ontology_metadata.up.sql
-- Ontology metadata tables: column and table semantic annotations (final-state schema)

-- Column-level semantic annotations
CREATE TABLE engine_ontology_column_metadata (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    schema_column_id uuid NOT NULL,

    -- Core classification (typed columns)
    classification_path text,
    purpose text,  -- identifier, timestamp, flag, measure, enum, text, json
    semantic_type text,  -- soft_delete_timestamp, currency_cents, etc.
    role text,
    description text,
    confidence numeric(4,3),

    -- Type-specific features (single JSONB for extensibility)
    features jsonb DEFAULT '{}' NOT NULL,

    -- Processing flags
    needs_enum_analysis boolean NOT NULL DEFAULT false,
    needs_fk_resolution boolean NOT NULL DEFAULT false,
    needs_cross_column_check boolean NOT NULL DEFAULT false,
    needs_clarification boolean NOT NULL DEFAULT false,
    clarification_question text,

    -- User overrides
    is_sensitive boolean,

    -- Analysis metadata
    analyzed_at timestamp with time zone,
    llm_model_used text,

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,

    PRIMARY KEY (id),
    CONSTRAINT engine_column_metadata_unique UNIQUE (project_id, schema_column_id),
    CONSTRAINT engine_column_metadata_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_column_metadata_schema_column_id_fkey
        FOREIGN KEY (schema_column_id) REFERENCES engine_schema_columns(id) ON DELETE CASCADE,
    CONSTRAINT engine_column_metadata_confidence_check
        CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    CONSTRAINT engine_column_metadata_source_check
        CHECK (source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_column_metadata_last_edit_source_check
        CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_column_metadata_role_check
        CHECK (role IS NULL OR role IN ('primary_key', 'foreign_key', 'attribute', 'measure', 'dimension', 'identifier')),
    CONSTRAINT engine_column_metadata_created_by_fkey
        FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    CONSTRAINT engine_column_metadata_updated_by_fkey
        FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
);

COMMENT ON TABLE engine_ontology_column_metadata IS 'Column-level semantic annotations with provenance tracking';
COMMENT ON COLUMN engine_ontology_column_metadata.schema_column_id IS 'FK to engine_schema_columns - the physical column this metadata describes';
COMMENT ON COLUMN engine_ontology_column_metadata.classification_path IS 'Hierarchical classification path from LLM (e.g., identifier/foreign_key)';
COMMENT ON COLUMN engine_ontology_column_metadata.purpose IS 'Column purpose: identifier, timestamp, flag, measure, enum, text, json';
COMMENT ON COLUMN engine_ontology_column_metadata.semantic_type IS 'Detailed semantic type: soft_delete_timestamp, currency_cents, etc.';
COMMENT ON COLUMN engine_ontology_column_metadata.role IS 'Semantic role: primary_key, foreign_key, attribute, measure, dimension, identifier';
COMMENT ON COLUMN engine_ontology_column_metadata.confidence IS 'Classification confidence score (0-1)';
COMMENT ON COLUMN engine_ontology_column_metadata.features IS 'Type-specific features: timestamp_features, boolean_features, enum_features, identifier_features, monetary_features';
COMMENT ON COLUMN engine_ontology_column_metadata.needs_enum_analysis IS 'Flag: needs enum value extraction and labeling';
COMMENT ON COLUMN engine_ontology_column_metadata.needs_fk_resolution IS 'Flag: needs FK target identification';
COMMENT ON COLUMN engine_ontology_column_metadata.needs_cross_column_check IS 'Flag: needs cross-column pattern analysis';
COMMENT ON COLUMN engine_ontology_column_metadata.needs_clarification IS 'Flag: needs user clarification to proceed';
COMMENT ON COLUMN engine_ontology_column_metadata.clarification_question IS 'Question to ask user when needs_clarification is true';
COMMENT ON COLUMN engine_ontology_column_metadata.is_sensitive IS 'Manual sensitive data override: NULL=auto-detect, TRUE=always sensitive, FALSE=never sensitive';
COMMENT ON COLUMN engine_ontology_column_metadata.analyzed_at IS 'When this column was last analyzed';
COMMENT ON COLUMN engine_ontology_column_metadata.llm_model_used IS 'LLM model used for classification';
COMMENT ON COLUMN engine_ontology_column_metadata.source IS 'How this metadata was created: inferred (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_ontology_column_metadata.last_edit_source IS 'How this metadata was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_ontology_column_metadata.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_ontology_column_metadata.updated_by IS 'UUID of user who last updated this metadata';

-- Indexes
CREATE INDEX idx_column_metadata_project ON engine_ontology_column_metadata(project_id);
CREATE INDEX idx_column_metadata_schema_column ON engine_ontology_column_metadata(schema_column_id);
CREATE INDEX idx_column_metadata_needs_analysis ON engine_ontology_column_metadata(project_id)
    WHERE needs_enum_analysis = true OR needs_fk_resolution = true OR needs_cross_column_check = true;
CREATE INDEX idx_column_metadata_needs_clarification ON engine_ontology_column_metadata(project_id)
    WHERE needs_clarification = true;

CREATE TRIGGER update_engine_column_metadata_updated_at
    BEFORE UPDATE ON engine_ontology_column_metadata
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_ontology_column_metadata ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_column_metadata FORCE ROW LEVEL SECURITY;
CREATE POLICY column_metadata_access ON engine_ontology_column_metadata FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

-- Table-level semantic annotations
CREATE TABLE engine_ontology_table_metadata (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    schema_table_id uuid NOT NULL,

    -- Core classification (typed columns)
    table_type text,  -- transactional, reference, logging, ephemeral, junction
    description text,
    usage_notes text,
    is_ephemeral boolean NOT NULL DEFAULT false,
    preferred_alternative text,  -- table to use instead if this one is ephemeral/deprecated
    confidence numeric(4,3),

    -- Type-specific features (single JSONB for extensibility)
    features jsonb DEFAULT '{}' NOT NULL,

    -- Analysis metadata
    analyzed_at timestamp with time zone,
    llm_model_used text,

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,

    PRIMARY KEY (id),
    CONSTRAINT engine_table_metadata_unique UNIQUE (project_id, schema_table_id),
    CONSTRAINT engine_table_metadata_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_table_metadata_schema_table_id_fkey
        FOREIGN KEY (schema_table_id) REFERENCES engine_schema_tables(id) ON DELETE CASCADE,
    CONSTRAINT engine_table_metadata_confidence_check
        CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    CONSTRAINT engine_table_metadata_table_type_check
        CHECK (table_type IS NULL OR table_type IN ('transactional', 'reference', 'logging', 'ephemeral', 'junction')),
    CONSTRAINT engine_table_metadata_source_check
        CHECK (source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_table_metadata_last_edit_source_check
        CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_table_metadata_created_by_fkey
        FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    CONSTRAINT engine_table_metadata_updated_by_fkey
        FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
);

COMMENT ON TABLE engine_ontology_table_metadata IS 'Table-level semantic annotations with provenance tracking';
COMMENT ON COLUMN engine_ontology_table_metadata.schema_table_id IS 'FK to engine_schema_tables - the physical table this metadata describes';
COMMENT ON COLUMN engine_ontology_table_metadata.table_type IS 'Table classification: transactional, reference, logging, ephemeral, junction';
COMMENT ON COLUMN engine_ontology_table_metadata.description IS 'What this table represents and contains';
COMMENT ON COLUMN engine_ontology_table_metadata.usage_notes IS 'Guidance on when to use or not use this table';
COMMENT ON COLUMN engine_ontology_table_metadata.is_ephemeral IS 'True for transient/temporary tables not suitable for analytics';
COMMENT ON COLUMN engine_ontology_table_metadata.preferred_alternative IS 'Table to use instead if this one is ephemeral or deprecated';
COMMENT ON COLUMN engine_ontology_table_metadata.confidence IS 'Classification confidence score (0-1)';
COMMENT ON COLUMN engine_ontology_table_metadata.features IS 'Type-specific features: relationship_summary, temporal_features, size_features';
COMMENT ON COLUMN engine_ontology_table_metadata.analyzed_at IS 'When this table was last analyzed';
COMMENT ON COLUMN engine_ontology_table_metadata.llm_model_used IS 'LLM model used for classification';
COMMENT ON COLUMN engine_ontology_table_metadata.source IS 'How this metadata was created: inferred (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_ontology_table_metadata.last_edit_source IS 'How this metadata was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_ontology_table_metadata.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_ontology_table_metadata.updated_by IS 'UUID of user who last updated this metadata';

-- Indexes
CREATE INDEX idx_table_metadata_project ON engine_ontology_table_metadata(project_id);
CREATE INDEX idx_table_metadata_schema_table ON engine_ontology_table_metadata(schema_table_id);
CREATE INDEX idx_table_metadata_ephemeral ON engine_ontology_table_metadata(project_id)
    WHERE is_ephemeral = true;

CREATE TRIGGER update_engine_table_metadata_updated_at
    BEFORE UPDATE ON engine_ontology_table_metadata
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_ontology_table_metadata ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_table_metadata FORCE ROW LEVEL SECURITY;
CREATE POLICY table_metadata_access ON engine_ontology_table_metadata FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
