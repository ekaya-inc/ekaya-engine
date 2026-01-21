-- 006_ontology_dag.up.sql
-- DAG execution state for ontology extraction workflow

CREATE TABLE engine_ontology_dag (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    datasource_id uuid NOT NULL,
    ontology_id uuid,
    status character varying(30) DEFAULT 'pending'::character varying NOT NULL,
    current_node character varying(50),
    schema_fingerprint text,
    owner_id uuid,
    last_heartbeat timestamp with time zone,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontology_dag_status_check CHECK ((status)::text = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])),
    CONSTRAINT engine_ontology_dag_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_ontology_dag_datasource_id_fkey FOREIGN KEY (datasource_id) REFERENCES engine_datasources(id) ON DELETE CASCADE,
    CONSTRAINT engine_ontology_dag_ontology_id_fkey FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_ontology_dag IS 'DAG execution state for ontology extraction workflow';
COMMENT ON COLUMN engine_ontology_dag.status IS 'Execution status: pending, running, completed, failed, cancelled';
COMMENT ON COLUMN engine_ontology_dag.current_node IS 'Node currently executing: EntityDiscovery, EntityEnrichment, etc.';
COMMENT ON COLUMN engine_ontology_dag.schema_fingerprint IS 'Hash of schema for detecting changes between runs';

CREATE INDEX idx_engine_ontology_dag_project ON engine_ontology_dag USING btree (project_id);
CREATE INDEX idx_engine_ontology_dag_datasource ON engine_ontology_dag USING btree (datasource_id);
CREATE INDEX idx_engine_ontology_dag_status ON engine_ontology_dag USING btree (status);
CREATE INDEX idx_engine_ontology_dag_heartbeat ON engine_ontology_dag USING btree (last_heartbeat) WHERE ((status)::text = 'running'::text);
CREATE UNIQUE INDEX idx_engine_ontology_dag_unique_active ON engine_ontology_dag USING btree (datasource_id) WHERE ((status)::text = ANY (ARRAY['pending'::text, 'running'::text]));

CREATE TRIGGER update_engine_ontology_dag_updated_at
    BEFORE UPDATE ON engine_ontology_dag
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Per-node DAG state
CREATE TABLE engine_dag_nodes (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    dag_id uuid NOT NULL,
    node_name character varying(50) NOT NULL,
    node_order integer NOT NULL,
    status character varying(30) DEFAULT 'pending'::character varying NOT NULL,
    progress jsonb,
    started_at timestamp with time zone,
    completed_at timestamp with time zone,
    duration_ms integer,
    error_message text,
    retry_count integer DEFAULT 0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_dag_nodes_dag_id_node_name_key UNIQUE (dag_id, node_name),
    CONSTRAINT engine_dag_nodes_status_check CHECK ((status)::text = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'skipped'::text])),
    CONSTRAINT engine_dag_nodes_dag_id_fkey FOREIGN KEY (dag_id) REFERENCES engine_ontology_dag(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_dag_nodes IS 'Per-node execution state within an ontology DAG';
COMMENT ON COLUMN engine_dag_nodes.node_name IS 'Node name: EntityDiscovery, EntityEnrichment, RelationshipDiscovery, RelationshipEnrichment, OntologyFinalization, ColumnEnrichment';
COMMENT ON COLUMN engine_dag_nodes.node_order IS 'Execution order (1-6)';
COMMENT ON COLUMN engine_dag_nodes.progress IS 'Node progress: {current: N, total: M, message: "..."}';

CREATE INDEX idx_engine_dag_nodes_dag ON engine_dag_nodes USING btree (dag_id);
CREATE INDEX idx_engine_dag_nodes_order ON engine_dag_nodes USING btree (dag_id, node_order);
CREATE INDEX idx_engine_dag_nodes_status ON engine_dag_nodes USING btree (dag_id, status);

CREATE TRIGGER update_engine_dag_nodes_updated_at
    BEFORE UPDATE ON engine_dag_nodes
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- RLS
ALTER TABLE engine_ontology_dag ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_dag_access ON engine_ontology_dag FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

ALTER TABLE engine_dag_nodes ENABLE ROW LEVEL SECURITY;
CREATE POLICY dag_nodes_access ON engine_dag_nodes FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR dag_id IN (
        SELECT id FROM engine_ontology_dag WHERE project_id = current_setting('app.current_project_id', true)::uuid
    ));
