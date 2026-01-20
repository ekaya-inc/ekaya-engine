# FIX: MCP Tool Error Handling Standard

## Status: REQUIRED FOR ALL MCP TOOLS

This document establishes the **mandatory** error handling pattern for all MCP tools in ekaya-engine. This is not optional guidance—all MCP tools must follow this pattern.

---

## The Problem

Claude Desktop (and other MCP clients) do not surface JSON-RPC error messages to the LLM. When a tool returns a Go error:

```go
return nil, fmt.Errorf("table '%s' not found", tableName)
```

The server sends a proper JSON-RPC error response, but Claude only sees:

```
<error>Tool execution failed</error>
```

This prevents Claude from understanding what went wrong and adjusting its approach.

---

## The Solution

**All actionable errors must be returned as successful results with error information in the payload.**

### Required Pattern

```go
// WRONG - Claude won't see the error message
if table == nil {
    return nil, fmt.Errorf("table '%s' not found in schema", tableName)
}

// CORRECT - Claude receives the full error
if table == nil {
    return NewErrorResult("TABLE_NOT_FOUND",
        fmt.Sprintf("table '%s' not found in schema registry. Run refresh_schema() after creating tables.", tableName)), nil
}
```

### Error Response Structure

All error results use this structure:

```json
{
  "error": true,
  "code": "TABLE_NOT_FOUND",
  "message": "table 'foo' not found in schema registry. Run refresh_schema() after creating tables.",
  "details": {}  // Optional additional context
}
```

### Helper Functions

Use the helpers in `pkg/mcp/tools/errors.go`:

```go
// Simple error
return NewErrorResult("ENTITY_NOT_FOUND", "entity 'User' does not exist"), nil

// Error with additional context
return NewErrorResultWithDetails("VALIDATION_ERROR",
    "invalid aliases parameter",
    map[string]any{
        "invalid_element_index": i,
        "invalid_element_type":  fmt.Sprintf("%T", elem),
    }), nil
```

---

## When to Use Which Pattern

| Situation | Pattern | Rationale |
|-----------|---------|-----------|
| Invalid parameters | `NewErrorResult` | Claude can fix and retry |
| Resource not found | `NewErrorResult` | Claude should know and adjust |
| Validation failure | `NewErrorResult` | Claude can correct input |
| Business rule violation | `NewErrorResult` | Claude should understand why |
| Database connection failed | Go error | System issue, not actionable by Claude |
| Internal panic/crash | Go error | True unexpected failure |
| Authentication failed | Go error | Security boundary |

**Rule of thumb:** If Claude could reasonably adjust its approach after seeing the error message, use `NewErrorResult`. If it's a system failure that Claude cannot fix, use a Go error.

---

## Standard Error Codes

Use these codes consistently across all tools:

| Code | When to Use |
|------|-------------|
| `invalid_parameters` | Required parameter missing, wrong type, or invalid value |
| `TABLE_NOT_FOUND` | Referenced table doesn't exist in schema |
| `COLUMN_NOT_FOUND` | Referenced column doesn't exist in table |
| `ENTITY_NOT_FOUND` | Referenced entity doesn't exist in ontology |
| `RELATIONSHIP_NOT_FOUND` | Referenced relationship doesn't exist |
| `ontology_not_found` | No active ontology for project |
| `permission_denied` | Tool not enabled for project |
| `resource_conflict` | Resource already exists (when creating) |
| `validation_error` | Input failed business validation |
| `query_error` | SQL query failed (syntax, timeout, etc.) |

---

## Implementation Checklist

Every MCP tool must be audited against this checklist:

### Parameter Validation
- [ ] Required parameters checked and return `invalid_parameters` if missing
- [ ] Array parameters validated element-by-element with type checks
- [ ] Enum parameters validated against allowed values

### Resource Lookups
- [ ] Table lookups return `TABLE_NOT_FOUND` with guidance
- [ ] Column lookups return `COLUMN_NOT_FOUND`
- [ ] Entity lookups return `ENTITY_NOT_FOUND`
- [ ] All "not found" messages include actionable guidance

### Business Logic
- [ ] Business rule violations return error results with explanation
- [ ] Validation errors include what was wrong and how to fix it

### System Errors
- [ ] Database failures return Go errors (not error results)
- [ ] Authentication failures return Go errors
- [ ] Internal panics are not caught and converted to error results

