# FIX: BUG-8 - Ontology Extraction Includes Deselected Columns

**Bug Reference:** BUGS-ontology-extraction.md, BUG-8
**Severity:** Critical
**Type:** Security/Privacy Issue

## Problem Summary

Ontology extraction processes **all columns** (including deselected ones) even though admins explicitly deselect columns to exclude PII, legacy data, or irrelevant fields. Deselected columns appear in the ontology with full semantic enrichment (descriptions, synonyms, enum values).

This is a **critical security issue** because:
1. Admins deselect columns containing PII (emails, passwords, SSNs, etc.)
2. The ontology extraction still processes and stores metadata about these columns
3. LLM prompts may include deselected column data during enrichment
4. MCP clients can query information about columns that should be hidden

## Root Cause

### Repository Methods Don't Filter by Selection

**File:** `pkg/repositories/schema_repository.go`

**`GetColumnsByTables` (line 472):**
```sql
SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
       -- ... all columns
FROM engine_schema_columns c
JOIN engine_schema_tables t ON c.schema_table_id = t.id
WHERE c.project_id = $1
  AND t.table_name = ANY($2)
  AND c.deleted_at IS NULL
  AND t.deleted_at IS NULL
-- ❌ MISSING: AND c.is_selected = true
```

**`ListColumnsByTable` (line 393):**
```sql
SELECT id, project_id, schema_table_id, column_name, data_type,
       -- ... all columns
FROM engine_schema_columns
WHERE project_id = $1 AND schema_table_id = $2 AND deleted_at IS NULL
-- ❌ MISSING: AND is_selected = true
```

Both methods return ALL columns regardless of `is_selected` status.

### Callers Affected

These services use the unfiltered column data:

| File | Line | Function | Impact |
|------|------|----------|--------|
| `pkg/services/column_enrichment.go` | 256 | Column enrichment | PII columns enriched with descriptions |
| `pkg/services/ontology_finalization.go` | 200, 409 | Ontology finalization | PII columns in final ontology |
| `pkg/services/data_change_detection.go` | 155, 367 | Change detection | PII columns monitored |
| `pkg/services/incremental_dag_service.go` | 674 | Incremental updates | PII columns processed |
| `pkg/services/ontology_context.go` | (via GetColumnsContext) | MCP context | PII columns exposed to MCP |

## The Fix

### Option A: Add Filter Parameter to Repository Methods (Recommended)

Add `selectedOnly bool` parameter to repository methods:

**`GetColumnsByTables`:**
```go
func (r *schemaRepository) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
    // ...
    query := `
        SELECT c.id, ...
        FROM engine_schema_columns c
        JOIN engine_schema_tables t ON c.schema_table_id = t.id
        WHERE c.project_id = $1
          AND t.table_name = ANY($2)
          AND c.deleted_at IS NULL
          AND t.deleted_at IS NULL`

    if selectedOnly {
        query += ` AND c.is_selected = true`
    }
    // ...
}
```

**`ListColumnsByTable`:**
```go
func (r *schemaRepository) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
    // ...
    query := `
        SELECT id, ...
        FROM engine_schema_columns
        WHERE project_id = $1 AND schema_table_id = $2 AND deleted_at IS NULL`

    if selectedOnly {
        query += ` AND is_selected = true`
    }
    // ...
}
```

### Option B: Create New Methods (Alternative)

Add new methods that always filter by selection:
- `GetSelectedColumnsByTables`
- `ListSelectedColumnsByTable`

Keep existing methods for admin/management use cases.

## Implementation Steps

### Step 1: Update Repository Interface ✓

**File:** `pkg/repositories/schema_repository.go`

Add `selectedOnly` parameter to:
- `GetColumnsByTables(ctx, projectID, tableNames, selectedOnly bool)`
- `ListColumnsByTable(ctx, projectID, tableID, selectedOnly bool)`

### Step 2: Update Repository Implementation ✓

