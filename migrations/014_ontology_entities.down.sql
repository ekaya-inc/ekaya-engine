-- Migration 014 rollback: Revert entity table renames and remove aliases

-- ============================================================================
-- Step 1: Drop aliases table
-- ============================================================================

DROP TABLE IF EXISTS engine_ontology_entity_aliases;

-- ============================================================================
-- Step 2: Remove soft delete fields from entities table
-- ============================================================================

-- Drop the partial index first
DROP INDEX IF EXISTS idx_engine_ontology_entities_active;

-- Remove soft delete columns
ALTER TABLE engine_ontology_entities
    DROP COLUMN IF EXISTS deletion_reason,
    DROP COLUMN IF EXISTS is_deleted;

-- ============================================================================
-- Step 3: Rename tables back to schema namespace
-- ============================================================================

-- Rename occurrences table back
ALTER TABLE engine_ontology_entity_occurrences RENAME TO engine_schema_entity_occurrences;

-- Rename main entity table back
ALTER TABLE engine_ontology_entities RENAME TO engine_schema_entities;
