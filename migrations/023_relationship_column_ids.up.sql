-- 023_relationship_column_ids.up.sql
-- Add foreign key references to schema columns for entity relationships.
-- This enables JOINing to get column types without duplicating type data.

-- Add source_column_id with FK to engine_schema_columns
ALTER TABLE engine_entity_relationships
    ADD COLUMN IF NOT EXISTS source_column_id UUID REFERENCES engine_schema_columns(id) ON DELETE SET NULL;

-- Add target_column_id with FK to engine_schema_columns
ALTER TABLE engine_entity_relationships
    ADD COLUMN IF NOT EXISTS target_column_id UUID REFERENCES engine_schema_columns(id) ON DELETE SET NULL;

-- Indexes for efficient JOINs when fetching column types
CREATE INDEX IF NOT EXISTS idx_entity_rel_source_col ON engine_entity_relationships(source_column_id);
CREATE INDEX IF NOT EXISTS idx_entity_rel_target_col ON engine_entity_relationships(target_column_id);

COMMENT ON COLUMN engine_entity_relationships.source_column_id IS 'FK to engine_schema_columns for source column; allows JOIN to get column type';
COMMENT ON COLUMN engine_entity_relationships.target_column_id IS 'FK to engine_schema_columns for target column; allows JOIN to get column type';