Modify SQL queries to conditionally add `AND is_selected = true` when `selectedOnly=true`.

### Step 3: Update All Callers ✓

Update all services to pass `selectedOnly=true` during ontology extraction:

| File | Call Site | Action |
|------|-----------|--------|
| `pkg/services/column_enrichment.go:256` | `GetColumnsByTables` | Pass `true` |
| `pkg/services/ontology_finalization.go:200` | `GetColumnsByTables` | Pass `true` |
| `pkg/services/ontology_finalization.go:409` | `GetColumnsByTables` | Pass `true` |
| `pkg/services/data_change_detection.go:155` | `ListColumnsByTable` | Pass `true` |
| `pkg/services/data_change_detection.go:367` | `ListColumnsByTable` | Pass `true` |
| `pkg/services/incremental_dag_service.go:674` | `GetColumnsByTables` | Pass `true` |

### Step 4: Update MCP Context Service ✓

**File:** `pkg/services/ontology_context.go`

Ensure `GetColumnsContext` uses `selectedOnly=true`.

### Step 5: Update Tests ✓

Update test mocks and expectations to handle `selectedOnly` parameter.

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/repositories/schema_repository.go` | Add `selectedOnly` parameter to methods |
| `pkg/repositories/schema_repository_test.go` | Update tests |
| `pkg/services/column_enrichment.go` | Pass `selectedOnly=true` |
| `pkg/services/ontology_finalization.go` | Pass `selectedOnly=true` |
| `pkg/services/data_change_detection.go` | Pass `selectedOnly=true` |
| `pkg/services/incremental_dag_service.go` | Pass `selectedOnly=true` |
| `pkg/services/ontology_context.go` | Pass `selectedOnly=true` |
| Various test files | Update mocks |

## Testing

### Security Verification

1. Create project with table having email column
2. Deselect the email column
3. Run ontology extraction
4. Verify email column NOT in ontology

```sql
-- Verify deselected column not in ontology
SELECT oc.* FROM engine_ontology_columns oc
JOIN engine_ontologies o ON oc.ontology_id = o.id
WHERE o.project_id = '...' AND o.is_active = true
AND oc.column_name = 'email';
-- Should return 0 rows
```

### MCP Verification

```javascript
// MCP client should NOT see deselected columns
mcp__get_context({ depth: 'columns', tables: ['users'] })
// Response should NOT include deselected columns
```

### Integration Test

```go
func TestColumnEnrichment_OnlySelectedColumns(t *testing.T) {
    // Setup: Create table with deselected column
    // Run column enrichment
    // Verify: Deselected column NOT processed
}
```

## Success Criteria

- [x] Deselected columns do NOT appear in the ontology
- [x] `get_context(depth='columns')` only returns selected columns
- [x] Column enrichment only processes selected columns
- [x] MCP clients cannot query deselected column metadata
- [x] LLM prompts do not include deselected column information
- [x] All tests pass

## Security Considerations

### Data Exposure Risks

| Risk | Current Behavior | After Fix |
|------|------------------|-----------|
| PII in ontology | ❌ Deselected columns enriched | ✅ Deselected columns excluded |
| PII in LLM prompts | ❌ May include deselected columns | ✅ Only selected columns in prompts |
| PII via MCP | ❌ MCP can query deselected columns | ✅ MCP only sees selected columns |

### Audit Trail

Consider adding logging when columns are filtered out:
```go
logger.Debug("Filtered deselected columns",
    zap.Int("total", len(allColumns)),
    zap.Int("selected", len(selectedColumns)),
    zap.Int("filtered", len(allColumns)-len(selectedColumns)))
```

## Notes

This fix is similar to BUG-7 (tables) but for columns. The pattern should be consistent:
- Tables: `ListTablesByDatasource(ctx, projectID, datasourceID, selectedOnly=true)`
- Columns: `GetColumnsByTables(ctx, projectID, tableNames, selectedOnly=true)`

Both should respect the admin's selection choices during ontology extraction.
