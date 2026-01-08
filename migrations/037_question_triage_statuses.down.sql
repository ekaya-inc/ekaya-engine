-- Migration 037 rollback: Remove triage statuses and status_reason

-- Remove status_reason column
ALTER TABLE engine_ontology_questions
    DROP COLUMN status_reason;

-- Revert status check constraint to original values
ALTER TABLE engine_ontology_questions
    DROP CONSTRAINT engine_ontology_questions_status_check;

ALTER TABLE engine_ontology_questions
    ADD CONSTRAINT engine_ontology_questions_status_check
    CHECK (status IN ('pending', 'answered', 'skipped', 'deleted'));
