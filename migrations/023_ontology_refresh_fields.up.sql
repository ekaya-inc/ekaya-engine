-- 021_ontology_refresh_fields.up.sql
-- Add fields needed for ontology refresh: confidence on entities, is_stale flag
-- This enables incremental refresh where non-stale items are preserved.

-- Add confidence to entities (relationships already have it)
-- Lower default (0.5) for entities since they may be inferred vs FK-derived
ALTER TABLE engine_ontology_entities
    ADD COLUMN IF NOT EXISTS confidence numeric(3,2) DEFAULT 0.5 NOT NULL;

ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_confidence_check
    CHECK (confidence >= 0 AND confidence <= 1);

COMMENT ON COLUMN engine_ontology_entities.confidence IS 'Confidence score 0.0-1.0: higher for FK-derived, lower for LLM-inferred';

-- Add is_stale flag to entities
-- Stale items need re-evaluation during refresh but are not deleted
ALTER TABLE engine_ontology_entities
    ADD COLUMN IF NOT EXISTS is_stale boolean DEFAULT false NOT NULL;

COMMENT ON COLUMN engine_ontology_entities.is_stale IS 'True when schema has changed and this entity needs re-evaluation';

-- Add is_stale flag to relationships
ALTER TABLE engine_entity_relationships
    ADD COLUMN IF NOT EXISTS is_stale boolean DEFAULT false NOT NULL;

COMMENT ON COLUMN engine_entity_relationships.is_stale IS 'True when schema has changed and this relationship needs re-evaluation';

-- Index for efficiently finding stale items during refresh
CREATE INDEX idx_engine_ontology_entities_stale ON engine_ontology_entities(ontology_id) WHERE is_stale = true;
CREATE INDEX idx_engine_entity_relationships_stale ON engine_entity_relationships(ontology_id) WHERE is_stale = true;
