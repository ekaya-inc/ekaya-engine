-- Migration 010: Add relationship detection workflow support
-- Extends engine_ontology_workflows to support phased workflow (relationships → ontology)

-- ============================================================================
-- Extend engine_ontology_workflows with phase and datasource_id
-- ============================================================================

-- Add phase column to distinguish between relationships and ontology phases
ALTER TABLE engine_ontology_workflows
    ADD COLUMN phase VARCHAR(20) NOT NULL DEFAULT 'relationships';

-- Add check constraint for valid phases
ALTER TABLE engine_ontology_workflows
    ADD CONSTRAINT engine_ontology_workflows_phase_check
    CHECK (phase IN ('relationships', 'ontology'));

-- Add datasource_id for relationships phase (relationships are per-datasource)
-- Nullable because ontology phase is still per-project
ALTER TABLE engine_ontology_workflows
    ADD COLUMN datasource_id UUID REFERENCES engine_datasources(id) ON DELETE CASCADE;

-- Add index for datasource queries
CREATE INDEX idx_engine_ontology_workflows_datasource
    ON engine_ontology_workflows(datasource_id, phase, state)
    WHERE datasource_id IS NOT NULL;

-- Add comment explaining the phase field
COMMENT ON COLUMN engine_ontology_workflows.phase IS
    'Workflow phase: relationships (scanning + FK detection) or ontology (LLM analysis)';

COMMENT ON COLUMN engine_ontology_workflows.datasource_id IS
    'Datasource for relationships phase; NULL for ontology phase (project-level)';
