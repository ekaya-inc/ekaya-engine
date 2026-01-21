-- 011_pending_changes.up.sql
-- Pending ontology changes for review workflow

CREATE TABLE engine_ontology_pending_changes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,

    -- Change classification
    change_type text NOT NULL,
    change_source text NOT NULL DEFAULT 'schema_refresh',

    -- What changed
    table_name text,
    column_name text,
    old_value jsonb,
    new_value jsonb,

    -- Suggested ontology action
    suggested_action text,
    suggested_payload jsonb,

    -- Review state
    status text NOT NULL DEFAULT 'pending',
    reviewed_by text,
    reviewed_at timestamp with time zone,

    -- Metadata
    created_at timestamp with time zone DEFAULT now() NOT NULL,

    PRIMARY KEY (id),
    CONSTRAINT engine_pending_changes_change_type_check CHECK (change_type IN (
        'new_table', 'dropped_table',
        'new_column', 'dropped_column', 'modified_column',
        'new_enum_value', 'cardinality_change', 'new_fk_pattern'
    )),
    CONSTRAINT engine_pending_changes_change_source_check CHECK (change_source IN (
        'schema_refresh', 'data_scan', 'manual'
    )),
    CONSTRAINT engine_pending_changes_status_check CHECK (status IN (
        'pending', 'approved', 'rejected', 'auto_applied'
    )),
    CONSTRAINT engine_pending_changes_project_id_fkey FOREIGN KEY (project_id)
        REFERENCES engine_projects(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_ontology_pending_changes IS 'Pending schema/data changes for ontology review before applying';
COMMENT ON COLUMN engine_ontology_pending_changes.change_type IS 'Type of change: new_table, dropped_table, new_column, dropped_column, modified_column, etc.';
COMMENT ON COLUMN engine_ontology_pending_changes.change_source IS 'Origin: schema_refresh (DDL sync), data_scan (data analysis), manual';
COMMENT ON COLUMN engine_ontology_pending_changes.table_name IS 'Affected table name (schema.table format)';
COMMENT ON COLUMN engine_ontology_pending_changes.column_name IS 'Affected column name (for column-level changes)';
COMMENT ON COLUMN engine_ontology_pending_changes.old_value IS 'Previous state (type, enum values, etc.)';
COMMENT ON COLUMN engine_ontology_pending_changes.new_value IS 'New state after the change';
COMMENT ON COLUMN engine_ontology_pending_changes.suggested_action IS 'Recommended ontology action: create_entity, update_entity, create_column_metadata, etc.';
COMMENT ON COLUMN engine_ontology_pending_changes.suggested_payload IS 'Parameters for the suggested action';
COMMENT ON COLUMN engine_ontology_pending_changes.status IS 'Review state: pending, approved, rejected, auto_applied';
COMMENT ON COLUMN engine_ontology_pending_changes.reviewed_by IS 'Who reviewed: admin, mcp, auto';

-- Indexes
CREATE INDEX idx_pending_changes_project_status ON engine_ontology_pending_changes(project_id, status);
CREATE INDEX idx_pending_changes_project_created ON engine_ontology_pending_changes(project_id, created_at DESC);
CREATE INDEX idx_pending_changes_change_type ON engine_ontology_pending_changes(change_type);

-- RLS
ALTER TABLE engine_ontology_pending_changes ENABLE ROW LEVEL SECURITY;
CREATE POLICY pending_changes_access ON engine_ontology_pending_changes FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
