-- Revert cardinality constraint: change N:M back to N:N

-- Revert N:M back to N:N
UPDATE engine_entity_relationships SET cardinality = 'N:N' WHERE cardinality = 'N:M';

ALTER TABLE engine_entity_relationships
DROP CONSTRAINT IF EXISTS engine_entity_relationships_cardinality_check;

ALTER TABLE engine_entity_relationships
ADD CONSTRAINT engine_entity_relationships_cardinality_check
CHECK (cardinality IN ('1:1', '1:N', 'N:1', 'N:N', 'unknown'));
