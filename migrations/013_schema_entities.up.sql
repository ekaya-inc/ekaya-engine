-- Migration 013: Add schema entity tables
-- Supports entity-based relationship discovery with role semantics

-- ============================================================================
-- Table: engine_schema_entities
-- Stores discovered domain entities (user, account, order, etc.)
-- ============================================================================

CREATE TABLE engine_schema_entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    ontology_id UUID NOT NULL REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    name TEXT NOT NULL,                    -- "user", "account", "order"
    description TEXT,                      -- LLM explanation of what this entity represents
    primary_schema TEXT NOT NULL,          -- Schema where entity is primarily defined
    primary_table TEXT NOT NULL,           -- Table where entity is primarily defined
    primary_column TEXT NOT NULL,          -- Column that represents the entity's primary key
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(ontology_id, name)
);

-- Index for project queries
CREATE INDEX idx_engine_schema_entities_project
    ON engine_schema_entities(project_id);

-- Index for ontology queries
CREATE INDEX idx_engine_schema_entities_ontology
    ON engine_schema_entities(ontology_id);

-- Index for primary location lookups
CREATE INDEX idx_engine_schema_entities_primary_location
    ON engine_schema_entities(primary_schema, primary_table, primary_column);

COMMENT ON TABLE engine_schema_entities IS
    'Domain entities discovered during relationship analysis (e.g., user, account, order)';

COMMENT ON COLUMN engine_schema_entities.name IS
    'Entity name in singular form (e.g., "user", "account", "order")';

COMMENT ON COLUMN engine_schema_entities.description IS
    'LLM-generated explanation of what this entity represents in the domain';

COMMENT ON COLUMN engine_schema_entities.primary_schema IS
    'Schema containing the primary/canonical table for this entity';

COMMENT ON COLUMN engine_schema_entities.primary_table IS
    'Primary/canonical table where this entity is defined';

COMMENT ON COLUMN engine_schema_entities.primary_column IS
    'Primary key column in the primary table';

-- ============================================================================
-- Table: engine_schema_entity_occurrences
-- Tracks where entities appear across the schema with optional role semantics
-- ============================================================================

CREATE TABLE engine_schema_entity_occurrences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_schema_entities(id) ON DELETE CASCADE,
    schema_name TEXT NOT NULL,             -- Schema where this occurrence appears
    table_name TEXT NOT NULL,              -- Table where this occurrence appears
    column_name TEXT NOT NULL,             -- Column that references the entity
    role TEXT,                             -- Optional role: "visitor", "host", "owner", NULL for generic
    confidence FLOAT NOT NULL DEFAULT 1.0, -- Confidence score (0.0-1.0) from LLM analysis
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_id, schema_name, table_name, column_name),
    CHECK (confidence >= 0.0 AND confidence <= 1.0)
);

-- Index for finding occurrences by entity
CREATE INDEX idx_engine_schema_entity_occurrences_entity
    ON engine_schema_entity_occurrences(entity_id);

-- Index for finding occurrences by table (for join path discovery)
CREATE INDEX idx_engine_schema_entity_occurrences_table
    ON engine_schema_entity_occurrences(schema_name, table_name);

-- Index for finding occurrences by role (for semantic queries)
CREATE INDEX idx_engine_schema_entity_occurrences_role
    ON engine_schema_entity_occurrences(role)
    WHERE role IS NOT NULL;

COMMENT ON TABLE engine_schema_entity_occurrences IS
    'Tracks where each entity appears across the schema, with optional role semantics';

COMMENT ON COLUMN engine_schema_entity_occurrences.role IS
    'Optional semantic role (e.g., "visitor", "host", "owner") or NULL for generic references';

COMMENT ON COLUMN engine_schema_entity_occurrences.confidence IS
    'LLM confidence score (0.0-1.0) for this entity-column mapping';
