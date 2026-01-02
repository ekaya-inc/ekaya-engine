-- Migration 021 rollback: Remove constraints field from queries

ALTER TABLE engine_queries
    DROP COLUMN constraints;
