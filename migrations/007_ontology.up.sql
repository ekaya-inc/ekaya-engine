-- Migration 007: Ontology extraction system
-- Creates all tables needed for LLM-powered ontology extraction workflow:
--   - engine_ontology_workflows: Workflow lifecycle and progress tracking
--   - engine_ontologies: Tiered ontology storage (domain/entity/column)
--   - engine_ontology_chat_messages: Chat refinement history
--   - engine_project_knowledge: Project-level facts
--   - engine_llm_conversations: LLM request/response debugging
--   - engine_workflow_state: Per-entity extraction state (ephemeral)
--   - engine_ontology_questions: Questions for user clarification

-- ============================================================================
-- Table: engine_ontology_workflows
-- Manages the lifecycle of ontology extraction workflows
-- ============================================================================

CREATE TABLE engine_ontology_workflows (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    state VARCHAR(50) NOT NULL DEFAULT 'pending',
    progress JSONB NOT NULL DEFAULT '{}',
    task_queue JSONB NOT NULL DEFAULT '[]',
    config JSONB NOT NULL DEFAULT '{}',
    error_message TEXT,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    -- Ownership for multi-server robustness
    owner_id UUID,
    last_heartbeat TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Check constraint for valid workflow states
ALTER TABLE engine_ontology_workflows
    ADD CONSTRAINT engine_ontology_workflows_state_check
    CHECK (state IN ('pending', 'running', 'paused', 'awaiting_input', 'completed', 'failed'));

-- Query indexes
CREATE INDEX idx_engine_ontology_workflows_project ON engine_ontology_workflows(project_id);
CREATE INDEX idx_engine_ontology_workflows_state ON engine_ontology_workflows(project_id, state);
CREATE INDEX idx_workflow_heartbeat ON engine_ontology_workflows (last_heartbeat)
    WHERE state = 'running';

-- RLS
ALTER TABLE engine_ontology_workflows ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_workflows_access ON engine_ontology_workflows
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger
CREATE TRIGGER update_engine_ontology_workflows_updated_at
    BEFORE UPDATE ON engine_ontology_workflows
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Table: engine_ontologies
-- Stores the tiered ontology (domain summary, entity summaries, column details)
-- ============================================================================

CREATE TABLE engine_ontologies (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    version INTEGER NOT NULL DEFAULT 1,
    is_active BOOLEAN NOT NULL DEFAULT true,
    domain_summary JSONB,
    entity_summaries JSONB,
    column_details JSONB,
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique constraint: one version per project
CREATE UNIQUE INDEX idx_engine_ontologies_unique
    ON engine_ontologies(project_id, version);

-- Query indexes
CREATE INDEX idx_engine_ontologies_project ON engine_ontologies(project_id);

-- Unique constraint: only one active ontology per project
CREATE UNIQUE INDEX idx_engine_ontologies_single_active
    ON engine_ontologies(project_id)
    WHERE is_active = true;

-- RLS
ALTER TABLE engine_ontologies ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontologies_access ON engine_ontologies
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger
CREATE TRIGGER update_engine_ontologies_updated_at
    BEFORE UPDATE ON engine_ontologies
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Add ontology_id FK to workflows (now that ontologies table exists)
-- ============================================================================

ALTER TABLE engine_ontology_workflows
    ADD COLUMN ontology_id UUID REFERENCES engine_ontologies(id) ON DELETE CASCADE;

CREATE INDEX idx_engine_ontology_workflows_ontology ON engine_ontology_workflows(ontology_id);

-- ============================================================================
-- Table: engine_ontology_chat_messages
-- Stores chat history for the ontology refinement chat interface
-- ============================================================================

CREATE TABLE engine_ontology_chat_messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    role VARCHAR(50) NOT NULL,
    content TEXT NOT NULL,
    tool_calls JSONB,
    tool_call_id VARCHAR(255),
    metadata JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Check constraint for valid chat roles
ALTER TABLE engine_ontology_chat_messages
    ADD CONSTRAINT engine_ontology_chat_messages_role_check
    CHECK (role IN ('user', 'assistant', 'system', 'tool'));

-- Query indexes
CREATE INDEX idx_engine_ontology_chat_messages_project ON engine_ontology_chat_messages(project_id);
CREATE INDEX idx_engine_ontology_chat_messages_ontology ON engine_ontology_chat_messages(ontology_id);
CREATE INDEX idx_engine_ontology_chat_messages_created ON engine_ontology_chat_messages(project_id, created_at DESC);

-- RLS
ALTER TABLE engine_ontology_chat_messages ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_chat_messages_access ON engine_ontology_chat_messages
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- ============================================================================
-- Table: engine_project_knowledge
-- Stores project-level facts learned during ontology refinement
-- ============================================================================

CREATE TABLE engine_project_knowledge (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    fact_type VARCHAR(100) NOT NULL,
    key VARCHAR(255) NOT NULL,
    value TEXT NOT NULL,
    context TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique constraint: one fact per project/type/key combination
CREATE UNIQUE INDEX idx_engine_project_knowledge_unique
    ON engine_project_knowledge(project_id, fact_type, key);

-- Query indexes
CREATE INDEX idx_engine_project_knowledge_project ON engine_project_knowledge(project_id);
CREATE INDEX idx_engine_project_knowledge_type ON engine_project_knowledge(project_id, fact_type);

-- RLS
ALTER TABLE engine_project_knowledge ENABLE ROW LEVEL SECURITY;
CREATE POLICY project_knowledge_access ON engine_project_knowledge
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger
CREATE TRIGGER update_engine_project_knowledge_updated_at
    BEFORE UPDATE ON engine_project_knowledge
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Table: engine_llm_conversations
-- Captures verbatim LLM requests and responses for debugging and analytics
-- ============================================================================

CREATE TABLE engine_llm_conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- For streaming: groups iterations within a single user request
    conversation_id UUID,
    iteration INT NOT NULL DEFAULT 1,

    -- Model info
    endpoint TEXT NOT NULL,
    model TEXT NOT NULL,

    -- Request (VERBATIM)
    request_messages JSONB NOT NULL,
    request_tools JSONB,
    temperature DECIMAL(3,2),

    -- Response (VERBATIM)
    response_content TEXT,
    response_tool_calls JSONB,

    -- Metrics
    prompt_tokens INT,
    completion_tokens INT,
    total_tokens INT,
    duration_ms INT NOT NULL,

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'success',
    error_message TEXT,

    -- Flexible context for caller-specific metadata
    context JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for common queries
CREATE INDEX idx_llm_conversations_project ON engine_llm_conversations(project_id, created_at DESC);
CREATE INDEX idx_llm_conversations_conversation ON engine_llm_conversations(conversation_id) WHERE conversation_id IS NOT NULL;
CREATE INDEX idx_llm_conversations_context ON engine_llm_conversations USING GIN (context);

-- RLS policy
ALTER TABLE engine_llm_conversations ENABLE ROW LEVEL SECURITY;
CREATE POLICY llm_conversations_access ON engine_llm_conversations
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

COMMENT ON TABLE engine_llm_conversations IS 'Verbatim log of all LLM API calls for debugging and analytics';
COMMENT ON COLUMN engine_llm_conversations.conversation_id IS 'Groups related calls in multi-turn streaming conversations';
COMMENT ON COLUMN engine_llm_conversations.iteration IS 'Tool-calling iteration number within a single user request';
COMMENT ON COLUMN engine_llm_conversations.request_messages IS 'Full OpenAI-format message array sent to LLM';
COMMENT ON COLUMN engine_llm_conversations.context IS 'Caller-specific context (workflow_id, task_name, session_id, etc.)';

-- ============================================================================
-- Table: engine_workflow_state
-- Tracks the state of each entity during ontology extraction
-- This is ephemeral data - deleted when workflow completes successfully.
-- ============================================================================

CREATE TABLE engine_workflow_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    workflow_id UUID NOT NULL REFERENCES engine_ontology_workflows(id) ON DELETE CASCADE,

    -- Entity identification
    -- entity_type: 'global' | 'table' | 'column'
    -- entity_key: '' (global) | 'orders' (table) | 'orders.status' (column)
    entity_type VARCHAR(20) NOT NULL,
    entity_key TEXT NOT NULL,

    -- Extraction state
    status VARCHAR(30) NOT NULL DEFAULT 'pending',

    -- All extraction state: gathered data, LLM analysis, questions, answers
    state_data JSONB NOT NULL DEFAULT '{}',

    -- Change detection (for future incremental refresh)
    data_fingerprint TEXT,

    -- Error tracking
    last_error TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Check constraint for valid entity types
ALTER TABLE engine_workflow_state
    ADD CONSTRAINT engine_workflow_state_entity_type_check
    CHECK (entity_type IN ('global', 'table', 'column'));

-- Check constraint for valid status values (state machine)
-- pending → scanning → scanned → analyzing → complete
--                                    ↓
--                              needs_input → (answer) → analyzing
-- Any state can transition to: failed
ALTER TABLE engine_workflow_state
    ADD CONSTRAINT engine_workflow_state_status_check
    CHECK (status IN ('pending', 'scanning', 'scanned', 'analyzing',
                      'complete', 'needs_input', 'failed'));

-- Unique constraint: one state per entity per workflow
CREATE UNIQUE INDEX idx_workflow_state_entity
    ON engine_workflow_state(workflow_id, entity_type, entity_key);

-- Query indexes
CREATE INDEX idx_workflow_state_by_status
    ON engine_workflow_state(workflow_id, status);
CREATE INDEX idx_workflow_state_project
    ON engine_workflow_state(project_id);

-- RLS
ALTER TABLE engine_workflow_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY workflow_state_access ON engine_workflow_state
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger for updated_at
CREATE TRIGGER update_engine_workflow_state_updated_at
    BEFORE UPDATE ON engine_workflow_state
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Table: engine_ontology_questions
-- Questions generated during ontology extraction for user clarification
-- Questions are decoupled from workflow lifecycle for flexibility
-- ============================================================================

CREATE TABLE engine_ontology_questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id) ON DELETE CASCADE,

    -- Question content
    text TEXT NOT NULL,
    reasoning TEXT,                          -- Why the LLM generated this question
    category VARCHAR(100),                   -- e.g., 'relationship', 'terminology', 'business_rules'

    -- Priority and requirements
    priority INTEGER NOT NULL DEFAULT 3,     -- 1=critical, 2=high, 3=medium, 4=low, 5=optional
    is_required BOOLEAN NOT NULL DEFAULT false,

    -- What this question affects (for deterministic entity updates)
    -- Structure: {"tables": ["table1"], "columns": ["col1", "col2"], "entity_type": "table|column"}
    affects JSONB NOT NULL DEFAULT '{}',

    -- Source entity information (which entity generated this question)
    source_entity_type VARCHAR(20),          -- 'table', 'column', 'global'
    source_entity_key TEXT,                  -- e.g., 'users' or 'users.email'

    -- Status tracking
    status VARCHAR(50) NOT NULL DEFAULT 'pending',

    -- Answer information
    answer TEXT,
    answered_by UUID,
    answered_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Check constraint for valid question statuses
ALTER TABLE engine_ontology_questions
    ADD CONSTRAINT engine_ontology_questions_status_check
    CHECK (status IN ('pending', 'answered', 'skipped', 'deleted'));

-- Check constraint for valid priority values (1=highest, 5=lowest)
ALTER TABLE engine_ontology_questions
    ADD CONSTRAINT engine_ontology_questions_priority_check
    CHECK (priority >= 1 AND priority <= 5);

-- Query by project
CREATE INDEX idx_engine_ontology_questions_project ON engine_ontology_questions(project_id);

-- Query by ontology
CREATE INDEX idx_engine_ontology_questions_ontology ON engine_ontology_questions(ontology_id);

-- Efficient query for pending questions by priority
CREATE INDEX idx_engine_ontology_questions_pending ON engine_ontology_questions(project_id, status, priority)
    WHERE status = 'pending';

-- Query for required pending questions
CREATE INDEX idx_engine_ontology_questions_required ON engine_ontology_questions(project_id, is_required)
    WHERE status = 'pending' AND is_required = true;

-- RLS
ALTER TABLE engine_ontology_questions ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_questions_access ON engine_ontology_questions
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger
CREATE TRIGGER update_engine_ontology_questions_updated_at
    BEFORE UPDATE ON engine_ontology_questions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
