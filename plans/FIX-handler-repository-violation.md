# FIX: Handler Directly Accessing Repositories

## Problem

`pkg/handlers/ontology_enrichment_handler.go` violates the clean architecture pattern by directly importing and calling repository methods instead of delegating to services.

This breaks the layering principle: **Handlers → Services → Repositories**

## Root Cause

The handler was written with direct repository dependencies:

```go
// Line 10: Imports repository package directly
import "github.com/ekaya-inc/ekaya-engine/pkg/repositories"

// Lines 120-122: Struct holds repository interfaces
type OntologyEnrichmentHandler struct {
    ontologyRepo   repositories.OntologyRepository  // VIOLATION
    schemaRepo     repositories.SchemaRepository    // VIOLATION
    projectService services.ProjectService
    logger         *zap.Logger
}
```

The handler makes two direct repository calls:
1. **Line 171:** `h.schemaRepo.GetColumnsWithFeaturesByDatasource(ctx, projectID, datasourceID)`
2. **Line 184:** `h.ontologyRepo.GetActive(ctx, projectID)`

## Solution

Refactor the handler to use existing or new service methods.

### Step 1: Extend OntologyContextService

The `OntologyContextService` already has access to `ontologyRepo` and uses `GetActive()` in multiple places. Add a method to expose this for the handler.

**File:** `pkg/services/ontology_context.go`

Add to the interface:
```go
type OntologyContextService interface {
    // ... existing methods ...

    // GetActiveOntology returns the active ontology for a project (nil if none exists)
    GetActiveOntology(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error)
}
```

Add implementation:
```go
func (s *ontologyContextService) GetActiveOntology(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
    return s.ontologyRepo.GetActive(ctx, projectID)
}
```

### Step 2: Extend SchemaService

The `SchemaService` should expose schema column access. Check if this method already exists; if not, add it.

**File:** `pkg/services/schema.go`

Add to interface (if not present):
```go
type SchemaService interface {
    // ... existing methods ...

    // GetColumnsWithFeaturesByDatasource returns all columns with their extracted features
    GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error)
}
```

Add implementation:
```go
func (s *schemaService) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
    return s.schemaRepo.GetColumnsWithFeaturesByDatasource(ctx, projectID, datasourceID)
}
```

### Step 3: Refactor the Handler

**File:** `pkg/handlers/ontology_enrichment_handler.go`

1. Remove the repositories import (line 10)
2. Update struct to use services:

```go
type OntologyEnrichmentHandler struct {
    ontologyContextService services.OntologyContextService
    schemaService          services.SchemaService
    projectService         services.ProjectService
    logger                 *zap.Logger
}
```

3. Update constructor:

```go
func NewOntologyEnrichmentHandler(
    ontologyContextService services.OntologyContextService,
    schemaService services.SchemaService,
    projectService services.ProjectService,
    logger *zap.Logger,
) *OntologyEnrichmentHandler {
    return &OntologyEnrichmentHandler{
        ontologyContextService: ontologyContextService,
        schemaService:          schemaService,
        projectService:         projectService,
        logger:                 logger,
    }
}
```

4. Update `GetEnrichment` method:

```go
// Line 171: Change from
schemaColumnsByTable, err := h.schemaRepo.GetColumnsWithFeaturesByDatasource(r.Context(), projectID, datasourceID)
// To
schemaColumnsByTable, err := h.schemaService.GetColumnsWithFeaturesByDatasource(r.Context(), projectID, datasourceID)

// Line 184: Change from
ontology, err := h.ontologyRepo.GetActive(r.Context(), projectID)
// To
ontology, err := h.ontologyContextService.GetActiveOntology(r.Context(), projectID)
```

### Step 4: Update Dependency Wiring

Update wherever `NewOntologyEnrichmentHandler` is called (likely `main.go` or a wire file) to pass services instead of repositories.

## Files to Modify

| File | Change |
|------|--------|
| `pkg/services/ontology_context.go` | Add `GetActiveOntology` method to interface and implementation |
| `pkg/services/schema.go` | Add `GetColumnsWithFeaturesByDatasource` to interface and implementation (if not present) |
| `pkg/handlers/ontology_enrichment_handler.go` | Remove repository imports, update struct and constructor, update method calls |
| `main.go` or wiring file | Update `NewOntologyEnrichmentHandler` call to pass services |
| `pkg/handlers/ontology_enrichment_handler_test.go` | Update tests to mock services instead of repositories (if exists) |

## Verification

After making changes:

1. Run `go build ./...` to verify compilation
2. Run `make check` to run all tests
3. Verify no files in `pkg/handlers/` import `pkg/repositories` (except test files for setup):
   ```bash
   grep -r "ekaya-engine/pkg/repositories" pkg/handlers/*.go | grep -v "_test.go"
   ```
   Should return empty.

## Notes

- The `OntologyContextService` was chosen because it already has `ontologyRepo` as a dependency and calls `GetActive()` in multiple places (lines 64, 171, 271, 465)
- If `SchemaService` already has this method, Step 2 can be skipped
- Test files in `pkg/handlers/` that import repositories for test setup are acceptable
