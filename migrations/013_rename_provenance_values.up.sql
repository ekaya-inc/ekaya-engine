-- 013_rename_provenance_values.up.sql
-- Standardize provenance values: 'admin' -> 'manual', 'client' -> 'mcp'
-- This ensures consistent naming across all ontology elements

-- ============================================================================
-- Ontology Entities
-- ============================================================================

-- Update existing 'admin' values to 'manual'
UPDATE engine_ontology_entities SET created_by = 'manual' WHERE created_by = 'admin';
UPDATE engine_ontology_entities SET updated_by = 'manual' WHERE updated_by = 'admin';

-- Drop old constraints
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_created_by_check;
ALTER TABLE engine_ontology_entities DROP CONSTRAINT IF EXISTS engine_ontology_entities_updated_by_check;

-- Add new constraints with 'manual' instead of 'admin'
ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_created_by_check
    CHECK (created_by IN ('manual', 'mcp', 'inference'));

ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT engine_ontology_entities_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('manual', 'mcp', 'inference'));

COMMENT ON COLUMN engine_ontology_entities.created_by IS 'Source that created this entity: manual (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_ontology_entities.updated_by IS 'Source that last updated this entity';

-- ============================================================================
-- Entity Relationships
-- ============================================================================

-- Update existing 'admin' values to 'manual'
UPDATE engine_entity_relationships SET created_by = 'manual' WHERE created_by = 'admin';
UPDATE engine_entity_relationships SET updated_by = 'manual' WHERE updated_by = 'admin';

-- Drop old constraints
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_created_by_check;
ALTER TABLE engine_entity_relationships DROP CONSTRAINT IF EXISTS engine_entity_relationships_updated_by_check;

-- Add new constraints with 'manual' instead of 'admin'
ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_created_by_check
    CHECK (created_by IN ('manual', 'mcp', 'inference'));

ALTER TABLE engine_entity_relationships
    ADD CONSTRAINT engine_entity_relationships_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('manual', 'mcp', 'inference'));

COMMENT ON COLUMN engine_entity_relationships.created_by IS 'Source that created this relationship: manual (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_entity_relationships.updated_by IS 'Source that last updated this relationship';

-- ============================================================================
-- Column Metadata
-- ============================================================================

-- Update existing 'admin' values to 'manual'
UPDATE engine_ontology_column_metadata SET created_by = 'manual' WHERE created_by = 'admin';
UPDATE engine_ontology_column_metadata SET updated_by = 'manual' WHERE updated_by = 'admin';

-- Drop old constraints
ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_created_by_check;
ALTER TABLE engine_ontology_column_metadata DROP CONSTRAINT IF EXISTS engine_column_metadata_updated_by_check;

-- Add new constraints with 'manual' instead of 'admin'
ALTER TABLE engine_ontology_column_metadata
    ADD CONSTRAINT engine_column_metadata_created_by_check
    CHECK (created_by IN ('manual', 'mcp', 'inference'));

ALTER TABLE engine_ontology_column_metadata
    ADD CONSTRAINT engine_column_metadata_updated_by_check
    CHECK (updated_by IS NULL OR updated_by IN ('manual', 'mcp', 'inference'));

COMMENT ON COLUMN engine_ontology_column_metadata.created_by IS 'Source that created this metadata: manual (UI), mcp (Claude), inference (Engine)';
COMMENT ON COLUMN engine_ontology_column_metadata.updated_by IS 'Source that last updated this metadata';

-- ============================================================================
-- Business Glossary
-- ============================================================================

-- Update existing 'client' values to 'mcp'
UPDATE engine_business_glossary SET source = 'mcp' WHERE source = 'client';

-- Drop old constraint
ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_source_check;

-- Add new constraint with 'mcp' instead of 'client'
ALTER TABLE engine_business_glossary
    ADD CONSTRAINT engine_business_glossary_source_check
    CHECK (source IN ('inferred', 'manual', 'mcp'));

COMMENT ON COLUMN engine_business_glossary.source IS 'Origin: inferred (LLM during extraction), manual (UI), or mcp (Claude via MCP)';
