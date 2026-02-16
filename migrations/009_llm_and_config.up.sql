-- 009_llm_and_config.up.sql
-- LLM conversations, MCP config, business glossary, pending changes, installed apps, audit log

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
ALTER TABLE engine_llm_conversations FORCE ROW LEVEL SECURITY;
CREATE POLICY llm_conversations_access ON engine_llm_conversations FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- MCP server configuration (with audit_retention_days and alert_config folded in)
CREATE TABLE engine_mcp_config (
    project_id uuid NOT NULL,
    tool_groups jsonb DEFAULT '{"developer": {"enabled": false}}'::jsonb NOT NULL,
    agent_api_key_encrypted text,
    audit_retention_days INTEGER,
    alert_config JSONB,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (project_id),
    CONSTRAINT engine_mcp_config_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

COMMENT ON COLUMN engine_mcp_config.audit_retention_days IS 'Admin-configurable retention period in days for audit and query history data. NULL = 90 days (default).';
COMMENT ON COLUMN engine_mcp_config.alert_config IS 'Per-project alert configuration: master toggle, per-alert-type enable/disable, severity overrides, and thresholds';

CREATE INDEX idx_engine_mcp_config_agent_key ON engine_mcp_config USING btree (agent_api_key_encrypted) WHERE (agent_api_key_encrypted IS NOT NULL);

CREATE TRIGGER update_engine_mcp_config_updated_at
    BEFORE UPDATE ON engine_mcp_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_mcp_config ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_mcp_config FORCE ROW LEVEL SECURITY;
CREATE POLICY mcp_config_access ON engine_mcp_config FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- Business glossary with metric definitions (defining_sql nullable per migration 029)
CREATE TABLE engine_business_glossary (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    ontology_id uuid,
    term text NOT NULL,
    definition text NOT NULL,
    defining_sql text DEFAULT '',
    base_table text,
    output_columns jsonb,
    enrichment_status text DEFAULT 'pending'::text,
    enrichment_error text,

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_business_glossary_source_check CHECK (source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_business_glossary_last_edit_source_check CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
    CONSTRAINT engine_business_glossary_enrichment_status_check CHECK (enrichment_status IS NULL OR enrichment_status = ANY (ARRAY['pending'::text, 'success'::text, 'failed'::text])),
    CONSTRAINT engine_business_glossary_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_business_glossary_ontology_id_fkey FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    CONSTRAINT engine_business_glossary_created_by_fkey FOREIGN KEY (project_id, created_by) REFERENCES engine_users(project_id, user_id),
    CONSTRAINT engine_business_glossary_updated_by_fkey FOREIGN KEY (project_id, updated_by) REFERENCES engine_users(project_id, user_id)
);

-- Unique constraint on (project_id, ontology_id, term) to allow same term across ontologies
CREATE UNIQUE INDEX engine_business_glossary_project_ontology_term_unique ON engine_business_glossary USING btree (project_id, ontology_id, term);

COMMENT ON TABLE engine_business_glossary IS 'Business terms with definitive SQL definitions for MCP query composition';
COMMENT ON COLUMN engine_business_glossary.term IS 'Business term name (e.g., "Active Users", "Monthly Recurring Revenue")';
COMMENT ON COLUMN engine_business_glossary.definition IS 'Human-readable description of what this term means';
COMMENT ON COLUMN engine_business_glossary.defining_sql IS 'Optional executable SQL that defines this metric (SELECT statement). Empty for conceptual terms without direct SQL representation.';
COMMENT ON COLUMN engine_business_glossary.base_table IS 'Primary table being queried (for quick reference)';
COMMENT ON COLUMN engine_business_glossary.output_columns IS 'Array of output columns with name, type, and optional description';
COMMENT ON COLUMN engine_business_glossary.ontology_id IS 'Optional link to specific ontology version for CASCADE delete';
COMMENT ON COLUMN engine_business_glossary.enrichment_status IS 'Status of SQL enrichment: pending, success, or failed';
COMMENT ON COLUMN engine_business_glossary.enrichment_error IS 'Error message if enrichment failed, NULL otherwise';
COMMENT ON COLUMN engine_business_glossary.source IS 'How this term was created: inferred (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_business_glossary.last_edit_source IS 'How this term was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_business_glossary.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_business_glossary.updated_by IS 'UUID of user who last updated this term';

CREATE INDEX idx_business_glossary_project ON engine_business_glossary USING btree (project_id);
CREATE INDEX idx_business_glossary_ontology ON engine_business_glossary USING btree (ontology_id);
CREATE INDEX idx_business_glossary_base_table ON engine_business_glossary USING btree (base_table);
CREATE INDEX idx_business_glossary_source ON engine_business_glossary USING btree (source);
CREATE INDEX idx_business_glossary_enrichment_status ON engine_business_glossary USING btree (enrichment_status);

CREATE TRIGGER update_business_glossary_updated_at
    BEFORE UPDATE ON engine_business_glossary
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Glossary term aliases (alternative names for terms)
CREATE TABLE engine_glossary_aliases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    glossary_id uuid NOT NULL,
    alias text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_glossary_aliases_unique UNIQUE (glossary_id, alias),
    CONSTRAINT engine_glossary_aliases_glossary_id_fkey FOREIGN KEY (glossary_id) REFERENCES engine_business_glossary(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_glossary_aliases IS 'Alternative names for glossary terms (e.g., MAU = Monthly Active Users)';

CREATE INDEX idx_glossary_aliases_glossary ON engine_glossary_aliases USING btree (glossary_id);
CREATE INDEX idx_glossary_aliases_alias ON engine_glossary_aliases USING btree (alias);

-- Pending ontology changes for review workflow
CREATE TABLE engine_ontology_pending_changes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,

    -- Change classification
    change_type text NOT NULL,
    change_source text NOT NULL DEFAULT 'schema_refresh',

    -- What changed
    table_name text,
    column_name text,
    old_value jsonb,
    new_value jsonb,

    -- Suggested ontology action
    suggested_action text,
    suggested_payload jsonb,

    -- Review state
    status text NOT NULL DEFAULT 'pending',
    reviewed_by text,
    reviewed_at timestamp with time zone,

    -- Metadata
    created_at timestamp with time zone DEFAULT now() NOT NULL,

    PRIMARY KEY (id),
    CONSTRAINT engine_pending_changes_change_type_check CHECK (change_type IN (
        'new_table', 'dropped_table',
        'new_column', 'dropped_column', 'modified_column',
        'new_enum_value', 'cardinality_change', 'new_fk_pattern'
    )),
    CONSTRAINT engine_pending_changes_change_source_check CHECK (change_source IN (
        'schema_refresh', 'data_scan', 'manual'
    )),
    CONSTRAINT engine_pending_changes_status_check CHECK (status IN (
        'pending', 'approved', 'rejected', 'auto_applied'
    )),
    CONSTRAINT engine_pending_changes_project_id_fkey FOREIGN KEY (project_id)
        REFERENCES engine_projects(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_ontology_pending_changes IS 'Pending schema/data changes for ontology review before applying';
COMMENT ON COLUMN engine_ontology_pending_changes.change_type IS 'Type of change: new_table, dropped_table, new_column, dropped_column, modified_column, etc.';
COMMENT ON COLUMN engine_ontology_pending_changes.change_source IS 'Origin: schema_refresh (DDL sync), data_scan (data analysis), manual';
COMMENT ON COLUMN engine_ontology_pending_changes.table_name IS 'Affected table name (schema.table format)';
COMMENT ON COLUMN engine_ontology_pending_changes.column_name IS 'Affected column name (for column-level changes)';
COMMENT ON COLUMN engine_ontology_pending_changes.old_value IS 'Previous state (type, enum values, etc.)';
COMMENT ON COLUMN engine_ontology_pending_changes.new_value IS 'New state after the change';
COMMENT ON COLUMN engine_ontology_pending_changes.suggested_action IS 'Recommended ontology action: create_entity, update_entity, create_column_metadata, etc.';
COMMENT ON COLUMN engine_ontology_pending_changes.suggested_payload IS 'Parameters for the suggested action';
COMMENT ON COLUMN engine_ontology_pending_changes.status IS 'Review state: pending, approved, rejected, auto_applied';
COMMENT ON COLUMN engine_ontology_pending_changes.reviewed_by IS 'Who reviewed: admin, mcp, auto';

-- Indexes
CREATE INDEX idx_pending_changes_project_status ON engine_ontology_pending_changes(project_id, status);
CREATE INDEX idx_pending_changes_project_created ON engine_ontology_pending_changes(project_id, created_at DESC);
CREATE INDEX idx_pending_changes_change_type ON engine_ontology_pending_changes(change_type);

-- Installed applications per project (with activated_at from migration 030)
CREATE TABLE engine_installed_apps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    app_id VARCHAR(50) NOT NULL,  -- e.g., 'ai-data-liaison', 'mcp-server'
    installed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    installed_by VARCHAR(255),  -- User email/ID who installed
    settings JSONB DEFAULT '{}'::jsonb,
    activated_at TIMESTAMPTZ,

    CONSTRAINT unique_project_app UNIQUE (project_id, app_id)
);

-- RLS for tenant isolation
ALTER TABLE engine_installed_apps ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_installed_apps FORCE ROW LEVEL SECURITY;
CREATE POLICY installed_apps_access ON engine_installed_apps FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

-- Index for listing apps by project
CREATE INDEX idx_installed_apps_project ON engine_installed_apps(project_id);

-- Unified audit log for tracking changes across all ontology objects
CREATE TABLE engine_audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- What changed
    entity_type text NOT NULL,  -- 'entity', 'relationship', 'glossary_term', 'project_knowledge'
    entity_id uuid NOT NULL,    -- ID of the affected object
    action text NOT NULL,       -- 'create', 'update', 'delete'

    -- Who/how
    source text NOT NULL,       -- 'inferred', 'mcp', 'manual'
    user_id uuid,               -- Who triggered the action (from JWT, may be null for system operations)

    -- What changed (for updates)
    changed_fields jsonb,       -- {"description": {"old": "...", "new": "..."}}

    -- When
    created_at timestamp with time zone NOT NULL DEFAULT now(),

    CONSTRAINT engine_audit_log_entity_type_check CHECK (entity_type IN ('entity', 'relationship', 'glossary_term', 'project_knowledge')),
    CONSTRAINT engine_audit_log_action_check CHECK (action IN ('create', 'update', 'delete')),
    CONSTRAINT engine_audit_log_source_check CHECK (source IN ('inferred', 'mcp', 'manual'))
);

COMMENT ON TABLE engine_audit_log IS 'Chronological audit trail of changes to ontology objects';
COMMENT ON COLUMN engine_audit_log.entity_type IS 'Type of object: entity, relationship, glossary_term, project_knowledge';
COMMENT ON COLUMN engine_audit_log.entity_id IS 'UUID of the affected object';
COMMENT ON COLUMN engine_audit_log.action IS 'What happened: create, update, delete';
COMMENT ON COLUMN engine_audit_log.source IS 'How the action was triggered: inference (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_audit_log.user_id IS 'UUID of user who triggered the action (from JWT)';
COMMENT ON COLUMN engine_audit_log.changed_fields IS 'For updates, JSON object mapping field names to {old, new} values';

-- Indexes for common query patterns
CREATE INDEX idx_audit_log_project ON engine_audit_log(project_id);
CREATE INDEX idx_audit_log_entity ON engine_audit_log(entity_type, entity_id);
CREATE INDEX idx_audit_log_user ON engine_audit_log(user_id);
CREATE INDEX idx_audit_log_time ON engine_audit_log(project_id, created_at DESC);

-- RLS
ALTER TABLE engine_business_glossary ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_business_glossary FORCE ROW LEVEL SECURITY;
CREATE POLICY business_glossary_access ON engine_business_glossary FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

ALTER TABLE engine_glossary_aliases ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_glossary_aliases FORCE ROW LEVEL SECURITY;
CREATE POLICY glossary_aliases_access ON engine_glossary_aliases FOR ALL
    USING (glossary_id IN (
        SELECT id FROM engine_business_glossary
        WHERE rls_tenant_id() IS NULL OR project_id = rls_tenant_id()
    ));

ALTER TABLE engine_ontology_pending_changes ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_ontology_pending_changes FORCE ROW LEVEL SECURITY;
CREATE POLICY pending_changes_access ON engine_ontology_pending_changes FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());

ALTER TABLE engine_audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_audit_log FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_log_access ON engine_audit_log FOR ALL
    USING (rls_tenant_id() IS NULL OR project_id = rls_tenant_id())
    WITH CHECK (rls_tenant_id() IS NULL OR project_id = rls_tenant_id());
