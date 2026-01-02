-- Drop key columns table
DROP TABLE IF EXISTS engine_ontology_entity_key_columns;

-- Remove domain column from entities
ALTER TABLE engine_ontology_entities DROP COLUMN IF EXISTS domain;
