-- Migration 005: Queries table for saved SQL queries
-- Stores user-defined queries with metadata and usage statistics

CREATE TABLE engine_queries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    datasource_id UUID NOT NULL REFERENCES engine_datasources(id) ON DELETE CASCADE,

    -- Query definition
    natural_language_prompt TEXT NOT NULL,
    additional_context TEXT,
    sql_query TEXT NOT NULL,
    dialect TEXT NOT NULL,  -- SQL dialect (postgres, mysql, etc.) for syntax highlighting

    -- Organization
    is_enabled BOOLEAN NOT NULL DEFAULT true,  -- Admin can disable without deleting

    -- Usage tracking
    usage_count INTEGER NOT NULL DEFAULT 0,
    last_used_at TIMESTAMPTZ,

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- Query indexes for common access patterns
CREATE INDEX idx_engine_queries_project ON engine_queries(project_id);
CREATE INDEX idx_engine_queries_datasource ON engine_queries(project_id, datasource_id);
CREATE INDEX idx_engine_queries_enabled ON engine_queries(project_id, datasource_id, is_enabled)
    WHERE deleted_at IS NULL;

-- RLS Policy
ALTER TABLE engine_queries ENABLE ROW LEVEL SECURITY;
CREATE POLICY queries_access ON engine_queries
    FOR ALL
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Auto-update timestamp trigger (reuses existing function from migration 003)
CREATE TRIGGER update_engine_queries_updated_at
    BEFORE UPDATE ON engine_queries
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
