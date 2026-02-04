# PLAN: Column Schema Refactor

## Problem

Confused architecture for storing column information:

1. **`engine_schema_columns`** has:
   - Schema + stats (correct)
   - `metadata` JSONB storing `ColumnFeatures` from extraction (should be in ontology table)
   - `business_name`, `description` columns (vestigial, never used)
   - `is_sensitive` (should be in ontology table)

2. **`engine_ontology_column_metadata`** has:
   - Semantic annotations with provenance (correct)
   - Uses `table_name`/`column_name` as key instead of FK to `engine_schema_columns`
   - Missing most fields from `ColumnFeatures` struct

3. **Extraction writes to wrong place**: `ColumnFeatureExtractionService` writes to `engine_schema_columns.metadata` JSONB instead of the ontology table.

## Solution

Clear separation:
- **`engine_schema_columns`** = schema discovery + data stats
- **`engine_ontology_column_metadata`** = semantic enrichment with typed columns

## Target Schema

### `engine_schema_columns` (schema + stats)

```sql
CREATE TABLE engine_schema_columns (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
  schema_table_id uuid NOT NULL REFERENCES engine_schema_tables(id) ON DELETE CASCADE,
  column_name text NOT NULL,
  data_type text NOT NULL,
  is_nullable boolean NOT NULL,
  is_primary_key boolean NOT NULL DEFAULT false,
  is_unique boolean NOT NULL DEFAULT false,
  ordinal_position integer NOT NULL,
  default_value text,
  is_selected boolean NOT NULL DEFAULT false,
  -- Stats from data scanning
  distinct_count bigint,
  null_count bigint,
  row_count bigint,
  non_null_count bigint,
  min_length integer,
  max_length integer,
  is_joinable boolean,
  joinability_reason text,
  stats_updated_at timestamptz,
  -- Lifecycle
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz
);
```

**Removed:** `business_name`, `description`, `metadata`, `is_sensitive`, `sample_values`

**Note:** `sample_values` removed to avoid persisting data from target datasource into engine database.

### `engine_ontology_column_metadata` (semantic enrichment)

```sql
CREATE TABLE engine_ontology_column_metadata (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
  schema_column_id uuid NOT NULL REFERENCES engine_schema_columns(id) ON DELETE CASCADE,

  -- Core classification (typed columns)
  classification_path text,
  purpose text,  -- identifier, timestamp, flag, measure, enum, text, json
  semantic_type text,  -- soft_delete_timestamp, currency_cents, etc.
  role text CHECK (role IS NULL OR role IN ('primary_key', 'foreign_key', 'attribute', 'measure', 'dimension', 'identifier')),
  description text,
  confidence numeric,

  -- Type-specific features (single JSONB for extensibility)
  features jsonb DEFAULT '{}',

  -- Processing flags
  needs_enum_analysis boolean NOT NULL DEFAULT false,
  needs_fk_resolution boolean NOT NULL DEFAULT false,
  needs_cross_column_check boolean NOT NULL DEFAULT false,
  needs_clarification boolean NOT NULL DEFAULT false,
  clarification_question text,

  -- User overrides
  is_sensitive boolean,

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

  UNIQUE (project_id, schema_column_id)
);
```

**`features` JSONB structure:**
```json
{
  "timestamp_features": {"timestamp_purpose": "...", "is_soft_delete": false, ...},
  "boolean_features": {"true_meaning": "...", "false_meaning": "...", ...},
  "enum_features": {"is_state_machine": false, "values": [...], ...},
  "identifier_features": {"identifier_type": "...", "fk_target_table": "...", ...},
  "monetary_features": {"currency_unit": "cents", ...}
}
```

## Implementation Tasks

### Phase 1: Database Migration

- [x] 1.1 Create migration that drops and recreates `engine_schema_columns` with new schema
- [x] 1.2 Create migration that drops and recreates `engine_ontology_column_metadata` with new schema
- [x] 1.3 Add indexes and RLS policies (included in 1.1 and 1.2 migrations)

### Phase 2: Model Updates

- [x] 2.1 Update `models.SchemaColumn`:
  - Remove `BusinessName`, `Description`, `Metadata`, `IsSensitive`, `SampleValues`
  - Remove `GetColumnFeatures()` method

- [x] 2.2 Update `models.ColumnMetadata`:
  - Remove `TableName`, `ColumnName`
  - Add `SchemaColumnID uuid`
  - Add typed fields: `ClassificationPath`, `Purpose`, `SemanticType`, `Confidence`
  - Add `Features` JSONB field
  - Add processing flags
  - Add `AnalyzedAt`, `LLMModelUsed`

- [x] 2.3 Add helper methods on `ColumnMetadata`:
  - `GetTimestampFeatures() *TimestampFeatures`
  - `GetBooleanFeatures() *BooleanFeatures`
  - `GetEnumFeatures() *EnumFeatures`
  - `GetIdentifierFeatures() *IdentifierFeatures`
  - `GetMonetaryFeatures() *MonetaryFeatures`
  - `SetFeatures(features *ColumnFeatures)`

### Phase 3: Repository Updates

