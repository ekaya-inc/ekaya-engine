# MCP get_ontology - Complete Implementation Plan

Investigation date: 2026-01-02

## Executive Summary

The `get_ontology` MCP tool returns incomplete data because:
1. The service reads from JSONB columns that were never populated
2. Rich data exists in normalized tables but isn't being used
3. Some data requires additions to Entity Extraction workflow
4. Column-level semantics require a separate (deferred) Column workflow

This plan details how to make `get_ontology` fully functional by:
- Modifying the service to read from normalized tables
- Adding missing fields to Entity Extraction
- Creating an Ontology Finalization step
- Deferring expensive column-level semantic extraction

---

## Current Data Inventory

### What We Have (Normalized Tables)

| Table | Data | Count | Quality |
|-------|------|-------|---------|
| `engine_ontology_entities` | Entity name, description, primary_table | 38 | Rich LLM-generated descriptions |
| `engine_ontology_entity_occurrences` | Where entities appear (table/column) | 38 | Complete |
| `engine_ontology_entity_aliases` | Entity synonyms | 0 | Structure exists, not populated |
| `engine_entity_relationships` | Entity-to-entity relationships | 34 | FK + PK-match methods |
| `engine_schema_tables` | Table metadata (row_count, schema) | 38 per datasource | Structural only |
| `engine_schema_columns` | Column metadata (type, is_pk, is_joinable) | ~150 per datasource | Structural only |
| `engine_schema_relationships` | FK constraints | varies | Complete |

### What We Don't Have

| Data | Needed For | Solution |
|------|------------|----------|
| Entity domain categorization | `tables` depth | Add to Entity Extraction |
| Entity key columns | `entities`/`tables` depth | Add to Entity Extraction |
| Entity synonyms | `entities`/`tables` depth | Populate during Entity Extraction |
| Domain description | `domain` depth | Ontology Finalization (LLM) |
| Primary domains list | `domain` depth | Aggregate from entity domains |
| Column descriptions | `columns` depth | Deferred - Column Workflow |
| Column semantic types/roles | `columns` depth | Deferred - Column Workflow |

### JSONB Columns (Currently Empty)

| Column | Status | Decision |
|--------|--------|----------|
| `domain_summary` | NULL | Keep - populate during Ontology Finalization |
| `entity_summaries` | `{}` | Keep - optional denormalization cache |
| `column_details` | `{}` | Keep - populate during Column Workflow |

---

## Current Code State

### Service: OntologyContextService

**File:** `pkg/services/ontology_context.go`

**Current struct:**
```go
type ontologyContextService struct {
    ontologyRepo repositories.OntologyRepository
    entityRepo   repositories.OntologyEntityRepository
    schemaRepo   repositories.SchemaRepository
    logger       *zap.Logger
}
```

**Required struct (after Phase 1):**
```go
type ontologyContextService struct {
    ontologyRepo     repositories.OntologyRepository
    entityRepo       repositories.OntologyEntityRepository
    relationshipRepo repositories.EntityRelationshipRepository  // NEW
    schemaRepo       repositories.SchemaRepository
    projectService   services.ProjectService                    // NEW - for GetDefaultDatasourceID
    logger           *zap.Logger
}
```

### Repository Methods - What Exists vs Needed

**EntityRelationshipRepository** (`pkg/repositories/entity_relationship_repository.go`):
| Method | Status |
|--------|--------|
| `GetByProject(ctx, projectID)` | ✓ EXISTS |
| `GetByTables(ctx, projectID, tableNames)` | ✗ NEEDS ADDING |

**SchemaRepository** (`pkg/repositories/schema_repository.go`):
| Method | Status | Notes |
|--------|--------|-------|
| `ListTablesByDatasource(ctx, projectID, datasourceID)` | ✓ EXISTS | Need datasourceID |
| `ListColumnsByTable(ctx, projectID, tableID)` | ✓ EXISTS | Per-table |
| `ListColumnsByDatasource(ctx, projectID, datasourceID)` | ✓ EXISTS | All columns |
| `GetColumnsByTables(ctx, projectID, tableNames)` | ✗ NEEDS ADDING | Convenience method |
| `GetColumnCountByProject(ctx, projectID)` | ✗ NEEDS ADDING | Or derive from existing |

