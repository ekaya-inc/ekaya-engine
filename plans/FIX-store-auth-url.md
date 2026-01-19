# FIX: Store auth_url Per Project for Multi-Auth-Server Support

## Problem

When a user navigates to a project with `?auth_url=http://localhost:5002`, the server correctly uses this auth URL for the initial OAuth flow. However, when re-authentication is triggered (e.g., on 401/403 during back navigation), the server reverts to its **default auth URL** instead of the per-project auth URL.

### Reproduction Steps
1. Create two projects in ekaya-central emulator (localhost:5002)
2. Open Project1 in Tab1: `http://localhost:3443/projects/{pid1}?auth_url=http://localhost:5002`
3. Open Project2 in Tab2: `http://localhost:3443/projects/{pid2}?auth_url=http://localhost:5002`
4. Navigate within Tab1 (e.g., Datasource → Back)
5. **Bug**: Tab1 redirects to `https://us.dev.ekaya.ai/login` instead of `http://localhost:5002`

### User Impact
- Users testing with local emulators get redirected to production auth
- Multi-tenant scenarios where different projects use different auth servers break
- Tab-scoped authentication becomes unreliable

---

## Root Cause Analysis

### Current Flow (Broken)

```
1. User navigates: /projects/{pid}?auth_url=http://localhost:5002
   └─ auth_url extracted in ConfigProvider (config.ts:25-28)
   └─ Passed to /.well-known/oauth-authorization-server?auth_url=...
   └─ Backend validates it, returns authServerUrl in config
   └─ Cached in memory only

2. User navigates within app → 401/403 response
   └─ fetchWithAuth() detects 401/403 (api.ts:30)
   └─ Calls initiateOAuthFlow(config, projectId)
   └─ BUT: Original ?auth_url query param is GONE from URL
   └─ Backend's wellknown.go queries project.GetAuthServerURL(projectId)
   └─ Returns EMPTY because auth_url was never saved to project
   └─ Falls back to default auth server ❌
```

### Key Code Points

| File | Line | Issue |
|------|------|-------|
| `ui/src/services/config.ts` | 25-28 | `getAuthUrlFromQuery()` extracts but doesn't persist |
| `ui/src/services/config.ts` | 43-58 | `fetchConfig()` passes auth_url but doesn't save it |
| `pkg/handlers/wellknown.go` | 135-146 | Looks up `auth_server_url` from project, but it's never stored |
| `pkg/services/projects.go` | 229-252 | `GetAuthServerURL()` reads from DB but nothing writes there |
| `ui/src/lib/api.ts` | 38-55 | Re-auth uses cached config without original auth_url |

### The Gap

The infrastructure exists:
- `ProjectService.GetAuthServerURL()` can read `auth_server_url` from project parameters
- `wellknown.go` queries this when no `auth_url` query param is present

But **nothing saves the `auth_url` to project parameters** when it's first provided.

---

## Proposed Fix

### Approach: Save auth_url to Project Parameters on First Load

When the frontend receives a valid config with an `authServerUrl` that came from a query parameter, make an API call to persist it to the project's parameters in the backend.

### Implementation

#### 1. Backend: Add endpoint to update project auth_server_url

**File: `pkg/handlers/projects.go`**

Add a new endpoint or extend existing PATCH endpoint:

```go
// PATCH /api/projects/:id/auth-server-url
func (h *ProjectHandler) UpdateAuthServerURL(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "id")

    var req struct {
        AuthServerURL string `json:"auth_server_url"`
    }
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, "Invalid request", http.StatusBadRequest)
        return
    }

    // Validate auth_server_url against whitelist
    if !h.isAllowedAuthServer(req.AuthServerURL) {
        http.Error(w, "Auth server URL not allowed", http.StatusForbidden)
        return
    }

    // Update project parameters
    pid, _ := uuid.Parse(projectID)
    if err := h.projectService.UpdateAuthServerURL(r.Context(), pid, req.AuthServerURL); err != nil {
        http.Error(w, "Failed to update", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
}
```

