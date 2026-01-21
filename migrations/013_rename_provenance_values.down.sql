-- 013_rename_provenance_values.down.sql
-- Revert provenance values: 'manual' -> 'admin', 'mcp' -> 'client'

-- ============================================================================
-- Ontology Entities
-- ============================================================================

-- Update 'manual' back to 'admin'
UPDATE engine_ontology_entities SET created_by = 'admin' WHERE created_by = 'manual';
UPDATE engine_ontology_entities SET updated_by = 'admin' WHERE updated_by = 'manual';

-- Drop new constraints
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_created_by_check;
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_updated_by_check;

-- Restore old constraints with 'admin'
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_created_by_check
    CHECK (created_by IN ('admin', 'mcp', 'inference'));

ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('admin', 'mcp', 'inference'));

COMMENT ON COLUMN engine_ontology_entities.created_by IS 'Source that created this entity: admin (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_ontology_entities.updated_by IS 'Source that last updated this entity';

-- ============================================================================
-- Entity Relationships
-- ============================================================================

-- Update 'manual' back to 'admin'
UPDATE engine_entity_relationships SET created_by = 'admin' WHERE created_by = 'manual';
UPDATE engine_entity_relationships SET updated_by = 'admin' WHERE updated_by = 'manual';

-- Drop new constraints
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_created_by_check;
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_updated_by_check;

-- Restore old constraints with 'admin'
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_created_by_check
    CHECK (created_by IN ('admin', 'mcp', 'inference'));

ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('admin', 'mcp', 'inference'));

COMMENT ON COLUMN engine_entity_relationships.created_by IS 'Source that created this relationship: admin (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_entity_relationships.updated_by IS 'Source that last updated this relationship';

-- ============================================================================
-- Column Metadata
-- ============================================================================

-- Update 'manual' back to 'admin'
UPDATE engine_ontology_column_metadata SET created_by = 'admin' WHERE created_by = 'manual';
UPDATE engine_ontology_column_metadata SET updated_by = 'admin' WHERE updated_by = 'manual';

-- Drop new constraints
ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_created_by_check;
ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_updated_by_check;

-- Restore old constraints with 'admin'
ALTER TABLE engine_ontology_column_metadata
    ADD CONSTRAINT engine_column_metadata_created_by_check
    CHECK (created_by IN ('admin', 'mcp', 'inference'));

ALTER TABLE engine_ontology_column_metadata
    ADD CONSTRAINT engine_column_metadata_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('admin', 'mcp', 'inference'));

COMMENT ON COLUMN engine_ontology_column_metadata.created_by IS 'Source that created this metadata: admin (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_ontology_column_metadata.updated_by IS 'Source that last updated this metadata';

-- ============================================================================
-- Business Glossary
-- ============================================================================

-- Update 'mcp' back to 'client'
UPDATE engine_business_glossary SET source = 'client' WHERE source = 'mcp';

-- Drop new constraint
ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_source_check;

-- Restore old constraint with 'client'
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_source_check
    CHECK (source IN ('inferred', 'manual', 'client'));

COMMENT ON COLUMN engine_business_glossary.source IS 'Origin: inferred (LLM during extraction), manual (UI), or client (MCP)';