**Note:** Existing methods require `datasourceID`. Use `ProjectService.GetDefaultDatasourceID(ctx, projectID)` to get it. The service will also need `projectService services.ProjectService` as a dependency.

**OntologyEntityRepository** (`pkg/repositories/ontology_entity_repository.go`):
| Method | Status |
|--------|--------|
| `GetByProject(ctx, projectID)` | ✓ EXISTS |
| `GetAliasesByEntity(ctx, entityID)` | ✓ EXISTS |
| `GetAllOccurrencesByProject(ctx, projectID)` | ✓ EXISTS |

### Dependency Injection

**File:** `main.go`

The `entityRelationshipRepo` is already created at line 143:
```go
entityRelationshipRepo := repositories.NewEntityRelationshipRepository()
```

The `OntologyContextService` is created at line 207:
```go
ontologyContextService := services.NewOntologyContextService(ontologyRepo, ontologyEntityRepo, schemaRepo, logger)
```

**To add relationshipRepo:**
1. Update struct in `pkg/services/ontology_context.go`
2. Update `NewOntologyContextService` constructor signature
3. Update line 207 in `main.go` to pass `entityRelationshipRepo`

### Test Patterns

**Unit tests:** Use mocks (see `pkg/services/ontology_context_test.go` for mock patterns)
**Integration tests:** Use `testhelpers.GetEngineDB(t)` for database access

### Database Access Patterns

- All queries must use tenant-scoped context via `database.SetTenantScope(ctx, scope)`
- Get tenant scope via `db.WithTenant(ctx, projectID)`
- Always `defer scope.Close()` after acquiring

---

## Implementation Plan

### Phase 1: Service Layer - Read from Normalized Tables

**Goal:** Make `get_ontology` work with existing data by reading from normalized tables instead of empty JSONB.

#### 1.1 Update GetDomainContext

**File:** `pkg/services/ontology_context.go`

**Current behavior:** Reads `DomainInfo` from `ontology.DomainSummary` (NULL) and counts from JSONB columns.

**Changes:**
```go
func (s *ontologyContextService) GetDomainContext(ctx context.Context, projectID uuid.UUID) (*models.OntologyDomainContext, error) {
    // Get entities from normalized table (already doing this)
    entities, err := s.entityRepo.GetByProject(ctx, projectID)

    // Get relationships from engine_entity_relationships (NEW)
    relationships, err := s.relationshipRepo.GetByProject(ctx, projectID)

    // Calculate counts from actual data (NEW)
    tableCount := len(entities)
    columnCount := s.schemaRepo.GetColumnCountByProject(ctx, projectID)

    // Build relationship graph from normalized data (NEW)
    relationshipEdges := transformRelationshipsToEdges(relationships, entities)

    // Domain description - use placeholder until Ontology Finalization exists
    domainDescription := ""
    if ontology.DomainSummary != nil {
        domainDescription = ontology.DomainSummary.Description
    }

    return &models.OntologyDomainContext{
        Domain: DomainInfo{
            Description:    domainDescription,
            PrimaryDomains: aggregateDomainsFromEntities(entities), // NEW
            TableCount:     tableCount,
            ColumnCount:    columnCount,
        },
        Entities:      entityBriefs,
        Relationships: relationshipEdges,
    }, nil
}
```

**New helper functions needed:**
- `transformRelationshipsToEdges(relationships, entities)` - converts `engine_entity_relationships` to `[]RelationshipEdge`
- `aggregateDomainsFromEntities(entities)` - collects unique domains (returns empty until Phase 2)

#### 1.2 Update GetTablesContext

**File:** `pkg/services/ontology_context.go`

**Current behavior:** Reads from `ontology.EntitySummaries` (empty `{}`), returns empty.