- [x] 3.1 Update `SchemaRepository` to remove column feature methods and dropped column references

  **Context:** As part of the column schema refactor, `engine_schema_columns` no longer stores `metadata` JSONB (which contained `ColumnFeatures`), `business_name`, `description`, `is_sensitive`, or `sample_values`. These fields have been moved to `engine_ontology_column_metadata`.

  **File:** `pkg/repositories/schema_repository.go`

  **Changes required:**
  1. Remove `UpdateColumnFeatures(ctx context.Context, columnID uuid.UUID, features *models.ColumnFeatures) error` method
  2. Remove `ClearColumnFeaturesByProject(ctx context.Context, projectID uuid.UUID) error` method
  3. Update all SQL queries that SELECT from `engine_schema_columns` to exclude dropped columns: `business_name`, `description`, `metadata`, `is_sensitive`, `sample_values`
  4. Update any INSERT/UPDATE queries to exclude these columns
  5. Update the `SchemaRepository` interface in `pkg/repositories/interfaces.go` to remove the deleted method signatures

  **Verification:** Run `make check` to ensure all tests pass and no code references the removed methods.

- [x] 3.2 Update `ColumnMetadataRepository` to use `schema_column_id` FK ✓

  **Context:** `engine_ontology_column_metadata` has been refactored to use `schema_column_id uuid` as a foreign key to `engine_schema_columns` instead of `table_name`/`column_name` text fields. The table also has new typed columns for features.

  **File:** `pkg/repositories/column_metadata_repository.go`

  **Schema reference (from migration):**
  ```sql
  CREATE TABLE engine_ontology_column_metadata (
    id uuid PRIMARY KEY,
    project_id uuid NOT NULL,
    schema_column_id uuid NOT NULL REFERENCES engine_schema_columns(id),
    classification_path text,
    purpose text,
    semantic_type text,
    role text,
    description text,
    confidence numeric,
    features jsonb DEFAULT '{}',
    needs_enum_analysis boolean NOT NULL DEFAULT false,
    needs_fk_resolution boolean NOT NULL DEFAULT false,
    needs_cross_column_check boolean NOT NULL DEFAULT false,
    needs_clarification boolean NOT NULL DEFAULT false,
    clarification_question text,
    is_sensitive boolean,
    analyzed_at timestamptz,
    llm_model_used text,
    source text NOT NULL DEFAULT 'inferred',
    last_edit_source text,
    created_by uuid,
    updated_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, schema_column_id)
  );
  ```

  **Changes required:**
  1. Update all existing methods that use `table_name`/`column_name` to use `schema_column_id` instead
  2. Add `GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)` method
  3. Add `UpsertFromExtraction(ctx context.Context, metadata *models.ColumnMetadata) error` method for the extraction pipeline (sets `source='inferred'`)
  4. Update `Upsert()` method to handle MCP/manual edits (respects `source` parameter, updates `last_edit_source`)
  5. Update all SQL queries to use the new typed columns instead of the old `table_name`/`column_name` approach
  6. Update the `ColumnMetadataRepository` interface in `pkg/repositories/interfaces.go`

  **Key distinction:**
  - `UpsertFromExtraction()` is for automated extraction pipeline - always sets `source='inferred'`
  - `Upsert()` is for MCP tools and manual edits - respects the provided `source` value and sets `last_edit_source`

  **Verification:** Run `make check` to ensure all tests pass.

### Phase 4: Service Updates

- [x] 4.1 Update `ColumnFeatureExtractionService`:
  - Write to `columnMetadataRepo.UpsertFromExtraction()` instead of `schemaRepo.UpdateColumnFeatures()`
  - Set `source='inferred'`

- [x] 4.2.1 Update RelationshipCandidateCollector to use ColumnMetadataRepository

  **Context:** As part of the column schema refactor, `ColumnFeatures` data has moved from `engine_schema_columns.metadata` JSONB to `engine_ontology_column_metadata` with typed columns. Services that previously called `SchemaColumn.GetColumnFeatures()` must now fetch from `ColumnMetadataRepository`.

  **File:** `pkg/services/relationship_candidate_collector.go`

  **Changes required:**
  1. Add `ColumnMetadataRepository` as a dependency to the service struct
  2. Update the constructor to accept the repository
  3. Find all calls to `GetColumnFeatures()` on `SchemaColumn` objects and replace with `columnMetadataRepo.GetBySchemaColumnID(ctx, column.ID)`
  4. Handle the case where no metadata exists (features may be nil for unanalyzed columns)
  5. Use the new typed getter methods on `ColumnMetadata` (e.g., `GetIdentifierFeatures()`, `GetEnumFeatures()`)

  **Interface reference:** `ColumnMetadataRepository.GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)`

  **Verification:** Run `make check` to ensure all tests pass.

