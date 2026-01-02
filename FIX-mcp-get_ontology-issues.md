# MCP get_ontology - Complete Implementation Plan

Investigation date: 2026-01-02
**Last updated:** 2026-01-02 (Phase 4 + 5 complete)

## Status

| Phase | Status | Date Completed | Notes |
|-------|--------|----------------|-------|
| Phase 1: Service Layer | ✅ **COMPLETE** | 2026-01-02 | Reads from normalized tables |
| Phase 2: Entity Extraction | ✅ **COMPLETE** | 2026-01-02 | Domain, key columns (with synonyms), aliases |
| Phase 3: Ontology Finalization | ✅ **COMPLETE** | 2026-01-02 | Domain description via LLM, auto-triggered |
| Phase 4: Column Workflow | ✅ **COMPLETE** | 2026-01-02 | Column semantics, enum values, FK roles |
| Phase 5: Project Conventions | ✅ **COMPLETE** | 2026-01-02 | Soft delete, timestamps, currency (bundled with Phase 3) |
| Phase 6: Business Glossary | 📋 **READY** | - | Metric definitions - see detailed spec below |

### Priority for Query Accuracy

| Phase | Impact | Cost | Recommendation |
|-------|--------|------|----------------|
| Phase 4 | **HIGH** | Medium (~$1 for 38 tables) | ✅ Complete |
| Phase 5 | **HIGH** | Low (no LLM calls) | ✅ Complete (bundled with Phase 3) |
| Phase 6 | Medium | Low | Ready to implement when needed |

---

## What Works Now (After All Phases Complete)

| Depth | Data Returned | Source |
|-------|---------------|--------|
| `domain` | Entity count, column count, relationship graph, **description**, **primary domains**, **conventions** | Normalized tables + DomainSummary |
| `entities` | Names, descriptions, occurrences, aliases, **key columns with synonyms** | Normalized tables |
| `tables` | Business names, descriptions, columns, relationships, aliases, **domain** | Normalized tables |
| `columns` | **Full semantic metadata**: descriptions, semantic types, roles, FK roles, enum values, synonyms | Schema + `column_details` JSONB |

**Remaining Optional Enhancement:**

| Gap | Impact | Phase |
|-----|--------|-------|
| Business metric definitions | MEDIUM - Agent may calculate revenue wrong | Phase 6 (optional) |

---

## Phase 1 Accomplishments ✅

**Problem Solved:** MCP `get_ontology` tool was returning empty data because service read from unpopulated JSONB columns.

**Solution:** Modified service to read from existing normalized tables.

**Key Changes:**
- `GetDomainContext`: Reads from `engine_ontology_entities`, `engine_entity_relationships`, `engine_schema_columns`
- `GetEntitiesContext`: Batch-fetches aliases via `GetAllAliasesByProject` (fixes N+1 query)
- `GetTablesContext`: Reads from normalized entity and schema tables
- `GetColumnsContext`: Returns structural metadata from `engine_schema_columns`

**Files Modified:**
- `pkg/services/ontology_context.go` - Service implementation
- `pkg/repositories/ontology_entity_repository.go` - Added `GetAllAliasesByProject`
- `pkg/repositories/entity_relationship_repository.go` - Added `GetByProject`, `GetByTables`
- `pkg/repositories/schema_repository.go` - Added `GetColumnCountByProject`, `GetColumnsByTables`

---

## Phase 2 Accomplishments ✅

**Goal:** Add domain categorization, key columns, and aliases during entity extraction.

### Database Migration
**File:** `migrations/022_entity_extraction_fields.up.sql`
```sql
-- Add domain column to entities
ALTER TABLE engine_ontology_entities ADD COLUMN domain VARCHAR(100);

-- Create key columns table
CREATE TABLE engine_ontology_entity_key_columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    column_name TEXT NOT NULL,
    synonyms JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_id, column_name)
);
CREATE INDEX idx_entity_key_columns_entity_id ON engine_ontology_entity_key_columns(entity_id);
```

### Model Updates
**File:** `pkg/models/ontology_entity.go`
- Added `Domain string` field to `OntologyEntity` struct
- Added `OntologyEntityKeyColumn` struct with `ID`, `EntityID`, `ColumnName`, `Synonyms`, `CreatedAt`

