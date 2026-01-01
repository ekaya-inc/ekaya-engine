# Plan: Fix Handler Direct Repository Access

## Problem

`pkg/handlers/entity_relationship_handler.go` violates clean architecture by calling the repository directly instead of going through the service layer.

**Location:** `entity_relationship_handler.go:130`
```go
relationships, err := h.relationshipRepo.GetByProject(r.Context(), projectID)
```

The handler has `relationshipService` (type `DeterministicRelationshipService`) injected but the `List` method bypasses it and calls `relationshipRepo.GetByProject()` directly.

This breaks the clean architecture pattern where:
- Handlers parse requests and call services
- Services contain business logic and call repositories
- Repositories handle data access

## Why This Matters

1. **No place for business logic** - If we need to add filtering, sorting, permissions, or caching, there's no service layer to put it
2. **Inconsistent pattern** - Other handlers properly use services (e.g., the `Discover` method on the same handler correctly calls `h.relationshipService.DiscoverRelationships()`)
3. **Testing difficulties** - Can't mock service behavior for handler tests
4. **Dependency confusion** - Handler shouldn't need to know about repository at all

## Solution

Add `GetByProject` method to `DeterministicRelationshipService` interface and implementation, then update the handler to use it.

## Implementation Steps

### Step 1: Add method to service interface [x]

**File:** `pkg/services/deterministic_relationship_service.go`

Add to the `DeterministicRelationshipService` interface (around line 24):

```go
type DeterministicRelationshipService interface {
    // DiscoverRelationships discovers relationships from FK constraints and PK-match
    // inference for a datasource. Requires entities to exist before calling.
    DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*DiscoveryResult, error)

    // GetByProject returns all entity relationships for a project.
    GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error)
}
```

### Step 2: Implement the method [x]

**File:** `pkg/services/deterministic_relationship_service.go`

Add implementation after the `DiscoverRelationships` method (around line 392):

```go
func (s *deterministicRelationshipService) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
    return s.relationshipRepo.GetByProject(ctx, projectID)
}
```

Note: This is intentionally thin - it just delegates to the repository. This is fine because:
- It establishes the proper layering
- Future business logic (filtering, permissions, etc.) has a home
- It's consistent with other services that start thin and grow

### Step 3: Update handler to remove repository dependency [x]

**File:** `pkg/handlers/entity_relationship_handler.go`

Remove the `relationshipRepo` field from the handler struct (lines 58-62):

```go
// BEFORE
type EntityRelationshipHandler struct {
    relationshipService services.DeterministicRelationshipService
    relationshipRepo    repositories.EntityRelationshipRepository  // REMOVE THIS
    logger              *zap.Logger
}

// AFTER
type EntityRelationshipHandler struct {
    relationshipService services.DeterministicRelationshipService
    logger              *zap.Logger
}
```

### Step 4: Update handler constructor [x]

**File:** `pkg/handlers/entity_relationship_handler.go`

Update `NewEntityRelationshipHandler` (lines 65-75):

```go
// BEFORE
func NewEntityRelationshipHandler(
    relationshipService services.DeterministicRelationshipService,
    relationshipRepo repositories.EntityRelationshipRepository,  // REMOVE THIS PARAMETER
    logger *zap.Logger,
) *EntityRelationshipHandler {
    return &EntityRelationshipHandler{
        relationshipService: relationshipService,
        relationshipRepo:    relationshipRepo,  // REMOVE THIS LINE
        logger:              logger,
    }
}

// AFTER
func NewEntityRelationshipHandler(
    relationshipService services.DeterministicRelationshipService,
    logger *zap.Logger,
) *EntityRelationshipHandler {
    return &EntityRelationshipHandler{
        relationshipService: relationshipService,
        logger:              logger,
    }
}
```

### Step 5: Update List method to use service [x]

**File:** `pkg/handlers/entity_relationship_handler.go`

Update the `List` method (around line 130):

```go
// BEFORE
relationships, err := h.relationshipRepo.GetByProject(r.Context(), projectID)

// AFTER
relationships, err := h.relationshipService.GetByProject(r.Context(), projectID)
```

### Step 6: Update main.go to remove repository from handler construction [x]

**File:** `main.go`

Find where `NewEntityRelationshipHandler` is called and remove the repository argument.

Search for the call pattern and update:

```go
// BEFORE (somewhere in main.go)
entityRelationshipHandler := handlers.NewEntityRelationshipHandler(
    deterministicRelationshipService,
    entityRelationshipRepo,  // REMOVE THIS ARGUMENT
    logger,
)

// AFTER
entityRelationshipHandler := handlers.NewEntityRelationshipHandler(
    deterministicRelationshipService,
    logger,
)
```

### Step 7: Verify imports [x]

After removing the repository from the handler:
- Remove `"github.com/ekaya-inc/ekaya-engine/pkg/repositories"` import from `entity_relationship_handler.go` if no longer needed
- Verify main.go still compiles (may no longer need to create `entityRelationshipRepo` if nothing else uses it)

**Result:** Verified complete:
- `entity_relationship_handler.go` no longer imports repositories (already removed in step 3)
- `main.go` compiles successfully
- `entityRelationshipRepo` still needed in main.go because `NewDeterministicRelationshipService` uses it

## Testing [x]

1. Run `make check` to ensure everything compiles and passes ✓
2. The existing integration tests in `pkg/handlers/entity_integration_test.go` should continue to pass ✓
3. Verify the `/api/projects/{pid}/relationships` endpoint still returns the same data ✓

## Files Changed

| File | Change |
|------|--------|
| `pkg/services/deterministic_relationship_service.go` | Add `GetByProject` to interface and implementation |
| `pkg/handlers/entity_relationship_handler.go` | Remove repo dependency, use service |
| `main.go` | Update handler construction |

## Estimated Scope

- ~10 lines of new service code
- ~10 lines of handler changes
- ~2 lines of main.go changes
- Low risk - straightforward refactor with no behavior change
