# PLAN: App Sync with Central — AI Data Liaison Sales Funnel

**Status:** READY
**Source:** `plans/DESIGN-engine-app-marketplace.md` (sections 1-3)
**Companion:** `ekaya-central/plans/DESIGN-app-marketplace-funnel.md` (central-side changes — not in this repo)

---

## Goal

Enable the sales funnel for AI Data Liaison: a CTO clicks an ad, lands on `try.ekaya.ai/ai-data-liaison`, views the feature, tries it with a mock UI, creates a new project with AI Data Liaison pre-configured, and the engine installs it automatically during provisioning.

The engine-side work is **sync infrastructure**: when ekaya-central tells the engine (via provision or project fetch) that an app is installed, the engine persists it to `engine_installed_apps` so the UI and services recognize it. Today this data is returned by central but ignored by the engine.

AI Data Liaison (`ai-data-liaison`) already exists as a known app ID in the engine. No new app IDs are needed — only the plumbing to sync installation state from central.

### Key Constraint: No Persistent JWT

The engine does **not** persist the admin's JWT. Calling central's API requires a valid admin token, which is only available during a live authenticated request. This means:

- **No true background sync is possible** — the engine cannot poll central on a timer
- Sync happens opportunistically during two request-time flows that already have the JWT: `SyncFromCentralAsync` (GET /projects) and `ProvisionFromClaims` (POST /projects)
- This is sufficient because both flows run on every admin session start, so app state is reconciled each time an admin opens the engine

---

## Current State (verified against codebase)

### InstalledAppService (`pkg/services/installed_app.go`)
- Interface with 9 methods: ListInstalled, IsInstalled, Install, Activate, Uninstall, CompleteCallback, GetSettings, UpdateSettings, GetApp
- Constructor takes: `repo InstalledAppRepository`, `centralClient CentralAppClient`, `nonceStore NonceStore`, `baseURL string`, `logger *zap.Logger`
- `CentralAppClient` interface (lines 20-24) only has InstallApp, ActivateApp, UninstallApp — no GetProject or sync methods
- `mcp-server` is a virtual app — never persisted to DB, always returned by ListInstalled with zero UUID

### KnownAppIDs (`pkg/models/installed_app.go`)
- Three constants: `AppIDMCPServer = "mcp-server"`, `AppIDAIDataLiaison = "ai-data-liaison"`, `AppIDAIAgents = "ai-agents"`
- `KnownAppIDs` is a `map[string]bool` (not `map[string]string` as the DESIGN doc shows)
- Used to validate app IDs on install — unknown IDs are rejected
- **AI Data Liaison is already registered** — no model changes needed

### InstalledAppRepository (`pkg/repositories/installed_app_repository.go`)
- Interface with 7 methods: List, Get, IsInstalled, Install, Activate, Uninstall, UpdateSettings
- No Upsert method — needed for sync (insert-or-update)
- Table: `engine_installed_apps` with columns: id, project_id, app_id, installed_at, installed_by, activated_at, settings

### Provision & Sync (`pkg/services/projects.go`)
- `ProvisionFromClaims` (line 648) calls `central.ProvisionProject()` which returns `ProjectInfo` including `Applications []ApplicationInfo`
- `SyncFromCentralAsync` (line 770) runs in background goroutine after GET /projects — calls `central.GetProject()` which also returns `ProjectInfo` with `Applications`
- **Gap:** SyncFromCentralAsync only syncs project **name**, ignores `Applications` entirely

### Central Client (`pkg/central/client.go`)
- `GetProject()` returns `*ProjectInfo` which includes `Applications []ApplicationInfo`
- `ApplicationInfo` struct: `Name string` + `Billing *BillingInfo` (optional)
- `BillingInfo` struct: `Status string` + `FreeSeatsLimit int`
- The Applications data is already available — we just need to use it

### ApplicationsPage UI (`ui/src/pages/ApplicationsPage.tsx`)
- Marketplace-style card grid showing available apps
- Hardcoded list of apps with install buttons
- Uses `useInstalledApps()` hook to track installation state
- `ui/src/types/installedApp.ts` defines `APP_ID_MCP_SERVER`, `APP_ID_AI_DATA_LIAISON`, `APP_ID_AI_AGENTS`

