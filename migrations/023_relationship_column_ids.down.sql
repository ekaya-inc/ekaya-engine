-- 023_relationship_column_ids.down.sql
-- Rollback relationship column ID foreign keys

-- Drop indexes
DROP INDEX IF EXISTS idx_entity_rel_source_col;
DROP INDEX IF EXISTS idx_entity_rel_target_col;

-- Drop columns
ALTER TABLE engine_entity_relationships
    DROP COLUMN IF EXISTS source_column_id;

ALTER TABLE engine_entity_relationships
    DROP COLUMN IF EXISTS target_column_id;
