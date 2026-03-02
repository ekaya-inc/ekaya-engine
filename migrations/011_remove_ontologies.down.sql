-- 011_remove_ontologies.down.sql
-- Reverse: recreate engine_ontologies and restore ontology_id columns

-- Step 1: Recreate engine_ontologies table
CREATE TABLE engine_ontologies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    domain_summary jsonb,
    column_details jsonb,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontologies_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_engine_ontologies_project ON engine_ontologies USING btree (project_id);
CREATE UNIQUE INDEX idx_engine_ontologies_unique ON engine_ontologies USING btree (project_id, version);
CREATE UNIQUE INDEX idx_engine_ontologies_single_active ON engine_ontologies USING btree (project_id) WHERE (is_active = true);

CREATE TRIGGER update_engine_ontologies_updated_at
    BEFORE UPDATE ON engine_ontologies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE engine_ontologies ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_ontologies FORCE ROW LEVEL SECURITY;
CREATE POLICY ontologies_access ON engine_ontologies FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- Step 2: Restore ontology_id on engine_ontology_dag
ALTER TABLE engine_ontology_dag ADD COLUMN ontology_id uuid;
ALTER TABLE engine_ontology_dag ADD CONSTRAINT engine_ontology_dag_ontology_id_fkey
    FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE;

-- Step 3: Restore ontology_id on engine_ontology_chat_messages
ALTER TABLE engine_ontology_chat_messages ADD COLUMN ontology_id uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE engine_ontology_chat_messages ALTER COLUMN ontology_id DROP DEFAULT;
ALTER TABLE engine_ontology_chat_messages ADD CONSTRAINT engine_ontology_chat_messages_ontology_id_fkey
    FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE;
CREATE INDEX idx_engine_ontology_chat_messages_ontology ON engine_ontology_chat_messages USING btree (ontology_id);

-- Step 4: Restore ontology_id on engine_ontology_questions
DROP INDEX IF EXISTS idx_engine_ontology_questions_content_hash;
ALTER TABLE engine_ontology_questions ADD COLUMN ontology_id uuid NOT NULL DEFAULT gen_random_uuid();
ALTER TABLE engine_ontology_questions ALTER COLUMN ontology_id DROP DEFAULT;
ALTER TABLE engine_ontology_questions ADD CONSTRAINT engine_ontology_questions_ontology_id_fkey
    FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE;
CREATE INDEX idx_engine_ontology_questions_ontology ON engine_ontology_questions USING btree (ontology_id);
CREATE UNIQUE INDEX idx_engine_ontology_questions_content_hash ON engine_ontology_questions(ontology_id, content_hash) WHERE content_hash IS NOT NULL;

-- Step 5: Restore ontology_id on engine_business_glossary
DROP INDEX IF EXISTS engine_business_glossary_project_term_unique;
ALTER TABLE engine_business_glossary ADD COLUMN ontology_id uuid;
ALTER TABLE engine_business_glossary ADD CONSTRAINT engine_business_glossary_ontology_id_fkey
    FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE;
CREATE INDEX idx_business_glossary_ontology ON engine_business_glossary USING btree (ontology_id);
CREATE UNIQUE INDEX engine_business_glossary_project_ontology_term_unique ON engine_business_glossary USING btree (project_id, ontology_id, term);

-- Step 6: Remove domain_summary from engine_projects
ALTER TABLE engine_projects DROP COLUMN domain_summary;
