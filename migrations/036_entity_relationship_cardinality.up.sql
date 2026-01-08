-- Add cardinality column to engine_entity_relationships
-- This stores the cardinality classification for entity relationships (e.g., "1:1", "1:N", "N:1", "N:N", "unknown")
-- Cardinality helps AI agents understand the nature of relationships between entities

ALTER TABLE engine_entity_relationships
ADD COLUMN cardinality TEXT NOT NULL DEFAULT 'unknown';

-- Add check constraint to ensure valid cardinality values
ALTER TABLE engine_entity_relationships
ADD CONSTRAINT engine_entity_relationships_cardinality_check
CHECK (cardinality IN ('1:1', '1:N', 'N:1', 'N:N', 'unknown'));

-- Create index for cardinality filtering
CREATE INDEX idx_engine_entity_relationships_cardinality ON engine_entity_relationships(cardinality);
