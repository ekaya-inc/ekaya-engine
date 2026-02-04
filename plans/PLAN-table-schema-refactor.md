# PLAN: Table Schema Refactor

## Problem

Similar issues to column schema:

1. **`engine_schema_tables`** has:
   - Schema + stats (correct)
   - `business_name`, `description`, `metadata` columns (vestigial, never used)

2. **`engine_table_metadata`** has:
   - Semantic annotations with provenance (correct concept)
   - Wrong name - should be `engine_ontology_table_metadata` for consistency with columns
   - Uses `table_name` as key instead of FK to `engine_schema_tables`
   - Missing analysis metadata (analyzed_at, llm_model_used, confidence)
   - Missing table classification (transactional, reference, logging, ephemeral)

3. **TableFeatureExtractionService not wired up**: Service exists but `SetTableFeatureExtractionMethods()` never called in main.go

## Solution

Clear separation matching the column pattern:
- **`engine_schema_tables`** = schema discovery + row counts
- **`engine_ontology_table_metadata`** = semantic enrichment with typed columns

## Target Schema

### `engine_schema_tables` (schema + stats)

```sql
CREATE TABLE engine_schema_tables (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
  datasource_id uuid NOT NULL REFERENCES engine_datasources(id) ON DELETE CASCADE,
  schema_name text NOT NULL,
  table_name text NOT NULL,
  is_selected boolean NOT NULL DEFAULT false,
  row_count bigint,
  -- Lifecycle
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz,

  UNIQUE (project_id, datasource_id, schema_name, table_name) WHERE deleted_at IS NULL
);
```

**Removed:** `business_name`, `description`, `metadata`

### `engine_ontology_table_metadata` (semantic enrichment)

```sql
CREATE TABLE engine_ontology_table_metadata (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
  schema_table_id uuid NOT NULL REFERENCES engine_schema_tables(id) ON DELETE CASCADE,

  -- Core classification (typed columns)
  table_type text,  -- transactional, reference, logging, ephemeral, junction
  description text,
  usage_notes text,
  is_ephemeral boolean NOT NULL DEFAULT false,
  preferred_alternative text,  -- table to use instead if this one is ephemeral/deprecated
  confidence numeric,

  -- Type-specific features (single JSONB for extensibility)
  features jsonb DEFAULT '{}',

  -- User overrides (entity deferred to separate feature)

  -- Analysis metadata
  analyzed_at timestamptz,
  llm_model_used text,

  -- Provenance
  source text NOT NULL DEFAULT 'inferred' CHECK (source IN ('inferred', 'mcp', 'manual')),
  last_edit_source text CHECK (last_edit_source IS NULL OR last_edit_source IN ('inferred', 'mcp', 'manual')),
  created_by uuid,
  updated_by uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),

  UNIQUE (project_id, schema_table_id)
);
```

**`features` JSONB structure (for future extensibility):**
```json
{
  "relationship_summary": {"incoming_fk_count": 5, "outgoing_fk_count": 2},
  "temporal_features": {"has_soft_delete": true, "has_audit_timestamps": true},
  "size_features": {"is_large_table": true, "growth_pattern": "append_only"}
}
```

## Implementation Tasks

### Phase 1: Database Migration

- [ ] 1.1 Drop `engine_table_metadata` table
- [ ] 1.2 Create migration that drops and recreates `engine_schema_tables` with new schema
- [ ] 1.3 Create `engine_ontology_table_metadata` with new schema
- [ ] 1.4 Add indexes and RLS policies

### Phase 2: Model Updates

- [ ] 2.1 Update `models.SchemaTable`:
  - Remove `BusinessName`, `Description`, `Metadata`

- [ ] 2.2 Rename `models.TableMetadata` and update:
  - Remove `DatasourceID`, `TableName`
  - Add `SchemaTableID uuid`
  - Add `TableType`, `Confidence`
  - Add `Features` JSONB field
  - Add `Entity`
  - Add `AnalyzedAt`, `LLMModelUsed`

### Phase 3: Repository Updates

- [ ] 3.1 Update `SchemaRepository`:
  - Update all queries to exclude dropped columns

- [ ] 3.2 Rename and update `TableMetadataRepository`:
  - Change from `table_name` to `schema_table_id`
  - Add `GetBySchemaTableID()`
  - Add `UpsertFromExtraction()` for extraction pipeline
  - Update `Upsert()` for MCP/manual edits

### Phase 4: Service Updates

- [ ] 4.1 Wire up `TableFeatureExtractionService` in main.go:
  - Create the service instance
  - Call `ontologyDAGService.SetTableFeatureExtractionMethods()`

- [ ] 4.2 Update `TableFeatureExtractionService`:
  - Write to `tableMetadataRepo.UpsertFromExtraction()` instead of current broken path
  - Add `table_type` to LLM response schema
  - Set `source='inferred'`

- [ ] 4.3 Update `OntologyContextService`:
  - Fetch table metadata from ontology table using `schema_table_id`

### Phase 5: MCP Tool Updates

- [ ] 5.1 Update `update_table` tool - use `schema_table_id`, write typed columns
- [ ] 5.2 Update `get_context` tool - fetch table metadata from ontology table
- [ ] 5.3 Update `search_schema` tool - remove dropped column references

### Phase 6: Handler Updates

- [ ] 6.1 Update `SchemaHandler` - remove dropped field references

### Phase 7: Cleanup

- [ ] 7.1 Delete dead code referencing old schema
- [ ] 7.2 Update tests
- [ ] 7.3 Update CLAUDE.md

## Notes

- Database will be dropped/recreated - no data migration needed
- No backward compatibility concerns
- After migration, run full ontology extraction to populate new schema
- The `table_type` classification should be added to the LLM prompt response format
