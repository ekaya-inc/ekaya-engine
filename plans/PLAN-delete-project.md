# PLAN: Delete Project via Central Redirect (ekaya-engine)

**Status:** TODO
**Depends on:** ekaya-central `PLAN-delete-project.md`

## Problem

Currently, deleting a project from ekaya-engine's Settings page calls `DELETE /api/projects/{pid}` which deletes the engine row (+ CASCADE) and redirects to the central project page. Central is never notified, leaving orphaned Firestore data (project, members, billing, seats, apps, telemetry) and potentially an active Stripe subscription.

## Solution

Model the delete-project flow on the existing **uninstall-app redirect/callback pattern**:

1. Engine calls central's new `POST /api/v1/projects/{pid}/delete` endpoint with a `callbackUrl`
2. Central returns a `redirectUrl` pointing to a confirmation page on central's UI
3. Browser navigates to central, where the user sees what will be cleaned up (billing, subscription, etc.)
4. Central cleans up all its data (cancel Stripe subscription, delete Firestore subcollections, delete project doc)
5. Central redirects browser back to engine's callback URL
6. Engine's UI picks up the callback, calls the engine API to complete local deletion
7. Engine deletes the local project (CASCADE handles all child tables)
8. UI navigates to central's projects list page

This follows the same nonce-based anti-replay pattern used by `installed_app.go`.

### Flow Diagram

```
Engine Settings Page: user clicks "Delete Project"
    │
    ▼
Engine UI: DELETE /api/projects/{pid}
    │
    ▼
Engine Service: builds callbackUrl with nonce, calls Central
    POST {papiURL}/api/v1/projects/{pid}/delete  { callbackUrl }
    │
    ▼
Central API: returns { status: "pending_delete", redirectUrl: "{central-frontend}/projects/{pid}/delete-confirm?callbackUrl=..." }
    │
    ▼
Engine returns { redirectUrl } to UI
    │
    ▼
Engine UI: window.location.href = redirectUrl (browser → central)
    │
    ▼
Central UI (ProjectDeleteConfirmPage):
  - Shows project name, billing summary, what will be deleted
  - User clicks "Confirm Delete":
      POST /api/v1/projects/{pid}/delete-confirm { callbackUrl }
      Central: cancels Stripe subscription, deletes all Firestore data
      Central returns { status: "deleted", callbackUrl }
      Central UI: window.location.href = callbackUrl (browser → engine)
  - User clicks "Cancel":
      Appends callback_status=cancelled to callbackUrl
      browser → engine
    │
    ▼
Engine UI (SettingsPage useEffect): detects callback params
  - If cancelled: clears params, does nothing
  - If success: POST /api/projects/{pid}/delete-callback { action: "delete", state: "{nonce}", status: "success" }
    │
    ▼
Engine Handler: validates nonce, deletes project from engine DB
    │
    ▼
Engine UI: window.location.href = projectsPageUrl (central's /projects list)
```

## Implementation Tasks

### Task 1: Add `DeleteProject` to Central Client

**File:** `pkg/central/client.go`

Add a new method following the same pattern as `UninstallApp`/`doAppAction`:

```go
func (c *Client) DeleteProject(ctx context.Context, baseURL, projectID, token, callbackUrl string) (*AppActionResponse, error)
```

- `POST {baseURL}/api/v1/projects/{projectID}/delete`
- Body: `{ "callbackUrl": "{callbackUrl}" }`
- Headers: `Authorization: Bearer {token}`
- Returns `AppActionResponse { Status, RedirectUrl }`

### Task 2: Add Delete Callback Endpoint

**File:** `pkg/handlers/projects.go`

Add a new route: `POST /api/projects/{pid}/delete-callback`

Handler:
1. Parse project ID from path
2. Parse `CallbackRequest { Action, Status, State }` from body (reuse the same struct from `installed_app.go`)
3. Validate action == "delete", status is "success" or "cancelled"
4. Call new `projectService.CompleteDeleteCallback(ctx, projectID, action, status, state)`

