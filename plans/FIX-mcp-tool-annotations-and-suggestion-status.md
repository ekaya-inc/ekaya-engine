# FIX: MCP tool annotations and invalid suggestion status filter

**Status:** Open
**Date:** 2026-03-12

## Problem 1: Read-only MCP tools missing annotations

The `list_query_suggestions` tool (and likely other tools in `dev_queries.go`) has **no annotations at all**, causing Claude Code to treat them as destructive/open-world and prompt for user confirmation on every call.

### Root Cause

Tools registered in `pkg/mcp/tools/dev_queries.go` were not given annotation options when created via `mcp.NewTool()`. By contrast, tools in `pkg/mcp/tools/queries.go` and `pkg/mcp/tools/questions.go` correctly set annotations like:

```go
mcp.WithReadOnlyHintAnnotation(true),
mcp.WithDestructiveHintAnnotation(false),
mcp.WithIdempotentHintAnnotation(true),
mcp.WithOpenWorldHintAnnotation(false),
```

### Fix

In `pkg/mcp/tools/dev_queries.go`:

- [ ] Audit every tool registration in the file and add appropriate annotations
- [ ] Read-only tools (e.g. `list_query_suggestions`) should get `ReadOnly=true, Destructive=false, Idempotent=true, OpenWorld=false`
- [ ] Mutating tools (e.g. `approve_query_suggestion`, `reject_query_suggestion`) should get `ReadOnly=false, Destructive=false, Idempotent=true, OpenWorld=false`
- [ ] Audit other tool files (`knowledge.go`, `glossary.go`, etc.) for the same missing annotations

---

## Problem 2: `list_query_suggestions` documents "approved" as a valid status filter but it's not meaningful

The `status` parameter description says "Filter by status: pending, approved, rejected (default: pending)" but the implementation in `dev_queries.go` (lines 119-134) only actually queries the database for `pending`. The `approved` and `rejected` branches silently return empty lists.

### Root Cause

Two issues:

1. **Implementation gap:** The handler only calls `deps.QueryService.ListPending()` for `status == "pending"`. For any other status value, it returns an empty list with no error — a silent no-op.

2. **Semantic mismatch:** When a new query suggestion is approved, its status changes to `approved` and it becomes a regular approved query (visible via `list_approved_queries`). When an update suggestion is approved, the suggestion record is **soft-deleted** (not status-changed). So filtering suggestions by `status=approved` is semantically confusing — approved items are no longer "suggestions."

### Fix

- [ ] Update the `status` parameter description in `pkg/mcp/tools/dev_queries.go` (line 68) to only document `pending` and `rejected` as valid values, since approved suggestions graduate to the approved queries list
- [ ] Implement `rejected` status filtering in the handler so it actually works (add a repository method if needed, or remove it from the docs)
- [ ] Consider returning an error or warning when an unsupported status value is passed, rather than silently returning empty results
