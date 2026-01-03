-- Migration 023: Ontology DAG System (rollback)
-- Drops DAG tables and recreates legacy workflow tables

-- Drop DAG tables
DROP TABLE IF EXISTS engine_dag_nodes;
DROP TABLE IF EXISTS engine_ontology_dag;

-- ============================================================================
-- Recreate: engine_ontology_workflows
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
    owner_id UUID,
    last_heartbeat TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE engine_ontology_workflows
    ADD CONSTRAINT engine_ontology_workflows_state_check
    CHECK (state IN ('pending', 'running', 'paused', 'awaiting_input', 'completed', 'failed'));

CREATE INDEX idx_engine_ontology_workflows_project ON engine_ontology_workflows(project_id);
CREATE INDEX idx_engine_ontology_workflows_state ON engine_ontology_workflows(project_id, state);
CREATE INDEX idx_workflow_heartbeat ON engine_ontology_workflows (last_heartbeat)
    WHERE state = 'running';

ALTER TABLE engine_ontology_workflows ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_workflows_access ON engine_ontology_workflows
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

CREATE TRIGGER update_engine_ontology_workflows_updated_at
    BEFORE UPDATE ON engine_ontology_workflows
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Add ontology_id FK (added in migration 007 after ontologies table created)
ALTER TABLE engine_ontology_workflows
    ADD COLUMN ontology_id UUID REFERENCES engine_ontologies(id) ON DELETE CASCADE;
CREATE INDEX idx_engine_ontology_workflows_ontology ON engine_ontology_workflows(ontology_id);

-- Add phase and datasource_id columns (added in migration 015)
ALTER TABLE engine_ontology_workflows
    ADD COLUMN phase VARCHAR(20),
    ADD COLUMN datasource_id UUID REFERENCES engine_datasources(id) ON DELETE CASCADE;

-- ============================================================================
-- Recreate: engine_workflow_state
-- ============================================================================

CREATE TABLE engine_workflow_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    workflow_id UUID NOT NULL REFERENCES engine_ontology_workflows(id) ON DELETE CASCADE,
    entity_type VARCHAR(20) NOT NULL,
    entity_key TEXT NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'pending',
    state_data JSONB NOT NULL DEFAULT '{}',
    data_fingerprint TEXT,
    last_error TEXT,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE engine_workflow_state
    ADD CONSTRAINT engine_workflow_state_entity_type_check
    CHECK (entity_type IN ('global', 'table', 'column'));

ALTER TABLE engine_workflow_state
    ADD CONSTRAINT engine_workflow_state_status_check
    CHECK (status IN ('pending', 'scanning', 'scanned', 'analyzing',
                      'complete', 'needs_input', 'failed'));

CREATE UNIQUE INDEX idx_workflow_state_entity
    ON engine_workflow_state(workflow_id, entity_type, entity_key);
CREATE INDEX idx_workflow_state_by_status
    ON engine_workflow_state(workflow_id, status);
CREATE INDEX idx_workflow_state_project
    ON engine_workflow_state(project_id);

ALTER TABLE engine_workflow_state ENABLE ROW LEVEL SECURITY;
CREATE POLICY workflow_state_access ON engine_workflow_state
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

CREATE TRIGGER update_engine_workflow_state_updated_at
    BEFORE UPDATE ON engine_workflow_state
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Recreate: engine_relationship_candidates
-- ============================================================================

CREATE TABLE engine_relationship_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES engine_ontology_workflows(id) ON DELETE CASCADE,
    datasource_id UUID NOT NULL REFERENCES engine_datasources(id) ON DELETE CASCADE,
    source_column_id UUID NOT NULL REFERENCES engine_schema_columns(id) ON DELETE CASCADE,
    target_column_id UUID NOT NULL REFERENCES engine_schema_columns(id) ON DELETE CASCADE,
    detection_method VARCHAR(20) NOT NULL,
    confidence DECIMAL(3,2) NOT NULL,
    llm_reasoning TEXT,
    value_match_rate DECIMAL(5,4),
    name_similarity DECIMAL(3,2),
    cardinality VARCHAR(10),
    join_match_rate DECIMAL(5,4),
    orphan_rate DECIMAL(5,4),
    target_coverage DECIMAL(5,4),
    source_row_count BIGINT,
    target_row_count BIGINT,
    matched_rows BIGINT,
    orphan_rows BIGINT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    is_required BOOLEAN NOT NULL DEFAULT false,
    user_decision VARCHAR(20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workflow_id, source_column_id, target_column_id)
);

ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_detection_method_check
    CHECK (detection_method IN ('value_match', 'name_inference', 'llm', 'hybrid'));

ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_status_check
    CHECK (status IN ('pending', 'accepted', 'rejected'));

ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_user_decision_check
    CHECK (user_decision IN ('accepted', 'rejected') OR user_decision IS NULL);

ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_cardinality_check
    CHECK (cardinality IN ('1:1', '1:N', 'N:1', 'N:M') OR cardinality IS NULL);

ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_confidence_check
    CHECK (confidence >= 0.00 AND confidence <= 1.00);

CREATE INDEX idx_rel_candidates_workflow ON engine_relationship_candidates(workflow_id);
CREATE INDEX idx_rel_candidates_status ON engine_relationship_candidates(workflow_id, status);
CREATE INDEX idx_rel_candidates_required ON engine_relationship_candidates(workflow_id)
    WHERE is_required = true AND status = 'pending';
CREATE INDEX idx_rel_candidates_datasource ON engine_relationship_candidates(datasource_id);

ALTER TABLE engine_relationship_candidates ENABLE ROW LEVEL SECURITY;
CREATE POLICY rel_candidates_access ON engine_relationship_candidates
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR datasource_id IN (
            SELECT id FROM engine_datasources
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

CREATE TRIGGER update_rel_candidates_updated_at
    BEFORE UPDATE ON engine_relationship_candidates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