**Changes:**
```go
func (s *ontologyContextService) GetTablesContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyTablesContext, error) {
    // Get entities for table info (NEW - replaces JSONB read)
    entities, err := s.entityRepo.GetByProject(ctx, projectID)
    entityByTable := indexEntitiesByTable(entities)

    // Get schema tables for structural metadata (NEW)
    schemaTables, err := s.schemaRepo.GetTablesByProject(ctx, projectID, tableNames)

    // Get schema columns for column overview (NEW)
    schemaColumns, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames)

    // Get relationships for this table (NEW)
    relationships, err := s.relationshipRepo.GetByTables(ctx, projectID, tableNames)

    tables := make(map[string]models.TableSummary)
    for _, schemaTable := range schemaTables {
        entity := entityByTable[schemaTable.TableName]
        columns := schemaColumns[schemaTable.TableName]

        tables[schemaTable.TableName] = models.TableSummary{
            Schema:       schemaTable.SchemaName,
            BusinessName: entity.Name,                    // Use entity name
            Description:  entity.Description,             // From engine_ontology_entities
            Domain:       "",                             // Empty until Phase 2
            RowCount:     schemaTable.RowCount,
            ColumnCount:  len(columns),
            Synonyms:     getEntityAliases(entity.ID),    // From engine_ontology_entity_aliases
            Columns:      buildColumnOverview(columns),   // From engine_schema_columns
            Relationships: buildTableRelationships(relationships, schemaTable.TableName),
        }
    }

    return &models.OntologyTablesContext{Tables: tables}, nil
}
```

**New helper functions needed:**
- `indexEntitiesByTable(entities)` - creates map[tableName]entity
- `buildColumnOverview(schemaColumns)` - converts schema columns to `[]ColumnOverview`
- `buildTableRelationships(relationships, tableName)` - filters relationships for table

#### 1.3 Update GetColumnsContext

**File:** `pkg/services/ontology_context.go`

**Current behavior:** Reads from `ontology.ColumnDetails` (empty `{}`), returns empty.

**Changes:**
```go
func (s *ontologyContextService) GetColumnsContext(ctx context.Context, projectID uuid.UUID, tableNames []string) (*models.OntologyColumnsContext, error) {
    // Validation (keep existing)
    if len(tableNames) == 0 {
        return nil, fmt.Errorf("table names required for columns depth")
    }

    // Get entities for table info (NEW)
    entities, err := s.entityRepo.GetByProject(ctx, projectID)
    entityByTable := indexEntitiesByTable(entities)

    // Get schema columns with full detail (NEW)
    schemaColumns, err := s.schemaRepo.GetColumnsByTables(ctx, projectID, tableNames)

    // Get relationships to determine FK info (NEW)
    relationships, err := s.relationshipRepo.GetByTables(ctx, projectID, tableNames)
    fkInfo := buildFKInfo(relationships)

    tables := make(map[string]models.TableDetail)
    for _, tableName := range tableNames {
        entity := entityByTable[tableName]
        columns := schemaColumns[tableName]

        columnDetails := make([]models.ColumnDetail, 0, len(columns))
        for _, col := range columns {
            columnDetails = append(columnDetails, models.ColumnDetail{
                Name:         col.ColumnName,
                Description:  col.Description,              // Empty until Column Workflow
                Synonyms:     nil,                          // Empty until Column Workflow
                SemanticType: "",                           // Empty until Column Workflow
                Role:         "",                           // Empty until Column Workflow
                EnumValues:   nil,                          // Empty until Column Workflow
                IsPrimaryKey: col.IsPrimaryKey,
                IsForeignKey: fkInfo[tableName][col.ColumnName] != nil,
                ForeignTable: getForeignTable(fkInfo, tableName, col.ColumnName),
            })
        }

        tables[tableName] = models.TableDetail{
            Schema:       "public",
            BusinessName: entity.Name,
            Description:  entity.Description,
            Columns:      columnDetails,
        }
    }

    return &models.OntologyColumnsContext{Tables: tables}, nil
}
```

#### 1.4 Add Repository Methods

**File:** `pkg/repositories/entity_relationship_repository.go`

```go
// GetByProject returns all relationships for a project's active ontology.
GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error)

// GetByTables returns relationships involving specific tables.
GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error)
```

**File:** `pkg/repositories/schema_repository.go`

```go
// GetColumnCountByProject returns total column count for selected tables.
GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error)

// GetColumnsByTables returns columns grouped by table name.
GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error)
```

#### 1.5 Tests for Phase 1

**File:** `pkg/services/ontology_context_integration_test.go`

