# Plan: Fix MCP Test Suite Issues

## Overview

This plan addresses issues discovered during the MCP tool test suite run on 2026-01-27. Each task references an ISSUE file with reproduction steps and observed behavior.

**Working Directory:** Project root (`ekaya-engine/`)

**Approach:** For each issue:
1. Read the ISSUE file to understand the problem
2. Investigate the codebase to find root cause
3. Implement the fix
4. Verify the fix resolves the issue
5. Delete the ISSUE file after the fix is committed

---

## Task 1: Fix Entity Creation Not Persisted [NOT REPRODUCIBLE] ✅

**Issue:** `./plans/ISSUE-entity-creation-not-persisted.md`

**Summary:** `update_entity` MCP tool returns `{"created": true}` but the entity is not actually persisted. Immediately calling `get_entity` returns `ENTITY_NOT_FOUND`.

**Status:** Investigation found no code bug. All integration tests pass, including a test that specifically reproduces the reported scenario with separate database connections.

**Findings:**
- Code correctly commits entity to database without requiring a transaction
- RLS policies are correctly configured
- Manual MCP testing confirms entity creation and retrieval works
- New integration test `TestEntityTools_Integration_SeparateConnections` verifies the exact scenario

**Likely cause:** Test environment issue (stale data, server restart, or configuration mismatch)

---

## Task 2: Fix Question Status Transition Protection

**Issue:** `./plans/ISSUE-question-status-transition-not-protected.md`

**Summary:** `skip_ontology_question` can change a question's status from "answered" to "skipped", overwriting the resolved state.

**Investigation:**
- Find the `skip_ontology_question`, `dismiss_ontology_question`, `escalate_ontology_question` implementations
- Check if there's status validation before transitions
- Determine valid state transitions (e.g., pending→answered, pending→skipped, but NOT answered→skipped)

**Fix approach:**
- Add status validation before allowing transitions
- Return error if question is already in a terminal state (answered, dismissed)

**Verification:**
```bash
# After fix:
# 1. Resolve a question
# 2. Try to skip the same question
# 3. Should return error, not success
```

---

## Task 3: Wrap MCP Errors in JSON Responses

**Issue:** `./plans/ISSUE-mcp-errors-not-wrapped-in-json.md`

**Summary:** The `execute` tool (and potentially others) returns MCP protocol errors (`MCP error -32603: ...`) instead of JSON error objects. This causes Claude Desktop to show error badges.

**Investigation:**
- Find where errors are returned from MCP tool handlers
- Understand the difference between returning an error vs returning a JSON object with error info
- Check the mcp-go library patterns for error handling

**Fix approach:**
- Catch expected errors (SQL syntax, constraint violations, table not found)
- Return them as JSON: `{"error": true, "code": "...", "message": "..."}`
- Only use MCP protocol errors for unexpected server failures

**Verification:**
```bash
# After fix:
# 1. Call execute with invalid SQL: "SELEKT * FROM users"
# 2. Should return JSON error, not MCP protocol error
# 3. Claude Desktop should NOT show error badge
```

---

## Task 4: Fix Input Errors Logged as Server Errors

**Issue:** `./plans/ISSUE-input-errors-logged-as-server-errors.md`

**Summary:** Input validation errors (bad SQL, missing tables, constraint violations) are logged as ERROR with full stack traces, making them look like server bugs.

**Investigation:**
- Review logging patterns in `pkg/mcp/tools/developer.go`, `glossary.go`, `dev_queries.go`
- Understand the difference between input errors and server errors
- Check if there's an existing pattern for "expected" vs "unexpected" errors

**Fix approach:**
- Use DEBUG or INFO level for input validation failures
- Remove stack traces from expected error conditions
- Keep ERROR + stack trace only for unexpected server failures

**Files to update:**
- `pkg/mcp/tools/developer.go:718`
- `pkg/mcp/tools/glossary.go:456`
- `pkg/mcp/tools/dev_queries.go:763`
- `pkg/mcp/tools/dev_queries.go:1120`

**Verification:**
```bash
# After fix:
# 1. Call execute with invalid SQL
# 2. Check server logs - should be DEBUG/INFO, no stack trace
```

---

## Task 5: Fix probe_relationship Schema Column Reference

**Issue:** `./plans/ISSUE-probe-relationship-schema-column-missing.md`

**Summary:** `probe_relationship` tool references non-existent column `sc.table_id`, generating WARN logs with stack traces on every call.

**Investigation:**
- Find the query at `pkg/mcp/tools/probe.go:561`
- Check the actual schema of the table being queried
- Determine correct column name or if the feature needs different implementation

**Verification:**
```bash
# After fix:
# 1. Call probe_relationship with valid entities
# 2. Check server logs - should be no WARN about missing column
```

---

## Task 6: Fix search_schema Entity Search Column Reference

**Issue:** `./plans/ISSUE-search-schema-deleted-at-column-missing.md`

**Summary:** `search_schema` tool references non-existent column `a.deleted_at` when searching entities, causing entity search to silently fail.

**Investigation:**
- Find the query at `pkg/mcp/tools/search.go:196`
- Check if entities table has soft delete support
- Determine if column should exist (migration missing) or query should be updated

**Verification:**
```bash
# After fix:
# 1. Call search_schema with query that should match entities
# 2. Check server logs - should be no error about missing column
# 3. Entity results should appear in search output
```

---

## Completion

After all tasks are complete:
1. Run the MCP test suite again to verify fixes
2. Check `output.log.txt` for any remaining ERROR/WARN logs
3. Ensure all ISSUE files have been deleted
