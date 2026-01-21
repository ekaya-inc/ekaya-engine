# PLAN-02: Empty Ontology on Project Creation

**Parent:** [PLAN-living-ontology-master.md](./PLAN-living-ontology-master.md)
**Dependencies:** None
**Enables:** Dev Tools Mode (Claude can use ontology MCP tools immediately)

## Goal

Create an empty ontology record when a project is provisioned, so MCP ontology tools work immediately without requiring "Extract Ontology" or AI Config.

## Current State

- `update_entity()` and other ontology tools fail with: `"no active ontology found for project"`
- Ontology only created when `OntologyDAGService.Start()` runs extraction
- Extraction requires AI Config, which isn't needed in Dev Tools mode
- Existing projects may not have ontology records

## Desired State

```
1. Create project via API
2. Connect MCP client
3. update_entity(name='User', description='...') → succeeds immediately
4. get_ontology(depth='entities') → returns { entities: [User] }
```

## Implementation

### 1. Add OntologyRepository to ProjectService

**File:** `pkg/services/projects.go`

```go
type projectService struct {
    projectRepo  repositories.ProjectRepository
    userRepo     repositories.UserRepository
    ontologyRepo repositories.OntologyRepository  // ADD
    db           *database.DB                      // ADD (for tenant context)
    logger       *slog.Logger
    // ... other fields
}

func NewProjectService(
    projectRepo repositories.ProjectRepository,
    userRepo repositories.UserRepository,
    ontologyRepo repositories.OntologyRepository,  // ADD
    db *database.DB,                                // ADD
    logger *slog.Logger,
    // ... other params
) ProjectService {
    return &projectService{
        projectRepo:  projectRepo,
        userRepo:     userRepo,
        ontologyRepo: ontologyRepo,  // ADD
        db:           db,            // ADD
        logger:       logger,
    }
}
```

### 2. Create Empty Ontology in Provision()

**File:** `pkg/services/projects.go` (in `Provision()` method, after project creation ~line 129)

```go
func (s *projectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
    // ... existing code to create project ...

    project := &models.Project{
        ProjectID:  projectID,
        Name:       name,
        Parameters: params,
        Status:     "active",
    }
    if err := s.projectRepo.Create(ctx, project); err != nil {
        return nil, fmt.Errorf("create project: %w", err)
    }

    // NEW: Create empty ontology for immediate MCP tool use
    s.createEmptyOntology(ctx, projectID)

    // ... rest of existing code ...
}

func (s *projectService) createEmptyOntology(ctx context.Context, projectID uuid.UUID) {
    emptyOntology := &models.TieredOntology{
        ProjectID:       projectID,
        Version:         1,
        IsActive:        true,
        EntitySummaries: make(map[string]*models.EntitySummary),
        ColumnDetails:   make(map[string][]models.ColumnDetail),
        Metadata:        make(map[string]any),
    }

    tenantCtx := s.db.WithTenant(ctx, projectID)
    if err := s.ontologyRepo.Create(tenantCtx, emptyOntology); err != nil {
        // Log but don't fail project creation
        s.logger.Warn("failed to create initial ontology",
            "project_id", projectID,
            "error", err,
        )
    }
}
```

### 3. Update main.go Wiring

**File:** `main.go`

Update `NewProjectService` call to include new dependencies:

```go
projectService := services.NewProjectService(
    projectRepo,
    userRepo,
    ontologyRepo,  // ADD
    db,            // ADD
    logger,
    // ... other params
)
```

### 4. Add ensureOntologyExists Helper for Backward Compatibility

For existing projects that don't have an ontology, add a helper function used by all ontology update tools.

**File:** `pkg/mcp/tools/ontology_entities.go` (or new `ontology_helpers.go`)

