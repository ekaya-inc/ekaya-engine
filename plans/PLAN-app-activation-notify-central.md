# Plan: App Lifecycle Protocol with ekaya-central

**Status:** TODO

**Goal:** When a user installs, activates, or uninstalls an application in ekaya-engine, coordinate with ekaya-central so billing state stays in sync. Currently, app installation only saves to the engine's local DB — central is never informed.

---

## Context

- ekaya-engine has `POST /api/projects/{pid}/apps/{appId}` to install apps
- ekaya-central will have app management endpoints at `/api/v1/projects/{pid}/apps/{appId}/{action}` (see `PLAN-app-activation-from-engine.md` in ekaya-central)
- The engine doesn't create service JWTs — it only validates user JWTs from central
- Existing central client (`pkg/central/client.go`) forwards the user's JWT for calls to central (ProvisionProject, GetProject, UpdateServerUrl)
- **Parallel development**: engine can be developed and unit-tested with mocked central responses. Central must be deployed before integration/end-to-end testing.

## Shared Protocol

Both plans must agree on this contract. If either side changes it, the other must be updated.

### Actions

| Action | Engine endpoint | Central endpoint | When |
|--------|----------------|------------------|------|
| **Install** | `POST /api/projects/{pid}/apps/{appId}` | `POST /api/v1/projects/{pid}/apps/{appId}/install` | User adds app to project |
| **Activate** | `POST /api/projects/{pid}/apps/{appId}/activate` | `POST /api/v1/projects/{pid}/apps/{appId}/activate` | User is ready to start using the app (billing begins) |
| **Uninstall** | `DELETE /api/projects/{pid}/apps/{appId}` | `POST /api/v1/projects/{pid}/apps/{appId}/uninstall` | User removes app from project |

### Status Strings

Both sides use the same status values:

| Status | Meaning |
|--------|---------|
| `installed` | Install completed, no redirect needed |
| `activated` | Activate completed, no redirect needed |
| `uninstalled` | Uninstall completed, no redirect needed |
| `pending_install` | Install requires user interaction — follow `redirectUrl` |
| `pending_activation` | Activate requires user interaction — follow `redirectUrl` |
| `pending_uninstall` | Uninstall requires user interaction — follow `redirectUrl` |

### Callback Query Parameters

When central redirects back to engine's callback URL, it appends these query params:

| Param | Values | Meaning |
|-------|--------|---------|
| `action` | `install`, `activate`, `uninstall` | Which action was being performed |
| `status` | `success`, `cancelled` | Whether the user completed or cancelled |
| `state` | `<nonce>` | The nonce from the original callback URL (engine validates this) |

### Flow

1. User triggers action in the engine UI
2. Engine calls central with the user's JWT and a `callbackUrl` in the POST body
3. Central processes the request and responds with either:
   - `{ "status": "installed" }` — action completed, no user interaction needed
   - `{ "status": "pending_activation", "redirectUrl": "https://central.ekaya.ai/..." }` — central needs the user to complete a billing flow
4. If no `redirectUrl`: engine updates local DB immediately and returns success to the frontend
5. If `redirectUrl` is present: engine returns the `redirectUrl` to the frontend as JSON (not a server-side 302). The frontend does `window.location.href = redirectUrl`. Central handles the billing UI and redirects the user's browser back to the engine's callback endpoint.
6. Engine's callback endpoint (a GET that receives a browser redirect) validates the nonce, completes the DB operation, then issues an HTTP 302 redirect to the appropriate UI page. The callback is unauthenticated — browser redirects don't carry Authorization headers, so security relies on the single-use nonce (same pattern as OAuth callbacks).

### Redirect/Callback Pattern

When central returns a `redirectUrl`, the user must complete a flow on central (e.g., accept trial terms, confirm uninstall with billing implications). This works like OAuth or Stripe Checkout:

1. Engine generates a nonce and stores it in an in-memory map (single-instance deployment; upgrade to Redis/DB if scaling to multiple replicas)
2. Frontend redirects user to central's `redirectUrl`
3. User completes (or cancels) the flow on central
4. Central issues an HTTP 302 redirect to engine's callback URL, appending `&status=success` or `&status=cancelled`
5. Engine's callback endpoint validates the nonce (no JWT auth — browser redirects don't carry Authorization headers), completes or aborts the action, then issues an HTTP 302 to the appropriate UI page

**If the user abandons the redirect** (closes tab, navigates away), no local state changes. The app remains in whatever state it was before. The user can retry the action. The nonce expires in memory naturally (no cleanup needed).

### App States

Apps have three states, tracked via the `activated_at` column on `engine_installed_apps`:

```
not-installed → installed (activated_at IS NULL) → activated (activated_at IS NOT NULL)
```

Uninstall removes the row entirely.

---

## Tasks

### 1. Add `activated_at` column to installed apps

**File:** Database migration

- Add `activated_at TIMESTAMPTZ` column to `engine_installed_apps` (nullable, default NULL)
- NULL = installed but not yet activated
- Non-null = activated (billing has started)

### 2. Add central client methods

**File:** `pkg/central/client.go`

Add three methods following the same pattern as ProvisionProject:

```go
// InstallApp notifies ekaya-central that an application is being installed.
// callbackUrl is the engine URL that central should redirect to if user interaction is needed.
func (c *Client) InstallApp(ctx context.Context, baseURL, projectID, appID, token, callbackUrl string) (*AppActionResponse, error)

// ActivateApp notifies ekaya-central that an application is being activated.
func (c *Client) ActivateApp(ctx context.Context, baseURL, projectID, appID, token, callbackUrl string) (*AppActionResponse, error)

// UninstallApp notifies ekaya-central that an application is being uninstalled.
func (c *Client) UninstallApp(ctx context.Context, baseURL, projectID, appID, token, callbackUrl string) (*AppActionResponse, error)
```

