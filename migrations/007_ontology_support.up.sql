-- 007_ontology_support.up.sql
-- Ontology support tables: chat messages, questions, project knowledge

-- Refinement chat history
CREATE TABLE engine_ontology_chat_messages (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    ontology_id uuid NOT NULL,
    role character varying(50) NOT NULL,
    content text NOT NULL,
    tool_calls jsonb,
    tool_call_id character varying(255),
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontology_chat_messages_role_check CHECK ((role)::text = ANY (ARRAY['user'::text, 'assistant'::text, 'system'::text, 'tool'::text])),
    CONSTRAINT engine_ontology_chat_messages_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_ontology_chat_messages_ontology_id_fkey FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE
);

CREATE INDEX idx_engine_ontology_chat_messages_project ON engine_ontology_chat_messages USING btree (project_id);
CREATE INDEX idx_engine_ontology_chat_messages_ontology ON engine_ontology_chat_messages USING btree (ontology_id);
CREATE INDEX idx_engine_ontology_chat_messages_created ON engine_ontology_chat_messages USING btree (project_id, created_at DESC);

-- Questions for clarification
CREATE TABLE engine_ontology_questions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    ontology_id uuid NOT NULL,
    text text NOT NULL,
    reasoning text,
    category character varying(100),
    priority integer DEFAULT 3 NOT NULL,
    is_required boolean DEFAULT false NOT NULL,
    affects jsonb DEFAULT '{}'::jsonb NOT NULL,
    source_entity_type character varying(20),
    source_entity_key text,
    content_hash character varying(16),
    status character varying(50) DEFAULT 'pending'::character varying NOT NULL,
    status_reason text,
    answer text,
    answered_by uuid,
    answered_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontology_questions_priority_check CHECK (priority >= 1 AND priority <= 5),
    CONSTRAINT engine_ontology_questions_status_check CHECK ((status)::text = ANY (ARRAY['pending'::text, 'answered'::text, 'skipped'::text, 'escalated'::text, 'dismissed'::text, 'deleted'::text])),
    CONSTRAINT engine_ontology_questions_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_ontology_questions_ontology_id_fkey FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE
);

COMMENT ON COLUMN engine_ontology_questions.status_reason IS 'Reason for skip/escalate/dismiss (e.g., "Need access to frontend repo")';
COMMENT ON COLUMN engine_ontology_questions.content_hash IS 'SHA256 hash of category|text (first 16 chars) for deduplication';
COMMENT ON CONSTRAINT engine_ontology_questions_status_check ON engine_ontology_questions IS 'Valid statuses: pending, answered, skipped (revisit later), escalated (needs human), dismissed (not worth pursuing), deleted';

CREATE INDEX idx_engine_ontology_questions_project ON engine_ontology_questions USING btree (project_id);
CREATE INDEX idx_engine_ontology_questions_ontology ON engine_ontology_questions USING btree (ontology_id);
CREATE INDEX idx_engine_ontology_questions_pending ON engine_ontology_questions USING btree (project_id, status, priority) WHERE ((status)::text = 'pending'::text);
CREATE INDEX idx_engine_ontology_questions_required ON engine_ontology_questions USING btree (project_id, is_required) WHERE ((status)::text = 'pending'::text AND is_required = true);
CREATE UNIQUE INDEX idx_engine_ontology_questions_content_hash ON engine_ontology_questions(ontology_id, content_hash) WHERE content_hash IS NOT NULL;

CREATE TRIGGER update_engine_ontology_questions_updated_at
    BEFORE UPDATE ON engine_ontology_questions
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Project-level facts learned during refinement
-- Note: ontology_id and key columns removed — facts have project-lifecycle scope
CREATE TABLE engine_project_knowledge (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    fact_type character varying(100) NOT NULL,
    value text NOT NULL,
    context text,

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_project_knowledge_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_project_knowledge_source_check CHECK (source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_project_knowledge_last_edit_source_check CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_project_knowledge_created_by_fkey FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    CONSTRAINT engine_project_knowledge_updated_by_fkey FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
);

COMMENT ON COLUMN engine_project_knowledge.source IS 'How this fact was created: inferred (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_project_knowledge.last_edit_source IS 'How this fact was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_project_knowledge.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_project_knowledge.updated_by IS 'UUID of user who last updated this fact';

CREATE INDEX idx_engine_project_knowledge_project ON engine_project_knowledge USING btree (project_id);
CREATE INDEX idx_engine_project_knowledge_project_type ON engine_project_knowledge USING btree (project_id, fact_type);

CREATE TRIGGER update_engine_project_knowledge_updated_at
    BEFORE UPDATE ON engine_project_knowledge
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_ontology_chat_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_chat_messages FORCE ROW LEVEL SECURITY;
CREATE POLICY ontology_chat_messages_access ON engine_ontology_chat_messages FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

ALTER TABLE engine_ontology_questions ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_questions FORCE ROW LEVEL SECURITY;
CREATE POLICY ontology_questions_access ON engine_ontology_questions FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

ALTER TABLE engine_project_knowledge ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_project_knowledge FORCE ROW LEVEL SECURITY;
CREATE POLICY project_knowledge_access ON engine_project_knowledge FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
