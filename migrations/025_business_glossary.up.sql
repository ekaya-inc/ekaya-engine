-- Migration 025: Business glossary for metric definitions
-- Enables reverse lookup from business term → schema/SQL pattern

-- ============================================================================
-- Business glossary table
-- ============================================================================
CREATE TABLE engine_business_glossary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    term TEXT NOT NULL,
    definition TEXT NOT NULL,
    sql_pattern TEXT,
    base_table TEXT,
    columns_used JSONB,
    filters JSONB,
    aggregation TEXT,
    source TEXT NOT NULL DEFAULT 'user',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, term)
);

-- Index for project-scoped queries
CREATE INDEX idx_business_glossary_project ON engine_business_glossary(project_id);

-- ============================================================================
-- Row Level Security
-- ============================================================================
ALTER TABLE engine_business_glossary ENABLE ROW LEVEL SECURITY;

CREATE POLICY business_glossary_access ON engine_business_glossary
    FOR ALL USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    )
    WITH CHECK (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- ============================================================================
-- Auto-update timestamp trigger
-- ============================================================================
CREATE TRIGGER update_business_glossary_updated_at
    BEFORE UPDATE ON engine_business_glossary
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Comments
-- ============================================================================
COMMENT ON TABLE engine_business_glossary IS
    'Business term definitions with technical mappings for metric calculations';

COMMENT ON COLUMN engine_business_glossary.term IS
    'Business term name (e.g., "Revenue", "GMV", "Active User")';

COMMENT ON COLUMN engine_business_glossary.definition IS
    'Human-readable description of what this term means';

COMMENT ON COLUMN engine_business_glossary.sql_pattern IS
    'SQL snippet showing how to calculate this metric';

COMMENT ON COLUMN engine_business_glossary.base_table IS
    'Primary table used for this metric';

COMMENT ON COLUMN engine_business_glossary.columns_used IS
    'JSON array of column names involved in the calculation';

COMMENT ON COLUMN engine_business_glossary.filters IS
    'JSON array of filter conditions (e.g., [{"column":"state","operator":"=","values":["completed"]}])';

COMMENT ON COLUMN engine_business_glossary.aggregation IS
    'Aggregation function used (e.g., "SUM", "COUNT", "AVG")';

COMMENT ON COLUMN engine_business_glossary.source IS
    'Origin of the term: "user" (manually defined), "suggested" (LLM-generated, interactive), or "discovered" (LLM-generated, DAG workflow)';

COMMENT ON COLUMN engine_business_glossary.created_by IS
    'User who created this term (null for suggested terms)';
