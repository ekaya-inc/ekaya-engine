-- 024_audit_log.up.sql
-- Unified audit log for tracking changes across all ontology objects

CREATE TABLE engine_audit_log (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- What changed
    entity_type text NOT NULL,  -- 'entity', 'relationship', 'glossary_term', 'project_knowledge'
    entity_id uuid NOT NULL,    -- ID of the affected object
    action text NOT NULL,       -- 'create', 'update', 'delete'

    -- Who/how
    source text NOT NULL,       -- 'inference', 'mcp', 'manual'
    user_id uuid,               -- Who triggered the action (from JWT, may be null for system operations)

    -- What changed (for updates)
    changed_fields jsonb,       -- {"description": {"old": "...", "new": "..."}}

    -- When
    created_at timestamp with time zone NOT NULL DEFAULT now(),

    CONSTRAINT engine_audit_log_entity_type_check CHECK (entity_type IN ('entity', 'relationship', 'glossary_term', 'project_knowledge')),
    CONSTRAINT engine_audit_log_action_check CHECK (action IN ('create', 'update', 'delete')),
    CONSTRAINT engine_audit_log_source_check CHECK (source IN ('inference', 'mcp', 'manual'))
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
ALTER TABLE engine_audit_log ENABLE ROW LEVEL SECURITY;
CREATE POLICY audit_log_access ON engine_audit_log FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);
