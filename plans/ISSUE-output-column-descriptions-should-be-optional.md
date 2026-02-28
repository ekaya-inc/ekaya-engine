# ISSUE: output_column_descriptions Should Be Optional in create_approved_query

**Status:** FIXED
**Severity:** Low
**Area:** MCP tool — `create_approved_query` (`pkg/mcp/tools/dev_queries.go`)

## Problem

The `create_approved_query` MCP tool requires `output_column_descriptions` even though:
1. The MCP tool schema marks it as optional
2. The UI does not require it
3. The field is not structurally necessary — the SQL SELECT clause already communicates what columns are returned

When omitted, the server returns:

```
MCP error -32603: failed to create query: output_column_descriptions parameter is required.
Provide descriptions for output columns, e.g., {"total": "Total count of records"}
```

## Why It Should Be Optional

The primary consumer of pre-approved queries is an LLM agent via `list_approved_queries` + `execute_approved_query`. When deciding which query to use, the agent has access to the query's `name`, `description`, `sql`, and `parameters` — the SQL SELECT clause already reveals the output columns.

Output column descriptions add value for ambiguous column names (e.g., `cpa`, `activation_rate`, `spend` needing currency/unit clarification), but for most queries they parrot the column name (`"id": "The ID"`, `"name": "Application name"`). Making them required:
- Creates unnecessary friction when creating queries
- Leads to low-quality filler descriptions to satisfy the validator
- Contradicts the tool schema which marks the field as optional

## Fix

- Remove the server-side requirement that `output_column_descriptions` be present
- Default to `{}` when omitted
- Keep the field available for queries where column semantics are non-obvious
