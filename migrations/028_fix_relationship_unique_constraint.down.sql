-- Migration 028 down: Restore original unique constraint

-- Drop the new constraint
ALTER TABLE engine_entity_relationships
    DROP CONSTRAINT engine_entity_relationships_unique_relationship;

-- Restore original constraint (will fail if there are now duplicate rows by the old definition)
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_ontology_id_source_entity_id_ta_key
    UNIQUE (ontology_id, source_entity_id, target_entity_id,
            source_column_schema, source_column_table, source_column_name);
