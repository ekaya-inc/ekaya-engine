# FIX: MCP Array Parameters Received as Serialized Strings

**Status:** FIXED
**Priority:** High — breaks parameterized query suggestions from some MCP clients
**Discovered:** 2026-02-11 during testing of `suggest_approved_query` via Claude Code MCP client

## Problem

When Claude Code calls MCP tools that accept array parameters (e.g., `parameters`, `tags`), some calls serialize the arrays as **JSON strings** instead of **native JSON arrays**. The server-side type assertion `args["parameters"].([]any)` silently fails on a string value, causing the parameters to be ignored entirely.

### Evidence from output.log

**Working call (test_data project, first call in session):**
```json
"parameters":[{"description":"Number of rows to return","example":20,"name":"limit",...}]
"tags":["inform_infos"]
```
`parameters` is a JSON array. `tags` is a JSON array. Type assertions succeed.

**Failing call (ai-admin project, same session, same server):**
```json
"parameters":"[{\"name\": \"limit\", \"type\": \"integer\", ...}]"
"tags":"[\"inform_infos\"]"
```
`parameters` is a **JSON string** containing escaped JSON. `tags` is a **JSON string**. Type assertions silently fail.

### Consequence

For `suggest_approved_query`: The `{{param}}` placeholders in the SQL are sent raw to PostgreSQL, causing `syntax error at or near "{"`. The tool returns an error and the query suggestion is rejected.

This affects **all tools** that use `WithArray()` — the silent failure means any array parameter could be dropped without warning.

## Root Cause Analysis

### Two Contributing Factors

**1. Missing `items` schema on `WithArray()` declarations**

All `WithArray("parameters", ...)` declarations specify only a description, no `items` schema:
```go
mcp.WithArray(
    "parameters",
    mcp.Description("Parameter definitions (inferred from SQL if omitted)"),
)
```

This produces a JSON Schema with `"type": "array"` but no `items`. The MCP client doesn't know the expected element structure. Without structural guidance, some MCP client implementations may serialize the value as a string rather than construct a proper JSON array.

**2. Silent failure on type assertion**

The handler code uses a nested type assertion that silently skips when the type doesn't match:
```go
if paramsArray, ok := args["parameters"].([]any); ok {
    // only reached if parameters is a native []any
}
// silently falls through — no error, no log
```

When `args["parameters"]` is a `string` instead of `[]any`, the assertion fails and the code continues as if no parameters were provided.

## Fix

### A. Add `items` schema to all `WithArray("parameters", ...)` declarations

The `mcp-go` SDK provides `mcp.Items(schema any)` for this. Add structured `items` to all parameter array declarations:

```go
mcp.WithArray(
    "parameters",
    mcp.Description("Parameter definitions for {{placeholder}} values in SQL"),
    mcp.Items(map[string]any{
        "type": "object",
        "properties": map[string]any{
            "name":        map[string]any{"type": "string", "description": "Parameter name matching {{name}} in SQL"},
            "type":        map[string]any{"type": "string", "description": "Data type: string, integer, decimal, uuid, date, timestamp, boolean"},
            "description": map[string]any{"type": "string", "description": "What this parameter represents"},
            "required":    map[string]any{"type": "boolean", "description": "Whether required (default: true)"},
            "example":     map[string]any{"description": "Example value for validation dry-run"},
        },
        "required": []string{"name", "type"},
    }),
),
```

Similarly for `tags`:
```go
mcp.WithArray(
    "tags",
    mcp.Description("Tags for organizing queries"),
    mcp.Items(map[string]any{"type": "string"}),
),
```

### B. Add defensive string-to-array deserialization with error feedback

Even with proper schemas, MCP clients may still send stringified arrays. Add a helper that:
1. Tries native `[]any` type assertion first
2. Falls back to `json.Unmarshal` when the value is a string
3. **Returns an MCP-visible error** if the value exists but can't be parsed as an array — so the MCP client gets actionable feedback instead of silent parameter dropping

