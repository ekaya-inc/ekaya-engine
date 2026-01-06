# PLAN: Finalize Business Glossary Feature

## Overview

Transform the business glossary from a fragmented "bits" storage into a first-class feature where each term has a **definitive SQL definition** that MCP clients can use to compose queries. The glossary answers the question: "How do you define X?" with executable SQL.

## Current State

- Table: `engine_business_glossary` with fragmented fields (`sql_pattern`, `base_table`, `columns_used`, `filters`, `aggregation`)
- Single MCP tool `get_glossary` returns everything (terms + definitions + all bits)
- Source values: `user`, `suggested`, `discovered` (inconsistent with other ontology tables)
- No SQL validation/testing capability
- UI displays bits separately without ability to edit SQL

## Target State

- Clean schema with `defining_sql` as the primary artifact
- Two MCP tools: `list_glossary` (discovery) and `get_glossary_sql` (targeted retrieval)
- Source values: `inferred`, `manual`, `client` (consistent with relationships)
- SQL validation via EXPLAIN + optional execution for output column capture
- UI with SQL editor, test functionality, and output column display
- Aliases for terms (synonyms like "Active Users" = "MAU")

---

## Phase 1: Database Migration

### 1.1 Drop and Recreate Glossary Table ✅

**Status:** Complete - Migration 031 created and tested successfully

**Implementation Notes:**
- Migration file: `migrations/031_glossary_defining_sql.{up,down}.sql`
- Test file: `migrations/031_glossary_defining_sql_test.go`
- Deleted old test file: `migrations/025_business_glossary_test.go` (obsolete)
- Schema changes:
  - Replaced fragmented fields (`sql_pattern`, `columns_used`, `filters`, `aggregation`) with single `defining_sql` TEXT field
  - Changed source values from `user`/`suggested`/`discovered` to `inferred`/`manual`/`client` for consistency with relationships table
  - Added `output_columns JSONB` field for storing column metadata captured during SQL testing
  - Added `base_table TEXT` field for quick reference (derived from SQL)
  - Added `created_by` and `updated_by` UUID fields for user tracking
  - Constraint: `source` must be IN ('inferred', 'manual', 'client')
  - Unique constraint on (project_id, term)
- RLS policy implemented for project isolation
- Indexes on project_id, source, and base_table
- Updated_at trigger configured
- Migration tested successfully: up and down migrations work correctly, RLS enforced, constraints validated

```sql
DROP TABLE IF EXISTS engine_business_glossary CASCADE;

CREATE TABLE engine_business_glossary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,

    -- Core fields
    term TEXT NOT NULL,
    definition TEXT NOT NULL,
    defining_sql TEXT NOT NULL,  -- The definitive SQL that defines this term

    -- Metadata
    base_table TEXT,  -- Primary table (derived from SQL but stored for quick reference)

    -- Output schema (populated when SQL is tested)
    output_columns JSONB,  -- [{name, type, description}] - same pattern as approved queries

    -- Source tracking
    source TEXT NOT NULL DEFAULT 'inferred',  -- 'inferred', 'manual', 'client'

    -- Audit
    created_by UUID,  -- User who created (null for inferred)
    updated_by UUID,  -- User who last updated
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT engine_business_glossary_project_term_unique UNIQUE (project_id, term),
    CONSTRAINT engine_business_glossary_source_check CHECK (source IN ('inferred', 'manual', 'client'))
);

-- Indexes
CREATE INDEX idx_business_glossary_project ON engine_business_glossary(project_id);
CREATE INDEX idx_business_glossary_source ON engine_business_glossary(source);
CREATE INDEX idx_business_glossary_base_table ON engine_business_glossary(base_table);

-- RLS Policy
ALTER TABLE engine_business_glossary ENABLE ROW LEVEL SECURITY;
CREATE POLICY business_glossary_access ON engine_business_glossary
    USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    )
    WITH CHECK (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Updated_at trigger
CREATE TRIGGER update_business_glossary_updated_at
    BEFORE UPDATE ON engine_business_glossary
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

### 1.2 Create Glossary Aliases Table ✅

**Status:** Complete - Included in Migration 031

**Implementation Notes:**
- Created as part of migration 031 (lines 73-100 in `migrations/031_glossary_defining_sql.up.sql`)
- Table structure:
  - `id` UUID PRIMARY KEY with default gen_random_uuid()
  - `glossary_id` UUID with FK to engine_business_glossary(id) ON DELETE CASCADE
  - `alias` TEXT NOT NULL
  - `created_at` TIMESTAMPTZ with default now()
  - Unique constraint on (glossary_id, alias)
- Indexes:
  - `idx_glossary_aliases_glossary` on glossary_id
  - `idx_glossary_aliases_alias` on alias
- RLS policy `glossary_aliases_access` that queries parent glossary table for project isolation
- Comprehensive test coverage in `migrations/031_glossary_defining_sql_test.go`:
  - Table and column existence
  - Column types validation
  - RLS enabled
  - Indexes presence
  - Unique constraint
  - Foreign key relationship
  - RLS policy existence

```sql
CREATE TABLE engine_glossary_aliases (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    glossary_id UUID NOT NULL REFERENCES engine_business_glossary(id) ON DELETE CASCADE,
    alias TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT engine_glossary_aliases_unique UNIQUE (glossary_id, alias)
);

