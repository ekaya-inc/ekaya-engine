# Plan: Enrichment API Lazy Loading

## Problem Statement

The Enrichment page has two issues:

1. **Large Payload**: The `/api/projects/{pid}/ontology/enrichment` endpoint returns ~233KB of JSON containing ALL table summaries AND ALL column details for ALL tables in a single response.

2. **Double API Call**: The endpoint is called twice in development mode.

## Investigation Findings

### Double API Call (Not a Bug)

**Root Cause:** React StrictMode in `ui/src/main.tsx:14-16`

React StrictMode intentionally double-invokes effects to catch bugs. This is **development-only behavior** and does NOT happen in production builds.

**Recommendation:** No fix needed. This is expected React behavior.

### Large Payload (Real Issue)

**Root Cause:** `pkg/handlers/ontology_enrichment_handler.go`

The `GetEnrichment` handler returns everything at once:
- `entity_summaries[]` - Table-level summaries (relatively small)
- `column_details[]` - ALL columns for ALL tables with features (bulk of the 233KB)

## Proposed Solution

Replace the single endpoint with two endpoints using lazy loading:

1. **Table List Endpoint** - Returns only table-level summaries (small payload)
2. **Table Columns Endpoint** - Returns columns for a specific table (on-demand)

### API Structure

```
GET /api/projects/{pid}/ontology/enrichment/tables
→ Returns: table list with summary stats (~5-10KB)

GET /api/projects/{pid}/ontology/enrichment/tables/{table}/columns
→ Returns: columns for specific table (~2-5KB per table)
```

### Response Examples

**Tables Endpoint:**
```json
{
  "tables": [
    {
      "table_name": "users",
      "business_name": "Users",
      "description": "Platform users and accounts",
      "column_count": 15,
      "enriched_column_count": 15,
      "has_features": true
    }
  ],
  "summary": {
    "total_tables": 40,
    "total_columns": 575,
    "enriched_columns": 575
  }
}
```

**Table Columns Endpoint:**
```json
{
  "table_name": "users",
  "columns": [
    {
      "name": "id",
      "description": "Primary key identifier",
      "semantic_type": "identifier",
      "role": "primary_key",
      "features": {
        "purpose": "primary_key",
        "confidence": 0.95,
        "identifier_features": { ... }
      }
    }
  ]
}
```

## Implementation

### Phase 1: Backend

**File:** `pkg/handlers/ontology_enrichment_handler.go`

Remove `GetEnrichment` handler and replace with:

```go
// GetEnrichmentTables returns table-level summaries without column details
func (h *OntologyEnrichmentHandler) GetEnrichmentTables(w http.ResponseWriter, r *http.Request) {
    // ... auth/tenant setup ...

    columnsByTable, err := h.schemaRepo.GetColumnsWithFeaturesByDatasource(ctx, projectID, datasourceID)

    tables := make([]TableSummaryResponse, 0, len(columnsByTable))
    totalColumns, totalEnriched := 0, 0
    for tableName, columns := range columnsByTable {
        enrichedCount := 0
        for _, col := range columns {
            if col.GetColumnFeatures() != nil {
                enrichedCount++
            }
        }
        tables = append(tables, TableSummaryResponse{
            TableName:           tableName,
            ColumnCount:         len(columns),
            EnrichedColumnCount: enrichedCount,
            HasFeatures:         enrichedCount > 0,
        })
        totalColumns += len(columns)
        totalEnriched += enrichedCount
    }

    server.RespondWithJSON(w, http.StatusOK, EnrichmentTablesResponse{
        Tables: tables,
        Summary: EnrichmentSummary{
            TotalTables:     len(tables),
            TotalColumns:    totalColumns,
            EnrichedColumns: totalEnriched,
        },
    })
}

// GetEnrichmentTableColumns returns column details for a specific table
func (h *OntologyEnrichmentHandler) GetEnrichmentTableColumns(w http.ResponseWriter, r *http.Request) {
    tableName := r.PathValue("table")
    if tableName == "" {
        server.RespondWithError(w, http.StatusBadRequest, "table name required")
        return
    }

    // ... auth/tenant setup ...

    columns, err := h.schemaRepo.GetColumnsByTable(ctx, projectID, datasourceID, tableName)
    columnDetails := buildColumnDetails(columns)

    server.RespondWithJSON(w, http.StatusOK, TableColumnsResponse{
        TableName: tableName,
        Columns:   columnDetails,
    })
}
```

**File:** `main.go`

Replace existing route with:

```go
mux.HandleFunc("GET /api/projects/{pid}/ontology/enrichment/tables",
    authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetEnrichmentTables)))

mux.HandleFunc("GET /api/projects/{pid}/ontology/enrichment/tables/{table}/columns",
    authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetEnrichmentTableColumns)))
```

### Phase 2: Repository Method

**File:** `pkg/repositories/schema_repository.go`

Add method to get columns for a specific table:

