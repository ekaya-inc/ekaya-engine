-- Preserve explicit updated_at values for schema changes that intentionally
-- use the application clock, while still suppressing no-op refresh updates.

CREATE OR REPLACE FUNCTION update_schema_tables_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF (OLD.is_selected IS DISTINCT FROM NEW.is_selected)
       OR (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    THEN
        IF NEW.updated_at IS DISTINCT FROM OLD.updated_at THEN
            RETURN NEW;
        END IF;

        NEW.updated_at = NOW();
    ELSE
        NEW.updated_at = OLD.updated_at;
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION update_schema_columns_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF (OLD.data_type IS DISTINCT FROM NEW.data_type)
       OR (OLD.is_nullable IS DISTINCT FROM NEW.is_nullable)
       OR (OLD.is_primary_key IS DISTINCT FROM NEW.is_primary_key)
       OR (OLD.is_unique IS DISTINCT FROM NEW.is_unique)
       OR (OLD.default_value IS DISTINCT FROM NEW.default_value)
       OR (OLD.is_selected IS DISTINCT FROM NEW.is_selected)
       OR (OLD.deleted_at IS DISTINCT FROM NEW.deleted_at)
    THEN
        IF NEW.updated_at IS DISTINCT FROM OLD.updated_at THEN
            RETURN NEW;
        END IF;

        NEW.updated_at = NOW();
    ELSE
        NEW.updated_at = OLD.updated_at;
    END IF;

    RETURN NEW;
END;
$$;