CREATE INDEX idx_glossary_aliases_glossary ON engine_glossary_aliases(glossary_id);
CREATE INDEX idx_glossary_aliases_alias ON engine_glossary_aliases(alias);

-- RLS (inherits from parent via FK, but add explicit policy for direct queries)
ALTER TABLE engine_glossary_aliases ENABLE ROW LEVEL SECURITY;
CREATE POLICY glossary_aliases_access ON engine_glossary_aliases
    USING (
        glossary_id IN (
            SELECT id FROM engine_business_glossary
            WHERE current_setting('app.current_project_id', true) IS NULL
               OR project_id = current_setting('app.current_project_id', true)::uuid
        )
    );
```

### 1.3 Migration File ✅

**Status:** Complete - Migration 031 created

**Implementation Notes:**
- Created `migrations/031_glossary_defining_sql.up.sql` and `migrations/031_glossary_defining_sql.down.sql`
- Includes both glossary table recreation and aliases table creation in a single migration
- Comprehensive test file `migrations/031_glossary_defining_sql_test.go` validates all aspects of the migration

---

## Phase 2: Backend Model & Repository Updates

### 2.1 Update Models (`pkg/models/glossary.go`) ✅

**Status:** Complete - Model updated with new schema fields

**Implementation Notes:**
- Updated BusinessGlossaryTerm model with new fields:
  - DefiningSQL (string) - The definitive SQL definition
  - OutputColumns ([]OutputColumn) - Columns returned by the SQL (reuses existing OutputColumn struct from query.go)
  - Aliases ([]string) - Alternative names for the term
  - UpdatedBy (*uuid.UUID) - User who last updated
- Removed old fragmented fields: SQLPattern, ColumnsUsed, Filters, Aggregation
- Removed Filter struct (no longer needed)
- Added GlossarySource constants: GlossarySourceInferred, GlossarySourceManual, GlossarySourceClient
- OutputColumn struct already exists in pkg/models/query.go (reused for consistency - no need to define again)
- Updated model comment to reflect SQL definition focus

**Final Model Structure:**
```go
// Source values for glossary terms
const (
    GlossarySourceInferred = "inferred" // LLM discovered during ontology extraction
    GlossarySourceManual   = "manual"   // Human added or edited via UI
    GlossarySourceClient   = "client"   // MCP client added dynamically
)

