-- 027_provenance_rename.up.sql
-- Unified provenance system for all ontology objects:
-- - source: method ('inferred', 'mcp', 'manual')
-- - last_edit_source: method of last edit
-- - created_by: user UUID who created
-- - updated_by: user UUID who last updated
-- All tables use composite FK (project_id, user_id) -> engine_users

-- ============================================
-- engine_ontology_entities
-- ============================================

-- Rename created_by (method) -> source
ALTER TABLE engine_ontology_entities
    DROP CONSTRAINT IF EXISTS engine_ontology_entities_created_by_check;
ALTER TABLE engine_ontology_entities
    RENAME COLUMN created_by TO source;
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_source_check
    CHECK (source IN ('manual', 'mcp', 'inferred'));

-- Rename updated_by (method) -> last_edit_source
ALTER TABLE engine_ontology_entities
    DROP CONSTRAINT IF EXISTS engine_ontology_entities_updated_by_check;
ALTER TABLE engine_ontology_entities
    RENAME COLUMN updated_by TO last_edit_source;
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('manual', 'mcp', 'inferred'));

-- Add user UUID columns with composite FK
ALTER TABLE engine_ontology_entities
    ADD COLUMN IF NOT EXISTS created_by UUID;
ALTER TABLE engine_ontology_entities
    ADD COLUMN IF NOT EXISTS updated_by UUID;
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);

-- ============================================
-- engine_entity_relationships
-- ============================================

-- Rename created_by (method) -> source
ALTER TABLE engine_entity_relationships
    DROP CONSTRAINT IF EXISTS engine_entity_relationships_created_by_check;
ALTER TABLE engine_entity_relationships
    RENAME COLUMN created_by TO source;
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_source_check
    CHECK (source IN ('manual', 'mcp', 'inferred'));

-- Rename updated_by (method) -> last_edit_source
ALTER TABLE engine_entity_relationships
    DROP CONSTRAINT IF EXISTS engine_entity_relationships_updated_by_check;
ALTER TABLE engine_entity_relationships
    RENAME COLUMN updated_by TO last_edit_source;
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('manual', 'mcp', 'inferred'));

-- Add user UUID columns with composite FK
-- Note: relationships have ontology_id, need to get project_id via ontology
ALTER TABLE engine_entity_relationships
    ADD COLUMN IF NOT EXISTS project_id UUID REFERENCES engine_projects(id);
ALTER TABLE engine_entity_relationships
    ADD COLUMN IF NOT EXISTS created_by UUID;
ALTER TABLE engine_entity_relationships
    ADD COLUMN IF NOT EXISTS updated_by UUID;

-- Backfill project_id from ontology
UPDATE engine_entity_relationships r
SET project_id = o.project_id
FROM engine_ontologies o
WHERE r.ontology_id = o.id AND r.project_id IS NULL;

-- Now add the FK constraints
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);

-- ============================================
-- engine_business_glossary
-- ============================================

-- Fix inconsistent source value: 'inferred' -> 'inferred'
UPDATE engine_business_glossary SET source = 'inferred' WHERE source = 'inferred';
ALTER TABLE engine_business_glossary
    DROP CONSTRAINT IF EXISTS engine_business_glossary_source_check;
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_source_check
    CHECK (source IN ('manual', 'mcp', 'inferred'));

-- Add last_edit_source
ALTER TABLE engine_business_glossary
    ADD COLUMN IF NOT EXISTS last_edit_source TEXT;
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('manual', 'mcp', 'inferred'));

-- Add composite FK for existing created_by/updated_by
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);

-- ============================================
-- engine_project_knowledge
-- ============================================

-- Add all provenance columns
ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'inferred';
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_source_check
    CHECK (source IN ('manual', 'mcp', 'inferred'));

ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS last_edit_source TEXT;
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('manual', 'mcp', 'inferred'));

ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS created_by UUID;
ALTER TABLE engine_project_knowledge
    ADD COLUMN IF NOT EXISTS updated_by UUID;
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_created_by_fkey
    FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id);
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_updated_by_fkey
    FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);