### Repository Updates
**File:** `pkg/repositories/ontology_entity_repository.go`
- All SELECT queries updated to include `domain` column
- `scanOntologyEntity` updated to handle nullable `domain`
- Added methods:
  - `CreateKeyColumn(ctx, keyColumn)` - Creates a key column record
  - `GetKeyColumnsByEntity(ctx, entityID)` - Gets key columns for one entity
  - `GetAllKeyColumnsByProject(ctx, projectID)` - Batch fetch all key columns (avoids N+1)

### LLM Prompt Updates
**File:** `pkg/services/entity_discovery_service.go`

Extended structs for key column synonyms:
```go
type keyColumnEnrichment struct {
    Name     string   `json:"name"`
    Synonyms []string `json:"synonyms,omitempty"`
}

type entityEnrichment struct {
    TableName        string                `json:"table_name"`
    EntityName       string                `json:"entity_name"`
    Description      string                `json:"description"`
    Domain           string                `json:"domain"`
    KeyColumns       []keyColumnEnrichment `json:"key_columns"`
    AlternativeNames []string              `json:"alternative_names"`
}
```

Extended `buildEntityEnrichmentPrompt()` to request:
1. Entity Name (existing)
2. Description (existing)
3. Domain - free-form business domain (e.g., "billing", "hospitality", "logistics")
4. Key Columns - 2-3 important business columns with synonyms per column
5. Alternative Names - synonyms users might use for the entity

Updated `enrichEntitiesWithLLM()` to:
1. Set `entity.Domain` from LLM response
2. Create key column records via `entityRepo.CreateKeyColumn()` **with synonyms**
3. Create alias records via `entityRepo.CreateAlias()` with `source="discovery"`

### Service Layer Updates
**File:** `pkg/services/ontology_context.go`

- `GetTablesContext`: Changed `Domain: ""` to `Domain: entity.Domain`
- `GetEntitiesContext`: Added call to `GetAllKeyColumnsByProject()` and converts to `KeyColumnInfo` in response

### Test Updates
**Files:** `pkg/services/ontology_context_test.go`, `pkg/services/relationship_workflow_test.go`
- Added mock implementations for new repository methods
- Updated `mockOntologyEntityRepository` with `keyColumns` field

---

## Phase 3 Accomplishments ✅

**Goal:** Generate domain-level summary after Entity and Relationship extraction complete.

### New Service
**File:** `pkg/services/ontology_finalization.go`

```go
type OntologyFinalizationService interface {
    Finalize(ctx context.Context, projectID uuid.UUID) error
}
```

Key implementation details:
- Auto-triggered from `relationshipWorkflowService.finalizeWorkflow()` after workflow completes
- Generates domain description via LLM (2-3 sentence summary)
- Aggregates primary domains from entity.Domain fields (sorted alphabetically for deterministic output)
- Stores in `engine_ontologies.domain_summary` JSONB column
- Debug logging for skipped relationships with missing entity names

### Integration
**File:** `pkg/services/relationship_workflow.go`
- Added `finalizationSvc OntologyFinalizationService` to service struct
- Auto-triggers `finalizationSvc.Finalize()` in `finalizeWorkflow()` after workflow completes
- Non-blocking: workflow still marked complete even if finalization fails

### Wiring
**File:** `main.go`
- Creates `ontologyFinalizationService` with required dependencies
- Passes to `NewRelationshipWorkflowService()` constructor

### Tests
**File:** `pkg/services/ontology_finalization_test.go`
- `TestOntologyFinalization_AggregatesDomains` - Verifies unique domain extraction
- `TestOntologyFinalization_GeneratesDomainDescription` - Mocks LLM, verifies storage
- `TestOntologyFinalization_SkipsIfNoEntities` - Empty project case
- `TestOntologyFinalization_HandlesEmptyDomains` - Graceful handling
- `TestOntologyFinalization_HandlesRelationshipDisplay` - Relationship formatting
- `TestOntologyFinalization_LLMFailure` - Verifies error propagation when LLM fails

### LLM Prompt
- System: "You are a data modeling expert..."
- Prompt includes entities (name, domain, description) and relationships
- Response format: `{"description": "2-3 sentence business summary"}`
- Temperature: 0.3 (analytical task)

---

