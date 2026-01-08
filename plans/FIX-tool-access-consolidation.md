# FIX: Consolidate Tool Access Control Logic

**Created:** 2025-01-08
**Completed:** 2025-01-08
**Status:** ✅ Completed
**Priority:** Low (code quality improvement, no functional changes)

---

## Problem

Each MCP tool file has its own `check*ToolEnabled` function that's nearly identical:
- `checkContextToolEnabled` in context.go
- `checkKnowledgeToolEnabled` in knowledge.go
- `checkQuestionToolEnabled` in questions.go
- `checkEntityToolEnabled` in entity.go
- `checkColumnToolEnabled` in column.go
- `checkGlossaryToolEnabled` in glossary.go
- `checkRelationshipToolEnabled` in relationship.go
- `checkProbeToolEnabled` in probe.go
- `checkSearchToolEnabled` in search.go

This creates ~45 lines of duplicated code per file (~400 lines total).

---

## Solution

A shared helper has been created in `pkg/mcp/tools/access.go`:

```go
// ToolAccessDeps defines the common dependencies needed for tool access control.
type ToolAccessDeps interface {
    GetDB() *database.DB
    GetMCPConfigService() services.MCPConfigService
    GetLogger() *zap.Logger
}

// CheckToolAccess verifies that the specified tool is enabled for the current project.
func CheckToolAccess(ctx context.Context, deps ToolAccessDeps, toolName string) (*ToolAccessResult, error)

// AcquireToolAccess maintains the 4-return-value signature for direct use in tools.
func AcquireToolAccess(ctx context.Context, deps ToolAccessDeps, toolName string) (uuid.UUID, context.Context, func(), error)
```

---

## Migration Steps

For each tool file:

1. Add interface implementation methods to the deps struct:
```go
func (d *KnowledgeToolDeps) GetDB() *database.DB { return d.DB }
func (d *KnowledgeToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }
func (d *KnowledgeToolDeps) GetLogger() *zap.Logger { return d.Logger }
```

2. Replace the local `check*ToolEnabled` function with:
```go
// Before
projectID, tenantCtx, cleanup, err := checkKnowledgeToolEnabled(ctx, deps, "update_project_knowledge")

// After
projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_project_knowledge")
```

3. Delete the old `check*ToolEnabled` function

---

## Files Updated

- [x] pkg/mcp/tools/knowledge.go
- [x] pkg/mcp/tools/context.go
- [x] pkg/mcp/tools/questions.go
- [x] pkg/mcp/tools/entity.go
- [x] pkg/mcp/tools/column.go
- [x] pkg/mcp/tools/glossary.go
- [x] pkg/mcp/tools/relationship.go
- [x] pkg/mcp/tools/probe.go
- [x] pkg/mcp/tools/search.go
- [x] pkg/mcp/tools/developer.go (additional)
- [x] pkg/mcp/tools/schema.go (additional)
- [x] pkg/mcp/tools/ontology.go (additional)

---

## Testing

After migration, run:
```bash
make check
```

All existing tests should pass since the logic is identical.

---

## Notes

- The shared helper `AcquireToolAccess` (renamed from `CheckToolAccessWithLegacySignature`) is used by all tool files
- All 12 tool files migrated to use the shared helper
- All tests pass (`make check`)
- ~400 lines of duplicated code removed
