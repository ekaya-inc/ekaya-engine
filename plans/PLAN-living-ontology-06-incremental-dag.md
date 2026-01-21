# PLAN-06: Incremental DAG Execution

**Parent:** [PLAN-living-ontology-master.md](./PLAN-living-ontology-master.md)
**Dependencies:** PLAN-05 (Change Queue & Precedence)
**Enables:** Full AI Config mode with continuous ontology freshness

## Goal

When pending changes are approved in AI Config mode, run targeted LLM enrichment only for what changed, instead of full re-extraction.

## Current State

- Ontology extraction runs all DAG nodes for entire schema
- Takes minutes for large schemas
- Any change requires full re-extraction
- No incremental update capability

## Desired State

```
1. New table detected → run EntityDiscovery + EntityEnrichment for ONLY that table
2. New column detected → run ColumnEnrichment for ONLY that column
3. New relationship detected → run RelationshipEnrichment for ONLY that relationship
4. Results merged into existing ontology respecting precedence
```

Time to enrich one new table: <30 seconds (vs minutes for full extraction)

## Implementation

### 1. Create Incremental DAG Service

**File:** `pkg/services/incremental_dag_service.go`

```go
type IncrementalDAGService interface {
    // Process a single approved change with LLM enrichment
    ProcessChange(ctx context.Context, change *PendingChange) error

    // Process batch of changes (groups by type for efficiency)
    ProcessChanges(ctx context.Context, changes []*PendingChange) error
}

type incrementalDAGService struct {
    ontologyRepo      repositories.OntologyRepository
    entityRepo        repositories.OntologyEntityRepository
    relationshipRepo  repositories.EntityRelationshipRepository
    columnRepo        repositories.OntologyColumnRepository
    aiConfigService   services.AIConfigService
    llmClient         llm.Client
    changeReviewSvc   services.ChangeReviewService
    logger            *slog.Logger
}
```

### 2. Targeted Entity Discovery

For new tables, run entity discovery only for that table:

```go
func (s *incrementalDAGService) processNewTable(
    ctx context.Context,
    change *PendingChange,
) error {
    // Check if AI Config is attached
    aiConfig, err := s.aiConfigService.GetForProject(ctx, change.ProjectID)
    if err != nil || aiConfig == nil {
        // No AI Config - just create basic entity from table name
        return s.createBasicEntity(ctx, change)
    }

    // Get table schema
    tableSchema, err := s.getTableSchema(ctx, change.ProjectID, change.TableName)
    if err != nil {
        return err
    }

    // Run LLM entity discovery for just this table
    prompt := buildEntityDiscoveryPrompt(tableSchema)
    response, err := s.llmClient.Complete(ctx, aiConfig, prompt)
    if err != nil {
        return fmt.Errorf("LLM entity discovery failed: %w", err)
    }

    // Parse response and create entity
    entity := parseEntityFromLLM(response, change.TableName)
    entity.CreatedBy = "inference"

    // Check precedence before creating
    canCreate, reason, _ := s.changeReviewSvc.CanUpdate(ctx, change.ProjectID, ChangeTarget{
        Type:       "entity",
        EntityName: entity.Name,
    }, "inference")

    if !canCreate {
        s.logger.Info("skipping entity creation due to precedence", "entity", entity.Name, "reason", reason)
        return nil
    }

    return s.entityRepo.Create(ctx, entity)
}

func buildEntityDiscoveryPrompt(table *models.SchemaTable) string {
    return fmt.Sprintf(`Analyze this database table and identify the business entity it represents.

Table: %s
Columns:
%s

Respond with JSON:
{
  "entity_name": "PascalCase entity name",
  "description": "Business description of what this entity represents",
  "aliases": ["alternative", "names"],
  "key_columns": ["important", "business", "columns"]
}`, table.TableName, formatColumns(table.Columns))
}
```

### 3. Targeted Column Enrichment

For new columns, run enrichment only for that column:

