# PLAN-05: Ontology Change Queue & Precedence

**Parent:** [PLAN-living-ontology-master.md](./PLAN-living-ontology-master.md)
**Dependencies:** PLAN-03 (Schema Change Detection), PLAN-04 (Data Change Detection)
**Enables:** PLAN-06 (Incremental DAG Execution)

## Goal

Provide a review/approve workflow for pending ontology changes with a clear precedence model: Admin > Claude MCP > Engine inference.

## Current State

- No pending changes queue
- Ontology updates applied immediately without review
- No provenance tracking on ontology elements
- No precedence rules

## Desired State

```
1. Changes detected (schema or data) → queued as pending
2. Claude or Admin reviews via MCP tool or UI
3. Approve/reject individual changes
4. Approved changes applied respecting precedence
5. All ontology elements track who created/updated them
```

## Precedence Model

| Priority | Source | Code | Description |
|----------|--------|------|-------------|
| 1 (highest) | Admin via UI | `admin` | Direct manual edits, always wins |
| 2 | Claude via MCP | `mcp` | `update_*` tool calls, wins over inference |
| 3 (lowest) | Engine inference | `inference` | Auto-detected or LLM-generated |

**Rules:**
- Lower priority cannot overwrite higher priority without explicit `force` flag
- Same priority: later update wins
- Provenance recorded on every change

## Implementation

### 1. Add Provenance Fields to Ontology Tables

**Migration:**

```sql
-- Add provenance to entities
ALTER TABLE engine_ontology_entities
    ADD COLUMN created_by TEXT NOT NULL DEFAULT 'inference',
    ADD COLUMN updated_by TEXT,
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ;

ALTER TABLE engine_ontology_entities
    ADD CONSTRAINT valid_created_by CHECK (created_by IN ('admin', 'mcp', 'inference'));

-- Add provenance to relationships
ALTER TABLE engine_entity_relationships
    ADD COLUMN created_by TEXT NOT NULL DEFAULT 'inference',
    ADD COLUMN updated_by TEXT,
    ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ADD COLUMN updated_at TIMESTAMPTZ;

-- Add provenance to column metadata (in ontology JSON or separate table)
-- If using engine_ontology_column_metadata table:
ALTER TABLE engine_ontology_column_metadata
    ADD COLUMN created_by TEXT NOT NULL DEFAULT 'inference',
    ADD COLUMN updated_by TEXT;
```

### 2. Create Change Review Service

**File:** `pkg/services/change_review.go`

```go
type ChangeReviewService interface {
    // List pending changes for review
    ListPending(ctx context.Context, projectID uuid.UUID, limit int) ([]PendingChange, error)

    // Approve a change (applies to ontology)
    ApproveChange(ctx context.Context, changeID uuid.UUID, reviewer string) error

    // Reject a change (marks as rejected, no ontology update)
    RejectChange(ctx context.Context, changeID uuid.UUID, reviewer string) error

    // Bulk approve all pending changes
    ApproveAll(ctx context.Context, projectID uuid.UUID, reviewer string) (int, error)

    // Check if update is allowed based on precedence
    CanUpdate(ctx context.Context, projectID uuid.UUID, target ChangeTarget, newSource string) (bool, string, error)
}

type ChangeTarget struct {
    Type       string // "entity", "relationship", "column"
    EntityName string
    TableName  string
    ColumnName string
}

type changeReviewService struct {
    pendingChangeRepo repositories.PendingChangeRepository
    entityRepo        repositories.OntologyEntityRepository
    relationshipRepo  repositories.EntityRelationshipRepository
    columnRepo        repositories.OntologyColumnRepository
    logger            *slog.Logger
}
```

### 3. Implement Precedence Check

