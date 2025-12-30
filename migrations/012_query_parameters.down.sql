-- Migration 012 rollback: Remove parameters column from engine_queries table

-- Drop the partial index for parameterized queries
DROP INDEX IF EXISTS idx_engine_queries_has_parameters;

-- Remove the parameters column
ALTER TABLE engine_queries
DROP COLUMN IF EXISTS parameters;
