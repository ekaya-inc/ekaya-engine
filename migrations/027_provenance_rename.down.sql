-- 027_provenance_rename.down.sql
-- Revert unified provenance system changes

-- ============================================
-- engine_project_knowledge
-- ============================================
ALTER TABLE engine_project_knowledge DROP CONSTRAINT IF EXISTS engine_project_knowledge_updated_by_fkey;
ALTER TABLE engine_project_knowledge DROP CONSTRAINT IF EXISTS engine_project_knowledge_created_by_fkey;
ALTER TABLE engine_project_knowledge DROP CONSTRAINT IF EXISTS engine_project_knowledge_last_edit_source_check;
ALTER TABLE engine_project_knowledge DROP CONSTRAINT IF EXISTS engine_project_knowledge_source_check;
ALTER TABLE engine_project_knowledge DROP COLUMN IF EXISTS updated_by;
ALTER TABLE engine_project_knowledge DROP COLUMN IF EXISTS created_by;
ALTER TABLE engine_project_knowledge DROP COLUMN IF EXISTS last_edit_source;
ALTER TABLE engine_project_knowledge DROP COLUMN IF EXISTS source;

-- ============================================
-- engine_business_glossary
-- ============================================
ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_updated_by_fkey;
ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_created_by_fkey;
ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_last_edit_source_check;
ALTER TABLE engine_business_glossary DROP COLUMN IF EXISTS last_edit_source;
-- Note: source, created_by, updated_by existed before this migration

-- ============================================
-- engine_entity_relationships
-- ============================================
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_updated_by_fkey;
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_created_by_fkey;
ALTER TABLE engine_entity_relationships DROP COLUMN IF EXISTS updated_by;
ALTER TABLE engine_entity_relationships DROP COLUMN IF EXISTS created_by;
ALTER TABLE engine_entity_relationships DROP COLUMN IF EXISTS project_id;

-- Rename last_edit_source -> updated_by
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_last_edit_source_check;
ALTER TABLE engine_entity_relationships RENAME COLUMN last_edit_source TO updated_by;
ALTER TABLE engine_entity_relationships ADD CONSTRAINT engine_entity_relationships_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('manual', 'mcp', 'inferred'));

-- Rename source -> created_by
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_source_check;
ALTER TABLE engine_entity_relationships RENAME COLUMN source TO created_by;
ALTER TABLE engine_entity_relationships ADD CONSTRAINT engine_entity_relationships_created_by_check
    CHECK (created_by IN ('manual', 'mcp', 'inferred'));

-- ============================================
-- engine_ontology_entities
-- ============================================
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_updated_by_fkey;
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_created_by_fkey;
ALTER TABLE engine_ontology_entities DROP COLUMN IF EXISTS updated_by;
ALTER TABLE engine_ontology_entities DROP COLUMN IF EXISTS created_by;

-- Rename last_edit_source -> updated_by
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_last_edit_source_check;
ALTER TABLE engine_ontology_entities RENAME COLUMN last_edit_source TO updated_by;
ALTER TABLE engine_ontology_entities ADD CONSTRAINT engine_ontology_entities_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('manual', 'mcp', 'inferred'));

-- Rename source -> created_by
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_source_check;
ALTER TABLE engine_ontology_entities RENAME COLUMN source TO created_by;
ALTER TABLE engine_ontology_entities ADD CONSTRAINT engine_ontology_entities_created_by_check
    CHECK (created_by IN ('manual', 'mcp', 'inferred'));
