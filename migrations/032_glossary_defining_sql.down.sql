-- Rollback Migration 031: Restore fragmented glossary schema
-- Note: This will lose data as the schemas are incompatible

DROP TABLE IF EXISTS engine_glossary_aliases CASCADE;
DROP TABLE IF EXISTS engine_business_glossary CASCADE;

-- Restore old schema (from migration 025)
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

CREATE INDEX idx_business_glossary_project ON engine_business_glossary(project_id);

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

CREATE TRIGGER update_business_glossary_updated_at
    BEFORE UPDATE ON engine_business_glossary
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
