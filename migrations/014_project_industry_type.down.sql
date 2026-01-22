-- 014_project_industry_type.down.sql
-- Remove industry_type column from engine_projects.

ALTER TABLE engine_projects
DROP COLUMN IF EXISTS industry_type;
