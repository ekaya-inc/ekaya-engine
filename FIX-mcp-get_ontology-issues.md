# MCP get_ontology - Complete Implementation Plan

Investigation date: 2026-01-02
**Last updated:** 2026-01-02 (Phase 3 complete)

## Status

| Phase | Status | Date Completed | Notes |
|-------|--------|----------------|-------|
| Phase 1: Service Layer | ✅ **COMPLETE** | 2026-01-02 | Reads from normalized tables |
| Phase 2: Entity Extraction | ✅ **COMPLETE** | 2026-01-02 | Domain, key columns (with synonyms), aliases |
| Phase 3: Ontology Finalization | ✅ **COMPLETE** | 2026-01-02 | Domain description via LLM, auto-triggered |
| Phase 4: Column Workflow | ⏸️ Deferred | - | Column-level semantics |

---

## What Works Now (After Phase 1 + 2 + 3)

| Depth | Data Returned | Source |
|-------|---------------|--------|
| `domain` | Entity count, column count, relationship graph, **description**, **primary domains** | Normalized tables + DomainSummary |
| `entities` | Names, descriptions, occurrences, aliases, **key columns with synonyms** | Normalized tables |
| `tables` | Business names, descriptions, columns, relationships, aliases, **domain** | Normalized tables |
| `columns` | Structural metadata (name, type, PK, FK) | Normalized tables |

**What's Still Missing:**
- ⏸️ Column semantic fields (requires Phase 4: Column Workflow - deferred)

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

## Phase 4: Column Workflow (Deferred)

**Goal:** Generate semantic column information via LLM.

**Defer until:**
- User feedback indicates `columns` depth semantic data is needed
- Performance/cost tradeoffs are understood (expensive: LLM call per table)

### What's Missing in `columns` Depth

Currently `GetColumnsContext` returns only structural metadata from `engine_schema_columns`:

| Field | Current | Phase 4 Would Add |
|-------|---------|-------------------|
| Name | ✅ | - |
| Type | ✅ | - |
| Is Primary Key | ✅ | - |
| Is Foreign Key | ✅ | - |
| Description | ❌ | LLM-generated description |
| Synonyms | ❌ | Alternative names for column |
| Semantic Type | ❌ | e.g., "email", "phone", "currency" |
| Role | ❌ | e.g., "identifier", "timestamp", "status" |
| Enum Values | ❌ | For status/type columns |

### Existing Infrastructure

**Model exists:** `models.ColumnDetail` in `pkg/models/ontology.go`
```go
type ColumnDetail struct {
    Name         string   `json:"name"`
    Type         string   `json:"type"`
    Description  string   `json:"description"`
    Synonyms     []string `json:"synonyms,omitempty"`
    SemanticType string   `json:"semantic_type,omitempty"`
    Role         string   `json:"role,omitempty"`
    EnumValues   []string `json:"enum_values,omitempty"`
    IsPrimaryKey bool     `json:"is_primary_key"`
    IsForeignKey bool     `json:"is_foreign_key"`
}
```

**Storage:** `engine_ontologies.column_details` JSONB column (currently empty)

**Repository method exists:** `ontologyRepo.UpdateColumnDetails(ctx, projectID, tableName, columns)`

### Implementation Approach

1. **Create Column Enrichment Service**
   - New file: `pkg/services/column_enrichment.go`
   - Batch columns by table to reduce LLM calls
   - One LLM call per table, not per column

2. **LLM Prompt Pattern** (similar to entity enrichment)
   ```
   Table: orders
   Columns: id, user_id, status, total_amount, created_at, ...

   For each column, provide:
   - Description (1 sentence)
   - Synonyms (alternative names users might use)
   - Semantic type (email, phone, currency, date, status, identifier, etc.)
   - Role (primary_key, foreign_key, timestamp, status, amount, etc.)
   - Enum values (if applicable, e.g., status: ["pending", "completed", "cancelled"])
   ```

3. **Trigger Options**
   - **Option A:** Manual endpoint `POST /api/projects/{id}/ontology/enrich-columns`
   - **Option B:** Auto-trigger after entity extraction (expensive)
   - **Option C:** On-demand per table when `columns` depth requested

4. **Progress Tracking**
   - Use `engine_workflow_state` table (existing)
   - Track per-table completion status

### Cost Considerations

| Tables | Estimated LLM Calls | Tokens (approx) |
|--------|---------------------|-----------------|
| 10 | 10 | ~5,000 |
| 50 | 50 | ~25,000 |
| 100 | 100 | ~50,000 |

Consider:
- Caching results (columns rarely change)
- Incremental enrichment (only new tables)
- User-triggered vs automatic

### Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `pkg/services/column_enrichment.go` | NEW | Column enrichment service |
| `pkg/services/ontology_context.go` | MODIFY | Read from column_details JSONB |
| `pkg/handlers/ontology_handler.go` | MODIFY | Add endpoint if manual trigger |
| `main.go` | MODIFY | Wire new service |

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
| `UpdateColumnDetails` | ✅ | Ready for Phase 4 |

---

## Important Notes for Future Sessions

1. **Existing projects need re-extraction:** Entity extraction must be re-run for existing projects to populate `domain`, key columns, and aliases. The migration only adds the columns - it doesn't populate data.

2. **Domain is free-form:** We chose free-form domain classification (not a fixed enum) to allow domain-specific terminology like "hospitality", "fintech", etc.

3. **Key columns stored in normalized table with synonyms:** Key columns are stored in `engine_ontology_entity_key_columns` with per-column synonyms in JSONB. The LLM generates synonyms for each key column during entity extraction.

4. **Phase 3 auto-triggers:** Ontology finalization runs automatically after relationship workflow completes. It's non-blocking - workflow still marked complete even if finalization fails.

5. **Test mocks updated:** `ontology_context_test.go`, `relationship_workflow_test.go`, and `ontology_finalization_test.go` have updated mocks. Any new tests should follow these patterns.

6. **All checks pass:** `make check` passes including format, lint, typecheck, and all tests.

7. **To test Phase 3 manually:** Clear ontology tables for a project, then run entity + relationship extraction. Finalization should auto-trigger and populate `domain_summary`.

```sql
-- Clear ontology data for a project
DELETE FROM engine_ontology_workflows WHERE project_id = '<project-id>';
DELETE FROM engine_ontologies WHERE project_id = '<project-id>';
DELETE FROM engine_workflow_state WHERE project_id = '<project-id>';
```
