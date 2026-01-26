-- 027_provenance_rename.up.sql
-- Update provenance CHECK constraints from 'inference' to 'inferred'
-- Add composite FK constraints for created_by/updated_by -> engine_users
-- Add project_id to relationships for FK support

-- ============================================
-- engine_ontology_entities
-- ============================================

-- Update CHECK constraints to use 'inferred'
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_source_check;
ALTER TABLE engine_ontology_entities ADD CONSTRAINT engine_ontology_entities_source_check
    CHECK (source IN ('inferred', 'mcp', 'manual'));

ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_last_edit_source_check;
ALTER TABLE engine_ontology_entities ADD CONSTRAINT engine_ontology_entities_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual'));

-- Update default value
ALTER TABLE engine_ontology_entities ALTER COLUMN source SET DEFAULT 'inferred';

-- Add composite FK constraints
ALTER TABLE engine_ontology_entities
    DROP CONSTRAINT IF EXISTS engine_ontology_entities_created_by_fkey;
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_ontology_entities
    DROP CONSTRAINT IF EXISTS engine_ontology_entities_updated_by_fkey;
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);

-- ============================================
-- engine_entity_relationships
-- ============================================

-- Update CHECK constraints to use 'inferred'
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_source_check;
ALTER TABLE engine_entity_relationships ADD CONSTRAINT engine_entity_relationships_source_check
    CHECK (source IN ('inferred', 'mcp', 'manual'));

ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_last_edit_source_check;
ALTER TABLE engine_entity_relationships ADD CONSTRAINT engine_entity_relationships_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual'));

-- Update default value
ALTER TABLE engine_entity_relationships ALTER COLUMN source SET DEFAULT 'inferred';

-- Add project_id column (needed for composite FK)
ALTER TABLE engine_entity_relationships
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES engine_projects(id);

-- Backfill project_id from ontology
UPDATE engine_entity_relationships r
SET project_id = o.project_id
FROM engine_ontologies o
WHERE r.ontology_id = o.id AND r.project_id IS NULL;

-- Add composite FK constraints
ALTER TABLE engine_entity_relationships
    DROP CONSTRAINT IF EXISTS engine_entity_relationships_created_by_fkey;
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_entity_relationships
    DROP CONSTRAINT IF EXISTS engine_entity_relationships_updated_by_fkey;
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);

-- ============================================
-- engine_business_glossary
-- ============================================

-- Update CHECK constraint to use 'inferred' (glossary uses 'inferred' already but let's ensure consistency)
ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_source_check;
ALTER TABLE engine_business_glossary ADD CONSTRAINT engine_business_glossary_source_check
    CHECK (source IN ('inferred', 'mcp', 'manual'));

-- Update default value
ALTER TABLE engine_business_glossary ALTER COLUMN source SET DEFAULT 'inferred';

-- Update last_edit_source constraint
ALTER TABLE engine_business_glossary
    ADD COLUMN IF NOT EXISTS last_edit_source TEXT;
ALTER TABLE engine_business_glossary
    DROP CONSTRAINT IF EXISTS engine_business_glossary_last_edit_source_check;
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual'));

-- Add composite FK constraints
ALTER TABLE engine_business_glossary
    DROP CONSTRAINT IF EXISTS engine_business_glossary_created_by_fkey;
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_business_glossary
    DROP CONSTRAINT IF EXISTS engine_business_glossary_updated_by_fkey;
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);

-- ============================================
-- engine_project_knowledge
-- ============================================

-- Add provenance columns if not exists
ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'inferred';
ALTER TABLE engine_project_knowledge
    DROP CONSTRAINT IF EXISTS engine_project_knowledge_source_check;
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_source_check
    CHECK (source IN ('inferred', 'mcp', 'manual'));

ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS last_edit_source TEXT;
ALTER TABLE engine_project_knowledge
    DROP CONSTRAINT IF EXISTS engine_project_knowledge_last_edit_source_check;
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual'));

ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS created_by UUID;
ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS updated_by UUID;

-- Add composite FK constraints
ALTER TABLE engine_project_knowledge
    DROP CONSTRAINT IF EXISTS engine_project_knowledge_created_by_fkey;
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_project_knowledge
    DROP CONSTRAINT IF EXISTS engine_project_knowledge_updated_by_fkey;
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);

-- ============================================
-- engine_ontology_column_metadata
-- ============================================

-- Update CHECK constraints on source columns to use 'inferred'
-- Note: created_by/updated_by are UUIDs, not TEXT, so no CHECK constraints needed
ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_source_check;
ALTER TABLE engine_ontology_column_metadata ADD CONSTRAINT engine_column_metadata_source_check
    CHECK (source IN ('inferred', 'mcp', 'manual'));

ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_last_edit_source_check;
ALTER TABLE engine_ontology_column_metadata ADD CONSTRAINT engine_column_metadata_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual'));

-- Update default value
ALTER TABLE engine_ontology_column_metadata ALTER COLUMN source SET DEFAULT 'inferred';

-- ============================================
-- engine_audit_log
-- ============================================

-- Update CHECK constraint to use 'inferred'
ALTER TABLE engine_audit_log DROP CONSTRAINT IF EXISTS engine_audit_log_source_check;
ALTER TABLE engine_audit_log ADD CONSTRAINT engine_audit_log_source_check
    CHECK (source IN ('inferred', 'mcp', 'manual'));