```go
var sourcePriority = map[string]int{
    "admin":     1,
    "mcp":       2,
    "inference": 3,
}

func (s *changeReviewService) CanUpdate(
    ctx context.Context,
    projectID uuid.UUID,
    target ChangeTarget,
    newSource string,
) (bool, string, error) {
    newPriority := sourcePriority[newSource]

    var existingSource string

    switch target.Type {
    case "entity":
        entity, err := s.entityRepo.GetByName(ctx, projectID, target.EntityName)
        if err != nil {
            return false, "", err
        }
        if entity == nil {
            return true, "", nil // New entity, no conflict
        }
        existingSource = entity.CreatedBy
        if entity.UpdatedBy != "" {
            existingSource = entity.UpdatedBy
        }

    case "relationship":
        // Similar lookup for relationships
        // ...

    case "column":
        // Similar lookup for column metadata
        // ...
    }

    existingPriority := sourcePriority[existingSource]

    if newPriority > existingPriority {
        return false, fmt.Sprintf(
            "cannot overwrite %s-created element with %s source (precedence: admin > mcp > inference)",
            existingSource, newSource,
        ), nil
    }

    return true, "", nil
}
```

### 4. Apply Change with Precedence

```go
func (s *changeReviewService) ApproveChange(
    ctx context.Context,
    changeID uuid.UUID,
    reviewer string, // "admin", "mcp", "auto"
) error {
    change, err := s.pendingChangeRepo.Get(ctx, changeID)
    if err != nil {
        return err
    }

    // Determine source for provenance
    source := "inference"
    if reviewer == "mcp" {
        source = "mcp"
    } else if reviewer == "admin" {
        source = "admin"
    }

    // Check precedence
    target := changeTargetFromChange(change)
    canUpdate, reason, err := s.CanUpdate(ctx, change.ProjectID, target, source)
    if err != nil {
        return err
    }
    if !canUpdate {
        return fmt.Errorf("precedence violation: %s", reason)
    }

    // Apply the change based on suggested action
    switch change.SuggestedAction {
    case "create_entity":
        if err := s.applyCreateEntity(ctx, change, source); err != nil {
            return err
        }
    case "update_entity":
        if err := s.applyUpdateEntity(ctx, change, source); err != nil {
            return err
        }
    case "create_relationship":
        if err := s.applyCreateRelationship(ctx, change, source); err != nil {
            return err
        }
    case "update_column":
        if err := s.applyUpdateColumn(ctx, change, source); err != nil {
            return err
        }
    // ... other actions
    }

    // Mark change as approved
    return s.pendingChangeRepo.UpdateStatus(ctx, changeID, "approved", reviewer)
}
```

### 5. MCP Tools for Change Review

**File:** `pkg/mcp/tools/ontology_changes.go`

```go
func registerApproveChangeTool(s *server.MCPServer, deps *OntologyToolDeps) {
    tool := mcp.NewTool(
        "approve_change",
        mcp.WithDescription(
            "Approve a pending ontology change, applying it to the ontology. "+
            "Respects precedence: admin > mcp > inference.",
        ),
        mcp.WithString("change_id", mcp.Required(), mcp.Description("UUID of the pending change to approve")),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps.BaseDeps, "approve_change")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        changeID, err := uuid.Parse(getStringParam(req, "change_id", ""))
        if err != nil {
            return NewErrorResult("invalid_id", "change_id must be a valid UUID"), nil
        }

        if err := deps.ChangeReviewService.ApproveChange(tenantCtx, changeID, "mcp"); err != nil {
            return NewErrorResult("approve_failed", err.Error()), nil
        }

        return NewJSONResult(map[string]any{
            "status":    "approved",
            "change_id": changeID.String(),
        }), nil
    })
}

func registerRejectChangeTool(s *server.MCPServer, deps *OntologyToolDeps) {
    tool := mcp.NewTool(
        "reject_change",
        mcp.WithDescription("Reject a pending ontology change. The change will not be applied."),
        mcp.WithString("change_id", mcp.Required(), mcp.Description("UUID of the pending change to reject")),
        mcp.WithString("reason", mcp.Description("Optional reason for rejection")),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps.BaseDeps, "reject_change")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        changeID, err := uuid.Parse(getStringParam(req, "change_id", ""))
        if err != nil {
            return NewErrorResult("invalid_id", "change_id must be a valid UUID"), nil
        }

        if err := deps.ChangeReviewService.RejectChange(tenantCtx, changeID, "mcp"); err != nil {
            return NewErrorResult("reject_failed", err.Error()), nil
        }

        return NewJSONResult(map[string]any{
            "status":    "rejected",
            "change_id": changeID.String(),
        }), nil
    })
}

func registerApproveAllChangesTool(s *server.MCPServer, deps *OntologyToolDeps) {
    tool := mcp.NewTool(
        "approve_all_changes",
        mcp.WithDescription(
            "Approve all pending ontology changes at once. "+
            "Use with caution - review individual changes first if unsure.",
        ),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps.BaseDeps, "approve_all_changes")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        count, err := deps.ChangeReviewService.ApproveAll(tenantCtx, projectID, "mcp")
        if err != nil {
            return NewErrorResult("approve_failed", err.Error()), nil
        }

        return NewJSONResult(map[string]any{
            "status":          "approved",
            "changes_applied": count,
        }), nil
    })
}
```