```go
func (s *incrementalDAGService) processNewColumn(
    ctx context.Context,
    change *PendingChange,
) error {
    aiConfig, err := s.aiConfigService.GetForProject(ctx, change.ProjectID)
    if err != nil || aiConfig == nil {
        return nil // Skip enrichment without AI Config
    }

    // Get column details and sample data
    columnInfo, err := s.getColumnInfo(ctx, change.ProjectID, change.TableName, change.ColumnName)
    if err != nil {
        return err
    }

    // Run LLM column enrichment
    prompt := buildColumnEnrichmentPrompt(columnInfo)
    response, err := s.llmClient.Complete(ctx, aiConfig, prompt)
    if err != nil {
        return fmt.Errorf("LLM column enrichment failed: %w", err)
    }

    // Parse and apply
    metadata := parseColumnMetadataFromLLM(response)
    metadata.CreatedBy = "inference"

    // Check precedence
    canUpdate, _, _ := s.changeReviewSvc.CanUpdate(ctx, change.ProjectID, ChangeTarget{
        Type:       "column",
        TableName:  change.TableName,
        ColumnName: change.ColumnName,
    }, "inference")

    if !canUpdate {
        return nil
    }

    return s.columnRepo.Upsert(ctx, change.ProjectID, change.TableName, change.ColumnName, metadata)
}
```

### 4. Targeted Relationship Enrichment

For new relationships, enrich just that relationship:

```go
func (s *incrementalDAGService) processNewRelationship(
    ctx context.Context,
    change *PendingChange,
) error {
    aiConfig, err := s.aiConfigService.GetForProject(ctx, change.ProjectID)
    if err != nil || aiConfig == nil {
        return s.createBasicRelationship(ctx, change)
    }

    // Get relationship context
    relInfo := change.SuggestedPayload
    fromTable := relInfo["from_table"].(string)
    toTable := relInfo["to_table"].(string)

    // Get entity names for these tables
    fromEntity, _ := s.entityRepo.GetByTable(ctx, change.ProjectID, fromTable)
    toEntity, _ := s.entityRepo.GetByTable(ctx, change.ProjectID, toTable)

    if fromEntity == nil || toEntity == nil {
        return s.createBasicRelationship(ctx, change)
    }

    // Run LLM relationship enrichment
    prompt := buildRelationshipEnrichmentPrompt(fromEntity, toEntity, relInfo)
    response, err := s.llmClient.Complete(ctx, aiConfig, prompt)
    if err != nil {
        return fmt.Errorf("LLM relationship enrichment failed: %w", err)
    }

    relationship := parseRelationshipFromLLM(response, fromEntity.Name, toEntity.Name)
    relationship.CreatedBy = "inference"

    return s.relationshipRepo.Create(ctx, relationship)
}
```

### 5. Batch Processing for Efficiency

Group changes by type and process together:

```go
func (s *incrementalDAGService) ProcessChanges(
    ctx context.Context,
    changes []*PendingChange,
) error {
    // Group by type
    byType := make(map[string][]*PendingChange)
    for _, c := range changes {
        byType[c.ChangeType] = append(byType[c.ChangeType], c)
    }

    // Process new tables first (entities before relationships)
    if tables := byType["new_table"]; len(tables) > 0 {
        if err := s.processNewTablesBatch(ctx, tables); err != nil {
            s.logger.Error("batch entity processing failed", "error", err)
        }
    }

    // Then new columns
    if columns := byType["new_column"]; len(columns) > 0 {
        if err := s.processNewColumnsBatch(ctx, columns); err != nil {
            s.logger.Error("batch column processing failed", "error", err)
        }
    }

    // Then relationships
    if rels := byType["new_fk_pattern"]; len(rels) > 0 {
        if err := s.processNewRelationshipsBatch(ctx, rels); err != nil {
            s.logger.Error("batch relationship processing failed", "error", err)
        }
    }

    // Then enum updates
    if enums := byType["new_enum_values"]; len(enums) > 0 {
        for _, e := range enums {
            s.processEnumUpdate(ctx, e) // No LLM needed
        }
    }

    return nil
}
```

