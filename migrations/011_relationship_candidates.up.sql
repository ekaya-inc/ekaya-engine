-- Migration 011: Relationship candidates table
-- Stores detected relationship candidates with metrics and user review state

-- ============================================================================
-- Table: engine_relationship_candidates
-- Tracks relationship candidates detected during the relationships phase
-- ============================================================================

CREATE TABLE engine_relationship_candidates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id UUID NOT NULL REFERENCES engine_ontology_workflows(id) ON DELETE CASCADE,
    datasource_id UUID NOT NULL REFERENCES engine_datasources(id) ON DELETE CASCADE,

    -- Source and target columns
    source_column_id UUID NOT NULL REFERENCES engine_schema_columns(id) ON DELETE CASCADE,
    target_column_id UUID NOT NULL REFERENCES engine_schema_columns(id) ON DELETE CASCADE,

    -- Detection metadata
    detection_method VARCHAR(20) NOT NULL,
        -- 'value_match': High value overlap
        -- 'name_inference': Column naming pattern (user_id → users.id)
        -- 'llm': LLM inferred relationship
        -- 'hybrid': Multiple methods agree

    -- Confidence and reasoning
    confidence DECIMAL(3,2) NOT NULL,  -- 0.00-1.00
    llm_reasoning TEXT,                 -- LLM explanation

    -- Metrics from sample-based detection
    value_match_rate DECIMAL(5,4),      -- Sample value overlap (0.0000-1.0000)
    name_similarity DECIMAL(3,2),       -- Column name similarity

    -- Metrics from test join (actual SQL join against datasource)
    cardinality VARCHAR(10),            -- "1:1", "1:N", "N:1", "N:M"
    join_match_rate DECIMAL(5,4),       -- Actual match rate from join
    orphan_rate DECIMAL(5,4),           -- % of source rows with no match
    target_coverage DECIMAL(5,4),       -- % of target rows that are referenced
    source_row_count BIGINT,            -- Total source rows
    target_row_count BIGINT,            -- Total target rows
    matched_rows BIGINT,                -- Source rows with matches
    orphan_rows BIGINT,                 -- Source rows without matches

    -- User review state
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
        -- 'pending': Awaiting user action (if is_required) or auto-decision
        -- 'accepted': Will be saved as relationship
        -- 'rejected': Will not be saved
    is_required BOOLEAN NOT NULL DEFAULT false,
        -- true: User must accept/reject before save
        -- false: Auto-decided based on confidence
    user_decision VARCHAR(20),
        -- 'accepted': User explicitly accepted
        -- 'rejected': User explicitly rejected
        -- NULL: Auto-decided

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Unique constraint: one candidate per source/target pair per workflow
    UNIQUE(workflow_id, source_column_id, target_column_id)
);

-- Check constraint for valid detection methods
ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_detection_method_check
    CHECK (detection_method IN ('value_match', 'name_inference', 'llm', 'hybrid'));

-- Check constraint for valid status values
ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_status_check
    CHECK (status IN ('pending', 'accepted', 'rejected'));

-- Check constraint for valid user decision values
ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_user_decision_check
    CHECK (user_decision IN ('accepted', 'rejected') OR user_decision IS NULL);

-- Check constraint for valid cardinality values
ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_cardinality_check
    CHECK (cardinality IN ('1:1', '1:N', 'N:1', 'N:M') OR cardinality IS NULL);

-- Check constraint: confidence must be between 0 and 1
ALTER TABLE engine_relationship_candidates
    ADD CONSTRAINT engine_relationship_candidates_confidence_check
    CHECK (confidence >= 0.00 AND confidence <= 1.00);

-- ============================================================================
-- Indexes for efficient queries
-- ============================================================================

-- Query by workflow
CREATE INDEX idx_rel_candidates_workflow ON engine_relationship_candidates(workflow_id);

-- Query by workflow and status (for fetching confirmed/needs_review/rejected)
CREATE INDEX idx_rel_candidates_status ON engine_relationship_candidates(workflow_id, status);

-- Efficiently find required pending candidates (blocks save)
CREATE INDEX idx_rel_candidates_required ON engine_relationship_candidates(workflow_id)
    WHERE is_required = true AND status = 'pending';

-- Query by datasource (for cleanup or reporting)
CREATE INDEX idx_rel_candidates_datasource ON engine_relationship_candidates(datasource_id);

-- ============================================================================
-- Row Level Security
-- ============================================================================

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

-- ============================================================================
-- Trigger for updated_at
-- ============================================================================

CREATE TRIGGER update_rel_candidates_updated_at
    BEFORE UPDATE ON engine_relationship_candidates
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Comments
-- ============================================================================

COMMENT ON TABLE engine_relationship_candidates IS
    'Relationship candidates detected during relationship discovery workflow';

COMMENT ON COLUMN engine_relationship_candidates.detection_method IS
    'Method used to detect candidate: value_match, name_inference, llm, or hybrid';

COMMENT ON COLUMN engine_relationship_candidates.confidence IS
    'Confidence score 0.00-1.00; ≥0.85 auto-decided, <0.85 requires user review';

COMMENT ON COLUMN engine_relationship_candidates.is_required IS
    'Whether user must explicitly accept/reject before workflow can be saved';

COMMENT ON COLUMN engine_relationship_candidates.status IS
    'Review status: pending (awaiting decision), accepted (will be saved), rejected (will not be saved)';