Each method:
- POSTs to `/api/v1/projects/{projectID}/apps/{appID}/{action}`
- Forwards user's JWT via `Authorization: Bearer` header
- Sends `{ "callbackUrl": "..." }` in the body
- Returns `AppActionResponse` which includes optional `redirectUrl`
- Returns error on failure — do not swallow errors

Response type:
```go
type AppActionResponse struct {
    Status      string `json:"status"`                // see Status Strings table above
    RedirectUrl string `json:"redirectUrl,omitempty"`  // if present, redirect user here
}
```

### 3. Add activate handler route

**File:** `pkg/handlers/installed_app.go`

Add a new route in `RegisterRoutes`:

```go
mux.HandleFunc("POST "+base+"/{appId}/activate",
    authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Activate)))
```

Add `Activate` handler method:
- Parse projectID and appID
- Call `installedAppService.Activate()`
- If service returns a `redirectUrl`, return JSON: `{ "success": true, "data": { "redirectUrl": "..." } }`
- If no redirect, return JSON: `{ "success": true, "data": { "status": "activated" } }`

### 4. Add callback endpoint

**File:** `pkg/handlers/installed_app.go`

Add a callback route:

```go
mux.HandleFunc("GET "+base+"/{appId}/callback", h.Callback)
```

**Important:** This endpoint is different from all other handlers in two ways:
1. It is **unauthenticated** — no auth middleware. Browser redirects (HTTP 302 from central) don't carry Authorization headers. Security relies on the single-use nonce, same pattern as OAuth callbacks.
2. It returns **HTTP 302 redirects** to UI pages, not JSON responses.

```
GET /api/projects/{pid}/apps/{appId}/callback?action=install|activate|uninstall&status=success|cancelled&state=<nonce>
```

- **No auth middleware** — the nonce is the sole security mechanism
- Validates nonce matches a pending action in the in-memory nonce store (nonce is single-use, tied to a specific action/project/app tuple)
- On `status=success`: completes the pending action (DB insert/update/delete), then HTTP 302 to UI page
- On `status=cancelled`: aborts, no local state change, then HTTP 302 to UI page
- Redirect targets after completion:
  - Uninstall → project page (`/projects/{pid}`)
  - Activate → next step in the app's setup flow
  - Install → app page (`/projects/{pid}/apps/{appId}`)

### 5. Update InstalledAppService

**File:** `pkg/services/installed_app.go`

- Add `centralClient` field to `installedAppService`
- Update `Install()`: call `centralClient.InstallApp()` first. If central returns `redirectUrl`, return it to the handler (don't save to DB yet). If no redirect, save to DB.
- Add `Activate()`: new method on the interface and implementation. Call `centralClient.ActivateApp()` first. If central returns `redirectUrl`, return it to the handler. If no redirect, set `activated_at = now()`.
- Update `Uninstall()`: call `centralClient.UninstallApp()` first. If central returns `redirectUrl`, return it to the handler. If no redirect, delete from DB.

**Extracting auth from context** — the correct calls are:
```go
token, ok := auth.GetToken(ctx)       // returns (string, bool)
claims, ok := auth.GetClaims(ctx)     // returns (*Claims, bool)
papiURL := claims.PAPI                // central API base URL
```

### 6. Update Install handler response shape

**File:** `pkg/handlers/installed_app.go`

The existing `Install` handler returns `{ "success": true, "data": <InstalledApp> }`. With the redirect flow, it must also handle the redirect case:

- If service returns `redirectUrl` → `{ "success": true, "data": { "redirectUrl": "..." } }`
- If service returns the installed app → `{ "success": true, "data": <InstalledApp> }` (same as today)

The frontend must check for the presence of `redirectUrl` in the response and redirect accordingly.

### 7. Nonce store

**File:** new file, e.g. `pkg/services/nonce_store.go`

Simple in-memory nonce store for callback validation:

```go
type NonceStore interface {
    Generate(action, projectID, appID string) string  // returns nonce
    Validate(nonce, action, projectID, appID string) bool  // returns true and deletes if valid
}
```

- Use `sync.Map` or a mutex-protected map
- Nonces are single-use (deleted on validation)
- No TTL needed for now — nonces are cleaned up on use, and abandoned ones are negligible in memory
- If scaling to multiple replicas, replace with Redis or a DB table

### 8. Update dependency injection

**File:** `main.go`

- Pass the central client to `NewInstalledAppService()`

---

## Notes

- **Fail-fast**: if central is unreachable or returns an error, the action fails. Billing is core to the product.
- **Central-first ordering**: call central before writing to local DB. No rollback logic needed.
- **Redirect is optional**: not every action requires user interaction on central. Central decides whether to return a `redirectUrl`.
- **Two response patterns**: the install/activate/uninstall API endpoints return JSON to the frontend. The callback endpoint issues HTTP 302 redirects to UI pages. These are different patterns — the callback is the only non-JSON endpoint in the handler.
- **Callback security**: the callback endpoint is unauthenticated (browser redirects don't carry Authorization headers). Security relies on the single-use nonce — same pattern as OAuth callbacks. The nonce is ephemeral, single-use, and tied to a specific (action, projectID, appID) tuple.
- **Abandoned redirects**: no local state changes until successful callback. If the user abandons, the app stays in its previous state and the action can be retried.
- Every installable app follows the same three-action protocol.
- The user's JWT already has `pid` and admin role — central verifies authorization using `authenticateProjectAccess('admin')`.
- No service JWT creation needed in the engine.
- **Parallel development**: engine can be developed with mocked central responses. Central must be deployed for integration testing.