### 6. Hook into Change Approval

When changes are approved, trigger incremental enrichment if AI Config exists:

**File:** `pkg/services/change_review.go`

```go
func (s *changeReviewService) ApproveChange(
    ctx context.Context,
    changeID uuid.UUID,
    reviewer string,
) error {
    // ... existing approval logic ...

    // After applying change, trigger incremental enrichment
    if s.hasAIConfig(ctx, change.ProjectID) && change.ChangeSource == "schema_refresh" {
        go func() {
            enrichCtx := context.Background()
            if err := s.incrementalDAG.ProcessChange(enrichCtx, change); err != nil {
                s.logger.Error("incremental enrichment failed", "change_id", changeID, "error", err)
            }
        }()
    }

    return nil
}
```

### 7. Auto-Apply Option for AI Config Mode

Allow admins to enable auto-approval of detected changes:

```go
type ProjectSettings struct {
    AutoApproveSchemaChanges bool `json:"auto_approve_schema_changes"`
    AutoApproveDataChanges   bool `json:"auto_approve_data_changes"`
}

// In refresh_schema, after detecting changes:
if settings.AutoApproveSchemaChanges {
    for _, change := range changes {
        if change.ChangeSource == "schema_refresh" {
            s.changeReviewSvc.ApproveChange(ctx, change.ID, "auto")
        }
    }
}
```

## Tasks

1. [x] Create `IncrementalDAGService` interface
2. [x] Implement `processNewTable()` with targeted entity discovery
3. [x] Implement `processNewColumn()` with targeted column enrichment
4. [x] Implement `processNewRelationship()` with targeted relationship enrichment
5. [x] Implement `processEnumUpdate()` (no LLM, just merge values)
6. [x] Implement `ProcessChanges()` batch method
7. [x] Build entity discovery prompt for single table
8. [x] Build column enrichment prompt for single column
9. [x] Build relationship enrichment prompt for single relationship
10. [x] Hook incremental enrichment into `ApproveChange()`
11. [x] Add `auto_approve_schema_changes` project setting
12. [x] Add `auto_approve_data_changes` project setting
13. [x] Test: new table → approve → entity created with LLM description
14. [x] Test: batch of 5 changes → processed efficiently
15. [x] Test: incremental enrichment respects precedence

## Testing

### Single Table Enrichment

```
1. Setup: AI Config attached, existing ontology with 10 entities
2. execute("CREATE TABLE reviews (id uuid PRIMARY KEY, product_id uuid, rating int, comment text)")
3. refresh_schema() → pending change for new table
4. approve_change(change_id)
   → Expected:
     - Entity "Review" created with LLM-generated description
     - Enrichment took <30 seconds
     - Existing 10 entities unchanged
```

### Batch Enrichment

```
1. Setup: AI Config attached
2. Create 3 new tables via execute()
3. refresh_schema() → 3 pending changes
4. approve_all_changes()
   → Expected:
     - 3 entities created with descriptions
     - Processed as batch (fewer LLM calls)
     - Total time <60 seconds
```

### Precedence Respected

```
1. Create entity "Order" via MCP (source: mcp)
2. Drop and recreate orders table
3. refresh_schema() → pending change for "new" table
4. approve_change()
   → Expected: LLM enrichment SKIPPED because mcp-created entity exists
   → Entity "Order" unchanged
```

## Performance Targets

| Operation | Target Time |
|-----------|-------------|
| Single table enrichment | <30s |
| Single column enrichment | <10s |
| Single relationship enrichment | <10s |
| Batch of 5 tables | <60s |
| Enum value update | <1s (no LLM) |

## Future Enhancements

1. **Parallel LLM calls** - Process multiple independent entities concurrently
2. **Smart batching** - Group related tables for context-aware enrichment
3. **Incremental summaries** - Update domain summary when new entities added
4. **Change impact analysis** - Show what else might be affected by a change
