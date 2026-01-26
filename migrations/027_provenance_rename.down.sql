-- 027_provenance_rename.down.sql
-- Revert: remove composite FKs, project_id from relationships, revert to 'inference'

-- ============================================
-- engine_audit_log
-- ============================================
ALTER TABLE engine_audit_log DROP CONSTRAINT IF EXISTS engine_audit_log_source_check;
ALTER TABLE engine_audit_log ADD CONSTRAINT engine_audit_log_source_check
    CHECK (source IN ('inference', 'mcp', 'manual'));

-- ============================================
-- engine_ontology_column_metadata
-- ============================================
ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_source_check;
ALTER TABLE engine_ontology_column_metadata ADD CONSTRAINT engine_column_metadata_source_check
    CHECK (source IN ('inference', 'mcp', 'manual'));

ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_last_edit_source_check;
ALTER TABLE engine_ontology_column_metadata ADD CONSTRAINT engine_column_metadata_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inference', 'mcp', 'manual'));

ALTER TABLE engine_ontology_column_metadata ALTER COLUMN source SET DEFAULT 'inference';

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

ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_source_check;
ALTER TABLE engine_business_glossary ADD CONSTRAINT engine_business_glossary_source_check
    CHECK (source IN ('inference', 'mcp', 'manual'));
ALTER TABLE engine_business_glossary ALTER COLUMN source SET DEFAULT 'inference';

-- ============================================
-- engine_entity_relationships
-- ============================================
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_updated_by_fkey;
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_created_by_fkey;
ALTER TABLE engine_entity_relationships DROP COLUMN IF EXISTS project_id;

ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_source_check;
ALTER TABLE engine_entity_relationships ADD CONSTRAINT engine_entity_relationships_source_check
    CHECK (source IN ('inference', 'mcp', 'manual'));

ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_last_edit_source_check;
ALTER TABLE engine_entity_relationships ADD CONSTRAINT engine_entity_relationships_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inference', 'mcp', 'manual'));

ALTER TABLE engine_entity_relationships ALTER COLUMN source SET DEFAULT 'inference';

-- ============================================
-- engine_ontology_entities
-- ============================================
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_updated_by_fkey;
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_created_by_fkey;

ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_source_check;
ALTER TABLE engine_ontology_entities ADD CONSTRAINT engine_ontology_entities_source_check
    CHECK (source IN ('inference', 'mcp', 'manual'));

ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_last_edit_source_check;
ALTER TABLE engine_ontology_entities ADD CONSTRAINT engine_ontology_entities_last_edit_source_check
    CHECK (last_edit_source IS NULL OR last_edit_source IN ('inference', 'mcp', 'manual'));

ALTER TABLE engine_ontology_entities ALTER COLUMN source SET DEFAULT 'inference';
