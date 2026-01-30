-- Revert entity promotion columns from engine_ontology_entities table.

ALTER TABLE engine_ontology_entities DROP COLUMN IF EXISTS promotion_reasons;
ALTER TABLE engine_ontology_entities DROP COLUMN IF EXISTS promotion_score;
ALTER TABLE engine_ontology_entities DROP COLUMN IF EXISTS is_promoted;