```go
// GetColumnsByTable returns columns for a specific table
func (r *schemaRepository) GetColumnsByTable(
    ctx context.Context,
    projectID, datasourceID uuid.UUID,
    tableName string,
) ([]*models.SchemaColumn, error) {
    query := `
        SELECT c.id, c.table_id, c.name, c.data_type, c.is_nullable,
               c.ordinal_position, c.is_primary_key, c.is_foreign_key,
               c.foreign_key_table, c.foreign_key_column, c.metadata,
               c.created_at, c.updated_at
        FROM engine_schema_columns c
        JOIN engine_schema_tables t ON c.table_id = t.id
        WHERE c.project_id = $1
          AND t.datasource_id = $2
          AND t.table_name = $3
          AND c.deleted_at IS NULL
          AND t.deleted_at IS NULL
        ORDER BY c.ordinal_position`
    // ...
}
```

### Phase 3: Frontend Changes

**File:** `ui/src/services/ontologyApi.ts`

Remove `getEnrichment` and add:

```typescript
async getEnrichmentTables(projectId: string): Promise<EnrichmentTablesResponse> {
    return this.makeRequest<EnrichmentTablesResponse>(
        `/${projectId}/ontology/enrichment/tables`
    );
}

async getEnrichmentTableColumns(
    projectId: string,
    tableName: string
): Promise<TableColumnsResponse> {
    return this.makeRequest<TableColumnsResponse>(
        `/${projectId}/ontology/enrichment/tables/${encodeURIComponent(tableName)}/columns`
    );
}
```

**File:** `ui/src/pages/EnrichmentPage.tsx`

Rewrite to use lazy loading:

```typescript
const [tables, setTables] = useState<TableSummary[]>([]);
const [selectedTable, setSelectedTable] = useState<string | null>(null);
const [tableColumns, setTableColumns] = useState<Map<string, Column[]>>(new Map());
const [loadingColumns, setLoadingColumns] = useState<string | null>(null);

// Fetch table list on mount (lightweight)
useEffect(() => {
    const fetchTables = async () => {
        const response = await ontologyApi.getEnrichmentTables(pid);
        setTables(response.tables);
    };
    fetchTables();
}, [pid]);

// Lazy load columns when table selected
const handleTableSelect = async (tableName: string) => {
    setSelectedTable(tableName);

    if (tableColumns.has(tableName)) {
        return; // Already cached
    }

    setLoadingColumns(tableName);
    const response = await ontologyApi.getEnrichmentTableColumns(pid, tableName);
    setTableColumns(prev => new Map(prev).set(tableName, response.columns));
    setLoadingColumns(null);
};
```

### Phase 4: Response Types

**File:** `pkg/handlers/ontology_enrichment_handler.go`

Remove old types, add:

```go
type TableSummaryResponse struct {
    TableName           string `json:"table_name"`
    BusinessName        string `json:"business_name,omitempty"`
    Description         string `json:"description,omitempty"`
    ColumnCount         int    `json:"column_count"`
    EnrichedColumnCount int    `json:"enriched_column_count"`
    HasFeatures         bool   `json:"has_features"`
}

type EnrichmentSummary struct {
    TotalTables     int `json:"total_tables"`
    TotalColumns    int `json:"total_columns"`
    EnrichedColumns int `json:"enriched_columns"`
}

type EnrichmentTablesResponse struct {
    Tables  []TableSummaryResponse `json:"tables"`
    Summary EnrichmentSummary      `json:"summary"`
}

type TableColumnsResponse struct {
    TableName string                   `json:"table_name"`
    Columns   []ColumnEnrichmentDetail `json:"columns"`
}
```

**File:** `ui/src/types/ontology.ts`

Remove old types, add:

```typescript
export interface TableSummary {
    table_name: string;
    business_name?: string;
    description?: string;
    column_count: number;
    enriched_column_count: number;
    has_features: boolean;
}

export interface EnrichmentTablesResponse {
    tables: TableSummary[];
    summary: {
        total_tables: number;
        total_columns: number;
        enriched_columns: number;
    };
}

export interface TableColumnsResponse {
    table_name: string;
    columns: ColumnDetail[];
}
```

## Implementation Tasks

### Backend
1. [ ] Add new response types (`TableSummaryResponse`, `EnrichmentTablesResponse`, `TableColumnsResponse`)
2. [ ] Add `GetColumnsByTable` repository method
3. [ ] Replace `GetEnrichment` with `GetEnrichmentTables` handler
4. [ ] Add `GetEnrichmentTableColumns` handler
5. [ ] Update routes in `main.go` (remove old, add new)
6. [ ] Remove old `EnrichmentResponse` type and related code

### Frontend
7. [ ] Update TypeScript types (remove old, add new)
8. [ ] Replace `getEnrichment` with `getEnrichmentTables` and `getEnrichmentTableColumns`
9. [ ] Rewrite `EnrichmentPage` for lazy loading
10. [ ] Add table selection UI (dropdown or expandable list)
11. [ ] Add loading state for column fetching
12. [ ] Cache fetched columns in component state

### Testing
13. [ ] Unit test new handlers
14. [ ] Integration test new endpoints
15. [ ] Manual test lazy loading behavior

## Expected Improvements

| Metric | Before | After |
|--------|--------|-------|
| Initial page load | ~233KB | ~5-10KB |
| Time to interactive | Slow (parse large JSON) | Fast (small payload) |
| Memory usage | High (all data in state) | Low (on-demand) |
| Per-table column load | N/A | ~2-5KB |

## Notes

- The double API call in development is React StrictMode behavior and is expected
- Column data is cached client-side in component state to avoid re-fetching
