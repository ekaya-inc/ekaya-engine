# FIX: Add Column Type to Entity Relationships

## Problem

The `engine_entity_relationships` table stores column names but not column types. The API response struct has `source_column_type` and `target_column_type` fields, but they're never populated. The UI previously displayed empty parentheses `()` where types should appear.

**Current workaround**: Column type display removed from UI (RelationshipsPage.tsx lines 580-582, 591-593 were deleted).

## Root Cause

When relationships are created during the ontology DAG, we have access to `SchemaColumn` objects (which include `DataType`), but we only store the column names - discarding the column IDs and type information.

## Solution

Add foreign keys from `engine_entity_relationships` to `engine_schema_columns` for both source and target columns. This allows JOINing to get column types (and any other column metadata) without duplicating data.

## Files to Modify

### 1. Migration (new file)

Create `migrations/0XX_entity_relationship_column_ids.up.sql`:

```sql
-- Add foreign keys to link entity relationships to schema columns
-- Nullable because: (1) existing rows need backfilling, (2) manual relationships
-- might reference columns not yet in schema

ALTER TABLE engine_entity_relationships
    ADD COLUMN source_column_id UUID REFERENCES engine_schema_columns(id) ON DELETE SET NULL,
    ADD COLUMN target_column_id UUID REFERENCES engine_schema_columns(id) ON DELETE SET NULL;

-- Index for JOIN performance
CREATE INDEX idx_entity_rel_source_col ON engine_entity_relationships(source_column_id);
CREATE INDEX idx_entity_rel_target_col ON engine_entity_relationships(target_column_id);

COMMENT ON COLUMN engine_entity_relationships.source_column_id IS
    'FK to schema column for source - allows JOINing to get column type and metadata';
COMMENT ON COLUMN engine_entity_relationships.target_column_id IS
    'FK to schema column for target - allows JOINing to get column type and metadata';
```

Create `migrations/0XX_entity_relationship_column_ids.down.sql`:

```sql
DROP INDEX IF EXISTS idx_entity_rel_target_col;
DROP INDEX IF EXISTS idx_entity_rel_source_col;
ALTER TABLE engine_entity_relationships DROP COLUMN IF EXISTS target_column_id;
ALTER TABLE engine_entity_relationships DROP COLUMN IF EXISTS source_column_id;
```

### 2. Model: `pkg/models/entity_relationship.go`

Add fields to `EntityRelationship` struct:

```go
type EntityRelationship struct {
    // ... existing fields ...
    SourceColumnID     *uuid.UUID `json:"source_column_id,omitempty"`     // FK to engine_schema_columns
    TargetColumnID     *uuid.UUID `json:"target_column_id,omitempty"`     // FK to engine_schema_columns
    SourceColumnType   string     `json:"source_column_type,omitempty"`   // Populated via JOIN, not stored
    TargetColumnType   string     `json:"target_column_type,omitempty"`   // Populated via JOIN, not stored
}
```

Note: `SourceColumnType` and `TargetColumnType` are transient fields populated by repository queries, not stored in the database.

### 3. Repository: `pkg/repositories/entity_relationship_repository.go`

#### Update `Create` method

Add `source_column_id` and `target_column_id` to INSERT:

