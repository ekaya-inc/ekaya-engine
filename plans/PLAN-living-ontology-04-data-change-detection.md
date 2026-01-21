# PLAN-04: Data Change Detection

**Parent:** [PLAN-living-ontology-master.md](./PLAN-living-ontology-master.md)
**Dependencies:** PLAN-03 (Schema Change Detection)
**Enables:** PLAN-05 (Change Queue)

## Goal

Detect data-level changes that affect ontology accuracy: new enum values, cardinality shifts, emerging FK patterns. Queue these as pending changes for review.

## Current State

- Ontology extraction samples data once during initial extraction
- No ongoing detection of data changes
- Enum values hardcoded at extraction time
- Cardinality determined once, never updated

## Desired State

On-demand or scheduled data scan detects:
```json
{
  "data_changes": [
    {
      "type": "new_enum_values",
      "table": "orders",
      "column": "status",
      "current_values": ["pending", "shipped", "delivered"],
      "new_values": ["cancelled", "refunded"]
    },
    {
      "type": "cardinality_change",
      "from_table": "users",
      "from_column": "id",
      "to_table": "orders",
      "to_column": "user_id",
      "old_cardinality": "1:1",
      "new_cardinality": "1:N"
    },
    {
      "type": "potential_enum",
      "table": "products",
      "column": "category",
      "distinct_count": 12,
      "sample_values": ["Electronics", "Clothing", "Home", ...]
    }
  ]
}
```

## Implementation

### 1. Create Data Scanner Service

**File:** `pkg/services/data_change_detection.go`

```go
type DataChangeDetectionService interface {
    // Full scan of all columns for changes
    ScanForChanges(ctx context.Context, projectID, datasourceID uuid.UUID) ([]PendingChange, error)

    // Scan specific tables (for targeted refresh)
    ScanTables(ctx context.Context, projectID, datasourceID uuid.UUID, tableNames []string) ([]PendingChange, error)
}

type dataChangeDetectionService struct {
    schemaRepo        repositories.SchemaRepository
    ontologyRepo      repositories.OntologyRepository
    pendingChangeRepo repositories.PendingChangeRepository
    datasourceAdapter adapters.DatasourceAdapter
    logger            *slog.Logger
}
```

### 2. Enum Value Detection

Detect new values for columns already marked as enums, and potential enums for unknown columns.

```go
func (s *dataChangeDetectionService) detectEnumChanges(
    ctx context.Context,
    conn adapters.QueryExecutor,
    column *models.SchemaColumn,
    existingMetadata *models.ColumnDetail,
) (*PendingChange, error) {
    // Query distinct values
    query := fmt.Sprintf(
        "SELECT DISTINCT %s FROM %s WHERE %s IS NOT NULL LIMIT 101",
        column.ColumnName, column.TableName, column.ColumnName,
    )

    rows, err := conn.Query(ctx, query)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var values []string
    for rows.Next() {
        var v string
        if err := rows.Scan(&v); err != nil {
            continue
        }
        values = append(values, v)
    }

    // If more than 100 distinct values, not an enum
    if len(values) > 100 {
        return nil, nil
    }

    // Check for new values if column already marked as enum
    if existingMetadata != nil && len(existingMetadata.EnumValues) > 0 {
        existingSet := make(map[string]bool)
        for _, v := range existingMetadata.EnumValues {
            existingSet[v] = true
        }

        var newValues []string
        for _, v := range values {
            if !existingSet[v] {
                newValues = append(newValues, v)
            }
        }

        if len(newValues) > 0 {
            return &PendingChange{
                ChangeType:   "new_enum_values",
                ChangeSource: "data_scan",
                TableName:    column.TableName,
                ColumnName:   column.ColumnName,
                OldValue:     map[string]any{"enum_values": existingMetadata.EnumValues},
                NewValue:     map[string]any{"new_values": newValues, "all_values": values},
                SuggestedAction: "update_column",
                SuggestedPayload: map[string]any{
                    "table":       column.TableName,
                    "column":      column.ColumnName,
                    "enum_values": values,
                },
                Status: "pending",
            }, nil
        }
    }

    // Check if unknown column looks like an enum (<=50 distinct, reasonable length)
    if existingMetadata == nil && len(values) <= 50 {
        maxLen := 0
        for _, v := range values {
            if len(v) > maxLen {
                maxLen = len(v)
            }
        }

        // Looks like an enum if values are short strings
        if maxLen <= 50 {
            return &PendingChange{
                ChangeType:   "potential_enum",
                ChangeSource: "data_scan",
                TableName:    column.TableName,
                ColumnName:   column.ColumnName,
                NewValue:     map[string]any{"distinct_count": len(values), "sample_values": values},
                SuggestedAction: "update_column",
                SuggestedPayload: map[string]any{
                    "table":       column.TableName,
                    "column":      column.ColumnName,
                    "enum_values": values,
                },
                Status: "pending",
            }, nil
        }
    }

    return nil, nil
}
```

