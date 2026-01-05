-- Migration 030 down: Recreate occurrences table
-- Recreates table structure from migration 013

CREATE TABLE engine_ontology_entity_occurrences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    schema_name TEXT NOT NULL,
    table_name TEXT NOT NULL,
    column_name TEXT NOT NULL,
    role TEXT,
    confidence FLOAT NOT NULL DEFAULT 1.0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_id, schema_name, table_name, column_name),
    CHECK (confidence >= 0.0 AND confidence <= 1.0)
);

-- Index for finding occurrences by entity
CREATE INDEX idx_engine_ontology_entity_occurrences_entity
    ON engine_ontology_entity_occurrences(entity_id);

-- Index for finding occurrences by table (for join path discovery)
CREATE INDEX idx_engine_ontology_entity_occurrences_table
    ON engine_ontology_entity_occurrences(schema_name, table_name);

-- Index for finding occurrences by role (for semantic queries)
CREATE INDEX idx_engine_ontology_entity_occurrences_role
    ON engine_ontology_entity_occurrences(role)
    WHERE role IS NOT NULL;

COMMENT ON TABLE engine_ontology_entity_occurrences IS
    'Tracks where each entity appears across the schema, with optional role semantics';

COMMENT ON COLUMN engine_ontology_entity_occurrences.role IS
    'Optional semantic role (e.g., "visitor", "host", "owner") or NULL for generic references';

COMMENT ON COLUMN engine_ontology_entity_occurrences.confidence IS
    'LLM confidence score (0.0-1.0) for this entity-column mapping';

-- Add RLS policy (from migration 024)
ALTER TABLE engine_ontology_entity_occurrences ENABLE ROW LEVEL SECURITY;

CREATE POLICY entity_occurrences_access ON engine_ontology_entity_occurrences
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR entity_id IN (
            SELECT id FROM engine_ontology_entities
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    )
    WITH CHECK (
        current_setting('app.current_project_id', true) IS NULL
        OR entity_id IN (
            SELECT id FROM engine_ontology_entities
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    );
