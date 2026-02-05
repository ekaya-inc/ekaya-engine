-- 023_table_schema_refactor.up.sql
-- Refactor engine_schema_tables to remove semantic/ontology fields and rename engine_table_metadata
--
-- This migration:
-- - Drops engine_table_metadata (old naming)
-- - Drops and recreates engine_schema_tables (removes business_name, description, metadata)
-- - Creates engine_ontology_table_metadata with new schema (FK to schema_table_id, typed columns)
--
-- WARNING: This is a destructive migration. All table data will be lost.
-- After migration, run schema discovery and ontology extraction to repopulate.

-- =============================================================================
-- Step 1: Drop engine_table_metadata (will be replaced by engine_ontology_table_metadata)
-- =============================================================================
DROP TABLE IF EXISTS engine_table_metadata CASCADE;

-- =============================================================================
-- Step 2: Handle dependent tables before dropping engine_schema_tables
-- =============================================================================

-- Drop engine_ontology_column_metadata (depends on engine_schema_columns)
DROP TABLE IF EXISTS engine_ontology_column_metadata CASCADE;

-- Drop engine_schema_relationships (depends on engine_schema_tables and engine_schema_columns)
DROP TABLE IF EXISTS engine_schema_relationships CASCADE;

-- Drop engine_schema_columns (depends on engine_schema_tables)
-- Note: engine_entity_relationships was dropped in migration 022
DROP TABLE IF EXISTS engine_schema_columns CASCADE;

-- =============================================================================
-- Step 3: Drop and recreate engine_schema_tables with new schema
-- =============================================================================
DROP TABLE IF EXISTS engine_schema_tables CASCADE;

CREATE TABLE engine_schema_tables (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    datasource_id uuid NOT NULL,
    schema_name text NOT NULL,
    table_name text NOT NULL,
    is_selected boolean DEFAULT false NOT NULL,
    row_count bigint,

    -- Lifecycle
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,

    PRIMARY KEY (id),
    CONSTRAINT engine_schema_tables_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_schema_tables_datasource_id_fkey
        FOREIGN KEY (datasource_id) REFERENCES engine_datasources(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_schema_tables IS 'Schema discovery: tables discovered from datasources';
COMMENT ON COLUMN engine_schema_tables.is_selected IS 'Whether this table is selected for analysis';
COMMENT ON COLUMN engine_schema_tables.row_count IS 'Approximate row count from schema stats';

-- Indexes
CREATE INDEX idx_engine_schema_tables_project ON engine_schema_tables USING btree (project_id);
CREATE INDEX idx_engine_schema_tables_datasource ON engine_schema_tables USING btree (project_id, datasource_id);
CREATE UNIQUE INDEX idx_engine_schema_tables_unique ON engine_schema_tables USING btree (project_id, datasource_id, schema_name, table_name)
    WHERE (deleted_at IS NULL);

-- Updated_at trigger
CREATE TRIGGER update_engine_schema_tables_updated_at
    BEFORE UPDATE ON engine_schema_tables
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_schema_tables ENABLE ROW LEVEL SECURITY;
CREATE POLICY schema_tables_access ON engine_schema_tables FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL
           OR project_id = current_setting('app.current_project_id', true)::uuid);

-- =============================================================================
-- Step 4: Recreate engine_schema_columns (same schema as migration 020)
-- =============================================================================
CREATE TABLE engine_schema_columns (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    schema_table_id uuid NOT NULL,
    column_name text NOT NULL,
    data_type text NOT NULL,
    is_nullable boolean NOT NULL,
    is_primary_key boolean DEFAULT false NOT NULL,
    is_unique boolean DEFAULT false NOT NULL,
    ordinal_position integer NOT NULL,
    default_value text,
    is_selected boolean DEFAULT false NOT NULL,

    -- Stats from data scanning
    distinct_count bigint,
    null_count bigint,
    row_count bigint,
    non_null_count bigint,
    min_length integer,
    max_length integer,
    is_joinable boolean,
    joinability_reason text,
    stats_updated_at timestamp with time zone,

    -- Lifecycle
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,

    PRIMARY KEY (id),
    CONSTRAINT engine_schema_columns_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_schema_columns_schema_table_id_fkey
        FOREIGN KEY (schema_table_id) REFERENCES engine_schema_tables(id) ON DELETE CASCADE
);

COMMENT ON COLUMN engine_schema_columns.min_length IS 'Minimum string length for text columns (NULL for non-text)';
COMMENT ON COLUMN engine_schema_columns.max_length IS 'Maximum string length for text columns (NULL for non-text)';
COMMENT ON COLUMN engine_schema_columns.is_joinable IS 'Whether this column has high cardinality suitable for joins';
COMMENT ON COLUMN engine_schema_columns.joinability_reason IS 'Explanation of joinability classification';
COMMENT ON COLUMN engine_schema_columns.stats_updated_at IS 'When column statistics were last refreshed';

-- Indexes
CREATE INDEX idx_engine_schema_columns_project ON engine_schema_columns USING btree (project_id);
CREATE INDEX idx_engine_schema_columns_table ON engine_schema_columns USING btree (schema_table_id);
CREATE INDEX idx_engine_schema_columns_joinable ON engine_schema_columns USING btree (schema_table_id, is_joinable)
    WHERE (deleted_at IS NULL AND is_joinable = true);
CREATE UNIQUE INDEX idx_engine_schema_columns_unique ON engine_schema_columns USING btree (schema_table_id, column_name)
    WHERE (deleted_at IS NULL);

-- Updated_at trigger
CREATE TRIGGER update_engine_schema_columns_updated_at
    BEFORE UPDATE ON engine_schema_columns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_schema_columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY schema_columns_access ON engine_schema_columns FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL
           OR project_id = current_setting('app.current_project_id', true)::uuid);

