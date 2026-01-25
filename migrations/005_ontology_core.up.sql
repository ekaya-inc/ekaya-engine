-- 005_ontology_core.up.sql
-- Ontology core tables: ontologies, entities, aliases, key columns, entity relationships

-- Tiered ontology storage
CREATE TABLE engine_ontologies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    version integer DEFAULT 1 NOT NULL,
    is_active boolean DEFAULT true NOT NULL,
    domain_summary jsonb,
    entity_summaries jsonb,
    column_details jsonb,
    metadata jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontologies_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_engine_ontologies_project ON engine_ontologies USING btree (project_id);
CREATE UNIQUE INDEX idx_engine_ontologies_unique ON engine_ontologies USING btree (project_id, version);
CREATE UNIQUE INDEX idx_engine_ontologies_single_active ON engine_ontologies USING btree (project_id) WHERE (is_active = true);

CREATE TRIGGER update_engine_ontologies_updated_at
    BEFORE UPDATE ON engine_ontologies
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Domain entities discovered during analysis
CREATE TABLE engine_ontology_entities (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    project_id uuid NOT NULL,
    ontology_id uuid NOT NULL,
    name text NOT NULL,
    description text,
    domain character varying(100),
    primary_schema text NOT NULL,
    primary_table text NOT NULL,
    primary_column text NOT NULL,
    is_deleted boolean DEFAULT false NOT NULL,
    deletion_reason text,

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inference',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontology_entities_ontology_id_name_key UNIQUE (ontology_id, name),
    CONSTRAINT engine_ontology_entities_project_id_fkey FOREIGN KEY (project_id) REFERENCES engine_projects(id) ON DELETE CASCADE,
    CONSTRAINT engine_ontology_entities_ontology_id_fkey FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    CONSTRAINT engine_ontology_entities_source_check CHECK (source IN ('inference', 'mcp', 'manual')),
    CONSTRAINT engine_ontology_entities_last_edit_source_check CHECK (last_edit_source IS NULL OR last_edit_source IN ('inference', 'mcp', 'manual'))
);

COMMENT ON TABLE engine_ontology_entities IS 'Domain entities discovered during relationship analysis (e.g., user, account, order)';
COMMENT ON COLUMN engine_ontology_entities.name IS 'Entity name in singular form (e.g., "user", "account", "order")';
COMMENT ON COLUMN engine_ontology_entities.description IS 'LLM-generated explanation of what this entity represents in the domain';
COMMENT ON COLUMN engine_ontology_entities.primary_schema IS 'Schema containing the primary/canonical table for this entity';
COMMENT ON COLUMN engine_ontology_entities.primary_table IS 'Primary/canonical table where this entity is defined';
COMMENT ON COLUMN engine_ontology_entities.primary_column IS 'Primary key column in the primary table';
COMMENT ON COLUMN engine_ontology_entities.is_deleted IS 'Soft delete flag - entities are never hard deleted';
COMMENT ON COLUMN engine_ontology_entities.deletion_reason IS 'Optional reason why the entity was soft deleted';
COMMENT ON COLUMN engine_ontology_entities.source IS 'How this entity was created: inference (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_ontology_entities.last_edit_source IS 'How this entity was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_ontology_entities.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_ontology_entities.updated_by IS 'UUID of user who last updated this entity';

CREATE INDEX idx_engine_ontology_entities_project ON engine_ontology_entities USING btree (project_id);
CREATE INDEX idx_engine_ontology_entities_ontology ON engine_ontology_entities USING btree (ontology_id);
CREATE INDEX idx_engine_ontology_entities_active ON engine_ontology_entities USING btree (project_id) WHERE (NOT is_deleted);
CREATE INDEX idx_engine_ontology_entities_primary_location ON engine_ontology_entities USING btree (primary_schema, primary_table, primary_column);

