-- Migration 024 (down): Remove RLS policies from ontology tables

DROP POLICY IF EXISTS entity_occurrences_access ON engine_ontology_entity_occurrences;
ALTER TABLE engine_ontology_entity_occurrences DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS entity_key_columns_access ON engine_ontology_entity_key_columns;
ALTER TABLE engine_ontology_entity_key_columns DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS entity_aliases_access ON engine_ontology_entity_aliases;
ALTER TABLE engine_ontology_entity_aliases DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS ontology_entities_access ON engine_ontology_entities;
ALTER TABLE engine_ontology_entities DISABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS entity_relationships_access ON engine_entity_relationships;
ALTER TABLE engine_entity_relationships DISABLE ROW LEVEL SECURITY;
