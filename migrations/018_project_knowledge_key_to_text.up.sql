-- 018_project_knowledge_key_to_text.up.sql
-- Increase the key column size for project knowledge facts
--
-- The fact parameter in update_project_knowledge was limited to 255 characters
-- because it mapped to a VARCHAR(255) column. Complex business rules often
-- exceed this limit. TEXT allows arbitrarily long facts.

ALTER TABLE engine_project_knowledge
    ALTER COLUMN key TYPE TEXT;