-- Entity alternative names (for query matching)
CREATE TABLE engine_ontology_entity_aliases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_id uuid NOT NULL,
    alias text NOT NULL,
    source character varying(50),
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontology_entity_aliases_entity_id_alias_key UNIQUE (entity_id, alias),
    CONSTRAINT engine_ontology_entity_aliases_entity_id_fkey FOREIGN KEY (entity_id) REFERENCES engine_ontology_entities(id) ON DELETE CASCADE
);

COMMENT ON TABLE engine_ontology_entity_aliases IS 'Alternative names for entities, used for query matching and discovery';
COMMENT ON COLUMN engine_ontology_entity_aliases.alias IS 'Alternative name for the entity (e.g., "customer" as alias for "user")';
COMMENT ON COLUMN engine_ontology_entity_aliases.source IS 'How this alias was created: discovery (auto-detected), user (manually added), query (learned from queries)';

CREATE INDEX idx_engine_ontology_entity_aliases_entity ON engine_ontology_entity_aliases USING btree (entity_id);
CREATE INDEX idx_engine_ontology_entity_aliases_alias ON engine_ontology_entity_aliases USING btree (alias);

-- Important business columns per entity
CREATE TABLE engine_ontology_entity_key_columns (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_id uuid NOT NULL,
    column_name text NOT NULL,
    synonyms jsonb DEFAULT '[]'::jsonb,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_ontology_entity_key_columns_entity_id_column_name_key UNIQUE (entity_id, column_name),
    CONSTRAINT engine_ontology_entity_key_columns_entity_id_fkey FOREIGN KEY (entity_id) REFERENCES engine_ontology_entities(id) ON DELETE CASCADE
);

CREATE INDEX idx_entity_key_columns_entity_id ON engine_ontology_entity_key_columns USING btree (entity_id);

-- Entity-to-entity relationships
CREATE TABLE engine_entity_relationships (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    ontology_id uuid NOT NULL,
    source_entity_id uuid NOT NULL,
    target_entity_id uuid NOT NULL,
    source_column_schema character varying(255) NOT NULL,
    source_column_table character varying(255) NOT NULL,
    source_column_name character varying(255) NOT NULL,
    target_column_schema character varying(255) NOT NULL,
    target_column_table character varying(255) NOT NULL,
    target_column_name character varying(255) NOT NULL,
    detection_method character varying(50) NOT NULL,
    confidence numeric(3,2) NOT NULL,
    status character varying(20) DEFAULT 'confirmed'::character varying NOT NULL,
    description text,
    association character varying(100),
    cardinality text DEFAULT 'unknown'::text NOT NULL,

    -- Provenance: source tracking (how it was created/modified)
    source text NOT NULL DEFAULT 'inference',
    last_edit_source text,

    -- Provenance: actor tracking (who created/modified)
    created_by uuid,
    updated_by uuid,

    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    PRIMARY KEY (id),
    CONSTRAINT engine_entity_relationships_confidence_check CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT engine_entity_relationships_cardinality_check CHECK (cardinality = ANY (ARRAY['1:1'::text, '1:N'::text, 'N:1'::text, 'N:M'::text, 'unknown'::text])),
    CONSTRAINT engine_entity_relationships_unique_relationship UNIQUE (ontology_id, source_entity_id, target_entity_id, source_column_schema, source_column_table, source_column_name, target_column_schema, target_column_table, target_column_name),
    CONSTRAINT engine_entity_relationships_ontology_id_fkey FOREIGN KEY (ontology_id) REFERENCES engine_ontologies(id) ON DELETE CASCADE,
    CONSTRAINT engine_entity_relationships_source_entity_id_fkey FOREIGN KEY (source_entity_id) REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    CONSTRAINT engine_entity_relationships_target_entity_id_fkey FOREIGN KEY (target_entity_id) REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    CONSTRAINT engine_entity_relationships_source_check CHECK (source IN ('inference', 'mcp', 'manual')),
    CONSTRAINT engine_entity_relationships_last_edit_source_check CHECK (last_edit_source IS NULL OR last_edit_source IN ('inference', 'mcp', 'manual'))
);

