-- 021_relationship_provenance.down.sql

ALTER TABLE engine_schema_relationships
    DROP CONSTRAINT IF EXISTS engine_schema_relationships_updated_by_fkey,
    DROP CONSTRAINT IF EXISTS engine_schema_relationships_created_by_fkey,
    DROP CONSTRAINT IF EXISTS engine_schema_relationships_last_edit_source_check,
    DROP CONSTRAINT IF EXISTS engine_schema_relationships_source_check,
    DROP COLUMN IF EXISTS updated_by,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS last_edit_source,
    DROP COLUMN IF EXISTS source;
