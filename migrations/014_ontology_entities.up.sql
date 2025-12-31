-- Migration 014: Rename entity tables and add soft delete + aliases
-- Renames engine_schema_entities -> engine_ontology_entities to align with ontology naming
-- Adds soft delete support and entity aliases table

-- ============================================================================
-- Step 1: Rename tables to ontology namespace
-- ============================================================================

-- Rename main entity table
ALTER TABLE engine_schema_entities RENAME TO engine_ontology_entities;

-- Rename occurrences table
ALTER TABLE engine_schema_entity_occurrences RENAME TO engine_ontology_entity_occurrences;

-- ============================================================================
-- Step 2: Add soft delete fields to entities table
-- ============================================================================

ALTER TABLE engine_ontology_entities
    ADD COLUMN is_deleted BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN deletion_reason TEXT;

-- Partial index for efficient querying of active entities
CREATE INDEX idx_engine_ontology_entities_active
    ON engine_ontology_entities(project_id)
    WHERE NOT is_deleted;

COMMENT ON COLUMN engine_ontology_entities.is_deleted IS
    'Soft delete flag - entities are never hard deleted';

COMMENT ON COLUMN engine_ontology_entities.deletion_reason IS
    'Optional reason why the entity was soft deleted';

-- ============================================================================
-- Step 3: Create entity aliases table
-- Allows entities to have multiple names for query matching
-- ============================================================================

CREATE TABLE engine_ontology_entity_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    source VARCHAR(50),  -- 'discovery', 'user', 'query'
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_id, alias)
);

-- Index for looking up aliases by entity
CREATE INDEX idx_engine_ontology_entity_aliases_entity
    ON engine_ontology_entity_aliases(entity_id);

-- Index for searching by alias text
CREATE INDEX idx_engine_ontology_entity_aliases_alias
    ON engine_ontology_entity_aliases(alias);

COMMENT ON TABLE engine_ontology_entity_aliases IS
    'Alternative names for entities, used for query matching and discovery';

COMMENT ON COLUMN engine_ontology_entity_aliases.alias IS
    'Alternative name for the entity (e.g., "customer" as alias for "user")';

COMMENT ON COLUMN engine_ontology_entity_aliases.source IS
    'How this alias was created: discovery (auto-detected), user (manually added), query (learned from queries)';
