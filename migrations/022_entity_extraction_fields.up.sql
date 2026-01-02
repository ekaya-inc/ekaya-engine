-- Add domain column to entities for business domain categorization
ALTER TABLE engine_ontology_entities ADD COLUMN domain VARCHAR(100);

-- Create table for entity key columns (important business columns)
CREATE TABLE engine_ontology_entity_key_columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    column_name TEXT NOT NULL,
    synonyms JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_id, column_name)
);

-- Index for efficient lookups
CREATE INDEX idx_entity_key_columns_entity_id ON engine_ontology_entity_key_columns(entity_id);
