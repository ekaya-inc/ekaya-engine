# Issue: MCP Errors Not Wrapped in JSON Response

## Observed Behavior

MCP tools return protocol-level errors (e.g., `MCP error -32603: ...`) instead of JSON error objects. This causes Claude Desktop to flash an error badge and disrupts the user experience.

## Expected Behavior

All errors should be wrapped in a JSON response with success status, allowing the client to handle them gracefully:
```json
{"error": true, "code": "EXECUTION_FAILED", "message": "syntax error at or near \"SELEKT\""}
```

## Examples of Problematic Responses

### Syntax Error
```
MCP error -32603: execution failed: failed to execute statement: ERROR: syntax error at or near "SELEKT" (SQLSTATE 42601)
```

### Table Not Found
```
MCP error -32603: execution failed: failed to execute statement: ERROR: relation "nonexistent_table_xyz" does not exist (SQLSTATE 42P01)
```

### Constraint Violation
```
MCP error -32603: execution failed: error during execution: ERROR: duplicate key value violates unique constraint "mcp_test_users_email_key" (SQLSTATE 23505)
```

### Multi-Statement Rejection
```
MCP error -32603: execution failed: failed to execute statement: ERROR: cannot insert multiple commands into a prepared statement (SQLSTATE 42601)
```

## Steps to Reproduce

1. Call `mcp__mcp_test_suite__execute` with invalid SQL:
   ```json
   {"sql": "SELEKT * FORM mcp_test_users"}
   ```

2. Observe MCP protocol error instead of JSON response

## Context

- Project ID: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- MCP Server: `mcp_test_suite`
- Test: `280-execute.md`
- Affects: `execute` tool (and likely other tools)

## Impact

- Claude Desktop shows error badge for expected error conditions
- Error handling is inconsistent (some tools return JSON errors, others MCP errors)
- Client cannot reliably distinguish between "expected" errors (bad SQL) and "unexpected" errors (server crash)

## Affected Tools (confirmed)

- `execute` - syntax errors, constraint violations, multi-statement rejection

## Possibly Affected Tools

- Any tool that can fail due to user input
- Query tools with invalid SQL
- Update tools with invalid data
