-- 021_relationship_provenance.up.sql
-- Add provenance tracking to schema relationships so semantic type and curation ownership are separate.

ALTER TABLE engine_schema_relationships
    ADD COLUMN source text,
    ADD COLUMN last_edit_source text,
    ADD COLUMN created_by uuid,
    ADD COLUMN updated_by uuid;

UPDATE engine_schema_relationships
SET source = CASE
    WHEN relationship_type = 'manual' THEN 'manual'
    ELSE 'inferred'
END
WHERE source IS NULL;

ALTER TABLE engine_schema_relationships
    ALTER COLUMN source SET NOT NULL;

ALTER TABLE engine_schema_relationships
    ADD CONSTRAINT engine_schema_relationships_source_check
        CHECK (source IN ('inferred', 'mcp', 'manual')),
    ADD CONSTRAINT engine_schema_relationships_last_edit_source_check
        CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    ADD CONSTRAINT engine_schema_relationships_created_by_fkey
        FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    ADD CONSTRAINT engine_schema_relationships_updated_by_fkey
        FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id);
