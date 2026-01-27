# Issue: search_schema Tool References Non-Existent deleted_at Column

## Observed Behavior

The `search_schema` tool logs errors when trying to search entities, referencing a column that doesn't exist:

```
DEBUG tools/search.go:196 Failed to search entities (ontology may not exist) {"project_id": "2b5b014f-191a-41b4-b207-85f7d5c3b04b", "error": "failed to search entities: ERROR: column a.deleted_at does not exist (SQLSTATE 42703)"}
```

This error occurs on every search query, regardless of input.

## Expected Behavior

The entity search query should reference columns that exist in the schema.

## Steps to Reproduce

1. Call `mcp__mcp_test_suite__search_schema` with any query:
   ```json
   {
     "query": "user"
   }
   ```

2. Observe DEBUG log with entity search error

## Context

- Project ID: `2b5b014f-191a-41b4-b207-85f7d5c3b04b`
- MCP Server: `mcp_test_suite`
- File: `pkg/mcp/tools/search.go:196`
- The tool still returns results (searches other sources), but entity search fails

## Possibly Related

- Entity table may not have `deleted_at` column
- Soft delete feature may not be fully implemented
- May affect search completeness if entities aren't being searched

## Impact

- Entity results missing from search_schema responses
- DEBUG logs cluttered with repeated errors
