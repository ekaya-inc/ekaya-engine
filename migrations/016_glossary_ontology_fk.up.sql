-- 016_glossary_ontology_fk.up.sql
-- Add ontology_id FK to engine_business_glossary for proper CASCADE delete

-- 1. Add ontology_id column as nullable (allows gradual migration)
ALTER TABLE engine_business_glossary
ADD COLUMN ontology_id uuid REFERENCES engine_ontologies(id) ON DELETE CASCADE;

-- 2. Backfill existing rows with the active ontology for their project
-- This is best-effort: rows without an active ontology will remain NULL
UPDATE engine_business_glossary bg
SET ontology_id = (
    SELECT id FROM engine_ontologies o
    WHERE o.project_id = bg.project_id AND o.is_active = true
    LIMIT 1
);

-- 3. Create index for efficient lookups by ontology
CREATE INDEX idx_engine_business_glossary_ontology ON engine_business_glossary USING btree (ontology_id);

-- 4. Drop the old unique constraint and create a new one that includes ontology_id
-- This allows the same term across different ontologies (important for ontology refresh)
ALTER TABLE engine_business_glossary DROP CONSTRAINT engine_business_glossary_project_term_unique;
CREATE UNIQUE INDEX engine_business_glossary_project_ontology_term_unique ON engine_business_glossary USING btree (project_id, ontology_id, term);