---

## Tools Requiring Audit

Based on `pkg/mcp/tools/`:

### Completed (Phase 1)
- [x] `get_ontology` - parameter validation errors converted
- [x] `update_entity` - parameter validation errors converted
- [x] `errors.go` - helper functions created

### Needs Audit and Update
- [ ] `update_column` - needs TABLE_NOT_FOUND, COLUMN_NOT_FOUND (see FIX-update-column-validation.md)
- [ ] `delete_column_metadata` - needs validation
- [ ] `update_relationship` - needs entity validation
- [ ] `delete_relationship` - needs entity validation
- [ ] `get_entity` - needs ENTITY_NOT_FOUND
- [ ] `delete_entity` - needs ENTITY_NOT_FOUND
- [ ] `probe_column` - needs TABLE_NOT_FOUND, COLUMN_NOT_FOUND
- [ ] `probe_columns` - needs validation for each column
- [ ] `probe_relationship` - needs entity validation
- [ ] `query` - needs query_error handling
- [ ] `execute` - needs query_error handling
- [ ] `sample` - needs TABLE_NOT_FOUND
- [ ] `search_schema` - parameter validation
- [ ] `get_schema` - parameter validation
- [ ] `get_context` - parameter validation
- [ ] `list_approved_queries` - parameter validation
- [ ] `execute_approved_query` - needs query_not_found, parameter validation
- [ ] `suggest_approved_query` - parameter validation
- [ ] `list_glossary` - parameter validation
- [ ] `get_glossary_sql` - needs TERM_NOT_FOUND
- [ ] `update_glossary_term` - parameter validation
- [ ] `delete_glossary_term` - needs TERM_NOT_FOUND
- [ ] `update_project_knowledge` - parameter validation
- [ ] `delete_project_knowledge` - needs FACT_NOT_FOUND
- [ ] `list_ontology_questions` - parameter validation
- [ ] `resolve_ontology_question` - needs QUESTION_NOT_FOUND
- [ ] `skip_ontology_question` - needs QUESTION_NOT_FOUND
- [ ] `dismiss_ontology_question` - needs QUESTION_NOT_FOUND
- [ ] `escalate_ontology_question` - needs QUESTION_NOT_FOUND
- [ ] `explain_query` - needs query_error handling
- [ ] `validate` - needs query_error handling
- [ ] `get_query_history` - parameter validation
- [ ] `health` / `echo` - minimal, likely fine

---

## Testing Requirements

Each tool's error handling must be tested:

1. **Unit test error results** - Verify `NewErrorResult` returns correct structure
2. **Integration test with Claude** - Verify Claude receives and can act on errors
3. **Test error codes** - Verify correct codes used for each error type

Example test pattern:

```go
func TestUpdateColumn_TableNotFound(t *testing.T) {
    // Setup: no tables in schema registry
    result, err := callUpdateColumn(ctx, "nonexistent_table", "foo", "description")

    require.NoError(t, err) // Tool call succeeded
    require.True(t, result.IsError)

    var errResp tools.ErrorResponse
    json.Unmarshal([]byte(result.Content[0].Text), &errResp)

    assert.True(t, errResp.Error)
    assert.Equal(t, "TABLE_NOT_FOUND", errResp.Code)
    assert.Contains(t, errResp.Message, "refresh_schema()")
}
```

---

## Related Documents

- **FIX-mcp-error-handling-and-logging.md** - Original implementation of error helpers and logging middleware (Phase 1 & 2 complete)
- **FIX-update-column-validation.md** - Specific fix for update_column tool validation
- **FIX-claude-db-creation-workflow.md** - Context for refresh_schema() tool that errors should reference

---

## Migration Priority

Prioritize tools by usage frequency and impact:

### High Priority (most used by Claude)
1. `update_column`, `update_entity`, `update_relationship` - ontology building
2. `query`, `execute` - data operations
3. `get_schema`, `get_context`, `get_ontology` - context retrieval
4. `probe_column`, `probe_columns` - schema exploration

### Medium Priority
5. `sample`, `search_schema` - exploration
6. `execute_approved_query`, `suggest_approved_query` - query management
7. Glossary tools
8. Question management tools

### Low Priority
9. `health`, `echo` - minimal error cases
10. Rarely used admin tools
