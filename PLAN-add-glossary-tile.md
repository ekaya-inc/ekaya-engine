# PLAN: Add Glossary Tile to Intelligence Section

## Context

Add a new "Glossary" tile to the Intelligence section on the ProjectDashboard. The tile allows users to view business terms (discovered and user-defined) with their technical mappings. Follows the same UX patterns as the Entities tile.

## Current Architecture

### Intelligence Section (ProjectDashboard.tsx:403-425)
- Tiles defined in `intelligenceTiles` array: Ontology, Entities, Relationships
- Each tile: `{ title, icon, path, disabled, color }`
- Rendered via `renderTile()` function (lines 457-514)
- Disabled logic uses: `isConnected`, `hasSelectedTables`, `activeAIConfig`

### Entities Tile Pattern (EntitiesPage.tsx)
- Loading state → Error state → Empty state → Data display
- Empty state links to Ontology page with "Run Ontology Extraction" message
- Fetches via `engineApi.listEntities(pid)`

### Glossary Backend (Already Exists)
- **Table:** `engine_business_glossary` (migration 025)
- **Model:** `pkg/models/glossary.go` - `BusinessGlossaryTerm` struct
- **Handler:** `pkg/handlers/glossary_handler.go`
  - `GET /api/projects/{pid}/glossary` - list terms
  - `POST /api/projects/{pid}/glossary` - create term
  - `PUT /api/projects/{pid}/glossary/{tid}` - update term
  - `DELETE /api/projects/{pid}/glossary/{tid}` - delete term
- **Service:** `pkg/services/glossary_service.go`
- **DAG Nodes:** `GlossaryDiscovery`, `GlossaryEnrichment` (run after ontology)

### Ontology Completion Detection
- `ontologyStatus?.progress.state === 'complete'` OR `ontologyStatus?.ontologyReady === true`
- Subscribe via `ontologyService.subscribe(callback)`

## Implementation Steps

### Step 1: Add TypeScript Types [x]

**Status:** COMPLETE

**Implementation:** Created `ui/src/types/glossary.ts` with three interfaces:
- `GlossaryFilter` - Filter conditions for glossary term definitions (column, operator, values)
- `BusinessGlossaryTerm` - Business term with technical mapping (id, term, definition, optional SQL details, source, timestamps)
- `GlossaryListResponse` - API response shape for listing terms (terms array, total count)

**Key decisions:**
- Source enum: 'user' | 'suggested' (note: 'discovered' removed as backend doesn't use it)
- Added `project_id` field to match backend model
- All SQL fields (sql_pattern, base_table, columns_used, filters, aggregation) are optional
- Timestamps as strings (will be parsed by UI components as needed)

**File:** `ui/src/types/glossary.ts`

### Step 2: Add API Method [x]

**Status:** COMPLETE

**Implementation:** Added `listGlossaryTerms` method to `ui/src/services/engineApi.ts`:
- Method signature: `async listGlossaryTerms(projectId: string): Promise<ApiResponse<GlossaryListResponse>>`
- Makes GET request to `/api/projects/{projectId}/glossary`
- Added `GlossaryListResponse` import to the type imports
- Added `export * from './glossary'` to `ui/src/types/index.ts` for type exports

**Files modified:**
- `ui/src/services/engineApi.ts` - Added method and import
- `ui/src/types/index.ts` - Added glossary type export

### Step 3: Create GlossaryPage Component

Create `ui/src/pages/GlossaryPage.tsx`:

**Structure (mirror EntitiesPage):**
1. **Loading state:** Spinner while fetching
2. **Error state:** Error message with retry button
3. **Empty state:**
   - If ontology not complete: "Run Ontology Extraction first" with link to `/projects/${pid}/ontology`
   - If ontology complete but no terms: "No glossary terms discovered yet"
4. **Data display:**
   - Summary card: total terms count
   - Terms list sorted alphabetically by `term` field
   - Each term shows: name, definition, source badge, expandable SQL details

**Key imports:**
- `BookOpen` icon from lucide-react
- `ontologyService` for checking completion status
- Card components from `@/components/ui/card`

**Sorting:** `terms.sort((a, b) => a.term.localeCompare(b.term))`

### Step 4: Add Route

In `ui/src/App.tsx`, add route inside the project routes (after Entities route ~line 44):

```typescript
<Route path="glossary" element={<GlossaryPage />} />
```

Import `GlossaryPage` at top.

### Step 5: Add Tile to Dashboard

In `ui/src/pages/ProjectDashboard.tsx`:

1. Import `BookOpen` icon from lucide-react (add to existing import ~line 14)

2. Add tile to `intelligenceTiles` array (after Relationships, ~line 425):

```typescript
{
  title: 'Glossary',
  icon: BookOpen,
  path: `/projects/${pid}/glossary`,
  disabled: !isConnected || !hasSelectedTables,
  color: 'cyan',
}
```

**Disabled logic:** Same as Entities - requires datasource and tables, but NOT AI config (database-derived data).

## Empty State Behavior

```typescript
// In GlossaryPage
const isOntologyComplete = ontologyStatus?.progress.state === 'complete'
  || ontologyStatus?.ontologyReady === true;

if (terms.length === 0) {
  if (!isOntologyComplete) {
    // Show: "Run Ontology Extraction first" with button to Ontology page
  } else {
    // Show: "No glossary terms discovered" - ontology ran but no terms found
  }
}
```

## UI Details

### Term Card Display
- **Term name:** Bold heading
- **Definition:** Description text below
- **Source badge:** Colored badge showing "Suggested" or "User"
- **Expandable section:** SQL pattern, base table, columns used, filters, aggregation

### Color Scheme
- Tile color: `cyan` (differentiates from Entities green)
- Source badges:
  - Suggested: amber
  - User: green

## Files to Modify/Create

| File | Action |
|------|--------|
| `ui/src/types/glossary.ts` | CREATE |
| `ui/src/services/engineApi.ts` | MODIFY - add `listGlossaryTerms` |
| `ui/src/pages/GlossaryPage.tsx` | CREATE |
| `ui/src/App.tsx` | MODIFY - add route |
| `ui/src/pages/ProjectDashboard.tsx` | MODIFY - add tile |

## Verification

1. `make check` passes (TypeScript, linting, tests)
2. Tile appears in Intelligence section
3. Tile disabled when no datasource/tables configured
4. Clicking tile navigates to Glossary page
5. Empty state shows appropriate message based on ontology status
6. Terms display alphabetically when present
7. Back button returns to dashboard