**File: `pkg/services/projects.go`**

Add method to ProjectService:

```go
func (s *projectService) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
    project, err := s.GetProject(ctx, projectID)
    if err != nil {
        return err
    }

    if project.Parameters == nil {
        project.Parameters = make(map[string]interface{})
    }
    project.Parameters["auth_server_url"] = authServerURL

    return s.repo.UpdateProject(ctx, project)
}
```

#### 2. Frontend: Save auth_url after successful config fetch

**File: `ui/src/services/config.ts`**

After `fetchConfig()` successfully returns with an auth_url from query parameter:

```typescript
export async function fetchConfig(): Promise<OAuthConfig> {
  const authUrl = getAuthUrlFromQuery();
  const projectId = getProjectIdFromPath();

  // ... existing fetch logic ...

  const config = await response.json();

  // If auth_url was provided via query param, persist it to project
  if (authUrl && projectId && config.authServerUrl === authUrl) {
    try {
      await saveAuthUrlToProject(projectId, authUrl);
    } catch (err) {
      console.warn('Failed to persist auth_url to project:', err);
      // Non-fatal - continue with cached value
    }
  }

  cachedConfig = config;
  return config;
}

async function saveAuthUrlToProject(projectId: string, authUrl: string): Promise<void> {
  const response = await fetch(`/api/projects/${projectId}/auth-server-url`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'include',
    body: JSON.stringify({ auth_server_url: authUrl }),
  });

  if (!response.ok) {
    throw new Error(`Failed to save auth_url: ${response.status}`);
  }
}

function getProjectIdFromPath(): string | null {
  const match = window.location.pathname.match(/\/projects\/([a-f0-9-]+)/);
  return match?.[1] ?? null;
}
```

#### 3. Alternative: Store in sessionStorage with Project Scope

If modifying the backend is not desirable, a frontend-only solution:

**File: `ui/src/services/config.ts`**

```typescript
const AUTH_URL_STORAGE_KEY = 'ekaya_project_auth_urls';

function getStoredAuthUrls(): Record<string, string> {
  try {
    return JSON.parse(sessionStorage.getItem(AUTH_URL_STORAGE_KEY) || '{}');
  } catch {
    return {};
  }
}

function storeAuthUrl(projectId: string, authUrl: string): void {
  const stored = getStoredAuthUrls();
  stored[projectId] = authUrl;
  sessionStorage.setItem(AUTH_URL_STORAGE_KEY, JSON.stringify(stored));
}

function getStoredAuthUrl(projectId: string): string | null {
  return getStoredAuthUrls()[projectId] || null;
}

export async function fetchConfig(): Promise<OAuthConfig> {
  const projectId = getProjectIdFromPath();

  // Check for auth_url: query param first, then stored value
  let authUrl = getAuthUrlFromQuery();
  if (!authUrl && projectId) {
    authUrl = getStoredAuthUrl(projectId);
  }

  // ... existing fetch with authUrl ...

  // Store for future use
  if (authUrl && projectId) {
    storeAuthUrl(projectId, authUrl);
  }

  return config;
}
```

**Limitation**: sessionStorage is cleared on tab close, so users would need to re-provide auth_url when opening a new tab.

---

## Recommended Approach

**Backend persistence (Option 1)** is recommended because:
- Survives tab close/browser restart
- Works across multiple tabs for same project
- Consistent with how `auth_server_url` is already stored during provisioning
- The infrastructure already exists (`GetAuthServerURL()`)

---

## Files to Modify

### Backend (Go)
1. `pkg/handlers/projects.go` - Add PATCH endpoint for auth_server_url
2. `pkg/services/projects.go` - Add `UpdateAuthServerURL()` method
3. `pkg/handlers/routes.go` - Register new route

### Frontend (TypeScript)
1. `ui/src/services/config.ts` - Save auth_url after successful config fetch
2. `ui/src/lib/api.ts` - (Optional) Add helper for the PATCH call

---

## Testing

