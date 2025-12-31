-- Migration 017: Entity Relationships
-- Creates table for entity-to-entity relationships discovered from FK constraints

CREATE TABLE engine_entity_relationships (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    source_entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    target_entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,

    -- Source column (where the FK/reference is)
    source_column_schema VARCHAR(255) NOT NULL,
    source_column_table VARCHAR(255) NOT NULL,
    source_column_name VARCHAR(255) NOT NULL,

    -- Target column (the PK being referenced)
    target_column_schema VARCHAR(255) NOT NULL,
    target_column_table VARCHAR(255) NOT NULL,
    target_column_name VARCHAR(255) NOT NULL,

    -- Detection metadata
    detection_method VARCHAR(50) NOT NULL,  -- 'foreign_key', 'pk_match'
    confidence DECIMAL(3,2) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    status VARCHAR(20) NOT NULL DEFAULT 'confirmed',  -- 'confirmed', 'pending', 'rejected'

    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    -- Prevent duplicate relationships for the same column pair
    UNIQUE(ontology_id, source_entity_id, target_entity_id, source_column_schema, source_column_table, source_column_name)
);

-- Index for looking up relationships by ontology
CREATE INDEX idx_engine_entity_relationships_ontology
    ON engine_entity_relationships(ontology_id);

-- Index for looking up relationships by source entity
CREATE INDEX idx_engine_entity_relationships_source
    ON engine_entity_relationships(source_entity_id);

-- Index for looking up relationships by target entity
CREATE INDEX idx_engine_entity_relationships_target
    ON engine_entity_relationships(target_entity_id);

COMMENT ON TABLE engine_entity_relationships IS
    'Entity-to-entity relationships discovered from FK constraints or inferred from PK matching';

COMMENT ON COLUMN engine_entity_relationships.detection_method IS
    'How the relationship was discovered: foreign_key (from DB constraint) or pk_match (inferred)';

COMMENT ON COLUMN engine_entity_relationships.confidence IS
    '1.0 for FK constraints, 0.7-0.95 for inferred relationships';

COMMENT ON COLUMN engine_entity_relationships.status IS
    'confirmed (auto-accepted), pending (needs review), rejected (user declined)';