```go
func TestGetDomainContext_ReturnsRelationshipsFromNormalizedTable(t *testing.T) {
    // Setup: project with entities and relationships in normalized tables
    // Assert: relationships are returned even when domain_summary JSONB is NULL
}

func TestGetTablesContext_ReturnsDataFromNormalizedTables(t *testing.T) {
    // Setup: project with entities in engine_ontology_entities
    // Assert: tables are returned even when entity_summaries JSONB is empty
}

func TestGetColumnsContext_ReturnsStructuralData(t *testing.T) {
    // Setup: project with schema_columns data
    // Assert: column structural data (name, type, is_pk) is returned
    // Assert: semantic fields are empty (description, role, etc.)
}

func TestGetDomainContext_CalculatesCountsFromNormalizedData(t *testing.T) {
    // Assert: TableCount equals entity count
    // Assert: ColumnCount equals schema_columns count
}
```

---

### Phase 2: Entity Extraction Additions

**Goal:** Add missing fields during Entity Extraction so `tables` depth has complete data.

#### 2.1 Add Domain Categorization

**When:** During entity extraction LLM call

**Prompt addition:**
```
For each entity, categorize it into one of these business domains:
- billing, customer, marketing, operations, product, sales, analytics, hr, inventory, unknown

Return as: "domain": "billing"
```

**Schema changes:**
```sql
ALTER TABLE engine_ontology_entities ADD COLUMN domain VARCHAR(50);
```

**Model changes:**
```go
// pkg/models/ontology_entity.go
type OntologyEntity struct {
    // ... existing fields
    Domain string `json:"domain"` // NEW: business domain categorization
}
```

#### 2.2 Add Key Columns Identification

**When:** During entity extraction LLM call

**Prompt addition:**
```
Identify 2-3 key business columns for this entity (not id/timestamps).
These are columns that business users would query on.

Return as: "key_columns": [{"name": "email", "synonyms": ["email_address"]}]
```

**Schema changes:**
```sql
CREATE TABLE engine_ontology_entity_key_columns (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_id UUID NOT NULL REFERENCES engine_ontology_entities(id) ON DELETE CASCADE,
    column_name TEXT NOT NULL,
    synonyms JSONB DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(entity_id, column_name)
);
```

#### 2.3 Populate Entity Aliases

**When:** During entity extraction LLM call

**Prompt addition:**
```
List alternative names/synonyms for this entity.
Example: "User" might have synonyms ["customer", "account", "member"]

Return as: "synonyms": ["customer", "account"]
```

**Note:** Table `engine_ontology_entity_aliases` already exists, just need to populate it.

#### 2.4 Tests for Phase 2

```go
func TestEntityExtraction_PopulatesDomain(t *testing.T) {
    // After entity extraction, entity.Domain should be set
}

func TestEntityExtraction_PopulatesKeyColumns(t *testing.T) {
    // After entity extraction, key columns should exist
}

func TestEntityExtraction_PopulatesAliases(t *testing.T) {
    // After entity extraction, aliases should exist
}
```

---

### Phase 3: Ontology Finalization

**Goal:** Generate domain-level summary after Entity and Relationship extraction complete.

#### 3.1 Create Ontology Finalization Service

**File:** `pkg/services/ontology_finalization.go`

```go
type OntologyFinalizationService interface {
    // Finalize generates domain-level summary after extractions complete.
    Finalize(ctx context.Context, projectID uuid.UUID) error
}

func (s *ontologyFinalizationService) Finalize(ctx context.Context, projectID uuid.UUID) error {
    // 1. Get all entities
    entities, _ := s.entityRepo.GetByProject(ctx, projectID)

    // 2. Get all relationships
    relationships, _ := s.relationshipRepo.GetByProject(ctx, projectID)

    // 3. Generate domain description via LLM
    domainDescription := s.generateDomainDescription(ctx, entities, relationships)

    // 4. Aggregate primary domains from entities
    primaryDomains := s.aggregateDomains(entities)

    // 5. Build relationship graph
    relationshipGraph := s.buildRelationshipGraph(relationships, entities)

    // 6. Save to domain_summary JSONB
    domainSummary := &models.DomainSummary{
        Description:       domainDescription,
        Domains:           primaryDomains,
        RelationshipGraph: relationshipGraph,
    }

    return s.ontologyRepo.UpdateDomainSummary(ctx, projectID, domainSummary)
}
```

