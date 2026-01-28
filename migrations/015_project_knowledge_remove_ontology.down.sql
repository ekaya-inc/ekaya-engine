-- 015_project_knowledge_remove_ontology.down.sql
-- Restore ontology_id to engine_project_knowledge

-- Drop the new unique constraint
DROP INDEX IF EXISTS idx_engine_project_knowledge_unique;

-- Add back the ontology_id column
ALTER TABLE engine_project_knowledge
    ADD COLUMN ontology_id uuid;

-- Recreate the foreign key constraint
ALTER TABLE engine_project_knowledge
    ADD CONSTRAINT engine_project_knowledge_ontology_id_fkey
    FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE;

-- Recreate the ontology index
CREATE INDEX idx_engine_project_knowledge_ontology
    ON engine_project_knowledge USING btree (ontology_id);

-- Recreate the original unique constraint (includes ontology_id)
CREATE UNIQUE INDEX idx_engine_project_knowledge_unique
    ON engine_project_knowledge USING btree (project_id, ontology_id, fact_type, key);

-- Restore the column comment
COMMENT ON COLUMN engine_project_knowledge.ontology_id IS 'Optional link to specific ontology version for CASCADE delete';
