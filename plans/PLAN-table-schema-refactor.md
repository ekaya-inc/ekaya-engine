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

### Phase 1: Database Migration (023_table_schema_refactor)

- [x] 1.1 Create single migration file `migrations/023_table_schema_refactor.up.sql` that:
  - Drops `engine_table_metadata` table
  - Drops and recreates `engine_schema_tables` with new schema (removes `business_name`, `description`, `metadata`)
  - Creates `engine_ontology_table_metadata` with new schema
  - Adds indexes and RLS policies

### Phase 2: Model Updates

- [x] 2.1 Update `models.SchemaTable` in `pkg/models/schema.go`:
  - Remove `BusinessName`, `Description`, `Metadata` fields

- [x] 2.2 Update `models.TableMetadata` in `pkg/models/table_metadata.go`:
  - Remove `DatasourceID`, `TableName`
  - Add `SchemaTableID uuid`
  - Add `TableType`, `Confidence`
  - Add `Features` JSONB field (with `TableMetadataFeatures` struct)
  - Add `AnalyzedAt`, `LLMModelUsed`
  - Add helper methods similar to ColumnMetadata pattern

### Phase 3: Repository Updates

- [x] 3.1 Update `SchemaRepository` in `pkg/repositories/schema_repository.go`:
  - Update all queries to exclude dropped columns (`business_name`, `description`, `metadata`)
  - Remove `UpdateTableMetadata()` method (no longer needed)

- [x] 3.2 Update `TableMetadataRepository` in `pkg/repositories/table_metadata_repository.go`:
  - Change from `datasource_id` + `table_name` key to `schema_table_id` FK
  - Update table name from `engine_table_metadata` to `engine_ontology_table_metadata`
  - Add `GetBySchemaTableID(ctx context.Context, schemaTableID uuid.UUID) (*models.TableMetadata, error)`
  - Add `UpsertFromExtraction(ctx context.Context, meta *models.TableMetadata) error` for extraction pipeline
  - Update `Upsert()` for MCP/manual edits with proper provenance handling

### Phase 4: Service Updates

- [x] 4.1 Wire up `TableFeatureExtractionService` in `main.go`:
  - Create `tableFeatureExtractionSvc` instance using `NewTableFeatureExtractionService()`
  - Call `ontologyDAGService.SetTableFeatureExtractionMethods(tableFeatureExtractionSvc)`
  - Note: The DAG node already exists and handles nil methods gracefully; wiring enables it

- [x] 4.2 Update `TableFeatureExtractionService` in `pkg/services/table_feature_extraction.go`:
  - Write to `tableMetadataRepo.UpsertFromExtraction()` instead of `tableMetadataRepo.Upsert()`
  - Add `table_type` classification to LLM prompt and response parsing
  - Ensure `source='inferred'` is set via UpsertFromExtraction

- [x] 4.3 Update `OntologyContextService` in `pkg/services/ontology_context.go`:
  - Fetch table metadata from `engine_ontology_table_metadata` using `schema_table_id`

### Phase 5: MCP Tool Updates

- [x] 5.1 Update `update_table` tool in `pkg/mcp/tools/table.go`:
  - Use `schema_table_id` instead of `table_name` for lookup
  - Write typed columns including `table_type`

- [x] 5.2 Update `get_context` tool in `pkg/mcp/tools/context.go`:
  - Fetch table metadata from ontology table using `schema_table_id`

- [x] 5.3 Update `search_schema` tool in `pkg/mcp/tools/search.go`:
  - Remove dropped column references (`business_name`, `description`, `metadata`)

### Phase 6: Handler Updates

- [x] 6.1 Update `SchemaHandler` in `pkg/handlers/schema_handler.go`:
  - Remove dropped field references from API responses

### Phase 7: Cleanup

- [x] 7.1 Delete dead code referencing old schema
- [x] 7.2 Update integration tests (table_metadata tests need schema_table_id changes)
- [ ] 7.3 Update CLAUDE.md (if any table metadata documentation exists)

## Notes

- Database will be dropped/recreated - no data migration needed
- No backward compatibility concerns
- After migration, run full ontology extraction to populate new schema
- The `table_type` classification should be added to the LLM prompt response format
