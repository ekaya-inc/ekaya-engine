# FIX: MCP Error Handling and Logging

## Context

When debugging MCP integrations, two issues make troubleshooting difficult:

1. **Claude Desktop eats error messages** - Server returns detailed errors, but Claude only sees "Tool execution failed"
2. **Server logs don't show MCP details** - Only HTTP status 200 logged, not the JSON-RPC error inside

**Solution:** Return actionable errors as successful MCP results with structured JSON (`{error: true, code: string, message: string}`). Keep system errors as Go errors.

**When to Use Which Pattern:**
| Situation | Pattern |
|-----------|---------|
| Invalid parameters | Return error result (Claude can fix) |
| Resource not found | Return error result (Claude should know) |
| Business logic failure | Return error result (actionable) |
| Internal server error | Return Go error (truly unexpected) |
| Database connection failed | Return Go error (system issue) |
| Authentication failed | Return Go error (security boundary) |

---

## Implementation Tasks

### Phase 1: Error Result Helper

1. [x] 1.1: Create error result helper functions (`pkg/mcp/tools/errors.go`)
2. [x] 1.2: Update get_ontology tool to use error results
3. [x] 1.3: Update update_entity tool to use error results
4. [x] 1.4: Test with Claude Desktop to verify error messages are visible

### Phase 2: MCP Logging Middleware

2. [x] 2.1: Create `pkg/middleware/mcp_logging.go`
3. [x] 2.2: Integrate into MCP handler chain
4. [x] 2.3: Add configuration options (`config.yaml` MCP section)
5. [x] 2.4: Add sensitive data redaction (`sanitizeArguments()`)

### Phase 3: Rollout to All Tools

1. [x] 3.1: Audit all MCP tools for error handling patterns
2. [ ] 3.2: Convert actionable errors to error results
   1. [x] 3.2.1: Convert high-priority query tools (`execute_approved_query`)
   2. [x] 3.2.2: Convert high-priority schema tools
      1. [x] 3.2.2.1: Convert get_schema tool
      2. [x] 3.2.2.2: Convert update_column tool
      3. [x] 3.2.2.3: Convert probe_column tool
   3. [ ] 3.2.3: Convert medium-priority entity and relationship tools
      1. [x] 3.2.3.1: Convert delete_entity tool
      2. [x] 3.2.3.2: Convert get_entity tool (note: list_entities does not exist)
      3. [x] 3.2.3.3: Convert relationship tools
         1. [x] 3.2.3.3.1: Convert update_relationship tool
         2. [x] 3.2.3.3.2: Convert delete_relationship tool
         3. [N/A] 3.2.3.3.3: get_relationship (tool does not exist)
         4. [N/A] 3.2.3.3.4: list_relationships (tool does not exist)
   4. [ ] 3.2.4: Convert low-priority exploration and admin tools
      1. [x] 3.2.4.1: Convert query management tools (`list_approved_queries`)
      2. [x] 3.2.4.2: Convert chat tool (N/A - tool does not exist as MCP tool)
      3. [ ] 3.2.4.3: Convert knowledge management tools
         1. [x] 3.2.4.3.1: Convert update_project_knowledge tool
         2. [N/A] 3.2.4.3.2: list_project_knowledge (tool does not exist)
         3. [x] 3.2.4.3.3: Convert delete_project_knowledge tool
         4. [ ] 3.2.4.3.4: Add comprehensive test coverage
            1. [x] 3.2.4.3.4.1: Set up test infrastructure
            2. [x] 3.2.4.3.4.2: Add parameter validation tests for update_project_knowledge
            3. [x] 3.2.4.3.4.3: Add resource validation tests for delete_project_knowledge
            4. [x] 3.2.4.3.4.4: Add successful operation tests
            5. [x] 3.2.4.3.4.5: Run full test suite and verify
      4. [x] 3.2.4.4: Document final error handling pattern (`plans/FIX-all-mcp-tool-error-handling.md`)
