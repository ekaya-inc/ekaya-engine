-- Migration 004 DOWN: Remove schema tables

-- Drop triggers
DROP TRIGGER IF EXISTS update_engine_schema_relationships_updated_at ON engine_schema_relationships;
DROP TRIGGER IF EXISTS update_engine_schema_columns_updated_at ON engine_schema_columns;
DROP TRIGGER IF EXISTS update_engine_schema_tables_updated_at ON engine_schema_tables;

-- Drop policies
DROP POLICY IF EXISTS schema_relationships_access ON engine_schema_relationships;
DROP POLICY IF EXISTS schema_columns_access ON engine_schema_columns;
DROP POLICY IF EXISTS schema_tables_access ON engine_schema_tables;

-- Drop tables (reverse dependency order)
DROP TABLE IF EXISTS engine_schema_relationships;
DROP TABLE IF EXISTS engine_schema_columns;
DROP TABLE IF EXISTS engine_schema_tables;

-- Note: Keep update_updated_at_column() - used by other tables
