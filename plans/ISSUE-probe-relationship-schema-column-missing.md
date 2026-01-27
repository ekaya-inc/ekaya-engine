# Issue: probe_relationship Tool References Non-Existent Column

## Observed Behavior

The `probe_relationship` tool generates a WARN log with stack trace referencing a column that doesn't exist in the database schema:

```
WARN tools/probe.go:561 Failed to fetch schema relationships with metrics {"project_id": "2b5b014f-191a-41b4-b207-85f7d5c3b04b", "error": "failed to query schema relationships: ERROR: column sc.table_id does not exist (SQLSTATE 42703)"}
```

This error occurs for every call to `probe_relationship`, regardless of input.

## Expected Behavior

The probe_relationship query should reference columns that exist in the schema.

## Steps to Reproduce

1. Call `mcp__mcp_test_suite__probe_relationship` with any valid entities:
   ```json
   {
     "from_entity": "Account",
     "to_entity": "User"
   }
   ```

2. Observe WARN log with stack trace

## Context

- Project ID: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- MCP Server: `mcp_test_suite`
- File: `pkg/mcp/tools/probe.go:561`
- The tool still returns results (falls back gracefully), but logs the warning

## Possibly Related

- Database migration may have changed column names
- Query may reference `sc.table_id` but schema uses different column name
- Could affect probe accuracy if relationship metrics aren't being fetched

## Impact

- WARN logs with stack traces for every probe_relationship call
- Potentially missing relationship metrics in probe results
