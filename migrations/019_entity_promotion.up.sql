-- Add promotion fields to engine_ontology_entities table.
-- These columns support the entity promotion model: distinguishing semantic entities
-- (promoted) from simple table metadata (demoted).

-- is_promoted: whether this entity should be included in default context responses.
-- Existing entities default to true for backwards compatibility.
ALTER TABLE engine_ontology_entities
ADD COLUMN IF NOT EXISTS is_promoted BOOLEAN NOT NULL DEFAULT true;

-- promotion_score: computed score from PromotionScore function (0-100).
-- NULL means not yet scored.
ALTER TABLE engine_ontology_entities
ADD COLUMN IF NOT EXISTS promotion_score INTEGER;

-- promotion_reasons: array of human-readable reasons explaining the promotion score.
ALTER TABLE engine_ontology_entities
ADD COLUMN IF NOT EXISTS promotion_reasons TEXT[];

COMMENT ON COLUMN engine_ontology_entities.is_promoted IS
'True if entity meets promotion criteria or was manually promoted. False for demoted entities.';

COMMENT ON COLUMN engine_ontology_entities.promotion_score IS
'Computed promotion score (0-100) from PromotionScore function.';

COMMENT ON COLUMN engine_ontology_entities.promotion_reasons IS
'Array of human-readable reasons explaining why the entity received its promotion score.';
