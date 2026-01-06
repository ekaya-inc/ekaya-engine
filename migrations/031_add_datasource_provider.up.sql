-- Add provider column to track PostgreSQL-compatible variants (supabase, neon, etc.)
ALTER TABLE engine_datasources
ADD COLUMN provider VARCHAR(50);

COMMENT ON COLUMN engine_datasources.provider IS
'Optional provider identifier for adapter variants (e.g., supabase, neon for postgres adapter)';