```go
// ensureOntologyExists returns the active ontology, creating an empty one if needed.
// This handles projects created before empty-ontology-on-creation was implemented.
func ensureOntologyExists(
    ctx context.Context,
    ontologyRepo repositories.OntologyRepository,
    projectID uuid.UUID,
) (*models.TieredOntology, error) {
    ontology, err := ontologyRepo.GetActive(ctx, projectID)
    if err != nil {
        return nil, fmt.Errorf("get active ontology: %w", err)
    }
    if ontology != nil {
        return ontology, nil
    }

    // Create empty ontology for backward compatibility
    ontology = &models.TieredOntology{
        ProjectID:       projectID,
        Version:         1,
        IsActive:        true,
        EntitySummaries: make(map[string]*models.EntitySummary),
        ColumnDetails:   make(map[string][]models.ColumnDetail),
        Metadata:        make(map[string]any),
    }
    if err := ontologyRepo.Create(ctx, ontology); err != nil {
        return nil, fmt.Errorf("create empty ontology: %w", err)
    }

    return ontology, nil
}
```

### 5. Update Ontology Update Tools

Update all ontology update tools to use `ensureOntologyExists` instead of failing.

**Files to update:**
- `pkg/mcp/tools/ontology_entities.go` - `update_entity`, `delete_entity`
- `pkg/mcp/tools/ontology_relationships.go` - `update_relationship`, `delete_relationship`
- `pkg/mcp/tools/ontology_columns.go` - `update_column`, `delete_column_metadata`

Example change pattern:

```go
// BEFORE
ontology, err := deps.OntologyRepo.GetActive(tenantCtx, projectID)
if err != nil {
    return nil, err
}
if ontology == nil {
    return NewErrorResult("ontology_not_found", "no active ontology found for project"), nil
}

// AFTER
ontology, err := ensureOntologyExists(tenantCtx, deps.OntologyRepo, projectID)
if err != nil {
    return NewErrorResult("ontology_error", err.Error()), nil
}
```

### 6. Update UI to Handle Empty Ontology

**File:** `ui/src/pages/OntologyPage.tsx`

```typescript
// Check for empty vs missing ontology
const hasOntologyContent = entities.length > 0 || relationships.length > 0;

if (!hasOntologyContent) {
    return (
        <EmptyOntologyState
            hasAIConfig={!!project.aiConfigId}
            onExtract={handleExtractOntology}
        />
    );
}

// EmptyOntologyState component shows:
// - "No entities yet" message
// - If AI Config: "Extract Ontology" button
// - If no AI Config: "Build via MCP tools or attach AI Config"
```

## Tasks

1. [x] Add `ontologyRepo` and `db` fields to `projectService` struct
2. [x] Update `NewProjectService()` signature and implementation
3. [x] Add `createEmptyOntology()` helper method
4. [x] Call `createEmptyOntology()` in `Provision()` after project creation
5. [x] Update `main.go` to pass new dependencies to `NewProjectService()`
6. [x] Add `ensureOntologyExists()` helper function
7. [x] Update `update_entity` tool to use `ensureOntologyExists()`
8. [x] Update `update_relationship` tool to use `ensureOntologyExists()`
9. [x] Update `update_column` tool to use `ensureOntologyExists()`
10. [x] Update `delete_*` tools to use `ensureOntologyExists()`
11. [ ] Add `EmptyOntologyState` component to UI
12. [ ] Update `OntologyPage` to show empty state gracefully
13. [x] Test: new project → `update_entity()` succeeds immediately
14. [x] Test: existing project without ontology → `update_entity()` creates ontology

## Testing

### New Project Flow
```
1. POST /api/projects (create new project)
2. Connect MCP client
3. update_entity(name='Test', description='Test entity')
   → Expected: succeeds, returns entity details
4. get_ontology(depth='entities')
   → Expected: { entities: [{ name: 'Test', ... }] }
```

### Existing Project Migration
```
1. Connect to project created before this change (no ontology)
2. update_entity(name='Test', description='...')
   → Expected: creates empty ontology, then adds entity, succeeds
3. get_ontology(depth='entities')
   → Expected: shows Test entity
```

### UI Empty State
```
1. Create new project via UI
2. Navigate to Ontology page
   → Expected: shows "No entities yet" with options to extract or build via MCP
3. If AI Config attached: "Extract Ontology" button visible
4. If no AI Config: informative message about MCP tools
```
