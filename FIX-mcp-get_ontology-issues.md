# MCP get_ontology - Complete Implementation Plan

Investigation date: 2026-01-02
**Last updated:** 2026-01-02 (Phase 2 complete)

## Status

| Phase | Status | Date Completed | Notes |
|-------|--------|----------------|-------|
| Phase 1: Service Layer | ✅ **COMPLETE** | 2026-01-02 | Reads from normalized tables |
| Phase 2: Entity Extraction | ✅ **COMPLETE** | 2026-01-02 | Domain, key columns (with synonyms), aliases |
| Phase 3: Ontology Finalization | ⏸️ Planned | - | Domain description via LLM |
| Phase 4: Column Workflow | ⏸️ Deferred | - | Column-level semantics |

---

## What Works Now (After Phase 1 + 2)

| Depth | Data Returned | Source |
|-------|---------------|--------|
| `domain` | Entity count, column count, relationship graph | Normalized tables |
| `entities` | Names, descriptions, occurrences, aliases, **key columns with synonyms** | Normalized tables |
| `tables` | Business names, descriptions, columns, relationships, aliases, **domain** | Normalized tables |
| `columns` | Structural metadata (name, type, PK, FK) | Normalized tables |

**What's Still Missing:**
- ⏸️ Domain-level description (requires Phase 3: Ontology Finalization)
- ⏸️ Primary domains aggregation in `domain` depth (requires Phase 3)
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

## Phase 3: Ontology Finalization (NEXT)

**Goal:** Generate domain-level summary after Entity and Relationship extraction complete.

### Existing Infrastructure (No Changes Needed)
- `models.DomainSummary` struct - `pkg/models/ontology.go:15`
- `ontologyRepo.UpdateDomainSummary()` - `pkg/repositories/ontology_repository.go:153`
- `GetDomainContext` reads from `ontology.DomainSummary` - `pkg/services/ontology_context.go:112-114`
- Reference patterns in `pkg/services/ontology_builder.go` (see `buildDomainSummaryFromEntities`)

### What Phase 3 Will Add

| Data | Where Used | Implementation |
|------|------------|----------------|
| Domain description | `domain` depth response | LLM-generated 2-3 sentence summary |
| Primary domains list | `domain` depth response | Aggregated unique domains from entities |

### Implementation Steps

1. **Create Ontology Finalization Service**
   - New file: `pkg/services/ontology_finalization.go`
   - Interface: `OntologyFinalizationService` with `Finalize(ctx, projectID)` method

2. **Implement Finalization Logic**
   ```go
   func (s *ontologyFinalizationService) Finalize(ctx context.Context, projectID uuid.UUID) error {
       // 1. Get all entities (with their domains now populated from Phase 2)
       entities, _ := s.entityRepo.GetByProject(ctx, projectID)

       // 2. Get all relationships
       relationships, _ := s.relationshipRepo.GetByProject(ctx, projectID)

       // 3. Aggregate primary domains from entity.Domain fields
       primaryDomains := s.aggregateUniqueDomains(entities)

       // 4. Generate domain description via LLM
       domainDescription := s.generateDomainDescription(ctx, entities, relationships)

       // 5. Save to domain_summary JSONB
       domainSummary := &models.DomainSummary{
           Description:       domainDescription,
           Domains:           primaryDomains,
           RelationshipGraph: s.buildRelationshipGraph(relationships, entities),
       }

       return s.ontologyRepo.UpdateDomainSummary(ctx, projectID, domainSummary)
   }
   ```

3. **LLM Prompt for Domain Description**
   ```
   You are analyzing a database schema. Based on the following entities and their relationships,
   provide a 2-3 sentence business description of what this database represents.

   Entities:
   {{range .Entities}}
   - {{.Name}} ({{.Domain}}): {{.Description}}
   {{end}}

   Key Relationships:
   {{range .Relationships}}
   - {{.SourceEntity}} → {{.TargetEntity}} ({{.Description}})
   {{end}}

   Provide a concise business summary:
   ```

4. **Trigger Options**
   - **Option A (Recommended):** Automatic after relationship extraction completes
   - **Option B:** Manual API endpoint `POST /api/projects/{id}/ontology/finalize`

5. **Update GetDomainContext**
   - Currently reads from `ontology.DomainSummary.Description` (which is NULL)
   - After Phase 3, this will be populated

6. **Wire Dependencies in main.go**

### Files to Create/Modify
- NEW: `pkg/services/ontology_finalization.go` - Finalization service
- MODIFY: `pkg/services/relationship_workflow.go` (if auto-trigger) OR
- NEW: `pkg/handlers/ontology_handler.go` (if manual endpoint)
- MODIFY: `main.go` - Wire dependencies

### Tests to Add
```go
func TestOntologyFinalization_GeneratesDomainDescription(t *testing.T)
func TestOntologyFinalization_AggregatesDomains(t *testing.T)
func TestOntologyFinalization_BuildsRelationshipGraph(t *testing.T)
```

---

## Phase 4: Column Workflow (Deferred)

**Goal:** Generate semantic column information. This is expensive (LLM call per column) and can be deferred.

**Defer until:**
- Phases 1-3 are complete and tested
- User feedback indicates `columns` depth semantic data is needed
- Performance/cost tradeoffs are understood

### Scope
| Field | Source | Cost |
|-------|--------|------|
| Column description | LLM per column | High |
| Column synonyms | LLM per column | High |
| Column semantic type | LLM per column | High |
| Column role | LLM per column | High |
| Enum values | Data sampling + LLM | High |

### Implementation Approach
- Batch columns by table (reduce LLM calls)
- Use workflow state (`engine_workflow_state`) to track progress
- Store in `column_details` JSONB column or normalized table

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

---

## Important Notes for Future Sessions

1. **Existing projects need re-extraction:** Entity extraction must be re-run for existing projects to populate `domain`, key columns, and aliases. The migration only adds the columns - it doesn't populate data.

2. **Domain is free-form:** We chose free-form domain classification (not a fixed enum) to allow domain-specific terminology like "hospitality", "fintech", etc.

3. **Key columns stored in normalized table with synonyms:** Key columns are stored in `engine_ontology_entity_key_columns` with per-column synonyms in JSONB. The LLM generates synonyms for each key column during entity extraction.

4. **Test mocks updated:** Both `ontology_context_test.go` and `relationship_workflow_test.go` have updated mocks with the new repository methods. Any new tests should follow these patterns.

5. **All checks pass:** `make check` passes including format, lint, typecheck, and all tests.
