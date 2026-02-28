# ISSUE: MCP Tool Errors Return Protocol-Level Errors Instead of JSON Content

**Status:** Fixed
**Severity:** Medium — causes silent failures in some MCP clients
**Area:** MCP tool error handling

## Problem

When an MCP tool call fails due to a validation error (e.g., missing required parameter), the server returns an MCP protocol-level error (`-32603`) instead of a successful tool response containing a JSON error body.

Some MCP clients do **not** surface protocol-level errors back to the LLM, which means the LLM never learns what went wrong and cannot self-correct. The tool call silently disappears.

## Observed Behavior

```
ekaya_marketing - create_approved_query (MCP)(
  name: "List All Applications",
  description: "Get all Ekaya applications with their billing and status info",
  sql: "SELECT id, name, slug, buyer, billing, billing_detail, status, gtm_role FROM applications ORDER BY id",
  tags: ["applications","reference"]
)

⎿  Error: MCP error -32603: failed to create query: output_column_descriptions parameter is required.
   Provide descriptions for output columns, e.g., {"total": "Total count of records"}
```

## Expected Behavior

The tool call should **succeed** at the MCP protocol level and return a JSON response indicating the error in the content body:

```json
{
  "success": false,
  "error": "output_column_descriptions parameter is required. Provide descriptions for output columns, e.g., {\"total\": \"Total count of records\"}"
}
```

This ensures:
1. All MCP clients relay the error to the LLM
2. The LLM can read the error message and retry with corrected parameters
3. The interaction follows the same JSON contract as successful responses

## Scope

This likely affects **all** MCP tool handlers that return errors via the MCP error code path rather than as JSON content. An audit should check:
- All tool handlers in `pkg/mcp/tools/` — especially `dev_queries.go`, `queries.go`, and any other tools that validate inputs
- The shared error-handling pattern used across tools
- Whether there's a central place to wrap validation errors as JSON content responses

## Fix Approach

Distinguish between:
- **Infrastructure errors** (server crash, DB connection lost) → MCP protocol error is appropriate
- **Validation/business errors** (missing param, invalid SQL, not found) → should return `{"success": false, "error": "..."}` as tool content
