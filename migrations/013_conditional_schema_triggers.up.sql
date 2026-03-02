-- 013_conditional_schema_triggers.up.sql
-- Replace generic updated_at triggers on schema tables with conditional versions
-- that only bump updated_at when ontology-relevant fields change.
-- This prevents false-positive "schema changed" detection after a no-op refresh.

-- Replace the generic trigger on engine_schema_tables with a conditional one.
CREATE OR REPLACE FUNCTION update_schema_tables_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Only bump updated_at when ontology-relevant fields change.
    -- row_count changes alone should NOT trigger ontology re-extraction.
    IF (OLD.is_selected IS DISTINCT FROM NEW.is_selected)
       OR (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    THEN
        NEW.updated_at = NOW();
    ELSE
        NEW.updated_at = OLD.updated_at;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS update_engine_schema_tables_updated_at ON engine_schema_tables;
CREATE TRIGGER update_engine_schema_tables_updated_at
    BEFORE UPDATE ON engine_schema_tables
    FOR EACH ROW EXECUTE FUNCTION update_schema_tables_updated_at();


-- Replace the generic trigger on engine_schema_columns with a conditional one.
CREATE OR REPLACE FUNCTION update_schema_columns_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    -- Only bump updated_at when ontology-relevant fields change.
    -- Stats (distinct_count, null_count, min_length, max_length) and
    -- ordinal_position changes should NOT trigger ontology re-extraction.
    IF (OLD.data_type IS DISTINCT FROM NEW.data_type)
       OR (OLD.is_nullable IS DISTINCT FROM NEW.is_nullable)
       OR (OLD.is_primary_key IS DISTINCT FROM NEW.is_primary_key)
       OR (OLD.is_unique IS DISTINCT FROM NEW.is_unique)
       OR (OLD.default_value IS DISTINCT FROM NEW.default_value)
       OR (OLD.is_selected IS DISTINCT FROM NEW.is_selected)
       OR (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    THEN
        NEW.updated_at = NOW();
    ELSE
        NEW.updated_at = OLD.updated_at;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS update_engine_schema_columns_updated_at ON engine_schema_columns;
CREATE TRIGGER update_engine_schema_columns_updated_at
    BEFORE UPDATE ON engine_schema_columns
    FOR EACH ROW EXECUTE FUNCTION update_schema_columns_updated_at();
