-- Remove cardinality column from engine_entity_relationships

DROP INDEX IF EXISTS idx_engine_entity_relationships_cardinality;

ALTER TABLE engine_entity_relationships
DROP CONSTRAINT IF EXISTS engine_entity_relationships_cardinality_check;

ALTER TABLE engine_entity_relationships
DROP COLUMN IF EXISTS cardinality;
