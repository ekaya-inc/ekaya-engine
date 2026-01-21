# FIX: Bug 4 - Approved Column Metadata Changes Not Reflected in probe_column

**Priority:** Medium
**Component:** MCP Server / Column Metadata

## Problem Statement

After approving a pending change to add enum values to a column, `probe_column` does not return the enum information.

**Reproduction Steps:**
1. Call `scan_data_changes` which detects enum values in `s7_tickets.ticket_type`
2. Approve the change via `approve_change`
3. Response shows "Change approved and applied successfully"
4. Call `probe_column(table='s7_tickets', column='ticket_type')`
5. Returns `{"table":"s7_tickets","column":"ticket_type"}` with no enum info

## Root Cause Analysis

### Two Separate Storage Locations

The system has **two separate storage locations** for column metadata that are not synchronized:

1. **`engine_column_metadata` table** - Used by MCP tools and approve_change
2. **`ontology.column_details` JSONB** - Used by probe_column

### Where approve_change Writes

**pkg/services/change_review_service.go:280-313** (`applyCreateColumnMetadata`):
```go
meta := &models.ColumnMetadata{
    ProjectID:  change.ProjectID,
    TableName:  change.TableName,
    ColumnName: change.ColumnName,
    CreatedBy:  reviewerSource,
}

if enumVals, ok := payload["enum_values"].([]any); ok {
    var vals []string
    for _, v := range enumVals {
        if s, ok := v.(string); ok {
            vals = append(vals, s)
        }
    }
    meta.EnumValues = vals
}

return s.columnMetadataRepo.Upsert(ctx, meta)  // Writes to engine_column_metadata
```

### Where probe_column Reads

**pkg/mcp/tools/probe.go:312-355**:
```go
// Get semantic information from ontology if available
ontology, err := deps.OntologyRepo.GetActive(ctx, projectID)
if ontology != nil {
    // Get column details from ontology
    columnDetails := ontology.GetColumnDetails(tableName)  // Reads from JSONB
    for _, colDetail := range columnDetails {
        if colDetail.Name == columnName {
            // Extract enum labels from ontology.ColumnDetails
            if len(colDetail.EnumValues) > 0 {
                enumLabels := make(map[string]string)
                // ...
            }
        }
    }
}
```

### The Data Flow Gap

| Operation | Storage Location |
|-----------|-----------------|
| `approve_change` for column metadata | `engine_column_metadata` table |
| `update_column` MCP tool | `engine_column_metadata` AND `ontology.column_details` JSONB |
| `probe_column` reads from | `ontology.column_details` JSONB only |
| Extraction enrichment writes to | `ontology.column_details` JSONB |

The `update_column` MCP tool writes to both locations (see pkg/mcp/tools/column.go:199-250), but `approve_change` only writes to `engine_column_metadata`.

## Tasks

- [x] Task 1: Update probe_column to read from both data sources (Option A)
- [ ] Task 2: Add integration test for full approve_change → probe_column flow

## Implementation Details

### Option A: Make probe_column read from both sources (Recommended)

Update `probe_column` to merge data from both `ontology.ColumnDetails` AND `engine_column_metadata`:

**pkg/mcp/tools/probe.go** (after line 355, add fallback):

```go
// If no enum labels from ontology, check engine_column_metadata
if response.Semantic == nil || len(response.Semantic.EnumLabels) == 0 {
    columnMeta, err := deps.ColumnMetadataRepo.GetByTableColumn(ctx, projectID, tableName, columnName)
    if err == nil && columnMeta != nil && len(columnMeta.EnumValues) > 0 {
        if response.Semantic == nil {
            response.Semantic = &probeColumnSemantic{}
        }
        enumLabels := make(map[string]string)
        for _, ev := range columnMeta.EnumValues {
            enumLabels[ev] = ev // Value as its own label (no enrichment)
        }
        response.Semantic.EnumLabels = enumLabels

        // Also merge other metadata
        if columnMeta.Description != nil && response.Semantic.Description == "" {
            response.Semantic.Description = *columnMeta.Description
        }
        if columnMeta.Entity != nil && response.Semantic.Entity == "" {
            response.Semantic.Entity = *columnMeta.Entity
        }
        if columnMeta.Role != nil && response.Semantic.Role == "" {
            response.Semantic.Role = *columnMeta.Role
        }
    }
}
```

### Option B: Make approve_change also update ontology JSONB

Modify `applyCreateColumnMetadata` and `applyUpdateColumnMetadata` to also update `ontology.ColumnDetails`:

```go
func (s *changeReviewService) applyCreateColumnMetadata(ctx context.Context, change *models.PendingChange, reviewerSource string) error {
    // ... existing code to create meta in engine_column_metadata ...

    // Also update ontology.ColumnDetails for consistency
    ontology, err := s.ontologyRepo.GetActive(ctx, change.ProjectID)
    if err == nil && ontology != nil {
        existingColumns := ontology.GetColumnDetails(change.TableName)
        // Find or create column detail
        found := false
        for i, col := range existingColumns {
            if col.Name == change.ColumnName {
                // Update existing
                existingColumns[i].EnumValues = convertToEnumValues(meta.EnumValues)
                found = true
                break
            }
        }
        if !found {
            existingColumns = append(existingColumns, models.ColumnDetail{
                Name:       change.ColumnName,
                EnumValues: convertToEnumValues(meta.EnumValues),
            })
        }
        s.ontologyRepo.UpdateColumnDetails(ctx, change.ProjectID, change.TableName, existingColumns)
    }

    return nil
}
```

### Recommendation

**Option A is preferred** because:
1. It's a localized fix in probe_column
2. It doesn't require changing the approval workflow
3. It handles the general case where data might exist in either location
4. It's consistent with how other MCP tools should handle merged data sources

## Files to Modify

### For Option A:
1. **pkg/mcp/tools/probe.go:355** (after existing ontology check)
   - Add fallback to check `engine_column_metadata` via `ColumnMetadataRepo`
   - Add `ColumnMetadataRepo` to `ProbeToolDeps` if not already present

### For Option B:
1. **pkg/services/change_review_service.go:280-313** (`applyCreateColumnMetadata`)
   - Add `ontologyRepo.UpdateColumnDetails` call after `columnMetadataRepo.Upsert`

2. **pkg/services/change_review_service.go:315-370** (`applyUpdateColumnMetadata`)
   - Add `ontologyRepo.UpdateColumnDetails` call after `columnMetadataRepo.Upsert`

## Testing Verification

After implementing:

1. Run `scan_data_changes` on a table with enum-like columns
2. Approve a detected enum change via `approve_change`
3. Immediately call `probe_column` for that column
4. Verify `enum_labels` appears in the response
5. Also verify enum labels persist across sessions

Additional test cases:
- Column has metadata in BOTH locations (ontology and column_metadata)
- Column has metadata in ONLY column_metadata (post-approval)
- Column has metadata in ONLY ontology (extraction-created)
- Column has NO metadata in either location
