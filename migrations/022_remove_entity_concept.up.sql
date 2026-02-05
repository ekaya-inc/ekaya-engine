-- 022_remove_entity_concept.up.sql
-- Remove all entity-related tables for v1.0 launch
--
-- The Entity concept (domain entities like "User", "Account", "Order" discovered
-- from schema analysis) is being deferred to post-v1.0. v1.0 focuses on
-- schema-based column and table enrichment without the entity abstraction layer.

-- Drop tables in dependency order (children first)
-- engine_ontology_entity_key_columns has FK to engine_ontology_entities
DROP TABLE IF EXISTS engine_ontology_entity_key_columns CASCADE;

-- engine_ontology_entity_aliases has FK to engine_ontology_entities
DROP TABLE IF EXISTS engine_ontology_entity_aliases CASCADE;

-- engine_entity_relationships has FKs to engine_ontology_entities
DROP TABLE IF EXISTS engine_entity_relationships CASCADE;

-- engine_ontology_entities is the parent table
DROP TABLE IF EXISTS engine_ontology_entities CASCADE;

-- Remove entity_summaries column from engine_ontologies
-- This column stored JSONB summaries of entities which are no longer used
ALTER TABLE engine_ontologies DROP COLUMN IF EXISTS entity_summaries;
