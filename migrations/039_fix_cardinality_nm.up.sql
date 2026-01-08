-- Fix cardinality constraint: replace N:N with N:M for many-to-many relationships
-- N:M is clearer notation (N:N looks confusingly like 1:1)

-- Update any existing N:N values to N:M
UPDATE engine_entity_relationships SET cardinality = 'N:M' WHERE cardinality = 'N:N';

-- Drop old constraint and add new one with N:M instead of N:N
ALTER TABLE engine_entity_relationships
DROP CONSTRAINT IF EXISTS engine_entity_relationships_cardinality_check;

ALTER TABLE engine_entity_relationships
ADD CONSTRAINT engine_entity_relationships_cardinality_check
CHECK (cardinality IN ('1:1', '1:N', 'N:1', 'N:M', 'unknown'));
