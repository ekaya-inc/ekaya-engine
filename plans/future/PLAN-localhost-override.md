# Plan: Allow Admins to Accept Localhost for Server Accessibility

**Status:** TODO

## Context

The AI Data Liaison page (`ui/src/pages/AIDataLiaisonPage.tsx`) has a Setup Checklist with 3 steps:
1. MCP Server set up (complete when ontology DAG is done)
2. MCP Server accessible (complete when server is not localhost AND is HTTPS)
3. Activate AI Data Liaison (disabled until steps 1 & 2 are complete)

Step 2 is driven by `GET /api/config/server-status` (`pkg/handlers/config.go:73`), which computes:
```go
AccessibleForBusinessUsers: !isLocalhost && isHTTPS
```

This is hardcoded from `config.BaseURL`. There is no project-level override. On localhost, step 2 can never be "complete", which blocks step 3 (Activate). Admins on shared machines or local dev need a way to say "localhost is fine" and proceed.

## Approach

Add a project-scoped server status endpoint that layers a project override on top of the global status. Store the override in `Project.Parameters` (existing JSONB field in `engine_projects` — no migration needed).

The existing global `GET /api/config/server-status` endpoint stays as-is for backward compatibility. Both `ServerSetupPage` and `AIDataLiaisonPage` switch to the new project-scoped endpoint.

## Changes

### 1. Backend: Add project-scoped server status endpoints
**File:** `pkg/handlers/projects.go`

**`GET /api/projects/{pid}/server-status`:**
- Compute `isLocalhost` and `isHTTPS` from `config.BaseURL` (same logic as `ConfigHandler.GetServerStatus` in `pkg/handlers/config.go:73-88`)
- Fetch the project (via project service or repository) and read `project.Parameters["localhost_override"]`
- If `localhost_override` is `true`, set `AccessibleForBusinessUsers = true` regardless of localhost/HTTPS
- Return `ProjectServerStatusResponse` (see type below)

**`POST /api/projects/{pid}/server-status/localhost-override`:**
- Requires admin role (check JWT claims `role == "admin"`)
- Request body: `{ "enabled": true }`  or `{ "enabled": false }`
- Read current `project.Parameters`, set `parameters["localhost_override"] = enabled`, save back
- Return updated `ProjectServerStatusResponse`

**Response type:**
```go
type ProjectServerStatusResponse struct {
    BaseURL                    string `json:"base_url"`
    IsLocalhost                bool   `json:"is_localhost"`
    IsHTTPS                    bool   `json:"is_https"`
    AccessibleForBusinessUsers bool   `json:"accessible_for_business_users"`
    LocalhostOverride          bool   `json:"localhost_override"`
}
```

Register routes in the projects handler's `RegisterRoutes` method.

### 2. Frontend: Update `ServerStatusResponse` type
**File:** `ui/src/types/mcp.ts`

Add `localhost_override?: boolean` to the existing `ServerStatusResponse` interface (lines 41-46).

### 3. Frontend: Add API methods
**File:** `ui/src/services/engineApi.ts`

Add two methods:
```ts
// GET /api/projects/{projectId}/server-status
async getProjectServerStatus(projectId: string): Promise<ServerStatusResponse | null> {
  try {
    const response = await this.makeRequest<ServerStatusResponse>(`/${projectId}/server-status`);
    return response.data ?? null;
  } catch {
    return null;
  }
}

// POST /api/projects/{projectId}/server-status/localhost-override
async setLocalhostOverride(
  projectId: string,
  enabled: boolean
): Promise<ApiResponse<ServerStatusResponse>> {
  return this.makeRequest<ServerStatusResponse>(
    `/${projectId}/server-status/localhost-override`,
    { method: 'POST', body: JSON.stringify({ enabled }) }
  );
}
```

### 4. Frontend: Update ServerSetupPage Verify & Sync section
**File:** `ui/src/pages/ServerSetupPage.tsx`

- Switch `fetchStatus` from `engineApi.getServerStatus()` to `engineApi.getProjectServerStatus(pid)` (pid is available from `useParams`)
- In the Verify & Sync card, when `isLocalhost && !localhostOverride`:
  - Show a secondary button: "Use Localhost" (outline variant)
  - Clicking it calls `engineApi.setLocalhostOverride(pid, true)` then refreshes status
- When `localhostOverride` is active:
  - Show an info banner: "Localhost override enabled — business users on this machine can connect"
  - Include an "Undo" link/button that calls `setLocalhostOverride(pid, false)`

### 5. Frontend: Update AIDataLiaisonPage
**File:** `ui/src/pages/AIDataLiaisonPage.tsx`

- In `fetchChecklistData`, switch from `engineApi.getServerStatus()` to `engineApi.getProjectServerStatus(pid)` (line ~83)
- Everything else works as-is because it reads `serverStatus?.accessible_for_business_users` which now respects the override

### 6. Update tests

**`ui/src/pages/__tests__/AIDataLiaisonPage.test.tsx`:**
- Update mock from `getServerStatus` to `getProjectServerStatus`
- Add test: step 2 shows complete when localhost override is active (mock returns `accessible_for_business_users: true, localhost_override: true`)

**Backend tests:**
- Test `GET /api/projects/{pid}/server-status` returns correct status with and without override
- Test `POST /api/projects/{pid}/server-status/localhost-override` toggles the flag and persists it

## Key Files
| File | Change |
|------|--------|
| `pkg/handlers/projects.go` | New endpoints + response type |
| `pkg/handlers/config.go` | Keep as-is (global endpoint preserved) |
| `pkg/models/project.go` | No changes — uses existing `Parameters` JSONB field |
| `ui/src/services/engineApi.ts` | New `getProjectServerStatus` + `setLocalhostOverride` methods |
| `ui/src/types/mcp.ts` | Add `localhost_override` to `ServerStatusResponse` |
| `ui/src/pages/ServerSetupPage.tsx` | Override button in Verify & Sync |
| `ui/src/pages/AIDataLiaisonPage.tsx` | Switch to project-scoped endpoint |
| `ui/src/pages/__tests__/AIDataLiaisonPage.test.tsx` | Update mocks |

## Verification

1. `make check` — all tests pass
2. Manual: on localhost, go to `/projects/{pid}/server-setup`, see "Use Localhost" button in Verify & Sync
3. Click it — banner shows override is active
4. Navigate to `/projects/{pid}/ai-data-liaison` — step 2 shows green check + "Manage"
5. Step 3 (Activate) is now enabled
6. Go back to server-setup, click "Undo" — override disabled, step 2 goes back to pending
