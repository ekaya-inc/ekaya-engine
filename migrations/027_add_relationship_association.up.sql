-- Add association column to engine_entity_relationships
ALTER TABLE engine_entity_relationships
    ADD COLUMN association VARCHAR(100);

COMMENT ON COLUMN engine_entity_relationships.association IS
    'Semantic association describing this direction of the relationship (e.g., "placed_by", "contains", "as host")';
