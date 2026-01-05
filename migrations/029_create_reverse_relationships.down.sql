-- Migration 029 down: Remove reverse relationships created by this migration
-- This removes relationships where the reverse relationship exists,
-- keeping only the "original" direction (arbitrary choice: keep where source_entity_id < target_entity_id)

DELETE FROM engine_entity_relationships r1
WHERE EXISTS (
    SELECT 1 FROM engine_entity_relationships r2
    WHERE r2.ontology_id = r1.ontology_id
      AND r2.source_entity_id = r1.target_entity_id
      AND r2.target_entity_id = r1.source_entity_id
      AND r2.source_column_schema = r1.target_column_schema
      AND r2.source_column_table = r1.target_column_table
      AND r2.source_column_name = r1.target_column_name
      AND r2.target_column_schema = r1.source_column_schema
      AND r2.target_column_table = r1.source_column_table
      AND r2.target_column_name = r1.source_column_name
      AND r1.source_entity_id > r1.target_entity_id  -- Keep only one direction
);
