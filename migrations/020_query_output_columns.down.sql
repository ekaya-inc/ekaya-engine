-- Migration 020 down: Remove output_columns field from queries

DROP INDEX IF EXISTS idx_engine_queries_has_output_columns;

ALTER TABLE engine_queries
    DROP COLUMN IF EXISTS output_columns;
