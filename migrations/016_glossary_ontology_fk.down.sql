-- 016_glossary_ontology_fk.down.sql
-- Revert ontology_id FK from engine_business_glossary

-- 1. Drop the new unique constraint
DROP INDEX engine_business_glossary_project_ontology_term_unique;

-- 2. Recreate the original unique constraint (without ontology_id)
ALTER TABLE engine_business_glossary ADD CONSTRAINT engine_business_glossary_project_term_unique UNIQUE (project_id, term);

-- 3. Drop the ontology index
DROP INDEX idx_engine_business_glossary_ontology;

-- 4. Remove the ontology_id column
ALTER TABLE engine_business_glossary DROP COLUMN ontology_id;