// BusinessGlossaryTerm represents a business term with its SQL definition.
type BusinessGlossaryTerm struct {
    ID            uuid.UUID      `json:"id"`
    ProjectID     uuid.UUID      `json:"project_id"`
    Term          string         `json:"term"`
    Definition    string         `json:"definition"`
    DefiningSQL   string         `json:"defining_sql"`
    BaseTable     string         `json:"base_table,omitempty"`
    OutputColumns []OutputColumn `json:"output_columns,omitempty"`
    Aliases       []string       `json:"aliases,omitempty"`
    Source        string         `json:"source"`
    CreatedBy     *uuid.UUID     `json:"created_by,omitempty"`
    UpdatedBy     *uuid.UUID     `json:"updated_by,omitempty"`
    CreatedAt     time.Time      `json:"created_at"`
    UpdatedAt     time.Time      `json:"updated_at"`
}
```

**Important for next tasks:**
- The model now aligns perfectly with the migration 031 schema
- OutputColumn is imported from the same package (already defined in query.go)
- All JSON tags match the database column names (snake_case in DB, automatically mapped by encoding/json)
- Next step: Update repository layer to work with these new fields

### 2.2 Update Repository (`pkg/repositories/glossary_repository.go`) ✅

**Status:** Complete - Repository fully aligned with new schema and alias support

**Implementation Details:**

**Interface Changes:**
- Added `GetByAlias(ctx, projectID, alias)` - Lookup term by alias name
- Added `CreateAlias(ctx, glossaryID, alias)` - Add alias to existing term
- Added `DeleteAlias(ctx, glossaryID, alias)` - Remove alias (returns ErrNotFound if not found)

**Create Method:**
- Uses new fields: `defining_sql`, `output_columns`, `updated_by`
- Removed old fields: `sql_pattern`, `columns_used`, `filters`, `aggregation`
- Automatically creates aliases if provided in `term.Aliases` slice
- Rolls back entire transaction if any alias creation fails

**Update Method:**
- Uses new schema fields throughout
- Replaces ALL aliases on update: DELETE all existing, then CREATE new ones
- This "replace-all" approach prevents orphaned aliases and simplifies logic
- Sets `updated_by` field

**Get Methods (GetByProject, GetByTerm, GetByID):**
- All use `LEFT JOIN engine_glossary_aliases` to include aliases
- Use `jsonb_agg(a.alias) FILTER (WHERE a.alias IS NOT NULL)` to aggregate aliases
- COALESCE to `'[]'::jsonb` for terms with no aliases
- GROUP BY all glossary fields (required by PostgreSQL for aggregates)
- Properly handle null JSONB in scanGlossaryTerm

**GetByAlias Method:**
- INNER JOIN to find glossary by alias match
- Second LEFT JOIN to fetch ALL aliases for the found term (not just searched alias)
- Returns `nil` if alias not found (not an error, consistent with GetByTerm behavior)

**Alias Management Methods:**
- CreateAlias: Direct INSERT into `engine_glossary_aliases`
- DeleteAlias: Direct DELETE with RowsAffected check → ErrNotFound if zero rows

**Helper Functions:**
- scanGlossaryTerm: Scans new fields including JSONB arrays for `output_columns` and `aliases`
- Handles null/empty JSONB gracefully (checks for "null" and "[]" string literals)
- jsonbValue: Now handles `[]models.OutputColumn`, returns nil for empty slices

**Test Coverage (23 tests, all passing):**
- Create: full fields, minimal fields, duplicate term constraint
- Update: success with alias replacement, not found error
- Delete: success, not found error
- GetByProject: returns all terms with aliases, empty result
- GetByTerm: success with aliases, not found
- GetByAlias: success (finds term by alias), not found (returns nil)
- GetByID: success, not found
- CreateAlias: adds new alias successfully
- DeleteAlias: removes alias successfully, not found returns ErrNotFound
- OutputColumns: storage and retrieval of multiple columns with types
- RLS: enforcement for ALL methods including alias operations

**Files Modified:**
- `pkg/repositories/glossary_repository.go` - Repository implementation (424 lines)
- `pkg/repositories/glossary_repository_test.go` - Comprehensive test suite (770 lines)

**Key Design Decisions:**
1. **Automatic alias management in Create/Update** - Simplifies caller code, ensures consistency
2. **Replace-all strategy for Update** - DELETE + INSERT instead of diff/merge, simpler and safer
3. **LEFT JOIN for aliases** - Handles terms without aliases gracefully, returns empty array
4. **GetByAlias returns nil when not found** - Consistent with GetByTerm (not an error to search for nonexistent term)
5. **RLS on alias table** - Uses subquery to parent glossary table for project isolation

**Important for Next Task (2.3 - Service Layer):**
- Repository layer is complete and tested
- Service layer can now add SQL validation logic (EXPLAIN, LIMIT 1 execution)
- Alias lookups work via GetByAlias OR GetByTerm (repository supports both)
- All CRUD operations properly handle aliases automatically

### 2.3 Update Service (`pkg/services/glossary_service.go`) ✅

**Status:** Complete - Service layer fully updated with SQL validation and alias support

**Implementation Details:**

**New Dependencies:**
- Added `datasourceSvc DatasourceService` - For accessing project datasources
- Added `adapterFactory datasource.DatasourceAdapterFactory` - For creating query executors
- Both injected via constructor: `NewGlossaryService(..., datasourceSvc, adapterFactory, ...)`
- Updated `main.go` wiring to pass these dependencies

**New Methods:**
- `TestSQL(ctx, projectID, sql)` - Validates SQL and captures output columns:
  1. Gets datasource for project (uses first datasource)
  2. Creates query executor adapter
  3. Executes SQL with LIMIT 1 to validate syntax and capture schema
  4. Returns `SQLTestResult{Valid, Error, OutputColumns, SampleRow}`
  - Returns structured error (not Go error) for validation failures
  - Uses `auth.RequireUserIDFromContext(ctx)` for adapter authentication

- `GetTermByName(ctx, projectID, termName)` - Lookup by term or alias:
  1. Tries exact term name match via `glossaryRepo.GetByTerm`
  2. Falls back to alias lookup via `glossaryRepo.GetByAlias`
  3. Returns nil (not error) if not found

- `CreateAlias(ctx, termID, alias)` - Adds alias to existing term
- `DeleteAlias(ctx, termID, alias)` - Removes alias from term

**Updated Methods:**
- `CreateTerm`:
  - Requires `defining_sql` field (validation added)
  - Calls `TestSQL` to validate before creation
  - Returns error if SQL invalid
  - Sets `output_columns` from test result
  - Sets default source to `models.GlossarySourceManual`

- `UpdateTerm`:
  - Requires `defining_sql` field
  - Compares with existing term to detect SQL changes
  - Re-validates via `TestSQL` only if SQL changed
  - Updates `output_columns` if SQL changed
  - Returns error if new SQL invalid

**Legacy Handling:**
- `parseSuggestTermsResponse` - Maps old `SQLPattern` to `DefiningSQL` (Phase 3 will update LLM prompts)
- `EnrichGlossaryTerms` - Still checks for empty `DefiningSQL` on inferred terms (will be updated in Phase 3)
- Both marked with comments noting Phase 3 updates needed

**Files Modified:**
- `pkg/services/glossary_service.go` - Service implementation (~850 lines)
- `main.go` - Updated service wiring to inject datasource dependencies
- `pkg/handlers/glossary_handler.go` - Request structs updated to use `defining_sql` and `aliases`
- `pkg/mcp/tools/glossary.go` - Maps `DefiningSQL` to `SQLPattern` for backwards compatibility (Phase 4 will split into two tools)

**Test Coverage:**
- Unit tests updated in `pkg/services/glossary_service_test.go`
- Integration tests updated in `pkg/handlers/glossary_integration_test.go`
- DAG adapter tests updated in `pkg/services/dag_adapters_test.go`

**Key Design Decisions:**
1. **Validation on write** - SQL must be valid before storing (fail-fast principle)
2. **Output columns captured automatically** - No manual specification needed
3. **Conditional re-validation** - Only test SQL if it changed (performance)
4. **Structured error handling** - TestSQL returns result object, not Go error (allows UI to show validation errors)
5. **Datasource adapter pattern** - Reuses existing query execution infrastructure

**Important for Next Task (Phase 3 - LLM Prompts):**
- Service layer is complete and tested
- Legacy LLM response parsing still works (SQLPattern → DefiningSQL mapping)
- Next phase should update LLM prompts to generate `defining_sql` directly
- `SuggestTerms` and `EnrichGlossaryTerms` methods ready for prompt updates
- SQL validation will automatically apply to LLM-generated terms

---

## Phase 3: LLM Prompt & Extraction Updates

### 3.1 Update Discovery Prompt (`glossary_service.go:buildSuggestTermsPrompt`) ✅

**Status:** Complete - Prompt and response parsing updated to generate executable SQL

**Implementation Details:**

**Prompt Changes (pkg/services/glossary_service.go:447-464):**
- Updated system instructions to request complete, executable SQL SELECT statements
- Emphasized that `defining_sql` must be ready to execute without modification
- Removed references to `sql_pattern`, `columns_used`, `filters`, `aggregation` (old bits)
- Added guidance on including all necessary clauses (FROM, JOIN, WHERE, GROUP BY, ORDER BY)
- Specified that column aliases should represent the business metric
- Added `aliases` field for alternative names (e.g., "MAU" for "Monthly Active Users")

**Example Format (pkg/services/glossary_service.go:543-552):**
```json
[
  {
    "term": "Revenue",
    "definition": "Total earned amount from completed transactions after fees",
    "defining_sql": "SELECT SUM(earned_amount) AS revenue\nFROM billing_transactions\nWHERE transaction_state = 'completed'",
    "base_table": "billing_transactions",
    "aliases": ["Total Revenue", "Gross Revenue"]
  }
]
```

**Response Parsing (pkg/services/glossary_service.go:563-569):**
- Updated `suggestedTermResponse` struct:
  - Removed: `SQLPattern`, `ColumnsUsed`, `Filters` ([]suggestedFilter), `Aggregation`
  - Added: `DefiningSQL` (string), `Aliases` ([]string)
  - Kept: `Term`, `Definition`, `BaseTable`
- Removed `suggestedFilter` struct and its custom UnmarshalJSON (no longer needed)
- Updated `parseSuggestTermsResponse` to map `DefiningSQL` directly (no legacy conversion)
- Aliases now automatically stored via repository layer

**Test Updates (pkg/services/glossary_service_test.go:613-627):**
- Updated mock LLM responses to use new format with `defining_sql` and `aliases`
- Removed old format fields (`sql_pattern`, `columns_used`, `filters`, `aggregation`)
- Added realistic SQL examples with proper SELECT statements

**Files Modified:**
- `pkg/services/glossary_service.go` - Prompt builder and response parser (~850 lines)
- `pkg/services/glossary_service_test.go` - Test mock responses (770+ lines)

**Important for Next Task (3.2):**
- Discovery prompt now generates complete SQL definitions
- SQL validation via `TestSQL` will catch any invalid LLM-generated SQL
- Enrichment node (task 3.2) may need similar updates or could be deprecated
- Consider consolidating discovery+enrichment into single phase (as noted in plan)

### 3.2 Update Enrichment System Message ✅

**Status:** Complete - Enrichment system message and prompt updated to generate complete SQL with aliases

**Implementation Details:**

**System Message Changes (pkg/services/glossary_service.go:794-810):**
- Updated `enrichTermSystemMessage()` to emphasize complete, executable SQL definitions
- Removed references to "SQL patterns" and fragmented fields (sql_pattern, columns_used, filters, aggregation)
- Added IMPORTANT section with explicit requirements:
  - SQL must be a complete SELECT statement that can be executed as-is
  - Must include all necessary FROM, JOIN, WHERE, GROUP BY, ORDER BY clauses
  - Must be a definition/calculation of the metric, not just a fragment
  - Must include column aliases that represent the metric
  - Must return meaningful column names for business users

**Prompt Response Format (pkg/services/glossary_service.go:892-905):**
- Updated example from old format: `{sql_pattern, columns_used, filters, aggregation}`
- New format: `{defining_sql, base_table, aliases}`
- Example now shows complete SELECT statement: `"SELECT SUM(amount) AS total_revenue\nFROM transactions\nWHERE status = 'completed'"`

**Response Parsing (pkg/services/glossary_service.go:907-911):**
- Updated `termEnrichment` struct:
  - Removed: `SQLPattern string`, `ColumnsUsed []string`, `Aggregation string`
  - Added: `DefiningSQL string`, `Aliases []string`
  - Kept: `BaseTable string`
- Updated enrichment application (pkg/services/glossary_service.go:767-770):
  - Sets `term.DefiningSQL = enrichment.DefiningSQL`
  - Sets `term.BaseTable = enrichment.BaseTable`
  - Sets `term.Aliases = enrichment.Aliases`

**Test Updates (pkg/services/glossary_service_test.go:1018-1063):**
- Updated mock LLM response to new format with complete SQL and aliases:
  ```json
  {
    "defining_sql": "SELECT SUM(amount) AS total_revenue\nFROM transactions\nWHERE status = 'completed'",
    "base_table": "transactions",
    "aliases": ["Total Revenue", "Gross Revenue"]
  }
  ```
- Fixed test setup: Creates truly unenriched term with empty DefiningSQL (bypasses creation validation)
- Updated assertions to verify DefiningSQL, BaseTable, and Aliases are all set correctly
- All enrichment tests pass successfully

**Files Modified:**
- `pkg/services/glossary_service.go` - System message, prompt builder, response parsing (~850 lines)
- `pkg/services/glossary_service_test.go` - Mock LLM responses and test assertions

**Key Changes:**
1. **Alignment with discovery** - Enrichment now uses identical format as discovery (task 3.1)
2. **Complete SQL requirement** - LLM must generate executable SELECT statements, not fragments or patterns
3. **Aliases support** - Enrichment captures alternative names for terms (e.g., "MAU" for "Monthly Active Users")
4. **Simplified structure** - Removed nested filter arrays and aggregation fields, just 3 core fields

**Important Context for Next Session:**
- Both discovery (task 3.1) and enrichment (task 3.2) now generate the same format
- Both produce complete, executable SQL with base_table and aliases
- The two phases could potentially be consolidated since they now serve the same purpose
- Post-discovery validation (task 3.4) will automatically apply to both phases via service layer SQL validation
- Any LLM-generated SQL that is invalid will be caught by `TestSQL()` in the service layer

### 3.3 Update Enrichment Node (or consolidate phases) ✅ COMMITTED

**Status:** Complete - Consolidated into single discovery phase

**Decision:** Removed the GlossaryEnrichment phase entirely from the DAG workflow.

**Reasoning:**
- After tasks 3.1 and 3.2, both discovery and enrichment generated identical output (complete executable SQL with `defining_sql`, `base_table`, and `aliases`)
- The enrichment service method already had a comment acknowledging it was a no-op: "DefiningSQL is now required, so this is a no-op for new schema"
- Discovery now generates complete SQL definitions, leaving nothing for enrichment to do
- The enrichment phase filtered for `term.DefiningSQL == ""`, which would never be true after discovery completes
- Keeping both phases added unnecessary DAG complexity and execution time with no benefit

**Implementation:**
1. Removed `DAGNodeGlossaryEnrichment` constant from `pkg/models/ontology_dag.go`
2. Removed enrichment node from DAG execution order (DAG now has 8 steps instead of 9)
3. Deleted node files:
   - `pkg/services/dag/glossary_enrichment_node.go`
   - `pkg/services/dag/glossary_enrichment_node_test.go`
4. Removed enrichment adapter from `pkg/services/dag_adapters.go`
5. Removed enrichment wiring from:
   - `pkg/services/ontology_dag_service.go` (removed field, setter, and switch case)
   - `main.go` (removed adapter registration)
6. Updated UI types in `ui/src/types/ontology.ts`:
   - Removed `GlossaryEnrichment` from `DAGNodeName` union type
   - Removed from `DAGNodeDescriptions` object
   - Updated description for `GlossaryDiscovery` to reflect that it now handles both discovery and definition
7. Updated tests:
   - `pkg/services/dag_adapters_test.go` (removed enrichment adapter tests)
   - `pkg/services/ontology_dag_service_test.go` (removed enrichment node tests, updated expected node order)

**Files Modified:**
- `pkg/models/ontology_dag.go` - Removed constant and order entry
- `pkg/services/dag_adapters.go` - Removed adapter implementation
- `pkg/services/ontology_dag_service.go` - Removed wiring and switch case
- `main.go` - Removed adapter registration
- `ui/src/types/ontology.ts` - Removed from UI types
- `pkg/services/dag_adapters_test.go` - Removed tests
- `pkg/services/ontology_dag_service_test.go` - Updated tests

**Files Deleted:**
- `pkg/services/dag/glossary_enrichment_node.go`
- `pkg/services/dag/glossary_enrichment_node_test.go`

**Note:** The `EnrichGlossaryTerms` service method remains in `pkg/services/glossary_service.go` but is no longer called by the DAG. It's harmless as a no-op and might be useful for backfilling old data if needed.

### 3.4 Post-Discovery Validation ✅

**Status:** Complete - SQL validation and output column capture integrated into discovery workflow

**Implementation Details:**

The `DiscoverGlossaryTerms` method (pkg/services/glossary_service.go:644-716) now validates each LLM-generated term before persisting:

1. **Pre-validation checks:**
   - Skips terms with no DefiningSQL (logs warning)
   - Skips duplicate terms (checks for existing term with same name)

2. **SQL validation (via TestSQL service method):**
   - Executes SQL with LIMIT 1 against project datasource
   - Captures syntax errors and validation failures
   - Returns structured result: `{Valid bool, Error string, OutputColumns []OutputColumn, SampleRow map}`
   - On failure: Logs warning with SQL text and error, skips term

3. **Output column capture:**
   - On successful validation, extracts output columns from query result
   - Sets `term.OutputColumns` from `testResult.OutputColumns`
   - Stores validated metadata alongside SQL definition

4. **Source assignment:**
   - All discovered terms get `source = models.GlossarySourceInferred`
   - Changed from "discovered" (old value) to "inferred" (consistent with relationships table)

**Test Coverage:**

Added comprehensive test in `pkg/services/glossary_service_test.go:847-908`:
- `TestGlossaryService_SuggestTerms_InvalidSQL` - Verifies that terms with invalid SQL are skipped
- Mock adapter factory returns validation error for SQL containing "INVALID" keyword
- Test creates scenario with 1 valid term + 1 invalid term
- Asserts only valid term is persisted, invalid term is silently skipped (logged at WARN level)
- Verifies output columns are captured for valid term

**Files Modified:**
- `pkg/services/glossary_service.go` - Added validation logic to discovery workflow (lines 647-692)
- `pkg/services/glossary_service_test.go` - Added test for invalid SQL handling
- `pkg/mcp/tools/glossary_test.go` - Updated test assertions for new field names
- `pkg/mcp/tools/glossary.go` - Fixed formatting (whitespace-only change)

**Key Design Decisions:**

1. **Fail gracefully** - Invalid SQL doesn't abort entire discovery, just skips that term
2. **Log warnings, not errors** - Invalid LLM output is expected occasionally, not exceptional
3. **No retry logic** - Discovery is one-shot, no auto-retry with error context (could be future enhancement)
4. **Source consistency** - Uses `GlossarySourceInferred` constant for alignment with relationships table

**Important Context for Next Session:**

- All discovered terms now have validated SQL and captured output columns before persistence
- The validation happens at write-time in `DiscoverGlossaryTerms`, not at read-time
- Invalid SQL from LLM silently skips with warning log (check `engine_llm_conversations` for failures)
- Output columns match the schema from `LIMIT 1` execution (names + types, no sample data stored)
- Next phase (Phase 4) can assume all persisted terms have valid `defining_sql` and `output_columns`

---

## Phase 4: MCP Tools

### 4.1 Refactor to Two Tools (`pkg/mcp/tools/glossary.go`) ✅ COMPLETED

**Status:** Complete - Two-tool pattern implemented, tested, and committed

**Implementation Notes:**
- Replaced single `get_glossary` tool with two specialized tools:
  1. `list_glossary` - Lightweight discovery tool returning only term, definition, and aliases
  2. `get_glossary_sql` - Full SQL definition tool with defining_sql, output_columns, base_table, and aliases
- Both tools use `GlossaryService.GetTermByName()` which supports lookup by term name OR alias
- `get_glossary_sql` handles not found gracefully with structured error response: `{"error": "Term not found", "term": "<searched-name>"}`
- Updated `ontologyToolNames` map in `pkg/mcp/tools/developer.go` to include both new tools
- Removed deprecated response types (`glossaryTermResponse`, `filterResponse`)
- Created new response types:
  - `listGlossaryResponse` - For lightweight discovery (term, definition, aliases)
  - `getGlossarySQLResponse` - For full SQL definition (all fields including defining_sql and output_columns)
- Comprehensive test coverage:
  - Unit tests for response converters (`toListGlossaryResponse`, `toGetGlossarySQLResponse`)
  - Integration tests for both tools
  - Tool registration tests
  - Filter map tests
- All tests pass, code formatted, linting clean

**Files Modified:**
- `pkg/mcp/tools/glossary.go` - Replaced single tool with two-tool pattern (237 lines)
- `pkg/mcp/tools/glossary_test.go` - Updated tests for new tools (409 lines)
- `pkg/mcp/tools/developer.go` - Updated ontologyToolNames map

**Context for Next Session:**
- MCP tool refactoring is complete - no more backwards compatibility mapping needed
- The two-tool pattern (list + get) aligns with REST best practices for resource discovery vs retrieval
- Service layer already supports name OR alias lookup via `GetTermByName()` - both tools leverage this
- Next task (4.2) is design-only documentation for future client updates - no code changes required

**Tool 1: `list_glossary`**
```go
// Returns terms + definitions for discovery (lightweight)
{
  "terms": [
    {
      "term": "Active Users",
      "definition": "Users who have engaged with the platform within the last 30 days",
      "aliases": ["MAU", "Monthly Active Users"]
    }
  ],
  "count": 5
}
```

**Tool 2: `get_glossary_sql`**
```go
// Input: { "term": "Active Users" }
// Also accepts alias: { "term": "MAU" }
// Returns full entry for query composition
{
  "term": "Active Users",
  "definition": "Users who have engaged with the platform within the last 30 days",
  "defining_sql": "SELECT COUNT(DISTINCT user_id) AS active_users\nFROM users\nWHERE deleted_at IS NULL\n  AND updated_at >= CURRENT_DATE - INTERVAL '30 days'",
  "base_table": "users",
  "output_columns": [
    {"name": "active_users", "type": "integer"}
  ],
  "aliases": ["MAU", "Monthly Active Users"]
}

