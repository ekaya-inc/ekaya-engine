-- Migration 031: Glossary with Defining SQL
-- Transforms glossary from fragmented "bits" storage to first-class SQL definitions
-- Each term has a definitive SQL that MCP clients can use to compose queries

-- ============================================================================
-- Drop old fragmented glossary table
-- ============================================================================
DROP TABLE IF EXISTS engine_business_glossary CASCADE;

-- ============================================================================
-- Create glossary table with defining_sql as primary artifact
-- ============================================================================
CREATE TABLE engine_business_glossary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- Core fields
    term TEXT NOT NULL,
    definition TEXT NOT NULL,
    defining_sql TEXT NOT NULL,  -- The definitive SQL that defines this term

    -- Metadata
    base_table TEXT,  -- Primary table (derived from SQL but stored for quick reference)

    -- Output schema (populated when SQL is tested)
    output_columns JSONB,  -- [{name, type, description}] - same pattern as approved queries

    -- Source tracking
    source TEXT NOT NULL DEFAULT 'inferred',  -- 'inferred', 'manual', 'client'

    -- Audit
    created_by UUID,  -- User who created (null for inferred)
    updated_by UUID,  -- User who last updated
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT engine_business_glossary_project_term_unique UNIQUE (project_id, term),
    CONSTRAINT engine_business_glossary_source_check CHECK (source IN ('inferred', 'manual', 'client'))
);

-- ============================================================================
-- Indexes
-- ============================================================================
CREATE INDEX idx_business_glossary_project ON engine_business_glossary(project_id);
CREATE INDEX idx_business_glossary_source ON engine_business_glossary(source);
CREATE INDEX idx_business_glossary_base_table ON engine_business_glossary(base_table);

-- ============================================================================
-- Row Level Security
-- ============================================================================
ALTER TABLE engine_business_glossary ENABLE ROW LEVEL SECURITY;

CREATE POLICY business_glossary_access ON engine_business_glossary
    USING (
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
-- Create glossary aliases table
-- ============================================================================
CREATE TABLE engine_glossary_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    glossary_id UUID NOT NULL REFERENCES engine_business_glossary(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT engine_glossary_aliases_unique UNIQUE (glossary_id, alias)
);

-- ============================================================================
-- Aliases indexes
-- ============================================================================
CREATE INDEX idx_glossary_aliases_glossary ON engine_glossary_aliases(glossary_id);
CREATE INDEX idx_glossary_aliases_alias ON engine_glossary_aliases(alias);

-- ============================================================================
-- Aliases RLS (inherits from parent via FK, but add explicit policy for direct queries)
-- ============================================================================
ALTER TABLE engine_glossary_aliases ENABLE ROW LEVEL SECURITY;

CREATE POLICY glossary_aliases_access ON engine_glossary_aliases
    USING (
        glossary_id IN (
            SELECT id FROM engine_business_glossary
            WHERE current_setting('app.current_project_id', true) IS NULL
               OR project_id = current_setting('app.current_project_id', true)::uuid
        )
    );

-- ============================================================================
-- Comments
-- ============================================================================
COMMENT ON TABLE engine_business_glossary IS
    'Business terms with definitive SQL definitions for MCP query composition';

COMMENT ON COLUMN engine_business_glossary.term IS
    'Business term name (e.g., "Active Users", "Monthly Recurring Revenue")';

COMMENT ON COLUMN engine_business_glossary.definition IS
    'Human-readable description of what this term means';

COMMENT ON COLUMN engine_business_glossary.defining_sql IS
    'Complete executable SQL that defines this metric (SELECT statement)';

COMMENT ON COLUMN engine_business_glossary.base_table IS
    'Primary table being queried (for quick reference)';

COMMENT ON COLUMN engine_business_glossary.output_columns IS
    'Array of output columns with name, type, and optional description';

COMMENT ON COLUMN engine_business_glossary.source IS
    'Origin: inferred (LLM during extraction), manual (UI), or client (MCP)';

COMMENT ON TABLE engine_glossary_aliases IS
    'Alternative names for glossary terms (e.g., MAU = Monthly Active Users)';
