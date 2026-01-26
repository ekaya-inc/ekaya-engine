-- 021_ontology_refresh_fields.down.sql
-- Rollback ontology refresh fields

-- Drop indexes first
DROP INDEX IF EXISTS idx_engine_ontology_entities_stale;
DROP INDEX IF EXISTS idx_engine_entity_relationships_stale;

-- Remove is_stale from relationships
ALTER TABLE engine_entity_relationships
    DROP COLUMN IF EXISTS is_stale;

-- Remove is_stale from entities
ALTER TABLE engine_ontology_entities
    DROP COLUMN IF EXISTS is_stale;

-- Remove confidence from entities
ALTER TABLE engine_ontology_entities
    DROP CONSTRAINT IF EXISTS engine_ontology_entities_confidence_check;

ALTER TABLE engine_ontology_entities
    DROP COLUMN IF EXISTS confidence;
