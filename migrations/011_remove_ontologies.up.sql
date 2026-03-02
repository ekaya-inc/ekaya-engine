-- 011_remove_ontologies.up.sql
-- Remove engine_ontologies table: move domain_summary to engine_projects,
-- drop ontology_id from all referencing tables, drop engine_ontologies.

-- Step 1: Add domain_summary to engine_projects
ALTER TABLE engine_projects ADD COLUMN domain_summary jsonb;

-- Step 2: Drop ontology_id from engine_ontology_questions
--   - Drop the content_hash unique index (references ontology_id)
--   - Drop FK constraint
--   - Drop the ontology index
--   - Drop the column
--   - Recreate content_hash unique index using project_id
DROP INDEX IF EXISTS idx_engine_ontology_questions_content_hash;
ALTER TABLE engine_ontology_questions DROP CONSTRAINT IF EXISTS engine_ontology_questions_ontology_id_fkey;
DROP INDEX IF EXISTS idx_engine_ontology_questions_ontology;
ALTER TABLE engine_ontology_questions DROP COLUMN ontology_id;
CREATE UNIQUE INDEX idx_engine_ontology_questions_content_hash ON engine_ontology_questions(project_id, content_hash) WHERE content_hash IS NOT NULL;

-- Step 3: Drop ontology_id from engine_business_glossary
--   - Drop the unique index on (project_id, ontology_id, term)
--   - Drop FK constraint
--   - Drop the ontology index
--   - Drop the column
--   - Recreate unique constraint as (project_id, term)
DROP INDEX IF EXISTS engine_business_glossary_project_ontology_term_unique;
ALTER TABLE engine_business_glossary DROP CONSTRAINT IF EXISTS engine_business_glossary_ontology_id_fkey;
DROP INDEX IF EXISTS idx_business_glossary_ontology;
ALTER TABLE engine_business_glossary DROP COLUMN ontology_id;
CREATE UNIQUE INDEX engine_business_glossary_project_term_unique ON engine_business_glossary USING btree (project_id, term);

-- Step 4: Drop ontology_id from engine_ontology_chat_messages
ALTER TABLE engine_ontology_chat_messages DROP CONSTRAINT IF EXISTS engine_ontology_chat_messages_ontology_id_fkey;
DROP INDEX IF EXISTS idx_engine_ontology_chat_messages_ontology;
ALTER TABLE engine_ontology_chat_messages DROP COLUMN ontology_id;

-- Step 5: Drop ontology_id from engine_ontology_dag
ALTER TABLE engine_ontology_dag DROP CONSTRAINT IF EXISTS engine_ontology_dag_ontology_id_fkey;
ALTER TABLE engine_ontology_dag DROP COLUMN ontology_id;

-- Step 6: Drop engine_ontologies table (all FKs already removed above)
DROP TABLE engine_ontologies;