### Task 3: Update Project Service for Redirect-Based Delete

**File:** `pkg/services/projects.go`

The `ProjectService` needs a `NonceStore` dependency (can share the same one from `InstalledAppService` or create a project-level one).

**Modify `Delete` method:**
1. Build callback URL using the same `buildCallbackURL` pattern from `installed_app.go`:
   - URL: `{baseURL}/projects/{pid}/settings?callback_action=delete&callback_state={nonce}`
   - Generate nonce via `NonceStore.Generate("delete", projectID, "project")`
2. Get auth context (JWT token, PAPI URL) from request context
3. Call `centralClient.DeleteProject(ctx, papiURL, projectID, token, callbackUrl)`
4. If central returns `redirectUrl`, return it (don't delete locally yet)
5. If central returns `status: "deleted"` with no redirect, delete locally (fallback for future no-billing path)

**Add `CompleteDeleteCallback` method:**
1. Validate nonce via `NonceStore.Validate(nonce, "delete", projectID, "project")`
2. If status == "cancelled", return nil (no-op)
3. If status == "success", call `projectRepo.Delete(ctx, projectID)`

**Return type:** Modify `Delete` to return a result struct (like `AppActionResult`) instead of just `error`, so the handler can check for a `RedirectUrl`.

### Task 4: Update Delete Handler to Return Redirect

**File:** `pkg/handlers/projects.go`

Modify existing `Delete` handler (or the response):
- If service returns a `RedirectUrl`, return `200 { success: true, data: { redirectUrl: "..." } }` instead of `204`
- If no redirect (direct delete), keep returning `204`

### Task 5: Update Settings Page UI for Callback Handling

**File:** `ui/src/pages/SettingsPage.tsx`

**Add callback detection (useEffect):**
- On mount, check URL search params for `callback_action=delete`, `callback_state`, `callback_status`
- If `callback_status === "cancelled"`: clear params, show toast "Deletion cancelled"
- If callback present with no cancelled status:
  - Call `engineApi.completeDeleteCallback(pid, "delete", "success", callbackState)`
  - On success, redirect to `urls.projectsPageUrl` or central projects list
  - On error, show error toast

**Update `handleDeleteProject`:**
- Current: calls `engineApi.deleteProject(pid)`, then redirects to central
- New: calls `engineApi.deleteProject(pid)`, checks response for `redirectUrl`
  - If `redirectUrl`: `window.location.href = redirectUrl`
  - If no redirect (204): redirect to `urls.projectsPageUrl` (same as today for the no-billing path)

### Task 6: Add Frontend API Methods

**File:** `ui/src/services/engineApi.ts`

**Update `deleteProject`:**
- Change from expecting 204 to accepting either 204 (no redirect) or 200 with `{ redirectUrl }` in response body

**Add `completeDeleteCallback`:**
```typescript
completeDeleteCallback(projectId: string, action: string, status: string, state: string): Promise<void>
// POST /api/projects/{projectId}/delete-callback
// Body: { action, status, state }
```

### Task 7: Register New Route

**Files:** `pkg/server/routes.go` (or wherever routes are registered)

Register `POST /api/projects/{pid}/delete-callback` with the appropriate auth middleware (same as the existing app callback route).

## Notes

- The nonce store is already in-memory (`pkg/services/nonce_store.go`). The same store (or a second instance) can be used for project delete nonces. The "entity ID" parameter can be `"project"` to distinguish from app nonces.
- Central always returns a redirect for delete since it needs to handle billing cleanup on its side. If a project has no billing, central can still show a lightweight confirmation and clean up immediately.
- The `force=true` escape hatch from the old plan is intentionally removed — the redirect pattern gives central full control over its cleanup.
- After the engine callback completes and the project is deleted locally, the user is redirected to central's projects list. The project will no longer exist in either system.
