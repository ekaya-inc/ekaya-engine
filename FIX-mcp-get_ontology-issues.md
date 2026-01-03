# MCP get_ontology - Issues and Remaining Work

**Last updated:** 2026-01-04

## Status

| Phase | Status | Notes |
|-------|--------|-------|
| Phase 1: Service Layer | ✅ COMPLETE | Reads from normalized tables |
| Phase 2: Entity Extraction | ✅ COMPLETE | Domain, key columns, aliases |
| Phase 3: Ontology Finalization | ✅ COMPLETE | Domain description, auto-triggered |
| Phase 4: Column Workflow | ✅ COMPLETE | Column semantics, enum values, FK roles |
| Phase 5: Project Conventions | ✅ COMPLETE | Soft delete, currency (bundled with Phase 3) |
| Phase 6: Business Glossary | 🚧 IN PROGRESS | Step 1 (migration) ✅, Steps 2-7 pending |

---

## Issues Discovered During MCP Client Testing (2026-01-03)

Tested by using Claude Code as an MCP client against ekaya-engine with a fully extracted ontology.

### What Worked Well

| Feature | Value Delivered |
|---------|-----------------|
| Domain summary | Immediate business context - understood platform purpose |
| Conventions | Knew to filter `deleted_at IS NULL` and divide currency by 100 |
| Entity descriptions | Distinguished Billing Engagement vs Billing Transaction |
| Table/column counts | Gauged schema complexity (38 tables, 564 columns) |
| Progressive disclosure | `domain` → `tables` → `columns` is sensible flow |

---

## Issue 1: Relationships on `users` table are incorrect ✅ COMPLETED

**Priority:** HIGH (actively misleading - worse than no data)

**Status:** Fixed in commit on 2026-01-03

### What Was Fixed

Changed `pkg/services/deterministic_relationship_service.go` to use `entityByPrimaryTable` instead of `occByTable`:

1. **Removed** the occurrence-based lookup (`occByTable`) that used "first occurrence wins" logic
2. **Added** `entityByPrimaryTable` map that uses `entity.PrimarySchema/PrimaryTable` to correctly identify which entity owns each table
3. **Updated** all three usages (Phase 1 FK relationships: source lookup, target lookup; Phase 2: PK-match inference)

The fix ensures that when looking up the entity for a table (e.g., `billing_engagements`), we get the entity that **owns** that table (`billing_engagement`) rather than an entity that merely has occurrences in it (`user` via host_id/visitor_id columns).

### Test Coverage Added

New test file `pkg/services/deterministic_relationship_service_test.go`:
- `TestEntityByPrimaryTableMapping` - Verifies the new PrimaryTable-based mapping works correctly
- `TestOldOccByTableBehaviorWasBroken` - Documents the bug in the old occurrence-based approach

### Original Problem

When drilling into `users` table at `depth: tables`, relationships showed:
```json
{"column":"user_id","references":"accounts.account_id","cardinality":""}
{"column":"user_id","references":"channels.channel_id","cardinality":""}
```

`user_id` does not FK to `accounts.account_id` or `channels.channel_id` - these are different ID spaces. The root cause was that the `occByTable` map was built with "first occurrence wins" logic, which could associate a table with the wrong entity.

---

## Issue 2: Column roles are empty ✅ COMPLETED

**Priority:** MEDIUM (missing context, but survivable)

**Status:** Fixed on 2026-01-03

### What Was Fixed

Implemented **Option A** - Update occurrences during column enrichment. After the LLM enriches columns with FK roles (e.g., "host", "visitor"), we now update the corresponding `engine_ontology_entity_occurrences.role` field.

**Changes Made:**

1. **Repository layer** (`pkg/repositories/ontology_entity_repository.go`):
   - Added `UpdateOccurrenceRole(ctx, entityID, tableName, columnName, role)` method to the `OntologyEntityRepository` interface
   - Implemented the method with a simple UPDATE query targeting the occurrence by entity_id, table_name, and column_name

2. **Service layer** (`pkg/services/column_enrichment.go`):
   - Added `updateOccurrenceRoles()` helper method that:
     - Extracts FK roles from LLM enrichment results
     - Maps FK target tables to entity IDs using `entityByPrimaryTable` (same pattern as relationship fix)
     - Updates each occurrence's role in the database
   - Called from `EnrichTable()` after saving column details to JSONB
   - Failures are logged but don't fail the enrichment (roles are supplementary data)