1. Start ekaya-central emulator on localhost:5002
2. Create project via emulator
3. Navigate to `http://localhost:3443/projects/{pid}?auth_url=http://localhost:5002`
4. Verify auth_url is saved to project (check Firestore/DB)
5. Navigate within app (Datasource → Back)
6. Verify re-auth redirects to localhost:5002, not us.dev.ekaya.ai
7. Open new tab without auth_url query param
8. Verify it uses the stored auth_url from step 4

---

## Security Considerations

- The `auth_url` must be validated against the whitelist before storing
- Only project admins should be able to update auth_server_url
- The existing whitelist validation in `wellknown.go` should be reused

---

## Implementation Tasks

### [x] Task 1: Add backend endpoint to update project auth_server_url

**Status:** Complete

**What was implemented:**

1. **`pkg/services/projects.go`** - Added `UpdateAuthServerURL(ctx, projectID, authServerURL)` method to `ProjectService` interface and implementation
   - Fetches project from repository
   - Initializes `Parameters` map if nil
   - Sets `auth_server_url` in parameters
   - Calls `repo.Update()` to persist
   - Logs the update for observability

2. **`pkg/handlers/projects.go`** - Added `PATCH /api/projects/{pid}/auth-server-url` endpoint
   - Handler now accepts `*config.Config` for auth URL validation
   - `UpdateAuthServerURLRequest` struct for JSON body parsing
   - Validates auth URL against whitelist using `cfg.ValidateAuthURL()`
   - Returns 400 for invalid project ID or malformed body
   - Returns 403 if auth URL is not in allowed list
   - Returns 404 if project not found
   - Returns 204 on success

3. **`main.go`** - Updated `NewProjectsHandler()` call to pass config

4. **Mock updates** - Added `UpdateAuthServerURL` to all mock `ProjectService` implementations:
   - `pkg/handlers/mocks_test.go`
   - `pkg/mcp/tools/mocks_test.go`
   - `pkg/services/datasource_test.go`
   - `pkg/services/mcp_config_test.go`
   - `pkg/services/ontology_context_test.go`
   - `pkg/services/ontology_context_integration_test.go`

5. **Tests** - Added comprehensive unit tests in `pkg/handlers/projects_test.go`:
   - `TestProjectsHandler_UpdateAuthServerURL_Success`
   - `TestProjectsHandler_UpdateAuthServerURL_InvalidProjectID`
   - `TestProjectsHandler_UpdateAuthServerURL_InvalidBody`
   - `TestProjectsHandler_UpdateAuthServerURL_AuthURLNotAllowed`
   - `TestProjectsHandler_UpdateAuthServerURL_ProjectNotFound`
   - `TestProjectsHandler_UpdateAuthServerURL_ServiceError`

**Key implementation details:**
- Route is registered via `RegisterRoutes()` in `ProjectsHandler` (no separate routes.go file needed)
- Auth URL validation reuses `config.ValidateAuthURL()` which checks against the JWKSEndpoints whitelist
- The endpoint is protected by `authMiddleware.RequireAuthWithPathValidation("pid")` - only authenticated users can update their own project's auth URL

### [x] Task 2: Frontend - Save auth_url after successful config fetch

**Status:** Complete

**What was implemented:**

1. **`ui/src/services/config.ts`** - Added two helper functions and modified `fetchConfig()`:
   - `getProjectIdFromPath()` - Extracts project ID from URL path (e.g., `/projects/{uuid}/...`)
   - `saveAuthUrlToProject(projectId, authUrl)` - Calls `PATCH /api/projects/{projectId}/auth-server-url` with `{ auth_server_url: authUrl }`
   - Modified `fetchConfig()` to call `saveAuthUrlToProject()` after successful config fetch when:
     - `authUrl` (from query param) exists
     - `projectId` exists (from URL path)
     - `config.authServerUrl === authUrl` (they match)
   - Uses `credentials: 'include'` to send cookies for auth
   - Failure is non-fatal - logs warning and continues

### [ ] Task 3: End-to-end testing

Follow the testing steps in the "Testing" section above to verify the complete flow works.
