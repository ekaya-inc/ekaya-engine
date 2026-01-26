-- 022_question_content_hash.down.sql
-- Rollback question content hash deduplication

-- Drop unique index
DROP INDEX IF EXISTS idx_engine_ontology_questions_content_hash;

-- Remove content_hash column
ALTER TABLE engine_ontology_questions
    DROP COLUMN IF EXISTS content_hash;
