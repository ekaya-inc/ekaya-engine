-- Remove association column from engine_entity_relationships
ALTER TABLE engine_entity_relationships
    DROP COLUMN association;
