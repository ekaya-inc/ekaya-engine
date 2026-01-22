-- 015_knowledge_ontology_fk.down.sql
-- Revert ontology_id FK from engine_project_knowledge

-- 1. Drop the new unique constraint
DROP INDEX idx_engine_project_knowledge_unique;

-- 2. Recreate the original unique constraint (without ontology_id)
CREATE UNIQUE INDEX idx_engine_project_knowledge_unique ON engine_project_knowledge USING btree (project_id, fact_type, key);

-- 3. Drop the ontology index
DROP INDEX idx_engine_project_knowledge_ontology;

-- 4. Remove the ontology_id column
ALTER TABLE engine_project_knowledge DROP COLUMN ontology_id;
