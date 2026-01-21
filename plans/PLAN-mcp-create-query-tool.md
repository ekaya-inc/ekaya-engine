# PLAN: MCP create_query Tool for Dev Tool Mode

## Purpose

In **Dev Tool Ontology Maintenance mode**, MCP clients (like Claude Code) assist with building and maintaining the ontology. This includes creating pre-approved queries without requiring admin approval workflow.

The `create_query` tool is the Dev Tool mode equivalent of `suggest_query`:
- `suggest_query` - For production use, requires admin approval
- `create_query` - For dev/maintenance mode, creates immediately

**When to use which:**
| Mode | Tool | Approval | Use Case |
|------|------|----------|----------|
| Production (Data Liaison) | `suggest_query` | Required | End-user MCP clients propose queries |
| Dev Tool (Ontology Maintenance) | `create_query` | None | Trusted MCP client builds query library |

---

## Implementation Status

| Component | Status | Notes |
|-----------|--------|-------|
| suggest_approved_query MCP tool | ✅ Done | Existing tool for suggesting queries |
| create_query MCP tool | ❌ Pending | New tool for direct creation |
| Tool visibility based on mode | ❌ Pending | Show create_query in dev mode, suggest_query in production |

---

## Feature Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    create_query Flow (Dev Tool Mode)                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. MCP Client (Claude Code) discovers need for a reusable query             │
│                              ↓                                               │
│  2. Client calls create_query with:                                          │
│     - name: Short identifier                                                 │
│     - description: What the query answers                                    │
│     - sql: The SQL query with {{parameters}}                                 │
│     - parameters: Parameter definitions (optional, can be inferred)          │
│     - tags: Categorization tags (optional)                                   │
│                              ↓                                               │
│  3. System validates SQL syntax                                              │
│                              ↓                                               │
│  4. Query is created with:                                                   │
│     - approval_status = 'approved'                                           │
│     - is_enabled = true                                                      │
│     - created_by = 'mcp'                                                     │
│                              ↓                                               │
│  5. Query immediately available in list_approved_queries                     │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Tool Specification

### create_query

**Purpose:** Create a new pre-approved query (Dev Tool mode only)

```
Input:
{
  "name": "revenue_by_category",                           // Required: unique identifier
  "description": "Total revenue by product category",      // Required: what it answers
  "sql": "SELECT c.name, SUM(amount) as revenue FROM categories c JOIN orders o ON ... WHERE o.date >= {{start_date}} GROUP BY c.name",  // Required
  "parameters": [                                          // Optional: inferred from SQL if omitted
    {
      "name": "start_date",
      "type": "date",
      "description": "Start of date range",
      "required": true
    }
  ],
  "tags": ["analytics", "revenue"],                        // Optional: for categorization
  "output_column_descriptions": {                          // Optional: describe output columns
    "name": "Category name",
    "revenue": "Total revenue in USD"
  }
}

Output (success):
{
  "query_id": "uuid",
  "name": "revenue_by_category",
  "status": "approved",
  "is_enabled": true,
  "message": "Query created and ready for use",
  "parameters": [...]  // Including any inferred parameters
}

Output (validation error):
{
  "error": true,
  "error_type": "VALIDATION_FAILED",
  "message": "SQL syntax error: column 'categorys' does not exist"
}

Output (duplicate name):
{
  "error": true,
  "error_type": "DUPLICATE_NAME",
  "message": "Query with name 'revenue_by_category' already exists"
}
```

**MCP Annotations:**
- `readOnlyHint`: false
- `destructiveHint`: false
- `idempotentHint`: false (creates new record)

---

## Tool Visibility Logic

**File:** `pkg/mcp/tools/developer.go`

The tool shown depends on the MCP server mode:

```go
// In Dev Tool mode (ontology maintenance):
//   - Show: create_query
//   - Hide: suggest_query
//
// In Production mode (data liaison):
//   - Show: suggest_query (if allowClientSuggestions enabled)
//   - Hide: create_query

if isDevToolMode {
    // create_query visible when approved_queries enabled
    if showApprovedQueries {
        tools = append(tools, "create_query")
    }
} else {
    // suggest_query visible when approved_queries + allowClientSuggestions enabled
    if showApprovedQueries && allowClientSuggestions {
        tools = append(tools, "suggest_query")
    }
}
```

**Mode Detection:**

Dev Tool mode is indicated by `ToolGroupConfig.DeveloperTools.Enabled = true`.

---

## Implementation Details

### 1. Add create_query Tool

**File:** `pkg/mcp/tools/queries.go`

