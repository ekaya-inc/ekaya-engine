-- 015_knowledge_ontology_fk.up.sql
-- Add ontology_id FK to engine_project_knowledge for proper CASCADE delete

-- 1. Add ontology_id column as nullable (allows gradual migration)
ALTER TABLE engine_project_knowledge
ADD COLUMN ontology_id uuid REFERENCES engine_ontologies(id) ON DELETE CASCADE;

-- 2. Backfill existing rows with the active ontology for their project
-- This is best-effort: rows without an active ontology will remain NULL
UPDATE engine_project_knowledge pk
SET ontology_id = (
    SELECT id FROM engine_ontologies o
    WHERE o.project_id = pk.project_id AND o.is_active = true
    LIMIT 1
);

-- 3. Create index for efficient lookups by ontology
CREATE INDEX idx_engine_project_knowledge_ontology ON engine_project_knowledge USING btree (ontology_id);

-- 4. Drop the old unique constraint and create a new one that includes ontology_id
-- This allows the same fact_type/key combination across different ontologies (important for ontology refresh)
DROP INDEX idx_engine_project_knowledge_unique;
CREATE UNIQUE INDEX idx_engine_project_knowledge_unique ON engine_project_knowledge USING btree (project_id, ontology_id, fact_type, key);