**Test Coverage:**

- Added `TestColumnEnrichmentService_UpdateOccurrenceRoles` - verifies roles are extracted from LLM response and passed to repository
- Updated mock implementations in related test files to satisfy the new interface method

### Original Problem

Every column showed `"role": ""` at `depth: tables`. The LLM correctly enriched columns with roles like "host" and "visitor", but this data was only stored in `column_details` JSONB and not propagated to `engine_ontology_entity_occurrences.role`.

---

## Issue 3: FK semantic roles not visible ✅ COMPLETED

**Priority:** MEDIUM (would help with multi-FK tables)

**Status:** Fixed on 2026-01-03

### What Was Fixed

Added `FKRole` field to `ColumnOverview` struct and updated `GetTablesContext()` to load enriched column data from `ontology.ColumnDetails` and merge FK roles. Also added `HasEnumValues` from enriched data.

**Changes Made:**

1. **Model layer** (`pkg/models/ontology_context.go`):
   - Added `FKRole string` field to `ColumnOverview` struct with `json:"fk_role,omitempty"` tag

2. **Service layer** (`pkg/services/ontology_context.go`):
   - Updated `GetTablesContext()` to build enriched column lookup from `ontology.ColumnDetails`
   - Merged `FKRole` and `HasEnumValues` from enriched data into `ColumnOverview`

**Test Coverage:**

- Added `TestGetTablesContext_FKRoles` - verifies FK roles (host, visitor) and enum values are exposed at tables depth

### Observed Behavior
`billing_engagements` has `host_id` and `visitor_id` - both reference `users`. The approved query showed this pattern, but `get_ontology` didn't expose which FK plays which role.

**What would help:** At table level, show `fk_role` for columns (e.g., `host_id → users (role: host)`).

### Root Cause Analysis

**Location:** `pkg/models/ontology_context.go:92-101`

The `ColumnOverview` struct used at tables depth does NOT have `FKRole`:
```go
type ColumnOverview struct {
    Name          string  `json:"name"`
    Type          string  `json:"type"`
    Role          string  `json:"role"`         // ← General role (dimension/measure)
    IsPrimaryKey  bool    `json:"is_primary_key"`
    Entity        *string `json:"entity,omitempty"`
    EntityRole    *string `json:"entity_role,omitempty"`
    HasEnumValues bool    `json:"has_enum_values"`
    // ← NO FKRole field!
}
```

But `ColumnDetail` (used at columns depth) DOES have `FKRole`:
```go
type ColumnDetail struct {
    // ...
    FKRole string `json:"fk_role,omitempty"` // payer, payee, host, visitor, etc.
    // ...
}
```

**In `GetTablesContext()`** (`pkg/services/ontology_context.go:326-334`), only schema data is used:
```go
columns := make([]models.ColumnOverview, 0, len(schemaColumns))
for _, col := range schemaColumns {
    columns = append(columns, models.ColumnOverview{
        Name:          col.ColumnName,
        Type:          col.DataType,
        IsPrimaryKey:  col.IsPrimaryKey,
        HasEnumValues: false,  // ← Doesn't load enriched data at all
    })
}
```

The enriched `column_details` JSONB (which contains `FKRole`) is NOT loaded at tables depth.

### Fix Required

1. Add `FKRole` field to `ColumnOverview` struct
2. In `GetTablesContext()`, load `ontology.ColumnDetails` (same as `GetColumnsContext()` does)
3. Merge `FKRole` from enriched data into `ColumnOverview`

### Files to Modify

| File | Lines | Change |
|------|-------|--------|
| `pkg/models/ontology_context.go` | 92-101 | Add `FKRole string` to `ColumnOverview` |
| `pkg/services/ontology_context.go` | 247-367 | Load column_details and merge FKRole |

---

## Issue 4: Duplicate relationships at domain level ✅ COMPLETED

