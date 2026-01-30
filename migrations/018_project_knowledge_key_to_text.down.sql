-- 018_project_knowledge_remove_key.down.sql
-- Re-add the key column to project knowledge
--
-- Note: This sets key = value for existing rows. The unique index may fail
-- if there are duplicate (project_id, fact_type, value) combinations.

-- Drop the new index
DROP INDEX IF EXISTS idx_engine_project_knowledge_project_type;

-- Add the key column back
ALTER TABLE engine_project_knowledge ADD COLUMN key TEXT NOT NULL DEFAULT '';

-- Copy value to key for existing rows
UPDATE engine_project_knowledge SET key = value;

-- Recreate the unique index (may fail if duplicates exist)
CREATE UNIQUE INDEX idx_engine_project_knowledge_unique ON engine_project_knowledge USING btree (project_id, ontology_id, fact_type, key);