```go
func registerCreateQueryTool(s *server.MCPServer, deps *MCPToolDeps) {
    tool := mcp.NewTool("create_query",
        mcp.WithDescription("Create a new pre-approved query. Use this to add reusable parameterized queries to the library."),
        mcp.WithString("name", mcp.Description("Unique name/identifier for the query"), mcp.Required()),
        mcp.WithString("description", mcp.Description("What business question this query answers"), mcp.Required()),
        mcp.WithString("sql", mcp.Description("SQL query with {{parameter}} placeholders"), mcp.Required()),
        mcp.WithArray("parameters", mcp.Description("Parameter definitions (optional, inferred from SQL if omitted)")),
        mcp.WithArray("tags", mcp.Description("Tags for categorization (optional)")),
        mcp.WithObject("output_column_descriptions", mcp.Description("Descriptions for output columns (optional)")),
    )

    s.AddTool(tool, createQueryHandler(deps))
}
```

### 2. Handler Implementation

```go
func createQueryHandler(deps *MCPToolDeps) server.ToolHandlerFunc {
    return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
        // 1. Extract parameters
        name := req.Params.Arguments["name"].(string)
        description := req.Params.Arguments["description"].(string)
        sql := req.Params.Arguments["sql"].(string)

        // 2. Validate SQL syntax
        if err := deps.QueryService.ValidateSQL(ctx, sql); err != nil {
            return NewErrorResult("VALIDATION_FAILED", err.Error()), nil
        }

        // 3. Infer parameters if not provided
        parameters := extractOrInferParameters(req, sql)

        // 4. Check for duplicate name
        existing, _ := deps.QueryRepo.GetByName(ctx, projectID, name)
        if existing != nil {
            return NewErrorResult("DUPLICATE_NAME", "Query with name '"+name+"' already exists"), nil
        }

        // 5. Create query with approved status
        query := &models.Query{
            ProjectID:             projectID,
            DatasourceID:          datasourceID,
            Name:                  name,
            NaturalLanguagePrompt: description,
            SQLQuery:              sql,
            Parameters:            parameters,
            Tags:                  tags,
            IsEnabled:             true,
            ApprovalStatus:        models.ApprovalStatusApproved,
            CreatedBy:             "mcp",
        }

        if err := deps.QueryRepo.Create(ctx, query); err != nil {
            return nil, fmt.Errorf("failed to create query: %w", err)
        }

        // 6. Return success
        return NewSuccessResult(map[string]any{
            "query_id":   query.ID,
            "name":       query.Name,
            "status":     "approved",
            "is_enabled": true,
            "message":    "Query created and ready for use",
            "parameters": query.Parameters,
        }), nil
    }
}
```

### 3. Tool Registration

**File:** `pkg/mcp/tools/queries.go`

```go
var queryToolNames = map[string]bool{
    "list_approved_queries":   true,
    "execute_approved_query":  true,
    "suggest_query":           true,  // Production mode
    "create_query":            true,  // Dev tool mode
}

func RegisterQueryTools(s *server.MCPServer, deps *MCPToolDeps, isDevToolMode bool) {
    registerListApprovedQueriesTool(s, deps)
    registerExecuteApprovedQueryTool(s, deps)

    if isDevToolMode {
        registerCreateQueryTool(s, deps)
    } else if deps.Config.AllowClientSuggestions {
        registerSuggestQueryTool(s, deps)
    }
}
```

---

## Comparison: create_query vs suggest_query

| Aspect | create_query | suggest_query |
|--------|--------------|---------------|
| **Mode** | Dev Tool (ontology maintenance) | Production (data liaison) |
| **Approval** | None - immediately approved | Requires admin approval |
| **Created by** | `mcp` provenance | `mcp` provenance, `pending` status |
| **Availability** | Immediate | After admin approval |
| **Trust level** | Trusted maintainer | Untrusted end-user |
| **Use case** | Building query library | Expanding query library safely |

---

## Files to Modify

| File | Change |
|------|--------|
| `pkg/mcp/tools/queries.go` | Add `registerCreateQueryTool` and handler |
| `pkg/mcp/tools/developer.go` | Update tool visibility logic for mode |
| `pkg/mcp/tools/queries_test.go` | Add tests for create_query |

---

## Testing

### Unit Tests

1. **create_query success** - Creates query with correct fields
2. **create_query validation error** - Invalid SQL rejected
3. **create_query duplicate name** - Duplicate name rejected
4. **create_query parameter inference** - Parameters extracted from SQL
5. **Tool visibility** - create_query visible in dev mode, hidden in production

### Integration Tests

1. Create query via MCP → Query appears in list_approved_queries
2. Create query → Execute query with parameters works
3. Dev mode toggle → Tool visibility changes

---

## Open Questions

1. **Should create_query also support update/upsert semantics?**
   - Could allow `update_query` or make `create_query` idempotent by name
   - Recommendation: Add separate `update_query` tool later if needed

2. **Should there be a `delete_query` tool?**
   - Useful for cleanup during development
   - Recommendation: Yes, add in same PR

3. **What about query versioning?**
   - Track changes to queries over time
   - Recommendation: Defer to future enhancement