## Phase 4 Accomplishments ✅

**Goal:** Generate semantic column information that enables AI agents to write accurate SQL queries.

### What Phase 4 Added

| Field | Description | Example | Query Impact |
|-------|-------------|---------|--------------|
| Description | Business meaning | "Total charged in cents" | Knows what to SELECT |
| Synonyms | Alternative names | ["price", "cost"] | Understands user intent |
| Semantic Type | Business classification | "currency_cents" | Knows to divide by 100 |
| Role | Analytical function | "dimension", "measure" | Correct aggregations |
| FKRole | FK disambiguation | "payer", "payee" | Correct JOIN conditions |
| Enum Values | Valid values + meanings | [{"value": "completed", "label": "Completed", "description": "..."}] | Correct WHERE filters |

### New Endpoint

`POST /api/projects/:id/ontology/enrich-columns`

Request:
```json
{"tables": ["users", "billing_transactions"]}  // optional filter
```

Response:
```json
{
  "tables_enriched": ["users", "billing_transactions"],
  "tables_failed": {},
  "duration_ms": 12500
}
```

### Key Implementation Details

1. **One LLM call per table** (not per column) - reduces cost
2. **Enum sampling via `GetDistinctValues()`** - up to 50 values per column
3. **Heuristic enum detection** - columns with `status`, `state`, `type`, `kind`, `category` in name
4. **Schema overlay** - PK/FK info always comes from current schema, enriched data from JSONB
5. **Continue on failure** - returns partial results if some tables fail

### LLM Prompt Structure

```
# Table: billing_transactions
Entity: "Billing Transaction" - Records financial transactions between users

## Columns to Analyze
| Column | Type | PK | FK | Sample Values |
|--------|------|----|----|---------------|
| transaction_state | text | no | no | completed, pending, failed, refunded |
| payer_user_id | uuid | no | yes→users | - |
| payee_user_id | uuid | no | yes→users | - |

## FK Role Context
These columns reference the same entity - identify what role each FK represents:
- users (payer_user_id, payee_user_id)

## For Each Column Provide:
1. description: 1 sentence explaining business meaning
2. semantic_type: identifier, currency_cents, timestamp_utc, status, etc.
3. role: dimension | measure | identifier | attribute
4. synonyms: alternative names users might use
5. enum_values: array of {value, label, description}
6. fk_role: if FK to same table as another column, what role does this FK represent?

## Response Format (JSON array)
```

### Storage

Uses existing `engine_ontologies.column_details` JSONB column (no migration needed).

Structure: `{"table_name": [ColumnDetail, ...]}`

### Model Update

Added `FKRole` field to `ColumnDetail` struct in `pkg/models/ontology.go`:

```go
type ColumnDetail struct {
    Name         string      `json:"name"`
    Description  string      `json:"description,omitempty"`
    Synonyms     []string    `json:"synonyms,omitempty"`
    SemanticType string      `json:"semantic_type,omitempty"`
    Role         string      `json:"role,omitempty"`    // dimension, measure, identifier, attribute
    FKRole       string      `json:"fk_role,omitempty"` // payer, payee, host, visitor, etc.
    EnumValues   []EnumValue `json:"enum_values,omitempty"`
    IsPrimaryKey bool        `json:"is_primary_key"`
    IsForeignKey bool        `json:"is_foreign_key"`
    ForeignTable string      `json:"foreign_table,omitempty"`
}
```

### Files Created/Modified

| File | Action | Purpose |
|------|--------|---------|
| `pkg/services/column_enrichment.go` | **NEW** | LLM enrichment + enum sampling |
| `pkg/models/ontology.go` | MODIFY | Added `FKRole` field |
| `pkg/services/ontology_context.go` | MODIFY | Merge enriched data with schema |
| `pkg/handlers/ontology.go` | MODIFY | Add `/enrich-columns` endpoint |
| `main.go` | MODIFY | Wire `ColumnEnrichmentService` |

### Cost Estimate (Actual)

| Tables | LLM Calls | Est. Tokens | Est. Cost (GPT-4) |
|--------|-----------|-------------|-------------------|
| 38 (Ekaya) | 38 | ~30,000 | ~$1.00 |

### How to Trigger Enrichment

