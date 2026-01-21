-- 008_llm_conversations.up.sql
-- LLM request/response logs for debugging and analytics

CREATE TABLE engine_llm_conversations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    conversation_id uuid,
    iteration integer DEFAULT 1 NOT NULL,
    endpoint text NOT NULL,
    model text NOT NULL,
    request_messages jsonb NOT NULL,
    request_tools jsonb,
    temperature numeric(3,2),
    response_content text,
    response_tool_calls jsonb,
    prompt_tokens integer,
    completion_tokens integer,
    total_tokens integer,
    duration_ms integer NOT NULL,
    status character varying(20) DEFAULT 'success'::character varying NOT NULL,
    error_message text,
    context jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_llm_conversations_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_llm_conversations IS 'Verbatim log of all LLM API calls for debugging and analytics';
COMMENT ON COLUMN engine_llm_conversations.conversation_id IS 'Groups related calls in multi-turn streaming conversations';
COMMENT ON COLUMN engine_llm_conversations.iteration IS 'Tool-calling iteration number within a single user request';
COMMENT ON COLUMN engine_llm_conversations.request_messages IS 'Full OpenAI-format message array sent to LLM';
COMMENT ON COLUMN engine_llm_conversations.context IS 'Caller-specific context (workflow_id, task_name, session_id, etc.)';

CREATE INDEX idx_llm_conversations_project ON engine_llm_conversations USING btree (project_id, created_at DESC);
CREATE INDEX idx_llm_conversations_conversation ON engine_llm_conversations USING btree (conversation_id) WHERE (conversation_id IS NOT NULL);
CREATE INDEX idx_llm_conversations_context ON engine_llm_conversations USING gin (context);

-- RLS
ALTER TABLE engine_llm_conversations ENABLE ROW LEVEL SECURITY;
CREATE POLICY llm_conversations_access ON engine_llm_conversations FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
