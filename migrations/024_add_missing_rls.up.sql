-- Migration 024: Add missing RLS policies to ontology tables
-- Enables Row Level Security on five ontology-related tables that were missing it

-- ============================================================================
-- RLS for engine_entity_relationships
-- ============================================================================
ALTER TABLE engine_entity_relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_relationships_access ON engine_entity_relationships
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR ontology_id IN (
            SELECT id FROM engine_ontologies
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    )
    WITH CHECK (
        current_setting('app.current_project_id', true) IS NULL
        OR ontology_id IN (
            SELECT id FROM engine_ontologies
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- ============================================================================
-- RLS for engine_ontology_entities
-- ============================================================================
ALTER TABLE engine_ontology_entities ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_entities_access ON engine_ontology_entities
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    )
    WITH CHECK (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- ============================================================================
-- RLS for engine_ontology_entity_aliases
-- ============================================================================
ALTER TABLE engine_ontology_entity_aliases ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_aliases_access ON engine_ontology_entity_aliases
    FOR ALL
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

-- ============================================================================
-- RLS for engine_ontology_entity_key_columns
-- ============================================================================
ALTER TABLE engine_ontology_entity_key_columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_key_columns_access ON engine_ontology_entity_key_columns
    FOR ALL
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

-- ============================================================================
-- RLS for engine_ontology_entity_occurrences
-- ============================================================================
ALTER TABLE engine_ontology_entity_occurrences ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_occurrences_access ON engine_ontology_entity_occurrences
    FOR ALL
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
