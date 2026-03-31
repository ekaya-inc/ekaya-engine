# FIX: MCP Events should use the real security levels and hide expected auth failures

**Status:** Open
**Date:** 2026-03-25

## Problem

The MCP Events page currently presents a security-level filter that does not match the values stored in the audit log.

Observed current behavior:

- The page dropdown shows `normal`, `elevated`, and `critical`
- The backend audit model and migration define `normal`, `warning`, and `critical`
- A real MCP event row can appear with `security = warning`, but there is no `warning` option in the dropdown
- Selecting `elevated` sends a filter value that does not correspond to any stored MCP security level

The screenshot from this report shows a concrete example:

```text
Mar 25, 01:13 PM
user: unknown
tool: -
event: mcp auth failure
status: Failed
security: warning
error: Invalid or expired token
security_flags: auth_failure
```

That specific error is expected in normal usage when an MCP session token has expired and the user needs to sign in again. It should not clutter the MCP Events page.

## Root Cause

There are two separate issues.

### 1. The frontend hardcodes the wrong security level

`ui/src/pages/MCPEventsPage.tsx` hardcodes `elevated` in two places:

- the Security Level dropdown
- the `securityColor(...)` chip styling helper

A repo-wide search shows `elevated` only in that page. The actual MCP audit model does not define it.

### 2. The MCP Events list endpoint returns expected auth-failure noise

`pkg/mcp/audit.go` intentionally records auth middleware failures as dedicated MCP audit rows:

- `event_type = mcp_auth_failure`
- `was_successful = false`
- `security_level = warning`
- `security_flags = ["auth_failure"]`

`pkg/repositories/mcp_audit_repository.go` then lists all MCP audit rows for the MCP Events page without excluding those expected auth-failure events.

## Why this matters

- Users cannot filter by a real security level that exists in the data
- The UI suggests an `elevated` level that the backend does not support
- Routine expired-token events make the page look noisier and more alarming than it should
- Actual tool failures and meaningful warning/critical events are harder to scan when expected auth-expiry rows are mixed in

## Existing implementation context

### Security-level source of truth

`migrations/010_mcp_audit.up.sql` documents the MCP audit security levels as:

- `normal`
- `warning`
- `critical`

`pkg/models/mcp_audit.go` defines the same constants:

- `MCPSecurityNormal`
- `MCPSecurityWarning`
- `MCPSecurityCritical`

No backend enum or migration comment defines `elevated`.

### MCP Events page

`ui/src/pages/MCPEventsPage.tsx` currently:

- stores the selected filter in `securityLevelFilter`
- passes it through to `engineApi.listAuditMCPEvents(...)` as `security_level`
- renders dropdown options `normal`, `elevated`, `critical`
- styles `elevated` as the amber warning state

This page is the only place in the repo still using `elevated`.

### MCP auth-failure write path

`pkg/mcp/audit.go`

- `RecordAuthFailure(...)` creates `MCPEventAuthFailure`
- sets `SecurityLevel` to `models.MCPSecurityWarning`
- adds `auth_failure` to `SecurityFlags`

This is why the page can legitimately display `warning` even though the dropdown does not offer it.

### MCP Events list path

The page reads from:

- `ui/src/services/engineApi.ts` -> `listAuditMCPEvents(...)`
- `pkg/handlers/audit_page_handler.go` -> `ListMCPEvents(...)`
- `pkg/services/audit_page_service.go` -> `ListMCPEvents(...)`
- `pkg/repositories/mcp_audit_repository.go` -> `List(...)`

The repository is the best place to suppress expected auth-failure rows because it owns both:

- the paginated data query
- the matching total-count query

Filtering later in the stack would risk count/pagination drift.

## Intended fix

Keep this narrow. Do not add compatibility aliases or broaden the behavior beyond the MCP Events listing.

### 1. Replace `elevated` with `warning` in the MCP Events UI

Update `ui/src/pages/MCPEventsPage.tsx` so the page uses the real MCP security levels:

- `normal`
- `warning`
- `critical`

The amber styling currently attached to `elevated` should move to `warning`.

Preferred approach:

- define a small local constant or shared page-level source of truth for the security options
- use that same vocabulary for both the dropdown and badge styling

Do not add a backend alias that accepts `elevated`.

### 2. Exclude expected auth failures from the MCP Events list endpoint

Update `pkg/repositories/mcp_audit_repository.go` so `List(...)` excludes the dedicated expected auth-failure event type from the MCP Events page results:

- exclude `event_type = 'mcp_auth_failure'`

Apply the same exclusion to both:

- the `COUNT(*)` query
- the paginated row query

This keeps pagination correct and removes the expected expired-token noise from the page without changing how the underlying audit event is recorded.

This fix should be narrowly targeted:

- hide the dedicated `mcp_auth_failure` event type from the MCP Events listing
- do not hide all `warning` rows
- do not hide generic `tool_error` rows that happen to carry other warning conditions

## File-by-file changes

### 1. `ui/src/pages/MCPEventsPage.tsx`

- [ ] Replace the Security Level dropdown options with `normal`, `warning`, `critical`
- [ ] Update `securityColor(...)` so `warning` receives the amber styling
- [ ] Remove the obsolete `elevated` value completely
- [ ] Keep the request parameter name as `security_level`

### 2. `pkg/repositories/mcp_audit_repository.go`

- [ ] Add a default `event_type != 'mcp_auth_failure'` exclusion in `List(...)`
- [ ] Apply that exclusion to both the total-count query and the data query
- [ ] Keep existing project, time-range, tool, event-type, and security-level filters working for the remaining rows

### 3. `pkg/handlers/audit_page_handler.go`

- [ ] No API shape change is required
- [ ] Keep the existing `security_level` query param contract unchanged

## Tests to add

### Frontend

Add `ui/src/pages/__tests__/MCPEventsPage.test.tsx`.

Cover at least:

- [ ] the page renders `warning` in the Security Level dropdown
- [ ] the page no longer renders `elevated`
- [ ] selecting `warning` results in a request with `security_level=warning`
- [ ] a row with `security_level = warning` renders with the amber warning styling

Use existing page-test patterns in `ui/src/pages/__tests__/ProjectDashboard.test.tsx` as a reference for router setup and mocking.

### Backend

Add a focused repository test for `pkg/repositories/mcp_audit_repository.go` (a new `pkg/repositories/mcp_audit_repository_test.go` is reasonable).

Seed at least:

- [ ] one normal MCP row such as `tool_call`
- [ ] one warning MCP row that should remain visible such as `tool_error`
- [ ] one `mcp_auth_failure` row

Assert that:

- [ ] `List(...)` returns the non-auth rows only
- [ ] `total` excludes the `mcp_auth_failure` row
- [ ] filtering by `security_level = warning` still returns warning rows that are not `mcp_auth_failure`

## Non-goals

- [ ] Do not add `elevated` as a new backend security level
- [ ] Do not add backward-compatibility alias handling for `elevated`
- [ ] Do not stop recording auth failures in `pkg/mcp/audit.go`
- [ ] Do not reclassify expected auth failures from `warning` to `normal`
- [ ] Do not broaden this task into alerting changes, dashboard-summary changes, or non-MCP audit behavior

## Expected outcome

After this fix:

- the MCP Events security dropdown matches the real stored enum values
- `warning` rows can be filtered explicitly
- expected expired-token `mcp_auth_failure` entries no longer clutter the MCP Events page
- genuine warning and critical MCP events remain visible