**Priority:** LOW (cosmetic, doesn't break queries)

**Status:** Fixed on 2026-01-03

### What Was Fixed

Added deduplication logic in `GetDomainContext()` to prevent duplicate relationships (same source→target entity pair) from appearing multiple times. When duplicates exist, the implementation keeps the relationship with the longest description to provide more context.

**Changes Made:**

1. **Service layer** (`pkg/services/ontology_context.go:129-160`):
   - Added `seen` map to track source→target pairs by key (e.g., "user→billing_engagement")
   - When a duplicate is found, compare label lengths and keep the longer one
   - This handles cases where the same entity pair has multiple FK columns (e.g., `host_id` and `visitor_id` both → `users`)

**Test Coverage:**

- Added `TestGetDomainContext_DeduplicatesRelationships` - verifies 3 duplicate relationships are deduplicated to 1, keeping the longest label
- Added `TestGetDomainContext_DeduplicatesRelationships_FirstWinsWhenSameLength` - verifies first relationship wins when labels have same length

### Original Problem

"Offer → Engagement Payment Intent" appeared 3+ times with slightly different labels because:
- Same entity pair can have multiple FK columns
- Same FK detected by multiple methods (FK constraint + PK-match inference)
- Different descriptions for same logical relationship

---

## Additional Feedback

**Approved query suggestion:** The "Get all engagements for a username" query only filters by host. Consider adding a version that searches by host OR visitor, with case-insensitive matching:
```sql
WHERE LOWER(h.username) = LOWER({{username}}) OR LOWER(v.username) = LOWER({{username}})
```

---

## Phase 6: Business Glossary (In Progress)

**Goal:** Define business metrics and calculations for executive reporting.

### Task Checklist

- [x] **Step 1: Database Migration** - Created `migrations/025_business_glossary.{up,down}.sql` with table, RLS, indexes, and trigger. Added integration test `migrations/025_business_glossary_test.go`.
- [x] **Step 2: Model** - Created `pkg/models/glossary.go` with `BusinessGlossaryTerm` and `Filter` structs. Model includes all fields from the spec: ID, ProjectID, Term, Definition, SQLPattern, BaseTable, ColumnsUsed ([]string), Filters ([]Filter), Aggregation, Source, CreatedBy, CreatedAt, UpdatedAt. Filter struct has Column, Operator, and Values fields. JSON tags include omitempty where appropriate.
- [x] **Step 3: Repository** ✅ COMPLETED
  - **File:** `pkg/repositories/glossary_repository.go`
  - **Interface:** `GlossaryRepository` with 6 methods: Create, Update, Delete, GetByProject, GetByTerm, GetByID
  - **Pattern:** Follows `ontology_entity_repository.go` - uses `database.GetTenantScope(ctx)`, RETURNING clause, proper JSONB handling
  - **Helper functions:** `nullString()`, `jsonbValue()`, `jsonUnmarshal()`, `scanGlossaryTerm()` for database field handling
  - **Tests:** `pkg/repositories/glossary_repository_test.go` (19 integration tests) covering CRUD, JSONB fields, edge cases, and RLS enforcement
  - **Key implementation notes:**
    - Uses `pgx.Row` interface pattern for scanning to support both QueryRow and Rows iteration
    - GetByTerm and GetByID return `nil, nil` for not-found (not an error), consistent with other repos
    - Delete and Update return error for not-found records
    - JSONB fields properly handle empty slices → NULL and NULL → empty slices
- [x] **Step 4: Service** ✅ COMPLETED
  - **File:** `pkg/services/glossary_service.go`
  - **Interface:** `GlossaryService` with 6 methods: CreateTerm, UpdateTerm, DeleteTerm, GetTerms, GetTerm, SuggestTerms
  - **Dependencies:** GlossaryRepository, OntologyRepository, OntologyEntityRepository, LLMClientFactory
  - **SuggestTerms implementation:**
    - Loads active ontology and entities for context
    - Builds LLM prompt with domain summary, conventions, entities, and column details
    - Uses `llm.ParseJSONResponse` to extract structured suggestions
    - Returns terms with source="suggested" (not persisted until user accepts)
  - **Validation:** CreateTerm/UpdateTerm validate term name and definition required
  - **Default source:** CreateTerm sets source="user" if not provided
  - **Tests:** `pkg/services/glossary_service_test.go` (13 tests) covering CRUD operations, validation, and SuggestTerms with various scenarios (success, no ontology, no entities, LLM errors, conventions, column details)
- [x] **Step 5: Handler** ✅ COMPLETED (2026-01-04)
  - **File:** `pkg/handlers/glossary_handler.go`
  - **Endpoints:** GET/POST `/glossary`, GET/PUT/DELETE `/glossary/{tid}`, POST `/glossary/suggest`
  - **Pattern:** Follows `entity_handler.go` with RegisterRoutes, uses ApiResponse wrapper
  - **Helper added:** `ParseTermID` in `pkg/handlers/params.go` for term ID extraction
  - **Tests:** `pkg/handlers/glossary_integration_test.go` (11 integration tests) covering all CRUD endpoints, validation, suggest, and error cases
  - **Implementation notes for next session:**
    - Request/response types defined: `GlossaryListResponse`, `CreateGlossaryTermRequest`, `UpdateGlossaryTermRequest`
    - All handlers follow thin pattern: parse params → call service → format response
    - Validation errors (missing term/definition) return 400, not found returns 404
    - Suggest endpoint checks for ontology and returns 400 if none exists
    - Uses same auth/tenant middleware pattern as other handlers
- [x] **Step 6: Register in main.go** ✅ COMPLETED (2026-01-04)
  - **File:** `main.go` (lines 367-371)
  - Added glossary repository, service, and handler wiring after ontologyDAGHandler registration
  - Dependencies: glossaryRepo, ontologyRepo, ontologyEntityRepo, llmFactory, logger
  - Routes registered with authMiddleware and tenantMiddleware
  - **Working endpoints:**
    - `GET /api/projects/{pid}/glossary` - List all terms
    - `POST /api/projects/{pid}/glossary` - Create term
    - `GET /api/projects/{pid}/glossary/{tid}` - Get single term
    - `PUT /api/projects/{pid}/glossary/{tid}` - Update term
    - `DELETE /api/projects/{pid}/glossary/{tid}` - Delete term
    - `POST /api/projects/{pid}/glossary/suggest` - LLM suggests terms based on ontology
- [ ] **Step 7: Expose in get_ontology** - Add `get_glossary` MCP tool or include in domain depth
  - **Recommendation:** Create a new `get_glossary` MCP tool in `pkg/mcp/tools/` (Option A from spec)
  - **Key files to reference:**
    - `pkg/mcp/tools/ontology.go` - Pattern for creating MCP tools
    - `pkg/services/glossary_service.go` - `GetTerms()` method returns all glossary terms
    - `pkg/handlers/glossary_handler.go` - HTTP handler for reference
  - **Implementation steps:**
    1. Create tool registration in `pkg/mcp/tools/glossary.go`
    2. Add tool filtering support in `pkg/mcp/tools/developer.go` (NewToolFilter)
    3. Register the tool in `main.go` similar to RegisterOntologyTools

**Why This Matters for MCP Clients:** When asked "What's our revenue?", the agent needs to know:
- Revenue = `SUM(earned_amount)` not `SUM(total_amount)`
- Only include `transaction_state = 'completed'`
- Exclude refunds or subtract them

The glossary provides **reverse lookup** from business term → schema/SQL pattern, which cannot be inferred from schema alone.

### What Phase 6 Adds

| Term | Definition | SQL Pattern |
|------|------------|-------------|
| Revenue | Earned amount after fees | `SUM(earned_amount) WHERE transaction_state = 'completed'` |
| GMV | Gross merchandise value | `SUM(total_amount) WHERE transaction_state IN ('completed', 'refunded')` |
| Active User | User with recent activity | `WHERE last_active_at > NOW() - INTERVAL '30 days'` |
| Host | User who provides service | `WHERE user_id IN (SELECT DISTINCT host_id FROM sessions)` |

### Implementation Spec

#### Step 1: Database Migration

**File:** `migrations/025_business_glossary.up.sql`

> Note: Migration 024 already exists (`024_add_missing_rls.up.sql`), so use 025.

```sql
-- Business glossary for metric definitions and business term → schema mapping
CREATE TABLE engine_business_glossary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    term TEXT NOT NULL,
    definition TEXT NOT NULL,
    sql_pattern TEXT,
    base_table TEXT,
    columns_used JSONB,
    filters JSONB,
    aggregation TEXT,
    source TEXT NOT NULL DEFAULT 'user',
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, term)
);

-- Index for project-scoped queries
CREATE INDEX idx_business_glossary_project ON engine_business_glossary(project_id);

-- Row level security
ALTER TABLE engine_business_glossary ENABLE ROW LEVEL SECURITY;

CREATE POLICY business_glossary_access ON engine_business_glossary
    FOR ALL USING (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    )
    WITH CHECK (
        current_setting('app.current_project_id', true) IS NULL
        OR project_id = current_setting('app.current_project_id', true)::uuid
    );

-- Auto-update timestamp trigger
CREATE TRIGGER update_business_glossary_updated_at
    BEFORE UPDATE ON engine_business_glossary
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();
```

**File:** `migrations/025_business_glossary.down.sql`

```sql
DROP TABLE IF EXISTS engine_business_glossary;
```

#### Step 2: Model

**File:** `pkg/models/glossary.go`

```go
package models

import (
    "time"
    "github.com/google/uuid"
)

// BusinessGlossaryTerm represents a business term with its technical mapping.
type BusinessGlossaryTerm struct {
    ID          uuid.UUID   `json:"id"`
    ProjectID   uuid.UUID   `json:"project_id"`
    Term        string      `json:"term"`
    Definition  string      `json:"definition"`
    SQLPattern  string      `json:"sql_pattern,omitempty"`
    BaseTable   string      `json:"base_table,omitempty"`
    ColumnsUsed []string    `json:"columns_used,omitempty"`
    Filters     []Filter    `json:"filters,omitempty"`
    Aggregation string      `json:"aggregation,omitempty"`
    Source      string      `json:"source"` // "user" or "suggested"
    CreatedBy   *uuid.UUID  `json:"created_by,omitempty"`
    CreatedAt   time.Time   `json:"created_at"`
    UpdatedAt   time.Time   `json:"updated_at"`
}

// Filter represents a condition in the glossary term definition.
type Filter struct {
    Column   string   `json:"column"`
    Operator string   `json:"operator"` // =, IN, >, <, etc.
    Values   []string `json:"values"`
}
```

#### Step 3: Repository

**File:** `pkg/repositories/glossary_repository.go`

Follow the established pattern from `ontology_entity_repository.go`:

```go
package repositories

import (
    "context"
    "fmt"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/ekaya-inc/ekaya-engine/pkg/database"
    "github.com/ekaya-inc/ekaya-engine/pkg/models"
)

type GlossaryRepository interface {
    Create(ctx context.Context, term *models.BusinessGlossaryTerm) error
    Update(ctx context.Context, term *models.BusinessGlossaryTerm) error
    Delete(ctx context.Context, termID uuid.UUID) error
    GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
    GetByTerm(ctx context.Context, projectID uuid.UUID, term string) (*models.BusinessGlossaryTerm, error)
    GetByID(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error)
}

type glossaryRepository struct{}

func NewGlossaryRepository() GlossaryRepository {
    return &glossaryRepository{}
}

var _ GlossaryRepository = (*glossaryRepository)(nil)

// Implement CRUD methods following pattern in ontology_entity_repository.go
// Key SQL patterns:
// - Use database.GetTenantScope(ctx) for connection
// - Use RETURNING clause for Create
// - Use ON CONFLICT for upsert if needed
// - Scan JSONB columns with pgx JSON support
```

#### Step 4: Service

**File:** `pkg/services/glossary_service.go`

```go
package services

import (
    "context"
    "github.com/google/uuid"
    "go.uber.org/zap"
    "github.com/ekaya-inc/ekaya-engine/pkg/models"
    "github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

type GlossaryService interface {
    CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error
    UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error
    DeleteTerm(ctx context.Context, termID uuid.UUID) error
    GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
    GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error)
    SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
}

type glossaryService struct {
    glossaryRepo repositories.GlossaryRepository
    ontologyRepo repositories.OntologyRepository  // For SuggestTerms - needs schema context
    logger       *zap.Logger
}

func NewGlossaryService(
    glossaryRepo repositories.GlossaryRepository,
    ontologyRepo repositories.OntologyRepository,
    logger *zap.Logger,
) GlossaryService {
    return &glossaryService{
        glossaryRepo: glossaryRepo,
        ontologyRepo: ontologyRepo,
        logger:       logger.Named("glossary-service"),
    }
}

// SuggestTerms uses LLM to analyze ontology and suggest business terms
// Implementation: Load ontology, send to LLM with prompt asking for metric definitions
```

#### Step 5: Handler

**File:** `pkg/handlers/glossary_handler.go`

Follow pattern from `entity_handler.go`:

```go
package handlers

// Endpoints:
// GET    /api/projects/{pid}/glossary           - List all terms
// POST   /api/projects/{pid}/glossary           - Create term
// GET    /api/projects/{pid}/glossary/{termId}  - Get single term
// PUT    /api/projects/{pid}/glossary/{termId}  - Update term
// DELETE /api/projects/{pid}/glossary/{termId}  - Delete term
// POST   /api/projects/{pid}/glossary/suggest   - LLM suggests terms

// RegisterRoutes pattern:
// func (h *GlossaryHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware)
```

#### Step 6: Register in main.go

**File:** `main.go`

Add after other handler registrations (around line 360):

```go
// Glossary handler
glossaryRepo := repositories.NewGlossaryRepository()
glossaryService := services.NewGlossaryService(glossaryRepo, ontologyRepo, logger)
glossaryHandler := handlers.NewGlossaryHandler(glossaryService, logger)
glossaryHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)
```

#### Step 7: Expose in get_ontology

**Option A: New MCP tool** (Recommended)

Add `get_glossary` tool in `pkg/mcp/tools/ontology.go` that returns all terms for the project.

**Option B: Include in domain depth**

Add `Glossary []GlossaryTermBrief` to `OntologyDomainContext.DomainInfo`:

```go
type DomainInfo struct {
    // ... existing fields ...
    Glossary []GlossaryTermBrief `json:"glossary,omitempty"`
}

type GlossaryTermBrief struct {
    Term       string `json:"term"`
    Definition string `json:"definition"`
    SQLPattern string `json:"sql_pattern,omitempty"`
}
```

### Files to Create

| File | Purpose |
|------|---------|
| `migrations/025_business_glossary.up.sql` | Database table |
| `migrations/025_business_glossary.down.sql` | Rollback |
| `pkg/models/glossary.go` | Model struct |
| `pkg/repositories/glossary_repository.go` | CRUD operations |
| `pkg/services/glossary_service.go` | Business logic + LLM suggestion |
| `pkg/handlers/glossary_handler.go` | HTTP endpoints |

### Files to Modify

| File | Change |
|------|--------|
| `main.go` | Add handler registration (around line 360) |
| `pkg/mcp/tools/ontology.go` | Add `get_glossary` tool OR include in domain |
| `pkg/models/ontology_context.go` | (If Option B) Add Glossary to DomainInfo |

---

## Important Notes

1. **Column enrichment is manual:** Requires `POST /api/projects/:id/ontology/enrich-columns` to control LLM costs (~$1 for 38 tables).

2. **To clear ontology data for re-testing:**
   ```sql
   DELETE FROM engine_ontologies WHERE project_id = '<project-id>';
   DELETE FROM engine_ontology_dag WHERE project_id = '<project-id>';
   DELETE FROM engine_llm_conversations WHERE project_id = '<project-id>';
   DELETE FROM engine_project_knowledge WHERE project_id = '<project-id>';
   ```

---

## Implementation Order (Recommended)

1. **Issue 1 (wrong relationships)** - HIGH priority, simple fix
2. **Issue 4 (duplicates)** - LOW priority but quick win
3. **Issue 3 (FK roles)** - MEDIUM priority, requires model change
4. **Issue 2 (empty roles)** - MEDIUM priority, requires investigation of enrichment flow
5. **Phase 6 (Glossary)** - New feature, implement after bugs fixed

---

## Code Quality: Minor Nitpicks

### URL Construction Should Use net/url Package

**Location:** `pkg/services/mcp_config.go:239`

Current code uses `fmt.Sprintf`:
```go
ServerURL: fmt.Sprintf("%s/mcp/%s", s.baseURL, projectID.String())
```

**Issue:** String concatenation for URLs is fragile - doesn't handle:
- Trailing slashes on baseURL
- URL encoding of path segments
- Edge cases with special characters

**Fix:** Use `net/url` package for proper URL construction:
```go
import "net/url"

u, _ := url.Parse(s.baseURL)
u.Path = path.Join(u.Path, "mcp", projectID.String())
ServerURL: u.String()
```

Or use `url.JoinPath` (Go 1.19+):
```go
serverURL, _ := url.JoinPath(s.baseURL, "mcp", projectID.String())
```
