# FIX: Consolidate Tool Access Control Logic

**Created:** 2025-01-08
**Status:** Ready for implementation
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

// CheckToolAccessWithLegacySignature maintains the 4-return-value signature for migration.
func CheckToolAccessWithLegacySignature(ctx context.Context, deps ToolAccessDeps, toolName string) (uuid.UUID, context.Context, func(), error)
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
projectID, tenantCtx, cleanup, err := CheckToolAccessWithLegacySignature(ctx, deps, "update_project_knowledge")
```

3. Delete the old `check*ToolEnabled` function

---

## Files to Update

- [ ] pkg/mcp/tools/knowledge.go
- [ ] pkg/mcp/tools/context.go
- [ ] pkg/mcp/tools/questions.go
- [ ] pkg/mcp/tools/entity.go
- [ ] pkg/mcp/tools/column.go
- [ ] pkg/mcp/tools/glossary.go
- [ ] pkg/mcp/tools/relationship.go
- [ ] pkg/mcp/tools/probe.go
- [ ] pkg/mcp/tools/search.go

---

## Testing

After migration, run:
```bash
make check
```

All existing tests should pass since the logic is identical.

---

## Notes

- The shared helper is already created and ready to use
- Migration can be done incrementally (one file at a time)
- This is purely a refactoring - no behavioral changes
- Consider doing this as part of a dedicated cleanup PR