// If not found:
{
  "error": "Term not found",
  "term": "Unknown Term"
}
```

### 4.2 Design for Future Client Updates ✅

**Status:** Complete - Design document created

**Implementation Notes:**
- Created comprehensive design document: `DESIGN-glossary-client-updates.md`
- Documents three MCP tools: `create_glossary_term`, `update_glossary_term`, `delete_glossary_term`
- Existing repository and service interfaces already support all required operations
- Service layer validates SQL via `TestSQL` before persistence (no changes needed)
- Repository layer supports source parameter through `models.BusinessGlossaryTerm` struct
- Database schema already includes source constraint: `CHECK (source IN ('inferred', 'manual', 'client'))`

**Design includes:**
- Complete MCP tool specifications with input/output schemas
- Implementation pseudocode for each tool handler
- Error handling patterns for validation failures and conflicts
- Tool registration and filtering logic (developer tools toggle)
- Authentication/authorization considerations
- Testing strategy (unit, integration, manual)
- Migration path for future implementation
- Open questions and trade-offs

**Key decisions documented:**
1. Source changes to 'client' when client updates any term (clear attribution)
2. Clients can delete any term regardless of source (no restrictions)
3. Write tools gated by developer tools toggle (read-only tools always available)
4. Client-created terms have `created_by=NULL, updated_by=NULL` (no user association)
5. Structured error responses (not Go errors) for user-facing failures

**Next implementer should:**
- Read `DESIGN-glossary-client-updates.md` for full context
- Implement three tool handlers in `pkg/mcp/tools/glossary.go`
- Update tool filter map in `pkg/mcp/tools/developer.go`
- Add comprehensive unit and integration tests
- Test with Claude Desktop as MCP client

---

## Phase 5: UI Updates

### 5.1 Update Types (`ui/src/types/glossary.ts`) ✅ COMMITTED

**Status:** Complete - UI types updated and GlossaryPage aligned with new schema

**Implementation Notes:**

**Files Modified:**
1. `ui/src/types/glossary.ts` (complete rewrite with new schema):
   - Renamed `BusinessGlossaryTerm` → `GlossaryTerm` (matches backend `models.BusinessGlossaryTerm`)
   - Removed old fragmented fields: `sql_pattern`, `columns_used`, `filters` (object array), `aggregation`
   - Added new schema fields:
     - `defining_sql: string` - The complete executable SQL definition
     - `output_columns?: OutputColumn[]` - Columns returned by the SQL (imported from `ui/src/types/query.ts`)
     - `aliases?: string[]` - Alternative names for the term
     - `created_by?: string` - UUID of user who created (null for inferred terms)
     - `updated_by?: string` - UUID of user who last updated
   - Changed source type from `'user' | 'suggested'` to `'inferred' | 'manual' | 'client'` (aligned with backend constants)
   - Added new interfaces for future API operations (not yet used, ready for task 5.4):
     - `TestSQLResult` - Result from POST /api/projects/{pid}/glossary/test-sql
     - `CreateGlossaryTermRequest` - Body for POST /api/projects/{pid}/glossary
     - `UpdateGlossaryTermRequest` - Body for PUT /api/projects/{pid}/glossary/{id}
     - `TestSQLRequest` - Body for POST /api/projects/{pid}/glossary/test-sql

2. `ui/src/pages/GlossaryPage.tsx` (display logic updated):
   - Changed import from `BusinessGlossaryTerm` to `GlossaryTerm`
   - Updated source badge rendering:
     - Old: "Suggested" (yellow) or "User" (green)
     - New: "Inferred" (amber), "Manual" (green), or "Client" (blue)
   - Replaced "SQL Pattern" section with "Defining SQL" (same code block style)
   - Replaced "Columns Used" with "Output Columns" showing name, type, and description per column
   - Removed "Filters" and "Aggregation" sections entirely
   - Added "Aliases" section with purple tag/chip display (matches pattern from other pages)
   - Updated `hasSqlDetails` logic to check: `defining_sql || base_table || output_columns?.length > 0 || aliases?.length > 0`

**Verification:**
- TypeScript type checking passes (no type errors)
- UI build completes successfully (`make dev-ui`)
- GlossaryPage renders correctly with new field structure

**Important Context for Next Session (Task 5.2):**

**What's Complete:**
- Frontend types are now 100% aligned with backend schema (matches migration 031 and Go models exactly)
- GlossaryPage successfully displays all new fields: defining_sql, output_columns, aliases, updated source badges
- Existing read-only display functionality works correctly

**What's Missing (Task 5.2 scope):**
- No "Add Term" button in page header
- No "Edit" button per term
- No delete functionality
- UI is completely read-only (can only view terms created by backend/MCP)

**What's Missing (Task 5.3 scope):**
- No GlossaryTermEditor component yet
- No SQL editor/tester UI
- Users cannot create or modify terms through the web interface

**Technical Notes for Implementation:**
- The request/response interfaces are already defined in `ui/src/types/glossary.ts` and ready to use
- OutputColumn type is reused from query types (consistent UX pattern)
- Consider reusing CodeMirror/Monaco editor from QueriesPage for SQL editing (if available)
- Test SQL endpoint should validate before enabling Save button (fail-fast UX)
- Aliases should use tag-style multi-input (similar to keywords in other UIs)

### 5.2 Update GlossaryPage (`ui/src/pages/GlossaryPage.tsx`)

- Display `defining_sql` in a code block (collapsible, like current SQL Details)
- Show output columns table if available
- Show aliases as tags/chips
- Show source badge (inferred/manual/client)
- Add "Edit" button for each term
- Add "Add Term" button in header

### 5.3 Create GlossaryTermEditor Component

New component for Add/Edit modal or page:
- Term name input
- Definition textarea
- SQL editor (reuse CodeMirror/Monaco setup from QueriesPage if available)
- "Test SQL" button that:
  1. Calls `/api/glossary/test-sql` endpoint
  2. Shows validation result (success/error)
  3. On success, displays output columns and sample row
  4. Enables "Save" button only after successful test
- Aliases input (tag-style multi-input)
- Base table (auto-detected from SQL or manual override)

### 5.4 API Endpoints

Add to `pkg/handlers/glossary_handler.go`:

```
POST /api/projects/:pid/glossary/test-sql
  Body: { "sql": "SELECT ..." }
  Response: { "valid": true, "output_columns": [...], "sample_row": {...} }
           or { "valid": false, "error": "..." }

