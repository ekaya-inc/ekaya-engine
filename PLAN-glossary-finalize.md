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

### 2.2 Update Repository (`pkg/repositories/glossary_repository.go`)

- Update CRUD operations for new schema
- Add `GetByAlias(ctx, projectID, alias)` method for alias lookup
- Add `CreateAlias(ctx, glossaryID, alias)` and `DeleteAlias(ctx, glossaryID, alias)`
- Remove all references to old fields (`sql_pattern`, `columns_used`, `filters`, `aggregation`)

### 2.3 Update Service (`pkg/services/glossary_service.go`)

- Add `TestSQL(ctx, projectID, sql)` method:
  1. Run EXPLAIN to validate syntax and schema references
  2. Execute with LIMIT 1 to capture output columns
  3. Return `{valid: bool, error: string, outputColumns: []OutputColumn}`
- Update `CreateTerm` to require `defining_sql` and run validation
- Update `UpdateTerm` to re-validate SQL if changed
- Add alias management methods

---

## Phase 3: LLM Prompt & Extraction Updates

### 3.1 Update Discovery Prompt (`glossary_service.go:buildSuggestTermsPrompt`)

Change the response format to generate `defining_sql` instead of bits:

```json
[
  {
    "term": "Active Users",
    "definition": "Users who have engaged with the platform within the last 30 days",
    "defining_sql": "SELECT COUNT(DISTINCT user_id) AS active_users\nFROM users\nWHERE deleted_at IS NULL\n  AND updated_at >= CURRENT_DATE - INTERVAL '30 days'",
    "base_table": "users",
    "aliases": ["MAU", "Monthly Active Users"]
  }
]
```

### 3.2 Update System Message

Emphasize:
- SQL must be complete and executable (SELECT statement)
- SQL should be a definition/calculation, not just a fragment
- Include meaningful aliases that business users might use
- Base table is the primary table being queried

### 3.3 Update Enrichment Node (if keeping two-phase)

Or consolidate into single discovery phase that generates complete definitions.

### 3.4 Post-Discovery Validation

After LLM generates terms:
1. Run EXPLAIN on each `defining_sql`
2. If invalid, log warning and skip term (or retry with error context)
3. Execute valid SQL with LIMIT 1 to capture output columns
4. Store validated terms with output columns

---

## Phase 4: MCP Tools

### 4.1 Refactor to Two Tools (`pkg/mcp/tools/glossary.go`)

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

### 4.2 Design for Future Client Updates

Repository and service interfaces should support:
- `CreateTerm` / `UpdateTerm` / `DeleteTerm` with source parameter
- These will be exposed as MCP tools later for client-driven updates

---

## Phase 5: UI Updates

### 5.1 Update Types (`ui/src/types/glossary.ts`)

```typescript
interface OutputColumn {
  name: string;
  type: string;
  description?: string;
}

interface GlossaryTerm {
  id: string;
  term: string;
  definition: string;
  defining_sql: string;
  base_table?: string;
  output_columns?: OutputColumn[];
  aliases?: string[];
  source: 'inferred' | 'manual' | 'client';
  created_at: string;
  updated_at: string;
}

interface TestSQLResult {
  valid: boolean;
  error?: string;
  output_columns?: OutputColumn[];
  sample_row?: Record<string, unknown>;
}
```

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
