# Issue: Input Validation Errors Logged as Server Errors with Stack Traces

## Observed Behavior

Several MCP tool handlers log ERROR level messages with full stack traces for **expected input validation failures**. These are user/input errors, not server bugs, but they appear in logs as if something went wrong operationally.

The server handles these errors properly (returns appropriate MCP error responses), but the ERROR+stacktrace logging:
1. Pollutes logs with false positives
2. Makes it harder to identify real server issues
3. Could trigger unnecessary alerts in production monitoring

## Expected Behavior

Input validation errors should be logged at DEBUG or INFO level (or use a special "expected error" classification), without stack traces. Stack traces should be reserved for unexpected server errors.

## Examples from Test Run

### 1. Execute Tool - SQL Syntax Error (tools/developer.go:718)
```
ERROR tools/developer.go:718 DDL/DML execution failed {"error": "failed to execute statement: ERROR: syntax error at or near \"SELEKT\" (SQLSTATE 42601)"}
github.com/ekaya-inc/ekaya-engine/pkg/mcp/tools.registerExecuteTool.func1
    /Users/damondanieli/.../pkg/mcp/tools/developer.go:718
[... full stack trace ...]
```
**Input:** `SELEKT * FORM mcp_test_users`
**This is expected:** User submitted invalid SQL.

### 2. Execute Tool - Table Not Found (tools/developer.go:718)
```
ERROR tools/developer.go:718 DDL/DML execution failed {"error": "failed to execute statement: ERROR: relation \"nonexistent_table_xyz\" does not exist (SQLSTATE 42P01)"}
[... full stack trace ...]
```
**Input:** `INSERT INTO nonexistent_table_xyz (col) VALUES (1)`
**This is expected:** User referenced non-existent table.

### 3. Execute Tool - Constraint Violation (tools/developer.go:718)
```
ERROR tools/developer.go:718 DDL/DML execution failed {"error": "error during execution: ERROR: duplicate key value violates unique constraint \"mcp_test_users_email_key\" (SQLSTATE 23505)"}
[... full stack trace ...]
```
**Input:** Inserting duplicate email
**This is expected:** User violated unique constraint.

### 4. Execute Tool - Multiple Statements (tools/developer.go:718)
```
ERROR tools/developer.go:718 DDL/DML execution failed {"error": "failed to execute statement: ERROR: cannot insert multiple commands into a prepared statement (SQLSTATE 42601)"}
[... full stack trace ...]
```
**Input:** Multiple SQL statements in one call
**This is expected:** Tool doesn't support multiple statements.

### 5. Update Glossary Term - Invalid SQL (tools/glossary.go:456)
```
ERROR tools/glossary.go:456 Failed to update glossary term {"term": "Available Users", "error": "SQL validation failed: failed to execute query: ERROR: syntax error at or near \"SQL\" (SQLSTATE 42601)"}
[... full stack trace ...]
```
**Input:** `SELEKT BAD SQL`
**This is expected:** User submitted invalid SQL.

### 6. Create Approved Query - Missing Output Columns (tools/dev_queries.go:763)
```
ERROR tools/dev_queries.go:763 Failed to create approved query {"error": "output_columns required: test query before saving to capture result columns"}
[... full stack trace ...]
```
**This is expected:** Tool requires dry-run first to capture output columns.

### 7. Delete Approved Query - Not Found (tools/dev_queries.go:1120)
```
ERROR tools/dev_queries.go:1120 Failed to delete approved query {"query_id": "00000000-0000-0000-0000-000000000000", "error": "not found"}
[... full stack trace ...]
```
**Input:** Non-existent query ID
**This is expected:** User requested deletion of non-existent resource.

## Affected Files

- `pkg/mcp/tools/developer.go:718` - `execute` tool
- `pkg/mcp/tools/glossary.go:456` - `update_glossary_term` tool
- `pkg/mcp/tools/dev_queries.go:763` - `create_approved_query` tool
- `pkg/mcp/tools/dev_queries.go:1120` - `delete_approved_query` tool

## Suggested Fix

Use a pattern like:
```go
// For expected input errors - no stack trace
logger.Debug("DDL/DML execution failed (user error)", zap.String("error", err.Error()))

// For unexpected server errors - full stack trace
logger.Error("DDL/DML execution failed", zap.Error(err))
```

Or use a custom error type that distinguishes input errors from server errors:

```go
if errors.Is(err, ErrInputValidation) {
    logger.Info("Input validation failed", zap.String("error", err.Error()))
} else {
    logger.Error("Unexpected error", zap.Error(err))
}
```

## Also Noted (Possibly Separate Issues)

### WARN with Stack Trace - Schema Query Error (tools/probe.go:561)
```
WARN tools/probe.go:561 Failed to fetch schema relationships with metrics {"error": "failed to query schema relationships: ERROR: column sc.table_id does not exist (SQLSTATE 42703)"}
[... full stack trace ...]
```
**This appears to be a real bug** - the query references a column that doesn't exist. This is NOT an input error.

### ERROR - Connection Busy (tools/queries.go:1174)
```
ERROR tools/queries.go:1174 Failed to log query execution {"error": "conn busy"}
```
**This is a server-side issue** - connection pool problem. Should remain ERROR level but may warrant investigation.

## Context

- Discovered during MCP test suite run
- Project ID: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- Log file: `tests/claude-mcp/output.log.txt`
