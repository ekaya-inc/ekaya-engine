-- 012_ontology_provenance.down.sql
-- Remove provenance tracking from ontology elements

-- Drop column metadata table
DROP TABLE IF EXISTS engine_ontology_column_metadata;

-- Remove provenance columns from engine_entity_relationships
ALTER TABLE engine_entity_relationships
    DROP CONSTRAINT IF EXISTS engine_entity_relationships_created_by_check,
    DROP CONSTRAINT IF EXISTS engine_entity_relationships_updated_by_check,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS updated_at;

-- Remove provenance columns from engine_ontology_entities
ALTER TABLE engine_ontology_entities
    DROP CONSTRAINT IF EXISTS engine_ontology_entities_created_by_check,
    DROP CONSTRAINT IF EXISTS engine_ontology_entities_updated_by_check,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS updated_by;
