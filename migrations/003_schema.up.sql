-- 003_schema.up.sql
-- Schema discovery tables: tables, columns, relationships (final-state schema)

-- Schema tables (discovered from datasources)
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
ALTER TABLE engine_schema_tables FORCE ROW LEVEL SECURITY;
CREATE POLICY schema_tables_access ON engine_schema_tables FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- Schema columns (discovered columns with stats)
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
ALTER TABLE engine_schema_columns FORCE ROW LEVEL SECURITY;
CREATE POLICY schema_columns_access ON engine_schema_columns FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- Schema relationships (FK/inferred column relationships)
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
ALTER TABLE engine_schema_relationships FORCE ROW LEVEL SECURITY;
CREATE POLICY schema_relationships_access ON engine_schema_relationships FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());
