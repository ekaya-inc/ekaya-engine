-- Migration 006 DOWN: Remove relationship discovery columns

-- Drop indexes first
DROP INDEX IF EXISTS idx_engine_schema_relationships_rejection;
DROP INDEX IF EXISTS idx_engine_schema_columns_joinable;

-- Remove columns from engine_schema_columns
ALTER TABLE engine_schema_columns
    DROP COLUMN IF EXISTS stats_updated_at;

ALTER TABLE engine_schema_columns
    DROP COLUMN IF EXISTS joinability_reason;

ALTER TABLE engine_schema_columns
    DROP COLUMN IF EXISTS is_joinable;

ALTER TABLE engine_schema_columns
    DROP COLUMN IF EXISTS non_null_count;

ALTER TABLE engine_schema_columns
    DROP COLUMN IF EXISTS row_count;

-- Remove columns from engine_schema_relationships
ALTER TABLE engine_schema_relationships
    DROP COLUMN IF EXISTS rejection_reason;

ALTER TABLE engine_schema_relationships
    DROP COLUMN IF EXISTS matched_count;

ALTER TABLE engine_schema_relationships
    DROP COLUMN IF EXISTS target_distinct;

ALTER TABLE engine_schema_relationships
    DROP COLUMN IF EXISTS source_distinct;

ALTER TABLE engine_schema_relationships
    DROP COLUMN IF EXISTS match_rate;
