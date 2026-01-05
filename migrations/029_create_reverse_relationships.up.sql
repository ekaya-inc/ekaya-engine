-- Migration 029: Create reverse rows for existing relationships
-- For each existing relationship, create a reverse row with swapped source/target

-- Insert reverse relationships for all existing relationships
-- This creates bidirectional relationship storage where each direction has its own row
INSERT INTO engine_entity_relationships (
    ontology_id,
    source_entity_id,
    target_entity_id,
    source_column_schema,
    source_column_table,
    source_column_name,
    target_column_schema,
    target_column_table,
    target_column_name,
    detection_method,
    confidence,
    status,
    created_at,
    description,
    association
)
SELECT
    r1.ontology_id,
    r1.target_entity_id AS source_entity_id,        -- swap
    r1.source_entity_id AS target_entity_id,        -- swap
    r1.target_column_schema AS source_column_schema, -- swap
    r1.target_column_table AS source_column_table,   -- swap
    r1.target_column_name AS source_column_name,     -- swap
    r1.source_column_schema AS target_column_schema, -- swap
    r1.source_column_table AS target_column_table,   -- swap
    r1.source_column_name AS target_column_name,     -- swap
    r1.detection_method,
    r1.confidence,
    r1.status,
    r1.created_at,
    NULL AS description,  -- reverse direction gets its own description later
    NULL AS association   -- reverse direction gets its own association later
FROM engine_entity_relationships r1
WHERE NOT EXISTS (
    -- Only insert if reverse doesn't already exist
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
);