### 6. Update Existing update_* Tools to Set Provenance

**File:** `pkg/mcp/tools/ontology_entities.go`

```go
// In update_entity handler:
entity := &models.OntologyEntity{
    Name:        name,
    Description: description,
    // ... other fields
    CreatedBy: "mcp",  // Set provenance
}

// If updating existing:
entity.UpdatedBy = "mcp"
entity.UpdatedAt = time.Now()
```

### 7. UI for Change Review

**File:** `ui/src/pages/PendingChangesPage.tsx`

```typescript
// New page showing pending changes queue
// - List of changes with type, table, suggested action
// - Approve/Reject buttons per change
// - Bulk approve all button
// - Filter by change type, status
// - Shows precedence warnings if applicable
```

## Tasks

1. [x] Create migration to add provenance fields to ontology tables
2. [x] Update entity/relationship/column models to include provenance
3. [x] Create `ChangeReviewService` interface
4. [x] Implement `CanUpdate()` precedence check
5. [x] Implement `ApproveChange()` with change application
6. [x] Implement `RejectChange()`
7. [x] Implement `ApproveAll()`
8. [x] Implement `list_pending_changes` MCP tool (if not done in PLAN-03)
9. [x] Implement `approve_change` MCP tool
10. [x] Implement `reject_change` MCP tool
11. [x] Implement `approve_all_changes` MCP tool
12. [x] Update `update_entity` to set `created_by: "mcp"`
13. [x] Update `update_relationship` to set `created_by: "mcp"`
14. [x] Update `update_column` to set `created_by: "mcp"`
15. [ ] Create UI page for pending changes review
16. [x] Test: inference change cannot overwrite mcp-created entity
17. [x] Test: mcp change can overwrite inference-created entity

## Testing

### Precedence Tests

```
1. Create entity via update_entity (source: mcp)
2. Trigger schema refresh that detects same entity (source: inference)
3. approve_change for inference change
   → Expected: FAIL - "cannot overwrite mcp-created element"

4. Admin edits entity description via UI (source: admin)
5. Claude calls update_entity with new description (source: mcp)
   → Expected: FAIL - "cannot overwrite admin-created element"

6. Create entity via inference (approved change)
7. Claude calls update_entity to improve description (source: mcp)
   → Expected: SUCCESS - mcp can overwrite inference
```

### Workflow Tests

```
1. refresh_schema() → creates pending changes
2. list_pending_changes() → shows 3 pending
3. approve_change(id1) → entity created
4. reject_change(id2) → marked rejected, no entity
5. approve_all_changes() → remaining applied
6. list_pending_changes() → empty (all processed)
```