- [x] 4.2.2 Update DeterministicRelationshipService to use ColumnMetadataRepository

  **Context:** As part of the column schema refactor, `ColumnFeatures` data has moved from `engine_schema_columns.metadata` JSONB to `engine_ontology_column_metadata` with typed columns. Services that previously called `SchemaColumn.GetColumnFeatures()` must now fetch from `ColumnMetadataRepository`.

  **File:** `pkg/services/deterministic_relationship_service.go`

  **Changes required:**
  1. Add `ColumnMetadataRepository` as a dependency to the service struct
  2. Update the constructor to accept the repository
  3. Find all calls to `GetColumnFeatures()` on `SchemaColumn` objects and replace with `columnMetadataRepo.GetBySchemaColumnID(ctx, column.ID)`
  4. Handle the case where no metadata exists (features may be nil for unanalyzed columns)
  5. Use the new typed getter methods on `ColumnMetadata` (e.g., `GetIdentifierFeatures()`)

  **Interface reference:** `ColumnMetadataRepository.GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)`

  **Verification:** Run `make check` to ensure all tests pass.

- [x] 4.2.3 Update TableFeatureExtractionService and ColumnEnrichmentService to use ColumnMetadataRepository

  **Context:** As part of the column schema refactor, `ColumnFeatures` data has moved from `engine_schema_columns.metadata` JSONB to `engine_ontology_column_metadata` with typed columns.

  **Files:**
  - `pkg/services/table_feature_extraction_service.go`
  - `pkg/services/column_enrichment_service.go`

  **Changes required for each service:**
  1. Add `ColumnMetadataRepository` as a dependency to the service struct
  2. Update the constructor to accept the repository
  3. Find all calls to `GetColumnFeatures()` on `SchemaColumn` objects and replace with `columnMetadataRepo.GetBySchemaColumnID(ctx, column.ID)`
  4. Handle the case where no metadata exists
  5. Use the new typed getter methods on `ColumnMetadata`

  **Interface reference:** `ColumnMetadataRepository.GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)`

  **Verification:** Run `make check` to ensure all tests pass.

- [x] 4.2.4 Update OntologyFinalizationService and RelationshipEnrichmentService to use ColumnMetadataRepository

  **Context:** As part of the column schema refactor, `ColumnFeatures` data has moved from `engine_schema_columns.metadata` JSONB to `engine_ontology_column_metadata` with typed columns.

  **Files:**
  - `pkg/services/ontology_finalization_service.go`
  - `pkg/services/relationship_enrichment_service.go`

  **Changes required for each service:**
  1. Add `ColumnMetadataRepository` as a dependency to the service struct
  2. Update the constructor to accept the repository
  3. Find all calls to `GetColumnFeatures()` on `SchemaColumn` objects and replace with `columnMetadataRepo.GetBySchemaColumnID(ctx, column.ID)`
  4. Handle the case where no metadata exists
  5. Use the new typed getter methods on `ColumnMetadata`

  **Interface reference:** `ColumnMetadataRepository.GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)`

  **Verification:** Run `make check` to ensure all tests pass.

- [x] 4.2.5 Update DataChangeDetectionService and ColumnFilterService to use ColumnMetadataRepository ✓

  **Context:** As part of the column schema refactor, `ColumnFeatures` data has moved from `engine_schema_columns.metadata` JSONB to `engine_ontology_column_metadata` with typed columns.

  **Files:**
  - `pkg/services/data_change_detection_service.go`
  - `pkg/services/column_filter_service.go`

  **Changes required for each service:**
  1. Add `ColumnMetadataRepository` as a dependency to the service struct
  2. Update the constructor to accept the repository
  3. Find all calls to `GetColumnFeatures()` on `SchemaColumn` objects and replace with `columnMetadataRepo.GetBySchemaColumnID(ctx, column.ID)`
  4. Handle the case where no metadata exists
  5. Use the new typed getter methods on `ColumnMetadata`

  **Interface reference:** `ColumnMetadataRepository.GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error)`

  **Verification:** Run `make check` to ensure all tests pass.

- [x] 4.3 Update `OntologyContextService` to fetch from ontology table ✓

### Phase 5: MCP Tool Updates

- [x] 5.1 Update `update_column` - use `schema_column_id`, write typed columns
- [x] 5.2 Update `get_column_metadata` - read typed columns
- [x] 5.3 Update `probe_column` - fetch from ontology table
- [x] 5.4 Update `get_context` - fetch from ontology table
- [x] 5.5 Update `search_schema` - remove dropped column references

### Phase 6: Handler Updates

- [ ] 6.1 Update `SchemaHandler` - remove dropped field references
- [ ] 6.2 Update `OntologyEnrichmentHandler` - fetch from ontology table

### Phase 7: Cleanup

- [ ] 7.1 Delete dead code:
  - `SchemaColumn.GetColumnFeatures()`
  - `SchemaRepository.UpdateColumnFeatures()`
  - `SchemaRepository.ClearColumnFeaturesByProject()`

- [ ] 7.2 Update tests

- [ ] 7.3 Update CLAUDE.md

## Notes

- Database will be dropped/recreated - no data migration needed
- No backward compatibility concerns
- After migration, run full ontology extraction to populate new schema
- Enum values and labels stored in `features['enum_features']['values']` - this is the ontology source for enum semantics
- `entity` column deferred - entity feature will be addressed separately after schema refactor
- `sample_values` not persisted - avoids storing target datasource data in engine database
