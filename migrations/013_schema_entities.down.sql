-- Migration 013 rollback: Remove schema entity tables

-- Drop tables (cascade will handle foreign key constraints)
DROP TABLE IF EXISTS engine_schema_entity_occurrences;
DROP TABLE IF EXISTS engine_schema_entities;
