-- Migration 037: Add triage statuses and status_reason to ontology questions
-- Adds 'escalated' and 'dismissed' statuses plus status_reason column
-- to support AI agent question triage workflow

-- Add status_reason column to store rationale for status changes
ALTER TABLE engine_ontology_questions
    ADD COLUMN status_reason TEXT;

COMMENT ON COLUMN engine_ontology_questions.status_reason IS 'Reason for skip/escalate/dismiss (e.g., "Need access to frontend repo")';

-- Update status check constraint to include new statuses
ALTER TABLE engine_ontology_questions
    DROP CONSTRAINT engine_ontology_questions_status_check;

ALTER TABLE engine_ontology_questions
    ADD CONSTRAINT engine_ontology_questions_status_check
    CHECK (status IN ('pending', 'answered', 'skipped', 'escalated', 'dismissed', 'deleted'));

COMMENT ON CONSTRAINT engine_ontology_questions_status_check ON engine_ontology_questions IS 'Valid statuses: pending, answered, skipped (revisit later), escalated (needs human), dismissed (not worth pursuing), deleted';
