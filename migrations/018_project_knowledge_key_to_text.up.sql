-- 018_project_knowledge_remove_key.up.sql
-- Remove the redundant key column from project knowledge
--
-- The key column was originally intended as a short identifier for deduplication,
-- but in practice both key and value stored the same fact content. This was
-- confusing for users and unnecessary. Deduplication is now handled by checking
-- existing facts before creating new ones.

-- Drop the unique index that depends on key
DROP INDEX IF EXISTS idx_engine_project_knowledge_unique;

-- Drop the key column
ALTER TABLE engine_project_knowledge DROP COLUMN key;

-- Create a new index for lookups by project and fact_type (no uniqueness constraint)
CREATE INDEX idx_engine_project_knowledge_project_type ON engine_project_knowledge USING btree (project_id, fact_type);