#### 3.2 LLM Prompt for Domain Description

```
You are analyzing a database schema. Based on the following entities and their relationships,
provide a 2-3 sentence business description of what this database represents.

Entities:
{{range .Entities}}
- {{.Name}}: {{.Description}}
{{end}}

Key Relationships:
{{range .Relationships}}
- {{.SourceEntity}} → {{.TargetEntity}} ({{.Description}})
{{end}}

Provide a concise business summary:
```

#### 3.3 Trigger Finalization

**Option A:** Automatic after relationship extraction completes
```go
// In relationship_workflow.go after SaveRelationships
func (s *relationshipWorkflowService) SaveRelationships(...) (int, error) {
    // ... existing save logic

    // Trigger ontology finalization
    if err := s.finalizationService.Finalize(ctx, projectID); err != nil {
        s.logger.Error("Ontology finalization failed", zap.Error(err))
        // Non-fatal - don't fail the workflow
    }

    return count, nil
}
```

**Option B:** Manual API endpoint
```go
// POST /api/projects/{id}/ontology/finalize
```

#### 3.4 Tests for Phase 3

```go
func TestOntologyFinalization_GeneratesDomainDescription(t *testing.T) {
    // After finalization, domain_summary.description should be populated
}

func TestOntologyFinalization_AggregatesDomains(t *testing.T) {
    // After finalization, domain_summary.domains should contain unique domains
}

func TestOntologyFinalization_BuildsRelationshipGraph(t *testing.T) {
    // After finalization, domain_summary.relationship_graph should have edges
}
```

---

### Phase 4: Column Workflow (Deferred)

**Goal:** Generate semantic column information. This is expensive (LLM call per column) and can be deferred.

#### 4.1 Scope

| Field | Source | Cost |
|-------|--------|------|
| Column description | LLM per column | High |
| Column synonyms | LLM per column | High |
| Column semantic type | LLM per column | High |
| Column role | LLM per column | High |
| Enum values | Data sampling + LLM | High |

#### 4.2 Implementation Approach

- Batch columns by table (reduce LLM calls)
- Use workflow state (`engine_workflow_state`) to track progress
- Store in `column_details` JSONB column
- Optional: Store in normalized table for flexibility

#### 4.3 Priority

**Defer until:**
- Phases 1-3 are complete and tested
- User feedback indicates `columns` depth is needed
- Performance/cost tradeoffs are understood

---

## Summary: What Each Phase Delivers

| Depth | After Phase 1 | After Phase 2 | After Phase 3 | After Phase 4 |
|-------|---------------|---------------|---------------|---------------|
| `domain` | Entities ✓, Relationships ✓, Counts ✓ | + PrimaryDomains | + Description | - |
| `entities` | All fields ✓ | + KeyColumns, Synonyms | - | - |
| `tables` | BusinessName ✓, Description ✓, Columns (structural) ✓ | + Domain, Synonyms | - | - |
| `columns` | Structural only (name, type, PK, FK) | - | - | + Semantic fields |

---

## Migration Path

### Step 1: Phase 1 (Service Layer)
- No schema changes
- No workflow changes
- Just read from existing normalized tables
- **Immediate improvement:** `domain`, `entities`, `tables` depths return data

### Step 2: Phase 2 (Entity Extraction)
- Schema migration for `domain` column
- New table for key columns
- Prompt changes for entity extraction
- **Requires:** Re-run entity extraction for existing projects

### Step 3: Phase 3 (Ontology Finalization)
- New service
- Trigger after relationship extraction
- **One-time:** Run finalization for existing projects

### Step 4: Phase 4 (Column Workflow)
- New workflow
- **Optional:** Only if `columns` depth semantic data is needed

---

## Testing Strategy

### Unit Tests
- Mock repositories, verify service transforms data correctly
- Test edge cases (empty data, missing relationships)

### Integration Tests
- Test full flow: normalized tables → service → response
- Verify data consistency

### End-to-End Tests
- Call MCP tool, verify response structure
- Test with real project data

### Test Data Requirements
- Project with populated `engine_ontology_entities`
- Project with populated `engine_entity_relationships`
- Project with populated `engine_schema_tables` and `engine_schema_columns`