```go
// extractArrayParam extracts an array parameter from MCP tool arguments.
// Handles both native JSON arrays and stringified JSON arrays (MCP client bug workaround).
// Returns (array, nil) on success, (nil, nil) if key is absent, or (nil, error) if
// the value exists but cannot be parsed as an array.
func extractArrayParam(args map[string]any, key string) ([]any, error) {
    val, exists := args[key]
    if !exists {
        return nil, nil
    }
    // Native JSON array — expected path
    if arr, ok := val.([]any); ok {
        return arr, nil
    }
    // Fallback: MCP client sent stringified JSON array
    if str, ok := val.(string); ok {
        var arr []any
        if err := json.Unmarshal([]byte(str), &arr); err == nil {
            // Log warning for observability
            return arr, nil
        }
        return nil, fmt.Errorf("parameter %q: expected a JSON array but received a string that could not be parsed — send as a native array, not a stringified one", key)
    }
    return nil, fmt.Errorf("parameter %q: expected a JSON array but received %T", key, val)
}
```

The error return uses `NewErrorResult()` (per architecture rule #6) so the MCP client LLM sees the fix guidance:
```go
arr, err := extractArrayParam(args, "parameters")
if err != nil {
    return NewErrorResult("invalid_parameters", err.Error()), nil
}
```

Apply this helper everywhere array parameters are extracted. Also update the existing `getStringSlice` helper in `ontology.go` to use it.

### C. Add warning log when string fallback is used

When the string-to-array fallback triggers, log a warning so we can track which clients/scenarios cause this.

## Affected Files

All 16 `WithArray()` declarations across 7 files have no `items` schema. All 13 `.([]any)` extraction points silently drop values.

### Array extraction points (must use `extractArrayParam`):

| File | Line | Parameter | Tool |
|------|------|-----------|------|
| `queries.go` | 135 | `tags` | `list_approved_queries` |
| `queries.go` | 626 | `parameters` | `suggest_approved_query` |
| `queries.go` | 651 | `tags` | `suggest_approved_query` |
| `queries.go` | 863 | `parameters` | `suggest_query_update` |
| `queries.go` | 886 | `tags` | `suggest_query_update` |
| `dev_queries.go` | 714 | `parameters` | `create_approved_query` |
| `dev_queries.go` | 740 | `tags` | `create_approved_query` |
| `dev_queries.go` | 989 | `parameters` | `update_approved_query` |
| `dev_queries.go` | 1018 | `tags` | `update_approved_query` |
| `ontology.go` | 37 | (via `getStringSlice`) | `get_ontology`, `get_context` |
| `probe.go` | 157 | `columns` | `probe_columns` |
| `column.go` | 370 | `enum_values` | `update_column` |
| `glossary.go` | 369 | `aliases` | `update_glossary_term` |
| `ontology_batch.go` | 261 | `updates` | `update_columns` |
| `ontology_batch.go` | 318 | `enum_values` (nested) | `update_columns` |

## Existing Tests to Update

`queries_test.go:945` — `TestListApprovedQueriesTool_ErrorResults` currently expects `"not-an-array"` (a non-JSON string) to be an error. After the fix, that test case must change: a **parsable** stringified array like `"[\"tag1\"]"` should succeed, while a truly unparsable string like `"not-an-array"` should still error with an actionable message.

## Checklist

### Step 1: TDD RED — Write failing tests for all affected tools

Write tests in `pkg/mcp/tools/helpers_test.go` and tool-specific test files that assert the desired behavior **before** implementing the fix. All tests should fail (RED) initially.

**A. `extractArrayParam` helper tests** (`helpers_test.go`):

| Test case | Input | Expected |
|-----------|-------|----------|
| Native array | `[]any{"a", "b"}` | Returns `["a", "b"]`, nil |
| Stringified parsable string array | `"[\"a\",\"b\"]"` | Returns `["a", "b"]`, nil |
| Stringified parsable object array | `"[{\"name\":\"limit\",\"type\":\"integer\"}]"` | Returns parsed array, nil |
| Unparsable string | `"not-an-array"` | Returns nil, error with actionable message |
| Wrong type (number) | `123` | Returns nil, error mentioning received type |
| Absent key | (key not in map) | Returns nil, nil |

**B. `getStringSlice` tests** (update existing in `ontology_test.go`):

| Test case | Input | Expected |
|-----------|-------|----------|
| Stringified string array | `"[\"users\",\"orders\"]"` | Returns `["users", "orders"]` |
| Unparsable string | `"not-an-array"` | Returns nil (current behavior preserved) |

**C. Tool-level tests for all affected array parameters:**

For each tool, add test cases using `HandleMessage` with JSON-RPC `tools/call` where array params are sent as stringified JSON. Test both parsable and unparsable inputs. Each tool needs two cases:

| Tool | Parameter | Parsable stringified input | Unparsable input |
|------|-----------|---------------------------|------------------|
| `list_approved_queries` | `tags` | `"[\"billing\"]"` → succeeds | `"not-json"` → error |
| `suggest_approved_query` | `parameters` | `"[{\"name\":\"limit\",\"type\":\"integer\",\"example\":20}]"` → succeeds | `"bad"` → error |
| `suggest_approved_query` | `tags` | `"[\"tag1\"]"` → succeeds | `"bad"` → error |
| `suggest_query_update` | `parameters` | same pattern | same pattern |
| `suggest_query_update` | `tags` | same pattern | same pattern |
| `create_approved_query` | `parameters` | same pattern | same pattern |
| `create_approved_query` | `tags` | same pattern | same pattern |
| `update_approved_query` | `parameters` | same pattern | same pattern |
| `update_approved_query` | `tags` | same pattern | same pattern |
| `get_ontology` | `tables` | `"[\"users\"]"` → succeeds | `"bad"` → error |
| `get_context` | `tables`, `include` | same pattern | same pattern |
| `probe_columns` | `columns` | `"[\"users.id\"]"` → succeeds | `"bad"` → error |
| `update_column` | `enum_values` | `"[\"active\",\"inactive\"]"` → succeeds | `"bad"` → error |
| `update_glossary_term` | `aliases` | `"[\"alias1\"]"` → succeeds | `"bad"` → error |
| `update_columns` | `updates` | stringified object array → succeeds | `"bad"` → error |

For tools that require auth/DB mocks and can't easily test via `HandleMessage`, test via the refactored shared extraction functions directly (see Step 2).

**D. Update existing test** (`queries_test.go:945`):
- Change `"not-an-array"` test case to expect the **new** error message format (mentioning "could not be parsed")
- Add new test case for parsable stringified array `"[\"tag1\",\"tag2\"]"` that expects **success** (no error)

### Step 2: Implement shared `extractArrayParam` helper

Create `extractArrayParam` in `pkg/mcp/tools/helpers.go`. Also create `extractStringSlice` that wraps `extractArrayParam` + string element validation — this replaces the current `getStringSlice` pattern and the inline tag-parsing logic duplicated across tools.

```go
// extractStringSlice extracts a string array parameter, handling stringified JSON.
// Returns ([]string, nil) on success, (nil, nil) if absent, (nil, error) if malformed.
func extractStringSlice(args map[string]any, key string) ([]string, error) {
    arr, err := extractArrayParam(args, key)
    if err != nil || arr == nil {
        return nil, err
    }
    result := make([]string, 0, len(arr))
    for i, item := range arr {
        str, ok := item.(string)
        if !ok {
            return nil, fmt.Errorf("parameter %q: element %d must be a string, got %T", key, i, item)
        }
        result = append(result, str)
    }
    return result, nil
}
```

Update `getStringSlice` in `ontology.go` to delegate to `extractStringSlice`.

### Step 3: Add `items` schema to all `WithArray()` declarations

Add structured `items` to all 16 `WithArray()` calls (see Fix section A above for schemas).

### Step 4: Apply shared helpers to all extraction points

Replace all 15 inline `.([]any)` type assertions with calls to `extractArrayParam` or `extractStringSlice`. Return errors via `NewErrorResult()` per architecture rule #6.

### Step 5: Add warning logs for string fallback

Log when the string-to-array fallback path is used for observability.

### Step 6: TDD GREEN — Verify all tests pass

- [ ] Run `make check`
- [ ] All RED tests from Step 1 now pass
- [ ] Existing tests still pass (with updated expectations where needed)

### Step 7: Manual verification

- [ ] Test with Claude Code MCP client: call `suggest_approved_query` with `parameters` array
- [ ] Verify parsable stringified arrays are handled correctly
- [ ] Verify unparsable strings return actionable error to MCP client
