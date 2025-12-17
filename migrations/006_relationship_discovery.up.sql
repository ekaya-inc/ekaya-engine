-- Migration 006: Add columns for relationship discovery tracking
-- Extends engine_schema_relationships and engine_schema_columns for discovery analytics

-- ============================================================================
-- Extend engine_schema_relationships with discovery metrics
-- ============================================================================

-- Match rate from value overlap analysis (0.0000-1.0000)
ALTER TABLE engine_schema_relationships
    ADD COLUMN IF NOT EXISTS match_rate DECIMAL(5,4);

-- Distinct value counts from discovery
ALTER TABLE engine_schema_relationships
    ADD COLUMN IF NOT EXISTS source_distinct BIGINT;

ALTER TABLE engine_schema_relationships
    ADD COLUMN IF NOT EXISTS target_distinct BIGINT;

-- Count of matched values during discovery
ALTER TABLE engine_schema_relationships
    ADD COLUMN IF NOT EXISTS matched_count BIGINT;

-- Why a candidate was rejected (NULL for accepted relationships)
ALTER TABLE engine_schema_relationships
    ADD COLUMN IF NOT EXISTS rejection_reason TEXT;

-- ============================================================================
-- Extend engine_schema_columns with joinability analysis
-- ============================================================================

-- Denormalized row count for efficient queries (same as parent table's row_count)
ALTER TABLE engine_schema_columns
    ADD COLUMN IF NOT EXISTS row_count BIGINT;

-- Non-null value count (row_count - null_count when computed)
ALTER TABLE engine_schema_columns
    ADD COLUMN IF NOT EXISTS non_null_count BIGINT;

-- Whether this column is suitable as a join key
ALTER TABLE engine_schema_columns
    ADD COLUMN IF NOT EXISTS is_joinable BOOLEAN;

-- Reason for joinability classification (pk, fk_target, unique_values, type_excluded, low_cardinality)
ALTER TABLE engine_schema_columns
    ADD COLUMN IF NOT EXISTS joinability_reason TEXT;

-- When column statistics were last computed
ALTER TABLE engine_schema_columns
    ADD COLUMN IF NOT EXISTS stats_updated_at TIMESTAMPTZ;

-- ============================================================================
-- Indexes for discovery queries
-- ============================================================================

-- Index for finding joinable columns efficiently
CREATE INDEX IF NOT EXISTS idx_engine_schema_columns_joinable
    ON engine_schema_columns(schema_table_id, is_joinable)
    WHERE deleted_at IS NULL AND is_joinable = true;

-- Index for filtering rejected relationship candidates
CREATE INDEX IF NOT EXISTS idx_engine_schema_relationships_rejection
    ON engine_schema_relationships(project_id, rejection_reason)
    WHERE deleted_at IS NULL AND rejection_reason IS NOT NULL;