### 3. Cardinality Detection

Detect when relationship cardinality has changed based on actual data.

```go
func (s *dataChangeDetectionService) detectCardinalityChanges(
    ctx context.Context,
    conn adapters.QueryExecutor,
    rel *models.SchemaRelationship,
    existingRel *models.EntityRelationship,
) (*PendingChange, error) {
    // Check actual cardinality from data
    query := fmt.Sprintf(`
        SELECT
            COUNT(DISTINCT %s) as source_distinct,
            COUNT(DISTINCT %s) as target_distinct,
            COUNT(*) as total_rows
        FROM %s
        WHERE %s IS NOT NULL
    `, rel.SourceColumn, rel.TargetColumn, rel.SourceTable, rel.SourceColumn)

    var sourceDistinct, targetDistinct, totalRows int64
    if err := conn.QueryRow(ctx, query).Scan(&sourceDistinct, &targetDistinct, &totalRows); err != nil {
        return nil, err
    }

    // Determine actual cardinality
    var actualCardinality string
    if sourceDistinct == totalRows && targetDistinct == totalRows {
        actualCardinality = "1:1"
    } else if sourceDistinct == totalRows {
        actualCardinality = "1:N"
    } else if targetDistinct == totalRows {
        actualCardinality = "N:1"
    } else {
        actualCardinality = "N:M"
    }

    // Compare with existing
    if existingRel != nil && existingRel.Cardinality != actualCardinality {
        return &PendingChange{
            ChangeType:   "cardinality_change",
            ChangeSource: "data_scan",
            TableName:    rel.SourceTable,
            ColumnName:   rel.SourceColumn,
            OldValue:     map[string]any{"cardinality": existingRel.Cardinality},
            NewValue:     map[string]any{"cardinality": actualCardinality},
            SuggestedAction: "update_relationship",
            SuggestedPayload: map[string]any{
                "from_entity": existingRel.FromEntity,
                "to_entity":   existingRel.ToEntity,
                "cardinality": actualCardinality,
            },
            Status: "pending",
        }, nil
    }

    return nil, nil
}
```

### 4. FK Pattern Detection

Detect columns that look like foreign keys but aren't declared as such.

```go
func (s *dataChangeDetectionService) detectPotentialFKs(
    ctx context.Context,
    conn adapters.QueryExecutor,
    column *models.SchemaColumn,
    allTables []*models.SchemaTable,
) (*PendingChange, error) {
    // Skip if already has FK
    if column.IsForeignKey {
        return nil, nil
    }

    // Look for columns named *_id that might reference other tables
    if !strings.HasSuffix(column.ColumnName, "_id") {
        return nil, nil
    }

    // Extract potential table name
    potentialTable := strings.TrimSuffix(column.ColumnName, "_id")

    // Check if table exists
    var targetTable *models.SchemaTable
    for _, t := range allTables {
        if t.TableName == potentialTable || t.TableName == potentialTable+"s" {
            targetTable = t
            break
        }
    }

    if targetTable == nil {
        return nil, nil
    }

    // Check if values actually match target table's PK
    query := fmt.Sprintf(`
        SELECT COUNT(*) as matches
        FROM %s src
        WHERE src.%s IN (SELECT id FROM %s)
    `, column.TableName, column.ColumnName, targetTable.TableName)

    var matches int64
    if err := conn.QueryRow(ctx, query).Scan(&matches); err != nil {
        return nil, err
    }

    // If >90% of values match, likely a FK
    totalQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s IS NOT NULL", column.TableName, column.ColumnName)
    var total int64
    conn.QueryRow(ctx, totalQuery).Scan(&total)

    if total > 0 && float64(matches)/float64(total) > 0.9 {
        return &PendingChange{
            ChangeType:   "new_fk_pattern",
            ChangeSource: "data_scan",
            TableName:    column.TableName,
            ColumnName:   column.ColumnName,
            NewValue: map[string]any{
                "target_table":  targetTable.TableName,
                "match_rate":    float64(matches) / float64(total),
            },
            SuggestedAction: "create_relationship",
            SuggestedPayload: map[string]any{
                "from_table":  column.TableName,
                "from_column": column.ColumnName,
                "to_table":    targetTable.TableName,
                "to_column":   "id",
            },
            Status: "pending",
        }, nil
    }

    return nil, nil
}
```

