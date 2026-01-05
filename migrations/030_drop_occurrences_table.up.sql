-- Migration 030: Drop occurrences table
-- Occurrences are now computed at runtime from relationships
-- See FIX-remove-occurrences.md for rationale

DROP TABLE IF EXISTS engine_ontology_entity_occurrences;
