-- 022_remove_entity_concept.down.sql
-- This is a one-way migration - entity concept is removed for v1.0
--
-- To re-add entity support post-v1.0, create a new forward migration rather
-- than using this down migration. The entity concept will likely evolve
-- before being re-introduced.

-- Add entity_summaries column back to engine_ontologies
ALTER TABLE engine_ontologies ADD COLUMN IF NOT EXISTS entity_summaries jsonb;

-- Note: Entity tables (engine_ontology_entities, engine_ontology_entity_aliases,
-- engine_ontology_entity_key_columns, engine_entity_relationships) are NOT
-- recreated by this down migration. They would need to be recreated from
-- 005_ontology_core.up.sql definitions if truly needed.
--
-- This is intentional: the entity concept will be redesigned post-v1.0,
-- so blindly restoring the old schema would not be useful.
