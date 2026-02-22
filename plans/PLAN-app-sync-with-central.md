# PLAN: App Sync with Central + Dynamic Registry

**Status:** READY
**Source:** `plans/DESIGN-engine-app-marketplace.md` (sections 1-3)
**Companion:** `ekaya-central/plans/DESIGN-app-marketplace-funnel.md` (central-side changes — not in this repo)

---

## Goal

Enable the engine to discover and reconcile app installations that happen outside the engine (e.g., from ekaya-central's web UI marketplace). Today, apps installed from central are invisible to the engine until the next full provision call. This plan adds background sync so the engine picks up new installations, activation changes, and billing status updates automatically.

Also adds the three new WASM app IDs to the registry so the engine recognizes them, and validates AI configuration before installing AI-powered apps.

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

### 1. Add new app ID constants to the model

**File:** `pkg/models/installed_app.go`

Add three new app ID constants and register them in KnownAppIDs:

```go
const (
    AppIDMCPServer           = "mcp-server"
    AppIDAIDataLiaison       = "ai-data-liaison"
    AppIDAIAgents            = "ai-agents"
    AppIDAIDriftMonitor      = "ai-drift-monitor"
    AppIDAIDataGuardian      = "ai-data-guardian"
    AppIDAIComplianceManager = "ai-compliance-manager"
)
```

Add a set identifying which apps require AI configuration:

```go
var AIRequiredApps = map[string]bool{
    AppIDAIDriftMonitor:      true,
    AppIDAIDataGuardian:      true,
    AppIDAIComplianceManager: true,
}
```

Update `KnownAppIDs` to include all six app IDs.

### 2. Add Upsert method to InstalledAppRepository

**File:** `pkg/repositories/installed_app_repository.go`

Add to the interface:

```go
Upsert(ctx context.Context, app *models.InstalledApp) error
```

Implementation should use PostgreSQL `ON CONFLICT (project_id, app_id) DO UPDATE SET activated_at = EXCLUDED.activated_at`. This only updates billing-related fields — it never overwrites `settings` (which is the engine's local state) or `installed_by` (which is the original installer).

**Verified:** The `engine_installed_apps` table already has `CONSTRAINT unique_project_app UNIQUE (project_id, app_id)` (see `migrations/009_llm_and_config.up.sql:219`). No migration needed.

### 3. Add SyncApps method to InstalledAppService

**File:** `pkg/services/installed_app.go`

Add a new method to the interface and implementation:

```go
// SyncApps reconciles local app state against central's source of truth.
// Called in the background after user authentication when fresh data from central is available.
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

### 4. Wire sync into SyncFromCentralAsync

**File:** `pkg/services/projects.go`

Extend `SyncFromCentralAsync` (line 770) to sync apps after syncing the project name. The `projectInfo` returned by `GetProject()` already contains `Applications []ApplicationInfo` — pass it to `InstalledAppService.SyncApps()`.

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

### 5. Also sync apps during initial provision

**File:** `pkg/services/projects.go`

In `ProvisionFromClaims` (line 648), after the project is created/updated, call `SyncApps` with the applications from the provision response. Currently, applications from provision are stored in `project.Parameters["applications"]` and used only for MCP/AI setup. Add an explicit sync call so the apps are persisted to `engine_installed_apps`.

Find where `ProvisionFromClaims` returns the result (around line 760) and add the sync call before returning. The provision flow already has a database context available.

### 6. Add AI config validation to Install

**File:** `pkg/services/installed_app.go`

In the `Install` method, before proceeding with installation, check if the app requires AI configuration:

```go
if models.AIRequiredApps[appID] {
    // Check if project has working AI config
    aiConfig, err := s.aiConfigService.GetEffective(ctx, projectID)
    if err != nil || aiConfig == nil || aiConfig.ConfigType == models.AIConfigNone {
        return nil, fmt.Errorf("app %q requires AI configuration; configure AI settings before installing", appID)
    }
}
```

**Dependencies:** This requires adding `AIConfigService` (or its `GetEffective` method via a smaller interface) to the InstalledAppService constructor. Check `pkg/services/ai_config.go` for the interface definition. The `AIConfigService` interface has a `GetEffective(ctx, projectID)` method that returns the resolved config.

**Constructor change:**

```go
func NewInstalledAppService(
    repo repositories.InstalledAppRepository,
    centralClient CentralAppClient,
    nonceStore NonceStore,
    aiConfigProvider AIConfigProvider, // NEW — small interface with just GetEffective
    baseURL string,
    logger *zap.Logger,
) InstalledAppService
```

Define a small interface to avoid coupling to the full AIConfigService:

```go
type AIConfigProvider interface {
    GetEffective(ctx context.Context, projectID uuid.UUID) (*models.AIConfig, error)
}
```

Update the call site in `main.go` where `NewInstalledAppService` is constructed to pass the AIConfigService.

**Note:** The sync flow (task 3) should NOT enforce this validation — apps installed from central have already been validated there. Only the direct Install method (user-initiated from engine UI) should check.

### 7. Add frontend app ID constants for new apps

**File:** `ui/src/types/installedApp.ts`

Add new constants:

```typescript
export const APP_ID_AI_DRIFT_MONITOR = 'ai-drift-monitor';
export const APP_ID_AI_DATA_GUARDIAN = 'ai-data-guardian';
export const APP_ID_AI_COMPLIANCE_MANAGER = 'ai-compliance-manager';
```

### 8. Add new app cards to ApplicationsPage

**File:** `ui/src/pages/ApplicationsPage.tsx`

Add cards for the three new apps in the available apps list. These should appear as "Coming Soon" or with install buttons depending on whether the WASM runtime is ready. For now, add them as informational cards (similar to the existing "Product Kit [BETA]" card pattern) with a "Coming Soon" badge.

Each card should have:
- App name and description
- An icon (choose appropriate Lucide icons)
- "Coming Soon" badge (since the WASM runtime isn't built yet)

This ensures users can see what's coming and the UI is ready when the apps ship.

### 9. Write tests

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

**Tests for Install with AI config validation:**
- Install AI-required app with valid AI config → succeeds
- Install AI-required app without AI config → returns error
- Install non-AI app without AI config → succeeds (no validation)

---

## Assumptions to Verify

1. **Central's GetProject returns applications.** The `ProjectInfo` struct includes `Applications`, and `ProvisionProject` populates it. Both endpoints use the same `doProjectRequest` parser (lines 300-336) which deserializes the full `ProjectInfo`. However, whether central's GET endpoint actually includes `applications` in its JSON response depends on the central-side implementation. If central omits applications from GET responses, this sync won't work and we'd need to either (a) update central or (b) use `ProvisionProject` for sync instead of `GetProject`.

2. ~~**Unique constraint exists on (project_id, app_id).**~~ **VERIFIED** — `CONSTRAINT unique_project_app UNIQUE (project_id, app_id)` exists in `migrations/009_llm_and_config.up.sql:219`.

3. **BillingInfo.Status values.** The plan treats any non-nil Billing with non-empty Status as "active." Confirm with the companion DESIGN what status values central sends (e.g., "active", "trial", "expired", "cancelled").

---

## Out of Scope

- **WASM runtime** — Covered in `plans/DESIGN-wasm-application-platform.md`. Separate plan needed.
- **First three AI apps** — Depend on WASM runtime. Separate plans needed.
- **Phase 2 dynamic registry** — Loading app catalog from central at startup. Deferred until we have more apps than the hardcoded list can handle.
- **Periodic background polling** — The DESIGN mentions optional periodic sync. Auth-time sync (via SyncFromCentralAsync) is sufficient for now.
- **App-specific UI pages** — Each WASM app will need its own output page, settings panel, and activity log. These should be planned alongside each app's implementation.
- **Notification delivery** — How apps send alerts (webhook, email, in-app). Deferred.
