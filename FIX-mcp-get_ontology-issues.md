# MCP get_ontology - Issues and Remaining Work

**Last updated:** 2026-01-03

## Status

| Phase | Status | Notes |
|-------|--------|-------|
| Phase 1: Service Layer | ✅ COMPLETE | Reads from normalized tables |
| Phase 2: Entity Extraction | ✅ COMPLETE | Domain, key columns, aliases |
| Phase 3: Ontology Finalization | ✅ COMPLETE | Domain description, auto-triggered |
| Phase 4: Column Workflow | ✅ COMPLETE | Column semantics, enum values, FK roles |
| Phase 5: Project Conventions | ✅ COMPLETE | Soft delete, currency (bundled with Phase 3) |
| Phase 6: Business Glossary | 📋 READY | Metric definitions - see spec below |

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

## Issue 2: Column roles are empty

**Priority:** MEDIUM (missing context, but survivable)

### Observed Behavior
Every column showed `"role": ""` at `depth: tables`.

**Expected:** Should see "dimension", "measure", "identifier", "attribute" from ColumnEnrichment.

### Root Cause Analysis

**The `Role` field IS correctly wired through**, but it's NULL in the database because roles are only populated during **optional column enrichment**.

**Data flow verification:**

1. **Creation** (`pkg/services/entity_discovery_service.go:175-180`):
   ```go
   occurrence := &models.OntologyEntityOccurrence{
       EntityID:   entity.ID,
       SchemaName: c.schemaName,
       TableName:  c.tableName,
       ColumnName: c.columnName,
       Confidence: c.confidence,
       // ← Role is NOT SET here - defaults to nil
   }
   ```

2. **Model** (`pkg/models/ontology_entity.go:37`):
   ```go
   Role *string `json:"role,omitempty"` // nullable pointer
   ```

3. **Service passthrough** (`pkg/services/ontology_context.go:205-209`):
   ```go
   occurrencesByEntityID[occ.EntityID] = append(..., models.EntityOccurrence{
       Table:  occ.TableName,
       Column: occ.ColumnName,
       Role:   occ.Role,  // ← Correctly passes through - just NULL
   })
   ```

**The Gap:** The column enrichment workflow (`POST /api/projects/:id/ontology/enrich-columns`) populates `engine_ontologies.column_details` JSONB with role data, but this enriched data is NOT used to update `engine_ontology_entity_occurrences.role`.

### Fix Required

**Option A (Update occurrences during enrichment):**
After column enrichment writes to `column_details` JSONB, also update matching rows in `engine_ontology_entity_occurrences.role`.

**Option B (Use enriched data at query time):**
In `GetEntitiesContext()`, also load `column_details` and merge roles into occurrences before returning.

Option A is cleaner - keeps data normalized in one place.

### Files to Modify

| File | Lines | Change |
|------|-------|--------|
| `pkg/services/column_enrichment_service.go` (or equivalent) | TBD | After enrichment, update occurrence roles |
| OR `pkg/services/ontology_context.go` | 174-210 | Load column_details and merge roles |

---

## Issue 3: FK semantic roles not visible

**Priority:** MEDIUM (would help with multi-FK tables)

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

## Issue 4: Duplicate relationships at domain level

**Priority:** LOW (cosmetic, doesn't break queries)

### Observed Behavior
"Offer → Engagement Payment Intent" appeared 3+ times with slightly different labels.

### Root Cause Analysis

**Location:** `pkg/services/ontology_context.go:129-148`

There is NO deduplication logic:
```go
relationships := make([]models.RelationshipEdge, 0, len(entityRelationships))
for _, rel := range entityRelationships {
    sourceName := entityNameByID[rel.SourceEntityID]
    targetName := entityNameByID[rel.TargetEntityID]
    if sourceName == "" || targetName == "" {
        continue
    }
    // ...
    relationships = append(relationships, models.RelationshipEdge{
        From:  sourceName,
        To:    targetName,
        Label: label,
    })
    // ← Appends ALL relationships, including duplicates
}
```

**Why duplicates exist in DB:**
- Same entity pair can have multiple FK columns (e.g., `host_id` and `visitor_id` both → `users`)
- Same FK detected by multiple methods (FK constraint + PK-match inference)
- Different descriptions for same logical relationship

### Fix Required

Deduplicate by source→target pair, keeping first (or longest) label:
```go
seen := make(map[string]bool)
for _, rel := range entityRelationships {
    // ... name lookup ...
    key := sourceName + "→" + targetName
    if seen[key] {
        continue  // Skip duplicate
    }
    seen[key] = true
    relationships = append(...)
}
```

### Files to Modify

| File | Lines | Change |
|------|-------|--------|
| `pkg/services/ontology_context.go` | 129-148 | Add deduplication map |

---

## Additional Feedback

**Approved query suggestion:** The "Get all engagements for a username" query only filters by host. Consider adding a version that searches by host OR visitor, with case-insensitive matching:
```sql
WHERE LOWER(h.username) = LOWER({{username}}) OR LOWER(v.username) = LOWER({{username}})
```

---

## Phase 6: Business Glossary (Ready to Implement)

**Goal:** Define business metrics and calculations for executive reporting.

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
