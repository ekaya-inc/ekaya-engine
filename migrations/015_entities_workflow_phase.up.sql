-- Migration 015: Add 'entities' to workflow phase constraint
-- Allows entity discovery to run as a separate workflow phase

-- Drop the existing constraint
ALTER TABLE engine_ontology_workflows
    DROP CONSTRAINT engine_ontology_workflows_phase_check;

-- Add updated constraint including 'entities'
ALTER TABLE engine_ontology_workflows
    ADD CONSTRAINT engine_ontology_workflows_phase_check
    CHECK (phase IN ('relationships', 'ontology', 'entities'));

-- Update comment to reflect the new phase
COMMENT ON COLUMN engine_ontology_workflows.phase IS
    'Workflow phase: relationships (FK detection), entities (entity discovery), or ontology (LLM analysis)';