```bash
curl -X POST https://localhost:3443/api/projects/{pid}/ontology/enrich-columns \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"tables": ["users", "billing_transactions"]}'  # optional
```

---

## Phase 5 Accomplishments ✅

**Note:** Phase 5 was bundled with Phase 3 (Ontology Finalization). Convention discovery runs automatically after relationship workflow completes.

### What Phase 5 Added

| Convention | Description | Query Impact |
|------------|-------------|--------------|
| Soft Delete | Detected via `deleted_at IS NULL` pattern | Agent adds filter to all queries |
| Currency | Detected via `*_amount` column pattern | Agent knows to divide by 100 |
| Audit Columns | Detected: `created_at`, `updated_at`, `deleted_at` | Agent excludes from SELECT * |

### Implementation

Convention discovery implemented in `pkg/services/ontology_finalization.go`:

```go
func (s *ontologyFinalizationService) discoverConventions(ctx context.Context, projectID uuid.UUID, entities []*models.OntologyEntity) (*models.ProjectConventions, error)
```

Key detection logic:
- **Soft Delete**: Looks for `deleted_at` column that is nullable
- **Currency**: Checks `*_amount` column values to detect cents vs dollars
- **Audit Columns**: Scans for common patterns (`created_at`, `updated_at`, `deleted_at`)

### Storage

Stored in `engine_ontologies.domain_summary` JSONB as part of `DomainSummary`:

```go
type DomainSummary struct {
    Description  string              `json:"description"`
    Domains      []string            `json:"domains"`
    Conventions  *ProjectConventions `json:"conventions,omitempty"`
}
```

### Exposure in get_ontology

Returned in `domain` depth response via `OntologyDomainContext`:

```json
{
  "domain": {
    "description": "...",
    "primary_domains": ["billing", "hospitality"],
    "conventions": {
      "soft_delete": {"enabled": true, "column": "deleted_at", "filter": "deleted_at IS NULL"},
      "currency": {"default_currency": "USD", "format": "cents"}
    }
  }
}
```

---

## Phase 6: Business Glossary (Ready to Implement)

**Goal:** Define business metrics and calculations for executive reporting.

**Why This Matters:** When asked "What's our revenue?", the agent needs to know:
- Revenue = `SUM(earned_amount)` not `SUM(total_amount)`
- Only include `transaction_state = 'completed'`
- Exclude refunds or subtract them

### What Phase 6 Adds

| Term | Definition | SQL Pattern |
|------|------------|-------------|
| Revenue | Earned amount after fees | `SUM(earned_amount) WHERE transaction_state = 'completed'` |
| GMV | Gross merchandise value | `SUM(total_amount) WHERE transaction_state IN ('completed', 'refunded')` |
| Active User | User with recent activity | `WHERE last_active_at > NOW() - INTERVAL '30 days'` |
| Host | User who provides service | `WHERE user_id IN (SELECT DISTINCT host_id FROM sessions)` |

---

### Implementation Spec for New Session

#### Step 1: Database Migration

**File:** `migrations/024_business_glossary.up.sql`

```sql
CREATE TABLE engine_business_glossary (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    term TEXT NOT NULL,
    definition TEXT NOT NULL,
    sql_pattern TEXT,           -- SQL template (optional)
    base_table TEXT,            -- Primary table for this metric
    columns_used JSONB,         -- ["earned_amount", "transaction_state"]
    filters JSONB,              -- {"transaction_state": ["completed"]}
    aggregation TEXT,           -- "SUM", "COUNT", "AVG"
    source TEXT DEFAULT 'user', -- 'user', 'llm_suggested', 'learned'
    created_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(project_id, term)
);

CREATE INDEX idx_business_glossary_project ON engine_business_glossary(project_id);
```

**File:** `migrations/024_business_glossary.down.sql`

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

