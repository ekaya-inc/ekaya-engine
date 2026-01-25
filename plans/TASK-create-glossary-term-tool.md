# TASK: Add create_glossary_term MCP Tool

**Priority:** 6 (Low)
**Status:** Not Started
**Parent:** PLAN-ontology-next.md
**Design Reference:** DESIGN-glossary-client-updates.md (archived)

## Overview

The glossary MCP tools are mostly implemented (`list_glossary`, `get_glossary_sql`, `update_glossary_term`, `delete_glossary_term`), but `create_glossary_term` is missing.

## Implementation

### Step 1: Add Tool Registration

**File:** `pkg/mcp/tools/glossary.go`

Add to `RegisterGlossaryTools`:
```go
func RegisterGlossaryTools(s *server.MCPServer, deps *GlossaryToolDeps) {
    registerListGlossaryTool(s, deps)
    registerGetGlossarySQLTool(s, deps)
    registerCreateGlossaryTermTool(s, deps)  // ADD THIS
    registerUpdateGlossaryTermTool(s, deps)
    registerDeleteGlossaryTermTool(s, deps)
}
```

### Step 2: Implement Tool

```go
func registerCreateGlossaryTermTool(s *server.MCPServer, deps *GlossaryToolDeps) {
    tool := mcp.NewTool(
        "create_glossary_term",
        mcp.WithDescription(
            "Create a new business glossary term with its SQL definition. "+
            "The SQL will be validated before saving. "+
            "Use this to add new business metrics like 'Revenue', 'Active Users', etc.",
        ),
        mcp.WithString("term", mcp.Required(),
            mcp.Description("The business term name (e.g., 'Daily Active Users')")),
        mcp.WithString("definition", mcp.Required(),
            mcp.Description("Human-readable description of what this term means")),
        mcp.WithString("defining_sql", mcp.Required(),
            mcp.Description("SQL query that calculates this metric")),
        mcp.WithString("base_table",
            mcp.Description("Primary table this term is derived from (optional)")),
        mcp.WithReadOnlyHintAnnotation(false),
        mcp.WithDestructiveHintAnnotation(false),
    )

    s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "create_glossary_term")
        if err != nil {
            return nil, err
        }
        defer cleanup()

        term, _ := req.RequireString("term")
        definition, _ := req.RequireString("definition")
        definingSQL, _ := req.RequireString("defining_sql")
        baseTable, _ := req.GetString("base_table")

        glossaryTerm := &models.BusinessGlossaryTerm{
            ProjectID:   projectID,
            Term:        term,
            Definition:  definition,
            DefiningSQL: definingSQL,
            BaseTable:   baseTable,
            Source:      models.GlossarySourceClient,
        }

        err = deps.GlossaryService.CreateTerm(tenantCtx, projectID, glossaryTerm)
        if err != nil {
            return NewErrorResult("create_failed", err.Error()), nil
        }

        // Return the created term
        result := map[string]any{
            "success": true,
            "term": map[string]any{
                "id":         glossaryTerm.ID.String(),
                "term":       glossaryTerm.Term,
                "definition": glossaryTerm.Definition,
            },
        }
        return NewJSONResult(result), nil
    })
}
```

### Step 3: Add to Developer Tools Filter

**File:** `pkg/mcp/tools/developer.go`

Ensure `create_glossary_term` is in the developer tools list so it requires the toggle.

## Testing

1. Enable developer tools for a project
2. Connect MCP client
3. Call `create_glossary_term` with valid SQL
4. Verify term appears in `list_glossary`
5. Test with invalid SQL - should return error

## Success Criteria

- [x] Tool registered and callable
- [ ] SQL validation works (invalid SQL rejected)
- [ ] Source set to 'client' for client-created terms
- [ ] Integration tests pass