-- =============================================================================
-- Step 5: Recreate engine_schema_relationships (same schema as migration 020)
-- =============================================================================
CREATE TABLE engine_schema_relationships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    source_table_id uuid NOT NULL,
    source_column_id uuid NOT NULL,
    target_table_id uuid NOT NULL,
    target_column_id uuid NOT NULL,
    relationship_type text NOT NULL,
    cardinality text DEFAULT 'unknown'::text NOT NULL,
    confidence numeric(4,3) DEFAULT 1.000 NOT NULL,
    inference_method text,
    is_validated boolean DEFAULT false NOT NULL,
    validation_results jsonb,
    is_approved boolean,
    match_rate numeric(5,4),
    source_distinct bigint,
    target_distinct bigint,
    matched_count bigint,
    rejection_reason text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    deleted_at timestamp with time zone,
    PRIMARY KEY (id),
    CONSTRAINT engine_schema_relationships_confidence_check
        CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT engine_schema_relationships_project_id_fkey
        FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_schema_relationships_source_table_id_fkey
        FOREIGN KEY (source_table_id) REFERENCES engine_schema_tables(id) ON DELETE CASCADE,
    CONSTRAINT engine_schema_relationships_source_column_id_fkey
        FOREIGN KEY (source_column_id) REFERENCES engine_schema_columns(id) ON DELETE CASCADE,
    CONSTRAINT engine_schema_relationships_target_table_id_fkey
        FOREIGN KEY (target_table_id) REFERENCES engine_schema_tables(id) ON DELETE CASCADE,
    CONSTRAINT engine_schema_relationships_target_column_id_fkey
        FOREIGN KEY (target_column_id) REFERENCES engine_schema_columns(id) ON DELETE CASCADE
);

CREATE INDEX idx_engine_schema_relationships_project ON engine_schema_relationships USING btree (project_id);
CREATE INDEX idx_engine_schema_relationships_source_table ON engine_schema_relationships USING btree (source_table_id);
CREATE INDEX idx_engine_schema_relationships_target_table ON engine_schema_relationships USING btree (target_table_id);
CREATE INDEX idx_engine_schema_relationships_rejection ON engine_schema_relationships USING btree (project_id, rejection_reason)
    WHERE (deleted_at IS NULL AND rejection_reason IS NOT NULL);
CREATE UNIQUE INDEX idx_engine_schema_relationships_unique ON engine_schema_relationships USING btree (source_column_id, target_column_id)
    WHERE (deleted_at IS NULL);

CREATE TRIGGER update_engine_schema_relationships_updated_at
    BEFORE UPDATE ON engine_schema_relationships
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE engine_schema_relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY schema_relationships_access ON engine_schema_relationships FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL
           OR project_id = current_setting('app.current_project_id', true)::uuid);

-- =============================================================================
-- Step 6: Recreate engine_ontology_column_metadata (same schema as migration 021)
-- =============================================================================
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
CREATE POLICY column_metadata_access ON engine_ontology_column_metadata FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

-- =============================================================================
-- Step 7: Create new engine_ontology_table_metadata table
-- Note: engine_entity_relationships was dropped in migration 022, no FK restoration needed
-- =============================================================================
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
CREATE POLICY table_metadata_access ON engine_ontology_table_metadata FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