// BusinessGlossaryTerm represents a defined business metric or concept.
type BusinessGlossaryTerm struct {
    ID          uuid.UUID        `json:"id"`
    ProjectID   uuid.UUID        `json:"project_id"`
    Term        string           `json:"term"`
    Definition  string           `json:"definition"`
    SQLPattern  string           `json:"sql_pattern,omitempty"`
    BaseTable   string           `json:"base_table,omitempty"`
    ColumnsUsed []string         `json:"columns_used,omitempty"`
    Filters     map[string][]string `json:"filters,omitempty"`  // column -> allowed values
    Aggregation string           `json:"aggregation,omitempty"` // SUM, COUNT, AVG
    Source      string           `json:"source"`               // user, llm_suggested, learned
    CreatedBy   *uuid.UUID       `json:"created_by,omitempty"`
    CreatedAt   time.Time        `json:"created_at"`
    UpdatedAt   time.Time        `json:"updated_at"`
}
```

#### Step 3: Repository

**File:** `pkg/repositories/glossary_repository.go`

```go
type GlossaryRepository interface {
    Create(ctx context.Context, term *models.BusinessGlossaryTerm) error
    Update(ctx context.Context, term *models.BusinessGlossaryTerm) error
    Delete(ctx context.Context, termID uuid.UUID) error
    GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
    GetByTerm(ctx context.Context, projectID uuid.UUID, term string) (*models.BusinessGlossaryTerm, error)
}
```

#### Step 4: Service

**File:** `pkg/services/glossary_service.go`

```go
type GlossaryService interface {
    // CRUD operations
    CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error
    UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error
    DeleteTerm(ctx context.Context, termID uuid.UUID) error
    GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)

    // LLM suggestion (optional)
    SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error)
}
```

**LLM Suggestion Prompt:**

```
Given this database schema with semantic column information:

Tables:
- billing_transactions: total_amount (currency_cents), earned_amount (currency_cents), transaction_state (status: completed, pending, failed, refunded)
- users: is_host (boolean), last_active_at (timestamp)

Suggest business metrics that executives might ask about. For each metric provide:
1. term: The business term (e.g., "Revenue", "GMV", "Active Users")
2. definition: Human-readable definition
3. base_table: Which table to query
4. columns_used: Which columns are involved
5. filters: Required WHERE conditions
6. aggregation: SUM, COUNT, AVG

Response as JSON array.
```

#### Step 5: Handler

**File:** `pkg/handlers/glossary_handler.go`

Endpoints:
- `GET /api/projects/:id/glossary` - List all terms
- `POST /api/projects/:id/glossary` - Create term
- `PUT /api/projects/:id/glossary/:termId` - Update term
- `DELETE /api/projects/:id/glossary/:termId` - Delete term
- `POST /api/projects/:id/glossary/suggest` - LLM suggests terms

#### Step 6: Expose in get_ontology

**Option A: New MCP tool** (Recommended)

Add new tool `get_glossary` in `pkg/mcp/tools/ontology.go`:

```go
mcpServer.AddTool(mcp.NewTool("get_glossary",
    mcp.WithDescription("Get business metric definitions for a project"),
    mcp.WithHandler(func(ctx context.Context, args map[string]any) (*mcp.ToolResult, error) {
        // Return all glossary terms
    }),
))
```

**Option B: Include in domain depth**

Add to `OntologyDomainContext.DomainInfo`:

```go
type DomainInfo struct {
    // ...existing fields
    Glossary []GlossaryTermBrief `json:"glossary,omitempty"`
}

type GlossaryTermBrief struct {
    Term       string `json:"term"`
    Definition string `json:"definition"`
    BaseTable  string `json:"base_table"`
}
```

#### Step 7: Wire in main.go

```go
// Repository
glossaryRepo := repositories.NewGlossaryRepository()

// Service
glossaryService := services.NewGlossaryService(glossaryRepo, llmFactory, logger)

