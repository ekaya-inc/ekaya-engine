-- 022_question_content_hash.up.sql
-- Add content_hash column for question deduplication across extraction runs.
-- The hash is SHA256(category + "|" + text), truncated to 16 hex characters.

-- Add content_hash column (nullable initially to allow backfill)
ALTER TABLE engine_ontology_questions
    ADD COLUMN IF NOT EXISTS content_hash character varying(16);

COMMENT ON COLUMN engine_ontology_questions.content_hash IS 'SHA256 hash of category|text (first 16 chars) for deduplication';

-- Unique constraint scoped to ontology to prevent duplicate questions within the same ontology
-- This allows the same question in different ontologies (different projects or extractions)
CREATE UNIQUE INDEX IF NOT EXISTS idx_engine_ontology_questions_content_hash
    ON engine_ontology_questions(ontology_id, content_hash)
    WHERE content_hash IS NOT NULL;
