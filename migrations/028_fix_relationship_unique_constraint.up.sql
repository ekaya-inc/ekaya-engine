-- Migration 028: Fix relationship unique constraint to support bidirectional relationships
-- The old constraint didn't include target column names, which prevents proper bidirectional storage
-- when multiple columns from the same source table reference the same target table.

-- Drop the old constraint
ALTER TABLE engine_entity_relationships
    DROP CONSTRAINT engine_entity_relationships_ontology_id_source_entity_id_ta_key;

-- Create new constraint that includes both source and target column names
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_unique_relationship
    UNIQUE (ontology_id, source_entity_id, target_entity_id,
            source_column_schema, source_column_table, source_column_name,
            target_column_schema, target_column_table, target_column_name);

COMMENT ON CONSTRAINT engine_entity_relationships_unique_relationship ON engine_entity_relationships IS
    'Ensures each specific column-to-column relationship is stored once per direction. Includes target columns to support multiple FKs from same source table.';
