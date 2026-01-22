-- 014_project_industry_type.up.sql
-- Add industry_type column to engine_projects for template selection.
-- This enables industry-appropriate glossary suggestions during ontology extraction.

ALTER TABLE engine_projects
ADD COLUMN industry_type text DEFAULT 'general';

COMMENT ON COLUMN engine_projects.industry_type IS 'Industry classification for template selection (e.g., general, video_streaming, marketplace, creator_economy)';