```go
query := `
    INSERT INTO engine_entity_relationships (
        id, ontology_id, source_entity_id, target_entity_id,
        source_column_schema, source_column_table, source_column_name,
        target_column_schema, target_column_table, target_column_name,
        source_column_id, target_column_id,  -- ADD THESE
        detection_method, confidence, status, description, created_at
    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
    ...`
```

#### Update `GetByProject` query

JOIN to `engine_schema_columns` to get types:

```go
query := `
    SELECT r.id, r.ontology_id, r.source_entity_id, r.target_entity_id,
           r.source_column_schema, r.source_column_table, r.source_column_name,
           r.target_column_schema, r.target_column_table, r.target_column_name,
           r.source_column_id, r.target_column_id,
           COALESCE(sc.data_type, '') as source_column_type,
           COALESCE(tc.data_type, '') as target_column_type,
           r.detection_method, r.confidence, r.status, r.description, r.created_at
    FROM engine_entity_relationships r
    JOIN engine_ontologies o ON r.ontology_id = o.id
    LEFT JOIN engine_schema_columns sc ON r.source_column_id = sc.id
    LEFT JOIN engine_schema_columns tc ON r.target_column_id = tc.id
    WHERE o.project_id = $1 AND o.is_active = true
    ORDER BY r.source_column_table, r.source_column_name`
```

#### Update `scanEntityRelationship`

Add scanning for new fields:

```go
err := row.Scan(
    &rel.ID, &rel.OntologyID, &rel.SourceEntityID, &rel.TargetEntityID,
    &rel.SourceColumnSchema, &rel.SourceColumnTable, &rel.SourceColumnName,
    &rel.TargetColumnSchema, &rel.TargetColumnTable, &rel.TargetColumnName,
    &rel.SourceColumnID, &rel.TargetColumnID,
    &rel.SourceColumnType, &rel.TargetColumnType,  // Transient fields from JOIN
    &rel.DetectionMethod, &rel.Confidence, &rel.Status, &rel.Description, &rel.CreatedAt,
)
```

**Important**: The `GetByOntology` and `GetByTables` methods need the same JOIN pattern.

### 4. Service: `pkg/services/deterministic_relationship_service.go`

#### Update `DiscoverFKRelationships` (around line 162)

Column IDs are already available - just store them:

```go
rel := &models.EntityRelationship{
    OntologyID:         ontology.ID,
    SourceEntityID:     sourceEntity.ID,
    TargetEntityID:     targetEntity.ID,
    SourceColumnSchema: sourceTable.SchemaName,
    SourceColumnTable:  sourceTable.TableName,
    SourceColumnName:   sourceCol.ColumnName,
    SourceColumnID:     &sourceCol.ID,  // ADD THIS
    TargetColumnSchema: targetTable.SchemaName,
    TargetColumnTable:  targetTable.TableName,
    TargetColumnName:   targetCol.ColumnName,
    TargetColumnID:     &targetCol.ID,  // ADD THIS
    DetectionMethod:    detectionMethod,
    Confidence:         1.0,
    Status:             models.RelationshipStatusConfirmed,
}
```

#### Update `DiscoverPKMatchRelationships`

Same pattern - find where `EntityRelationship` is created and add `SourceColumnID` and `TargetColumnID`.

### 5. LLM Tool: `pkg/llm/tool_executor.go`

#### Update `createEntityRelationship` (around line 700)

Need to look up column IDs before creating relationship:

```go
// Look up source column ID
sourceColumnID, err := e.schemaRepo.GetColumnIDByName(ctx, e.projectID, "public", args.SourceTable, args.SourceColumn)
if err != nil {
    // Log warning but continue - column ID is optional
    e.logger.Warn("Could not find source column ID", zap.Error(err))
}

// Look up target column ID
targetColumnID, err := e.schemaRepo.GetColumnIDByName(ctx, e.projectID, "public", args.TargetTable, args.TargetColumn)
if err != nil {
    e.logger.Warn("Could not find target column ID", zap.Error(err))
}

relationship := &models.EntityRelationship{
    // ... existing fields ...
    SourceColumnID:     sourceColumnID,  // May be nil
    TargetColumnID:     targetColumnID,  // May be nil
}
```

#### Add helper to SchemaRepository

Add `GetColumnIDByName` method to `pkg/repositories/schema_repository.go`:

```go
func (r *schemaRepository) GetColumnIDByName(ctx context.Context, projectID uuid.UUID, schemaName, tableName, columnName string) (*uuid.UUID, error) {
    scope, ok := database.GetTenantScope(ctx)
    if !ok {
        return nil, fmt.Errorf("no tenant scope in context")
    }

    query := `
        SELECT c.id
        FROM engine_schema_columns c
        JOIN engine_schema_tables t ON c.schema_table_id = t.id
        WHERE t.project_id = $1
          AND t.schema_name = $2
          AND t.table_name = $3
          AND c.column_name = $4
          AND t.deleted_at IS NULL
          AND c.deleted_at IS NULL
        LIMIT 1`

    var id uuid.UUID
    err := scope.Conn.QueryRow(ctx, query, projectID, schemaName, tableName, columnName).Scan(&id)
    if err == pgx.ErrNoRows {
        return nil, nil
    }
    if err != nil {
        return nil, err
    }
    return &id, nil
}
```

### 6. Handler: `pkg/handlers/entity_relationship_handler.go`

Map column types from model to response (around line 170):

```go
relResponses = append(relResponses, EntityRelationshipResponse{
    // ... existing mappings ...
    SourceColumnType: rel.SourceColumnType,  // ADD THIS
    TargetColumnType: rel.TargetColumnType,  // ADD THIS
})
```

### 7. UI: `ui/src/pages/RelationshipsPage.tsx`

Re-add column type display (was at lines 580-582 and 591-593):

```tsx
<span className="font-mono text-text-primary">
  {rel.source_column_name}
</span>
{rel.source_column_type && (
  <span className="text-text-tertiary text-xs">
    ({rel.source_column_type})
  </span>
)}
<ArrowRight className="h-3 w-3 text-text-tertiary flex-shrink-0" />
```

And similarly for target column type after `{rel.target_column_name}`.

## Testing

1. Run migration
2. Trigger ontology extraction on a test project
3. Verify `engine_entity_relationships` has populated `source_column_id` and `target_column_id`
4. Verify API response includes column types:
   ```bash
   curl http://localhost:3443/api/projects/{pid}/relationships | jq '.data.relationships[0]'
   ```
5. Verify UI displays column types like `user_id (uuid) → accounts.id (uuid)`

## Backfill Consideration

Existing relationships won't have column IDs. Options:

1. **Leave null** - UI handles gracefully with conditional rendering
2. **Backfill script** - Match on schema/table/column names to populate IDs
3. **Re-run ontology extraction** - Relationships are recreated with IDs

Recommend option 1 (leave null) for simplicity - new extractions will have the data.
