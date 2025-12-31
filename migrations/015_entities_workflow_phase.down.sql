-- Migration 015 down: Revert to original phase constraint

-- Drop the updated constraint
ALTER TABLE engine_ontology_workflows
    DROP CONSTRAINT engine_ontology_workflows_phase_check;

-- Restore original constraint (without 'entities')
ALTER TABLE engine_ontology_workflows
    ADD CONSTRAINT engine_ontology_workflows_phase_check
    CHECK (phase IN ('relationships', 'ontology'));

-- Restore original comment
COMMENT ON COLUMN engine_ontology_workflows.phase IS
    'Workflow phase: relationships (scanning + FK detection) or ontology (LLM analysis)';
