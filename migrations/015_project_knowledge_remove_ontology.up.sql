-- 015_project_knowledge_remove_ontology.up.sql
-- Remove ontology_id from engine_project_knowledge
--
-- Project knowledge represents domain facts that have project-lifecycle scope,
-- not ontology scope. Facts should persist across ontology re-extractions.
-- Only manual deletion or project deletion should remove knowledge facts.

-- Drop the unique constraint that includes ontology_id
DROP INDEX IF EXISTS idx_engine_project_knowledge_unique;

-- Drop the ontology-specific index
DROP INDEX IF EXISTS idx_engine_project_knowledge_ontology;

-- Drop the foreign key constraint to ontologies
ALTER TABLE engine_project_knowledge
    DROP CONSTRAINT IF EXISTS engine_project_knowledge_ontology_id_fkey;

-- Drop the ontology_id column
ALTER TABLE engine_project_knowledge
    DROP COLUMN IF EXISTS ontology_id;

-- Create new unique constraint without ontology_id
-- Facts are unique per project + fact_type + key
CREATE UNIQUE INDEX idx_engine_project_knowledge_unique
    ON engine_project_knowledge USING btree (project_id, fact_type, key);