### ProjectDashboard UI (`ui/src/pages/ProjectDashboard.tsx`)
- Shows conditional tiles for installed apps (MCP Server always, others if installed)
- Checks `app.app_id` against known constants

---

## Tasks

### 1. Add Upsert method to InstalledAppRepository

**File:** `pkg/repositories/installed_app_repository.go`

Add to the interface:

```go
Upsert(ctx context.Context, app *models.InstalledApp) error
```

Implementation should use PostgreSQL `ON CONFLICT (project_id, app_id) DO UPDATE SET activated_at = EXCLUDED.activated_at`. This only updates billing-related fields — it never overwrites `settings` (which is the engine's local state) or `installed_by` (which is the original installer).

**Verified:** The `engine_installed_apps` table already has `CONSTRAINT unique_project_app UNIQUE (project_id, app_id)` (see `migrations/009_llm_and_config.up.sql:219`). No migration needed.

### 2. Add SyncApps method to InstalledAppService

**File:** `pkg/services/installed_app.go`

Add a new method to the interface and implementation:

```go
// SyncApps reconciles local app state against central's source of truth.
// Called during request-time flows (provision and project fetch) when the admin's
// JWT is available and fresh data from central has already been retrieved.
//
// Rules:
//   - Central is source of truth for what's installed and billing status
//   - Engine is source of truth for local settings (JSONB) — sync never overwrites settings
//   - Never auto-uninstall — if central doesn't list an app that's locally installed, log it but don't delete
//   - Log all sync actions for debugging
SyncApps(ctx context.Context, projectID uuid.UUID, centralApps []central.ApplicationInfo) error
```

**Implementation logic:**

```
For each app in centralApps:
  1. Skip if app.Name not in KnownAppIDs (unknown app — central may know about apps the engine doesn't support yet)
  2. Skip if app.Name == "mcp-server" (virtual app, never persisted)
  3. Determine activated_at:
     - If app.Billing != nil && app.Billing.Status is active → set activated_at to now (if not already set)
     - If app.Billing == nil or status is not active → set activated_at to nil
  4. Call repo.Upsert() with the mapped InstalledApp

For locally installed apps not in centralApps:
  1. Fetch all local apps via repo.List()
  2. Compare against central list
  3. Log any discrepancies at WARN level (don't delete)
```

**Billing status mapping:** The `BillingInfo.Status` values from central need to be mapped. Treat any non-empty status as "active" for now (the exact values depend on central's implementation — the companion DESIGN covers this). If `Billing` is nil, treat as installed-but-not-activated.

**Dependencies:** This method needs access to the repository (already injected) but does NOT need the CentralAppClient (the caller provides the central data). No constructor changes needed.

### 3. Wire sync into SyncFromCentralAsync

**File:** `pkg/services/projects.go`

Extend `SyncFromCentralAsync` (line 770) to sync apps after syncing the project name. The admin's JWT is available here because `SyncFromCentralAsync` is called from the GET /projects handler which has the live request token. The `projectInfo` returned by `GetProject()` already contains `Applications []ApplicationInfo` — pass it to `InstalledAppService.SyncApps()`.

**Changes needed:**

1. Add `InstalledAppService` as a dependency of `projectService`. The current constructor (`NewProjectService` at `pkg/services/projects.go:108`) takes: `db, projectRepo, userRepo, ontologyRepo, mcpConfigRepo, agentAPIKeyService, centralClient, nonceStore, baseURL, logger`. It does **not** have InstalledAppService — add it as a new parameter and store it on the `projectService` struct.

2. After the existing name-sync logic (line 809-829), add:

```go
// Sync installed apps from central
if len(projectInfo.Applications) > 0 {
    if err := s.installedAppService.SyncApps(ctx, projectID, projectInfo.Applications); err != nil {
        s.logger.Error("Failed to sync apps from ekaya-central",
            zap.String("project_id", projectID.String()),
            zap.Error(err))
        // Don't return — app sync failure shouldn't block other sync operations
    }
}
```

**Important:** SyncFromCentralAsync already acquires a database scope (line 789-798) and sets tenant context. The InstalledAppService.SyncApps needs to work within this existing context — verify that the repository methods honor the tenant scope from context.

**Also important:** SyncFromCentralAsync runs in a goroutine with a 30-second timeout. App sync adds DB operations, but these should be fast (small number of apps). The timeout should be sufficient.

**Note on "Async":** Despite the goroutine, this is not a background process — it is a fire-and-forget side-effect of a live admin request. The goroutine just avoids blocking the HTTP response. The central API call inside still uses the admin's JWT passed in from the request handler.

### 4. Also sync apps during initial provision

**File:** `pkg/services/projects.go`

In `ProvisionFromClaims` (line 648), after the project is created/updated, call `SyncApps` with the applications from the provision response. Currently, applications from provision are stored in `project.Parameters["applications"]` and used only for MCP/AI setup. Add an explicit sync call so the apps are persisted to `engine_installed_apps`.

This is the **primary path for the sales funnel**: CTO creates a project on central with AI Data Liaison pre-selected → CTO downloads and starts the engine → engine provisions from central → `ProvisionFromClaims` receives the applications list including `ai-data-liaison` → SyncApps persists it → the engine UI shows AI Data Liaison as installed.

Find where `ProvisionFromClaims` returns the result (around line 760) and add the sync call before returning. The provision flow already has a database context available.

### 5. Write tests

**Tests for repository Upsert:**
- Upsert inserts new app when not exists
- Upsert updates activated_at when app exists
- Upsert does not overwrite settings
- Upsert does not overwrite installed_by

**Tests for SyncApps:**
- Syncs new app from central (app in central, not local → inserts)
- Updates billing status (app in both, different activation → updates)
- Ignores unknown app IDs (app in central with unrecognized name → skipped)
- Ignores mcp-server (virtual app → skipped)
- Logs but doesn't delete orphaned local apps (app local but not in central → logged, not deleted)
- Handles empty central apps list (no-op)

---

## Assumptions to Verify

1. **Central's GetProject returns applications.** The `ProjectInfo` struct includes `Applications`, and `ProvisionProject` populates it. Both endpoints use the same `doProjectRequest` parser (lines 300-336) which deserializes the full `ProjectInfo`. However, whether central's GET endpoint actually includes `applications` in its JSON response depends on the central-side implementation. If central omits applications from GET responses, this sync won't work and we'd need to either (a) update central or (b) use `ProvisionProject` for sync instead of `GetProject`. Both calls require the admin's JWT from the live request — the engine has no way to call central independently.

2. ~~**Unique constraint exists on (project_id, app_id).**~~ **VERIFIED** — `CONSTRAINT unique_project_app UNIQUE (project_id, app_id)` exists in `migrations/009_llm_and_config.up.sql:219`.

3. **BillingInfo.Status values.** The plan treats any non-nil Billing with non-empty Status as "active." Confirm with the companion DESIGN what status values central sends (e.g., "active", "trial", "expired", "cancelled").

4. **Central pre-selects apps during project creation.** The sales funnel assumes central's project creation flow can include pre-selected applications (e.g., AI Data Liaison) so they appear in the provision response. This is central-side work covered in the companion DESIGN.

---

## Out of Scope

- **Central-side funnel** — Landing pages, demo UI, install flow, project creation with pre-selected apps. Covered in `ekaya-central/plans/DESIGN-app-marketplace-funnel.md`.
- **New app IDs** — Future apps (ai-drift-monitor, ai-data-guardian, ai-compliance-manager) depend on the WASM runtime. They will use this same sync infrastructure when they ship.
- **WASM runtime** — Covered in `plans/DESIGN-wasm-application-platform.md`. Separate plan needed.
- **Periodic background polling** — The DESIGN mentions optional periodic sync. This is **architecturally impossible** today because the engine does not persist the admin JWT needed to call central's API. Would require either (a) persisting/refreshing admin tokens or (b) a service-to-service auth mechanism between engine and central.
- **AI config validation on install** — Future AI apps may need to validate AI config before installation. Deferred until those apps exist. AI Data Liaison already handles its own config requirements.
- **Notification delivery** — How apps send alerts (webhook, email, in-app). Deferred.
