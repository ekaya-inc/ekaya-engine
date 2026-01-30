-- 018_project_knowledge_key_to_text.down.sql
-- Revert key column back to VARCHAR(255)
--
-- Note: This may fail if any existing facts exceed 255 characters

ALTER TABLE engine_project_knowledge
    ALTER COLUMN key TYPE VARCHAR(255);