### 5. Add MCP Tool to Trigger Data Scan

**File:** `pkg/mcp/tools/ontology_changes.go`

```go
func registerScanDataChangesTool(s *server.MCPServer, deps *OntologyToolDeps) {
    tool := mcp.NewTool(
        "scan_data_changes",
        mcp.WithDescription(
            "Scan database for data-level changes that affect ontology accuracy. "+
            "Detects new enum values, cardinality shifts, and potential foreign keys. "+
            "Results are queued as pending changes for review.",
        ),
        mcp.WithArray("tables", mcp.Description("Specific tables to scan (default: all selected tables)")),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, datasourceID, tenantCtx, cleanup, err := AcquireToolAccessWithDatasource(ctx, deps.BaseDeps, "scan_data_changes")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        tables := getStringArrayParam(req, "tables", nil)

        var changes []PendingChange
        if len(tables) > 0 {
            changes, err = deps.DataChangeService.ScanTables(tenantCtx, projectID, datasourceID, tables)
        } else {
            changes, err = deps.DataChangeService.ScanForChanges(tenantCtx, projectID, datasourceID)
        }

        if err != nil {
            return NewErrorResult("scan_failed", err.Error()), nil
        }

        // Group changes by type for summary
        summary := make(map[string]int)
        for _, c := range changes {
            summary[c.ChangeType]++
        }

        return NewJSONResult(map[string]any{
            "changes_detected": len(changes),
            "summary":          summary,
        }), nil
    })
}
```

## Tasks

1. [x] Create `DataChangeDetectionService` interface in `pkg/services/`
2. [x] Implement enum value detection logic
3. [ ] Implement cardinality detection logic (deferred - requires EntityRelationshipRepository integration)
4. [x] Implement potential FK detection logic
5. [x] Create `ScanForChanges()` method that runs all detectors
6. [x] Create `ScanTables()` method for targeted scanning
7. [x] Add `DataChangeService` to `MCPToolDeps`
8. [x] Implement `scan_data_changes` MCP tool
9. [x] Add `scan_data_changes` to tool registry
10. [x] Test: add enum value to data → scan → pending change created
11. [ ] Test: change cardinality in data → scan → pending change created (blocked on task 3)

## Testing

```
1. Setup: table with status column, initially ['active', 'inactive']
2. Insert row with status='suspended'
3. scan_data_changes()
   → Returns: { changes_detected: 1, summary: { new_enum_values: 1 } }
4. list_pending_changes()
   → Shows: new_enum_values for status column with new value 'suspended'

5. Setup: users table, orders table with user_id column (no FK declared)
6. Populate orders with valid user_id values
7. scan_data_changes()
   → Returns: { changes_detected: 1, summary: { new_fk_pattern: 1 } }
8. list_pending_changes()
   → Shows: potential FK from orders.user_id to users.id
```

## Performance Considerations

- Data scans can be slow on large tables
- Consider:
  - Sampling instead of full table scans for initial detection
  - Background job queue for long-running scans
  - Rate limiting to avoid overloading datasource
  - Caching scan results with TTL