CREATE TRIGGER update_engine_entity_relationships_updated_at
    BEFORE UPDATE ON engine_entity_relationships
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE engine_entity_relationships IS 'Entity-to-entity relationships discovered from FK constraints or inferred from PK matching';
COMMENT ON COLUMN engine_entity_relationships.detection_method IS 'How the relationship was discovered: foreign_key (from DB constraint) or pk_match (inferred)';
COMMENT ON COLUMN engine_entity_relationships.confidence IS '1.0 for FK constraints, 0.7-0.95 for inferred relationships';
COMMENT ON COLUMN engine_entity_relationships.status IS 'confirmed (auto-accepted), pending (needs review), rejected (user declined)';
COMMENT ON COLUMN engine_entity_relationships.description IS 'Optional description of the relationship, typically provided when created through chat';
COMMENT ON COLUMN engine_entity_relationships.association IS 'Semantic association describing this direction of the relationship (e.g., "placed_by", "contains", "as host")';
COMMENT ON COLUMN engine_entity_relationships.source IS 'How this relationship was created: inference (Engine), mcp (Claude), manual (UI)';
COMMENT ON COLUMN engine_entity_relationships.last_edit_source IS 'How this relationship was last modified (null if never edited after creation)';
COMMENT ON COLUMN engine_entity_relationships.created_by IS 'UUID of user who triggered creation (from JWT)';
COMMENT ON COLUMN engine_entity_relationships.updated_by IS 'UUID of user who last updated this relationship';
COMMENT ON CONSTRAINT engine_entity_relationships_unique_relationship ON engine_entity_relationships IS 'Ensures each specific column-to-column relationship is stored once per direction. Includes target columns to support multiple FKs from same source table.';

CREATE INDEX idx_engine_entity_relationships_ontology ON engine_entity_relationships USING btree (ontology_id);
CREATE INDEX idx_engine_entity_relationships_source ON engine_entity_relationships USING btree (source_entity_id);
CREATE INDEX idx_engine_entity_relationships_target ON engine_entity_relationships USING btree (target_entity_id);
CREATE INDEX idx_engine_entity_relationships_cardinality ON engine_entity_relationships USING btree (cardinality);

-- RLS
ALTER TABLE engine_ontologies ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontologies_access ON engine_ontologies FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

ALTER TABLE engine_ontology_entities ENABLE ROW LEVEL SECURITY;
CREATE POLICY ontology_entities_access ON engine_ontology_entities FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid)
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR project_id = current_setting('app.current_project_id', true)::uuid);

ALTER TABLE engine_ontology_entity_aliases ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_aliases_access ON engine_ontology_entity_aliases FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR entity_id IN (
        SELECT id FROM engine_ontology_entities WHERE project_id = current_setting('app.current_project_id', true)::uuid
    ))
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR entity_id IN (
        SELECT id FROM engine_ontology_entities WHERE project_id = current_setting('app.current_project_id', true)::uuid
    ));

ALTER TABLE engine_ontology_entity_key_columns ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_key_columns_access ON engine_ontology_entity_key_columns FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR entity_id IN (
        SELECT id FROM engine_ontology_entities WHERE project_id = current_setting('app.current_project_id', true)::uuid
    ))
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR entity_id IN (
        SELECT id FROM engine_ontology_entities WHERE project_id = current_setting('app.current_project_id', true)::uuid
    ));

ALTER TABLE engine_entity_relationships ENABLE ROW LEVEL SECURITY;
CREATE POLICY entity_relationships_access ON engine_entity_relationships FOR ALL
    USING (current_setting('app.current_project_id', true) IS NULL OR ontology_id IN (
        SELECT id FROM engine_ontologies WHERE project_id = current_setting('app.current_project_id', true)::uuid
    ))
    WITH CHECK (current_setting('app.current_project_id', true) IS NULL OR ontology_id IN (
        SELECT id FROM engine_ontologies WHERE project_id = current_setting('app.current_project_id', true)::uuid
    ));
