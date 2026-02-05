-- 020_schema_columns_refactor.up.sql
-- Refactor engine_schema_columns to remove semantic/ontology fields
--
-- This migration drops and recreates engine_schema_columns with a cleaner schema:
-- - Removes: business_name, description, metadata, is_sensitive, sample_values
-- - Keeps: schema discovery fields + data stats
-- - Semantic enrichment moves to engine_ontology_column_metadata (task 1.2)
--
-- WARNING: This is a destructive migration. All column data will be lost.
-- After migration, run schema discovery to repopulate.

-- First, drop dependent tables that have FK references to engine_schema_columns
-- engine_schema_relationships has ON DELETE CASCADE, but we drop it to recreate cleanly
DROP TABLE IF EXISTS engine_schema_relationships CASCADE;

-- engine_entity_relationships has source_column_id/target_column_id with ON DELETE SET NULL
-- We need to temporarily drop the FK constraints, then re-add them
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_source_column_id_fkey;
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_target_column_id_fkey;

-- Now drop and recreate engine_schema_columns
DROP TABLE IF EXISTS engine_schema_columns CASCADE;

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

-- Recreate engine_schema_relationships (was dropped above)
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

-- Re-add FK constraints to engine_entity_relationships
-- These columns may contain invalid IDs after this migration, so we SET NULL
UPDATE engine_entity_relationships SET source_column_id = NULL WHERE source_column_id IS NOT NULL;
UPDATE engine_entity_relationships SET target_column_id = NULL WHERE target_column_id IS NOT NULL;

ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_source_column_id_fkey
    FOREIGN KEY (source_column_id) REFERENCES engine_schema_columns(id) ON DELETE SET NULL;

ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_target_column_id_fkey
    FOREIGN KEY (target_column_id) REFERENCES engine_schema_columns(id) ON DELETE SET NULL;