3. [ ] 3.3: Keep system errors as Go errors
   1. [ ] 3.3.1: Convert glossary tools to error results

      Convert glossary management tools: `update_glossary_term`, `get_glossary_sql`, `list_glossary` in `pkg/mcp/tools/glossary.go`.

      **Parameter validation:**
      - Empty term name → `NewErrorResult("invalid_parameters", "parameter 'term' cannot be empty")`
      - Invalid aliases array → `NewErrorResultWithDetails` with index and type details
      - Term not found → `NewErrorResult("TERM_NOT_FOUND", ...)`

      **Test coverage:** Add `TestUpdateGlossaryTermTool_ErrorResults`, `TestGetGlossarySQLTool_ErrorResults`

   2. [ ] 3.3.2: Convert ontology question tools to error results
      1. [x] 3.3.2.1: Convert resolve_ontology_question tool
      2. [ ] 3.3.2.2: Convert skip, dismiss, escalate tools
         1. [x] 3.3.2.2.1: Add common validation helpers (`validateQuestionID`, `validateReasonParameter`)
         2. [x] 3.3.2.2.2: Convert skip_ontology_question tool
         3. [ ] 3.3.2.2.3: Convert dismiss and escalate tools
            1. [x] 3.3.2.2.3.1: Verify shared validation helpers exist
            2. [x] 3.3.2.2.3.2: Convert dismiss_ontology_question tool
            3. [x] 3.3.2.2.3.3: Convert escalate_ontology_question tool
            4. [x] 3.3.2.2.3.4: Run full test suite and verify no regressions
      3. [x] 3.3.2.3: Convert list_ontology_questions tool
      4. [ ] 3.3.2.4: Add comprehensive integration tests

         Create `pkg/mcp/tools/questions_integration_test.go` with:
         - Use `testhelpers.GetEngineDB(t)` for test database
         - Test all four question status tools end-to-end
         - Test list_ontology_questions with all filters
         - Verify database state changes
         - Use proper cleanup for test isolation

   3. [ ] 3.4: Update remaining medium/low priority tools

      **entity.go - delete_entity:**
      - Validate name parameter (non-empty after trim)
      - Return error result if entity not found
      - Return error result if entity has relationships (with count/names in details)

      **relationship.go - update_relationship:**
      - Validate `from_entity` and `to_entity` (non-empty after trim)
      - Validate `cardinality` enum: ["1:1", "1:N", "N:1", "N:M", "unknown"]
      - Return error result if entities not found

      **relationship.go - delete_relationship:**
      - Validate `from_entity` and `to_entity` (non-empty after trim)
      - Return error result if entities or relationship not found

   4. [ ] 3.5: Document final error handling pattern

      Update `plans/FIX-mcp-error-handling-and-logging.md` with:
      - Decision tree: When to use `NewErrorResult()` vs Go error
      - Standard error codes table
      - Code templates for common patterns
      - Summary statistics (tools audited, updated, error codes defined)
      - Lessons learned and best practices

4. [ ] Document the error handling pattern

---

## Standard Error Codes

| Code | Meaning |
|------|---------|
| `invalid_parameters` | Required parameter missing or invalid |
| `ontology_not_found` | No active ontology for project |
| `TABLE_NOT_FOUND` | Table doesn't exist in schema |
| `COLUMN_NOT_FOUND` | Column doesn't exist in table |
| `ENTITY_NOT_FOUND` | Entity doesn't exist in ontology |
| `RELATIONSHIP_NOT_FOUND` | Relationship doesn't exist |
| `QUERY_NOT_FOUND` | Approved query doesn't exist |
| `QUERY_NOT_APPROVED` | Query exists but not approved/enabled |
| `TERM_NOT_FOUND` | Glossary term doesn't exist |
| `FACT_NOT_FOUND` | Project knowledge fact doesn't exist |
| `QUESTION_NOT_FOUND` | Ontology question doesn't exist |
| `security_violation` | SQL injection or security issue detected |
| `parameter_validation` | Parameter failed type/format validation |
| `type_validation` | Type conversion error |
| `query_error` | SQL execution error |
| `validation_error` | Business rule violation |
| `resource_conflict` | Resource already exists |

---

## Testing

### Error Result Testing

1. Connect Claude Desktop to local server
2. Call tool with invalid parameters
3. Verify Claude receives: `{"error": true, "code": "...", "message": "..."}`
4. Verify Claude can adjust and retry

### Logging Testing

1. Enable MCP logging (`config.yaml`: `mcp.log_requests: true`)
2. Call various tools
3. Verify logs show tool name, parameters (sanitized), success/failure, duration

---

## Key Files

- `pkg/mcp/tools/errors.go` - Error result helpers (`NewErrorResult`, `NewErrorResultWithDetails`)
- `pkg/mcp/tools/helpers.go` - Shared helpers (`trimString`)
- `pkg/middleware/mcp_logging.go` - MCP request/response logging middleware
- `pkg/config/config.go` - MCPConfig struct for logging configuration
- `plans/FIX-all-mcp-tool-error-handling.md` - Comprehensive error handling documentation
