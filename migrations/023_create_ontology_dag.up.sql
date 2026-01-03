-- Migration 023: Ontology DAG System
-- Creates tables for DAG-based workflow execution:
--   - engine_ontology_dag: DAG execution state
--   - engine_dag_nodes: Per-node execution state
-- And drops legacy workflow tables:
--   - engine_ontology_workflows
--   - engine_relationship_candidates
--   - engine_workflow_state

-- ============================================================================
-- Table: engine_ontology_dag
-- Stores the DAG execution state for ontology extraction
-- ============================================================================

CREATE TABLE engine_ontology_dag (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    datasource_id UUID NOT NULL REFERENCES engine_datasources(id) ON DELETE CASCADE,
    ontology_id UUID REFERENCES engine_ontologies(id) ON DELETE CASCADE,

    -- Execution state
    status VARCHAR(30) NOT NULL DEFAULT 'pending',
    current_node VARCHAR(50),

    -- Schema tracking
    schema_fingerprint TEXT,

    -- Ownership (multi-server support)
    owner_id UUID,
    last_heartbeat TIMESTAMPTZ,

    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Check constraint for valid DAG statuses
ALTER TABLE engine_ontology_dag
    ADD CONSTRAINT engine_ontology_dag_status_check
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'));

-- One active DAG per datasource (partial unique index)
CREATE UNIQUE INDEX idx_engine_ontology_dag_unique_active
    ON engine_ontology_dag(datasource_id)
    WHERE status IN ('pending', 'running');

-- Query indexes
CREATE INDEX idx_engine_ontology_dag_project ON engine_ontology_dag(project_id);
CREATE INDEX idx_engine_ontology_dag_status ON engine_ontology_dag(status);
CREATE INDEX idx_engine_ontology_dag_datasource ON engine_ontology_dag(datasource_id);
CREATE INDEX idx_engine_ontology_dag_heartbeat ON engine_ontology_dag(last_heartbeat)
    WHERE status = 'running';

-- RLS
ALTER TABLE engine_ontology_dag ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_dag_access ON engine_ontology_dag
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Trigger for updated_at
CREATE TRIGGER update_engine_ontology_dag_updated_at
    BEFORE UPDATE ON engine_ontology_dag
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE engine_ontology_dag IS
    'DAG execution state for ontology extraction workflow';

COMMENT ON COLUMN engine_ontology_dag.status IS
    'Execution status: pending, running, completed, failed, cancelled';

COMMENT ON COLUMN engine_ontology_dag.current_node IS
    'Node currently executing: EntityDiscovery, EntityEnrichment, etc.';

COMMENT ON COLUMN engine_ontology_dag.schema_fingerprint IS
    'Hash of schema for detecting changes between runs';

-- ============================================================================
-- Table: engine_dag_nodes
-- Stores per-node execution state within a DAG
-- ============================================================================

CREATE TABLE engine_dag_nodes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    dag_id UUID NOT NULL REFERENCES engine_ontology_dag(id) ON DELETE CASCADE,

    -- Node identification
    node_name VARCHAR(50) NOT NULL,
    node_order INT NOT NULL,

    -- Execution state
    status VARCHAR(30) NOT NULL DEFAULT 'pending',
    progress JSONB,

    -- Timing
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    duration_ms INT,

    -- Error handling
    error_message TEXT,
    retry_count INT NOT NULL DEFAULT 0,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- One node per name per DAG
    UNIQUE(dag_id, node_name)
);

-- Check constraint for valid node statuses
ALTER TABLE engine_dag_nodes
    ADD CONSTRAINT engine_dag_nodes_status_check
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'skipped'));

-- Query indexes
CREATE INDEX idx_engine_dag_nodes_dag ON engine_dag_nodes(dag_id);
CREATE INDEX idx_engine_dag_nodes_status ON engine_dag_nodes(dag_id, status);
CREATE INDEX idx_engine_dag_nodes_order ON engine_dag_nodes(dag_id, node_order);

-- RLS (via parent DAG)
ALTER TABLE engine_dag_nodes ENABLE ROW LEVEL SECURITY;
CREATE POLICY dag_nodes_access ON engine_dag_nodes
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR dag_id IN (
            SELECT id FROM engine_ontology_dag
            WHERE project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- Trigger for updated_at
CREATE TRIGGER update_engine_dag_nodes_updated_at
    BEFORE UPDATE ON engine_dag_nodes
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE engine_dag_nodes IS
    'Per-node execution state within an ontology DAG';

COMMENT ON COLUMN engine_dag_nodes.node_name IS
    'Node name: EntityDiscovery, EntityEnrichment, RelationshipDiscovery, RelationshipEnrichment, OntologyFinalization, ColumnEnrichment';

COMMENT ON COLUMN engine_dag_nodes.node_order IS
    'Execution order (1-6)';

COMMENT ON COLUMN engine_dag_nodes.progress IS
    'Node progress: {current: N, total: M, message: "..."}';

-- ============================================================================
-- Drop legacy workflow tables
-- These tables are replaced by the DAG system
-- ============================================================================

-- Drop engine_relationship_candidates (depends on engine_ontology_workflows)
DROP TABLE IF EXISTS engine_relationship_candidates;

-- Drop engine_workflow_state (depends on engine_ontology_workflows)
DROP TABLE IF EXISTS engine_workflow_state;

-- Drop engine_ontology_workflows
DROP TABLE IF EXISTS engine_ontology_workflows;