POST /api/projects/:pid/glossary
  Body: { "term": "...", "definition": "...", "defining_sql": "...", "aliases": [...] }
  Response: GlossaryTerm

PUT /api/projects/:pid/glossary/:id
  Body: { "term": "...", "definition": "...", "defining_sql": "...", "aliases": [...] }
  Response: GlossaryTerm

DELETE /api/projects/:pid/glossary/:id
  Response: 204 No Content
```

---

## Phase 6: Testing

### 6.1 Unit Tests

- `glossary_repository_test.go`: CRUD with new schema, alias operations
- `glossary_service_test.go`: SQL validation, output column capture
- `glossary_test.go` (MCP tools): Both tools with various inputs

### 6.2 Integration Tests

- `glossary_integration_test.go`: Full flow from API to database
- Test SQL validation against real datasource
- Test alias lookup

### 6.3 UI Tests

- GlossaryPage rendering with new fields
- GlossaryTermEditor form validation
- SQL test flow

---

## Implementation Order

1. **Database migration** - Create new schema (Phase 1)
2. **Models & Repository** - Update Go types and data access (Phase 2)
3. **Service layer** - SQL validation, alias support (Phase 2.3)
4. **LLM prompts** - Generate `defining_sql` instead of bits (Phase 3)
5. **MCP tools** - Two-tool pattern (Phase 4)
6. **API endpoints** - CRUD + test-sql (Phase 5.4)
7. **UI updates** - Display and editor (Phase 5.1-5.3)
8. **Testing** - All layers (Phase 6)

---

## Out of Scope (Future)

- MCP client creating/updating glossary terms (design accommodates, not implemented)
- Entity association (linking terms to entities)
- Versioning/history of term definitions
- Import/export of glossary terms
- Bulk operations

---

## Source Value Consistency

For reference, here's the source/detection pattern across ontology tables:

| Table | Field | Values | Meaning |
|-------|-------|--------|---------|
| `engine_entity_relationships` | `detection_method` | `foreign_key`, `pk_match`, `manual` | How relationship was discovered |
| `engine_business_glossary` | `source` | `inferred`, `manual`, `client` | Who/what created the term |

- **`inferred`** = LLM discovered during extraction (equivalent to `pk_match`)
- **`manual`** = Human added/edited via UI (same across all tables)
- **`client`** = MCP client added dynamically (new, glossary-first)

Note: `foreign_key` in relationships is special - it means the DDL itself declared it, not inference. Glossary doesn't have an equivalent since all SQL is either inferred or manually provided.
