# Plan: Add Query Results Display

## Goal

Display query execution results (up to 10 rows and 20 columns) in the QueriesView UI after executing or testing a query.

## Current State

### Backend (Already Complete)

The backend already returns full result data. No changes needed.

**Endpoints:**
- `POST /api/projects/{pid}/datasources/{did}/queries/{qid}/execute` - Execute saved query
- `POST /api/projects/{pid}/datasources/{did}/queries/test` - Test query before saving

**Response structure** (`ExecuteQueryResponse` in `pkg/handlers/queries.go:66-71`):
```go
type ExecuteQueryResponse struct {
    Columns  []string         `json:"columns"`
    Rows     []map[string]any `json:"rows"`
    RowCount int              `json:"row_count"`
}
```

**Frontend type** (`ui/src/types/query.ts:124-128`):
```typescript
export interface ExecuteQueryResponse {
  columns: string[];
  rows: Record<string, unknown>[];
  row_count: number;
}
```

### Frontend (Changes Needed)

Currently `QueriesView.tsx` shows only a toast notification with row count. Need to:
1. Store execution results in state
2. Display results in a data table component

## Library Selection: @tanstack/react-table

**Recommendation:** Use **@tanstack/react-table v8** (headless table library)

**Why this fits:**
- Headless design works perfectly with existing TailwindCSS styling
- No opinionated UI - matches existing Radix/Tailwind design system
- Lightweight (~15kb) compared to full-featured grids
- Built-in column visibility, sorting (future enhancements)
- Very popular (24k+ GitHub stars), well-maintained
- TypeScript-first with excellent type inference

**Alternatives considered:**
- `ag-grid-react` - Overkill for read-only display, larger bundle, themed UI
- `react-data-grid` - Good but more spreadsheet-focused
- Native HTML table - Requires manual horizontal scroll, column truncation handling

## Implementation Steps

### 1. Install dependency

```bash
cd ui && npm install @tanstack/react-table
```

### 2. Create QueryResultsTable component

**New file:** `ui/src/components/QueryResultsTable.tsx`

**Responsibilities:**
- Accept `columns: string[]` and `rows: Record<string, unknown>[]`
- Limit display to first 10 rows (configurable via prop)
- Limit display to first 20 columns with horizontal scroll
- Handle column truncation for long values (tooltip on hover)
- Show row count summary: "Showing 10 of 47 rows"
- Style with TailwindCSS to match existing dark theme

**Key features:**
- Column headers from `columns` array
- Cell rendering with type-aware formatting (dates, numbers, nulls)
- Horizontal scroll container for wide tables
- Sticky first column (optional enhancement)
- Copy cell value on click

### 3. Update QueriesView.tsx

**Changes to `ui/src/components/QueriesView.tsx`:**

1. Add state for results:
```typescript
const [queryResults, setQueryResults] = useState<ExecuteQueryResponse | null>(null);
```

2. Update `handleExecuteQuery` to store results:
```typescript
if (response.success && response.data) {
  setQueryResults(response.data);  // Add this line
  toast({ ... });
}
```

3. Update `handleTestQuery` similarly to store test results

4. Add QueryResultsTable below the Execute Query button in the view mode:
```tsx
{queryResults && (
  <QueryResultsTable
    columns={queryResults.columns}
    rows={queryResults.rows}
    totalRowCount={queryResults.row_count}
    maxRows={10}
    maxColumns={20}
  />
)}
```

5. Clear results when selecting a different query:
```typescript
onClick={() => {
  setSelectedQuery(query);
  setQueryResults(null);  // Clear previous results
  ...
}}
```

### 4. Component Props Interface

```typescript
interface QueryResultsTableProps {
  columns: string[];
  rows: Record<string, unknown>[];
  totalRowCount: number;
  maxRows?: number;      // Default: 10
  maxColumns?: number;   // Default: 20
}
```

### 5. UI/UX Details

**Results section layout:**
- Section header: "Query Results"
- Summary line: "Showing 10 of 47 rows • 8 columns"
- Horizontal scrollable table container
- Warning if results truncated: "Results limited to 10 rows"

**Table styling (TailwindCSS):**
- Dark theme matching existing cards (`bg-surface-secondary`)
- Header row: `bg-surface-tertiary text-text-secondary font-medium`
- Data rows: `text-text-primary text-sm`
- Alternating row colors for readability
- Cell padding: `px-3 py-2`
- Border: `border-border-light`

**Cell value formatting:**
- `null` → `<span className="text-text-tertiary italic">null</span>`
- Booleans → `true` / `false` with color coding
- Dates/timestamps → localized format
- Long strings → truncate with ellipsis, full value in tooltip
- Numbers → right-aligned

### 6. Testing

**New test file:** `ui/src/components/__tests__/QueryResultsTable.test.tsx`

Test cases:
- Renders column headers correctly
- Displays correct number of rows (respects maxRows)
- Shows row count summary
- Handles empty results
- Handles null values in cells
- Truncates to maxColumns
- Shows truncation warning when applicable

## File Changes Summary

| File | Change Type |
|------|-------------|
| `ui/package.json` | Add @tanstack/react-table dependency |
| `ui/src/components/QueryResultsTable.tsx` | New component |
| `ui/src/components/QueriesView.tsx` | Add results state and display |
| `ui/src/components/__tests__/QueryResultsTable.test.tsx` | New tests |

## No Backend Changes Required

The existing API endpoints already return full result data. The `limit` parameter on execute/test requests already controls how many rows the database returns - the frontend display limit (10 rows) is purely a UI concern for preview purposes.
