-- Migration 004: Schema tables for schema discovery functionality
-- Creates engine_schema_tables, engine_schema_columns, engine_schema_relationships

-- ============================================================================
-- Table: engine_schema_tables
-- Stores discovered tables from user datasources
-- ============================================================================

CREATE TABLE engine_schema_tables (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    datasource_id UUID NOT NULL REFERENCES engine_datasources(id) ON DELETE CASCADE,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    is_selected BOOLEAN NOT NULL DEFAULT false,
    row_count BIGINT,
    business_name TEXT,
    description TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Partial unique: same table can't exist twice (unless soft-deleted)
CREATE UNIQUE INDEX idx_engine_schema_tables_unique
    ON engine_schema_tables(project_id, datasource_id, schema_name, table_name)
    WHERE deleted_at IS NULL;

-- Query indexes
CREATE INDEX idx_engine_schema_tables_project ON engine_schema_tables(project_id);
CREATE INDEX idx_engine_schema_tables_datasource ON engine_schema_tables(project_id, datasource_id);

-- RLS
ALTER TABLE engine_schema_tables ENABLE ROW LEVEL SECURITY;
CREATE POLICY schema_tables_access ON engine_schema_tables
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger (reuse existing function from migration 003)
CREATE TRIGGER update_engine_schema_tables_updated_at
    BEFORE UPDATE ON engine_schema_tables
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Table: engine_schema_columns
-- Stores discovered columns with statistics
-- ============================================================================

CREATE TABLE engine_schema_columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    schema_table_id UUID NOT NULL REFERENCES engine_schema_tables(id) ON DELETE CASCADE,
    column_name TEXT NOT NULL,
    data_type TEXT NOT NULL,
    is_nullable BOOLEAN NOT NULL,
    is_primary_key BOOLEAN NOT NULL DEFAULT false,
    is_selected BOOLEAN NOT NULL DEFAULT false,
    ordinal_position INTEGER NOT NULL,
    distinct_count BIGINT,
    null_count BIGINT,
    business_name TEXT,
    description TEXT,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Partial unique: same column can't exist twice per table (unless soft-deleted)
CREATE UNIQUE INDEX idx_engine_schema_columns_unique
    ON engine_schema_columns(schema_table_id, column_name)
    WHERE deleted_at IS NULL;

-- Query indexes
CREATE INDEX idx_engine_schema_columns_project ON engine_schema_columns(project_id);
CREATE INDEX idx_engine_schema_columns_table ON engine_schema_columns(schema_table_id);

-- RLS
ALTER TABLE engine_schema_columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY schema_columns_access ON engine_schema_columns
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger
CREATE TRIGGER update_engine_schema_columns_updated_at
    BEFORE UPDATE ON engine_schema_columns
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Table: engine_schema_relationships
-- Stores FK, inferred, and manual relationships between columns
-- ============================================================================

CREATE TABLE engine_schema_relationships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    source_table_id UUID NOT NULL REFERENCES engine_schema_tables(id) ON DELETE CASCADE,
    source_column_id UUID NOT NULL REFERENCES engine_schema_columns(id) ON DELETE CASCADE,
    target_table_id UUID NOT NULL REFERENCES engine_schema_tables(id) ON DELETE CASCADE,
    target_column_id UUID NOT NULL REFERENCES engine_schema_columns(id) ON DELETE CASCADE,
    relationship_type TEXT NOT NULL,
    cardinality TEXT NOT NULL DEFAULT 'unknown',
    confidence DECIMAL(4,3) NOT NULL DEFAULT 1.000 CHECK (confidence >= 0 AND confidence <= 1),
    inference_method TEXT,
    is_validated BOOLEAN NOT NULL DEFAULT false,
    validation_results JSONB,
    is_approved BOOLEAN,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Partial unique: same column pair can't have duplicate relationships (unless soft-deleted)
CREATE UNIQUE INDEX idx_engine_schema_relationships_unique
    ON engine_schema_relationships(source_column_id, target_column_id)
    WHERE deleted_at IS NULL;

-- Query indexes
CREATE INDEX idx_engine_schema_relationships_project ON engine_schema_relationships(project_id);
CREATE INDEX idx_engine_schema_relationships_source_table ON engine_schema_relationships(source_table_id);
CREATE INDEX idx_engine_schema_relationships_target_table ON engine_schema_relationships(target_table_id);

-- RLS
ALTER TABLE engine_schema_relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY schema_relationships_access ON engine_schema_relationships
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger
CREATE TRIGGER update_engine_schema_relationships_updated_at
    BEFORE UPDATE ON engine_schema_relationships
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
