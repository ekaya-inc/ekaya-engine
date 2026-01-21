-- 012_ontology_provenance.up.sql
-- Add provenance tracking (created_by, updated_by) to ontology elements
-- for precedence model: Admin > MCP (Claude) > Inference (Engine)

-- Add provenance columns to engine_ontology_entities
ALTER TABLE engine_ontology_entities
    ADD COLUMN created_by text NOT NULL DEFAULT 'inference',
    ADD COLUMN updated_by text;

ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_created_by_check
    CHECK (created_by IN ('admin', 'mcp', 'inference'));

ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('admin', 'mcp', 'inference'));

COMMENT ON COLUMN engine_ontology_entities.created_by IS 'Source that created this entity: admin (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_ontology_entities.updated_by IS 'Source that last updated this entity';

-- Add provenance columns to engine_entity_relationships
ALTER TABLE engine_entity_relationships
    ADD COLUMN created_by text NOT NULL DEFAULT 'inference',
    ADD COLUMN updated_by text,
    ADD COLUMN updated_at timestamp with time zone;

ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_created_by_check
    CHECK (created_by IN ('admin', 'mcp', 'inference'));

ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('admin', 'mcp', 'inference'));

COMMENT ON COLUMN engine_entity_relationships.created_by IS 'Source that created this relationship: admin (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_entity_relationships.updated_by IS 'Source that last updated this relationship';
COMMENT ON COLUMN engine_entity_relationships.updated_at IS 'When this relationship was last updated';

-- Create column metadata table for storing semantic annotations per column
-- This provides finer-grained provenance than the ontologies.column_details JSONB
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

    -- Provenance
    created_by text NOT NULL DEFAULT 'inference',
    updated_by text,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone,

    PRIMARY KEY (id),
    CONSTRAINT engine_column_metadata_unique UNIQUE (project_id, table_name, column_name),
    CONSTRAINT engine_column_metadata_project_id_fkey FOREIGN KEY (project_id)
        REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_column_metadata_created_by_check
        CHECK (created_by IN ('admin', 'mcp', 'inference')),
    CONSTRAINT engine_column_metadata_updated_by_check
        CHECK (updated_by IS NULL OR updated_by IN ('admin', 'mcp', 'inference')),
    CONSTRAINT engine_column_metadata_role_check
        CHECK (role IS NULL OR role IN ('dimension', 'measure', 'identifier', 'attribute'))
);

COMMENT ON TABLE engine_ontology_column_metadata IS 'Column-level semantic annotations with provenance tracking';
COMMENT ON COLUMN engine_ontology_column_metadata.entity IS 'Entity this column belongs to (e.g., User, Account)';
COMMENT ON COLUMN engine_ontology_column_metadata.role IS 'Semantic role: dimension (group by), measure (aggregate), identifier (PK/FK), attribute (other)';
COMMENT ON COLUMN engine_ontology_column_metadata.enum_values IS 'Array of enum values with descriptions, e.g., ["ACTIVE - Normal account", "SUSPENDED - Temp hold"]';
COMMENT ON COLUMN engine_ontology_column_metadata.created_by IS 'Source that created this metadata: admin (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_ontology_column_metadata.updated_by IS 'Source that last updated this metadata';

-- Indexes
CREATE INDEX idx_column_metadata_project ON engine_ontology_column_metadata(project_id);
CREATE INDEX idx_column_metadata_table ON engine_ontology_column_metadata(project_id, table_name);

-- RLS
ALTER TABLE engine_ontology_column_metadata ENABLE ROW LEVEL SECURITY;
CREATE POLICY column_metadata_access ON engine_ontology_column_metadata FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
