-- 013_conditional_schema_triggers.down.sql
-- Revert to the generic trigger function for both tables.

DROP TRIGGER IF EXISTS update_engine_schema_tables_updated_at ON engine_schema_tables;
CREATE TRIGGER update_engine_schema_tables_updated_at
    BEFORE UPDATE ON engine_schema_tables
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

DROP TRIGGER IF EXISTS update_engine_schema_columns_updated_at ON engine_schema_columns;
CREATE TRIGGER update_engine_schema_columns_updated_at
    BEFORE UPDATE ON engine_schema_columns
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Clean up custom functions
DROP FUNCTION IF EXISTS update_schema_tables_updated_at();
DROP FUNCTION IF EXISTS update_schema_columns_updated_at();