// Handler
glossaryHandler := handlers.NewGlossaryHandler(glossaryService, logger)
glossaryHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)
```

### Files to Create

| File | Purpose |
|------|---------|
| `migrations/024_business_glossary.up.sql` | Database table |
| `migrations/024_business_glossary.down.sql` | Rollback |
| `pkg/models/glossary.go` | Model struct |
| `pkg/repositories/glossary_repository.go` | CRUD operations |
| `pkg/services/glossary_service.go` | Business logic + LLM suggestion |
| `pkg/handlers/glossary_handler.go` | HTTP endpoints |
| `pkg/mcp/tools/glossary.go` | MCP tool (optional) |

### Test Strategy

1. **Unit tests** for repository with mock DB
2. **Unit tests** for service with mock repo and mock LLM
3. **Integration test** with test container
4. **Manual test** via MCP `get_glossary` tool

### UI Considerations (Future)

Admin page at `/projects/:id/glossary`:
- Table of defined terms
- Add/Edit/Delete buttons
- "Suggest Metrics" button to trigger LLM

### Priority

Medium priority - implement after validating Phase 4 (column semantics) works well in production. Glossary provides incremental value for executive reporting use cases.

---

## Current Repository Method Summary

### OntologyEntityRepository (`pkg/repositories/ontology_entity_repository.go`)
| Method | Status | Notes |
|--------|--------|-------|
| `Create` | ✅ | Includes `domain` |
| `Update` | ✅ | Includes `domain` |
| `GetByProject` | ✅ | Includes `domain` |
| `GetByOntology` | ✅ | Includes `domain` |
| `GetByID` | ✅ | Includes `domain` |
| `GetByName` | ✅ | Includes `domain` |
| `GetAllAliasesByProject` | ✅ | Batch fetch, fixes N+1 |
| `CreateAlias` | ✅ | Used for alternative names |
| `CreateKeyColumn` | ✅ | **NEW in Phase 2** |
| `GetKeyColumnsByEntity` | ✅ | **NEW in Phase 2** |
| `GetAllKeyColumnsByProject` | ✅ | **NEW in Phase 2** - Batch fetch |

### EntityRelationshipRepository (`pkg/repositories/entity_relationship_repository.go`)
| Method | Status |
|--------|--------|
| `GetByProject` | ✅ |
| `GetByTables` | ✅ |

### SchemaRepository (`pkg/repositories/schema_repository.go`)
| Method | Status |
|--------|--------|
| `GetColumnCountByProject` | ✅ |
| `GetColumnsByTables` | ✅ |

### OntologyRepository (`pkg/repositories/ontology_repository.go`)
| Method | Status | Notes |
|--------|--------|-------|
| `UpdateDomainSummary` | ✅ | Used by Phase 3 finalization |
| `UpdateColumnDetails` | ✅ | Used by Phase 4 column enrichment |

### ColumnEnrichmentService (`pkg/services/column_enrichment.go`) - NEW in Phase 4
| Method | Purpose |
|--------|---------|
| `EnrichTable` | Enrich all columns for a single table |
| `EnrichProject` | Enrich all tables in a project |

---

## Important Notes for Future Sessions

1. **Existing projects need re-extraction:** Entity extraction must be re-run for existing projects to populate `domain`, key columns, and aliases. The migration only adds the columns - it doesn't populate data.

2. **Domain is free-form:** We chose free-form domain classification (not a fixed enum) to allow domain-specific terminology like "hospitality", "fintech", etc.

3. **Key columns stored in normalized table with synonyms:** Key columns are stored in `engine_ontology_entity_key_columns` with per-column synonyms in JSONB. The LLM generates synonyms for each key column during entity extraction.

4. **Phase 3 auto-triggers:** Ontology finalization runs automatically after relationship workflow completes. It's non-blocking - workflow still marked complete even if finalization fails.

5. **Phase 4 is manual:** Column enrichment requires explicit API call to `POST /api/projects/:id/ontology/enrich-columns`. This is intentional to control LLM costs (~$1 for 38 tables).

6. **Phase 5 bundled with Phase 3:** Convention discovery runs as part of ontology finalization - no separate step needed.

7. **Test mocks updated:** `ontology_context_test.go`, `relationship_workflow_test.go`, and `ontology_finalization_test.go` have updated mocks. Any new tests should follow these patterns.

8. **All checks pass:** `make check` passes including format, lint, typecheck, and all tests.

9. **To test Phase 4 manually:**
   ```bash
   # First run entity + relationship extraction via UI
   # Then trigger column enrichment:
   curl -X POST https://localhost:3443/api/projects/{pid}/ontology/enrich-columns \
     -H "Authorization: Bearer $TOKEN" \
     -H "Content-Type: application/json"

   # Verify via MCP get_ontology depth=columns
   ```

10. **To clear ontology data for re-testing:**
    ```sql
    DELETE FROM engine_ontology_workflows WHERE project_id = '<project-id>';
    DELETE FROM engine_ontologies WHERE project_id = '<project-id>';
    DELETE FROM engine_workflow_state WHERE project_id = '<project-id>';
    ```
