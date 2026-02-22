# PLAN: Install Apps from Provision — AI Data Liaison Sales Funnel

**Status:** DONE
**Source:** `plans/DESIGN-engine-app-marketplace.md` (sections 1-3)
**Companion:** `ekaya-central/plans/DESIGN-app-marketplace-funnel.md` (central-side changes — not in this repo)

---

## Goal

Enable the sales funnel for AI Data Liaison: a CTO clicks an ad, lands on `try.ekaya.ai/ai-data-liaison`, views the feature, tries it with a mock UI, and creates a **new project** with AI Data Liaison pre-configured. When the engine provisions this new project, it installs AI Data Liaison automatically.

### Simplified Scope

Ekaya-central only supports launching **new projects** with pre-selected applications. There is no need to sync apps into existing projects — if a project was created with AI Data Liaison selected, the engine will install it on first provision. Subsequent provisions are idempotent (app already installed → no-op).

This removes all sync infrastructure (no Upsert, no SyncApps, no changes to SyncFromCentralAsync). The only engine change is: **read the applications list from the provision response and install any that aren't already installed locally.**

AI Data Liaison (`ai-data-liaison`) already exists as a known app ID in the engine. No new app IDs are needed.

---

## Current State (verified against codebase)

### ProvisionFromClaims (`pkg/services/projects.go:648`)
- Calls `central.ProvisionProject()` which returns `ProjectInfo` including `Applications []ApplicationInfo`
- Stores `projectInfo.Applications` in `project.Parameters["applications"]` (line 703-705)
- Logs the app names (line 707-717)
- **Gap:** Never installs apps — the applications data is stored in parameters but not acted on
- Has tenant-scoped DB context available (line 737-743) after project and user are created

### InstalledAppService (`pkg/services/installed_app.go`)
- `Install` method (line 141) validates app ID, checks if already installed, **notifies central**, then persists locally
- The central notification is wrong for provision — central already knows about the app (it told us). We need a local-only install path.
- `IsInstalled` method can check if app is already present (for idempotency)

### InstalledAppRepository (`pkg/repositories/installed_app_repository.go`)
- `Install` method is a plain INSERT (no `ON CONFLICT`) — will fail on duplicate
- `IsInstalled` method returns bool — can be used to skip already-installed apps

### Central Client (`pkg/central/client.go`)
- `ApplicationInfo` struct: `Name string` + `Billing *BillingInfo` (optional)
- `BillingInfo` struct: `Status string` + `FreeSeatsLimit int`

### KnownAppIDs (`pkg/models/installed_app.go`)
- `ai-data-liaison` is already registered — no model changes needed

---

## Tasks

### 1. ~~Add EnsureInstalled method to InstalledAppService~~ DONE

**File:** `pkg/services/installed_app.go`

Add a new method to the interface and implementation:

```go
// EnsureInstalled installs an app locally if not already installed.
// Unlike Install, this does NOT notify central — it is used when central
// is the source of the installation (e.g., during provision).
// Idempotent: no-op if the app is already installed.
EnsureInstalled(ctx context.Context, projectID uuid.UUID, appID string) error
```

**Implementation logic:**

```
1. Skip if appID not in KnownAppIDs
2. Skip if appID == "mcp-server" (virtual app, never persisted)
3. Check repo.IsInstalled — if already installed, return nil (no-op)
4. Call repo.Install with a new InstalledApp (installed_by = "central-provision", settings = empty)
5. Log the installation
```

This method is intentionally simple: no central notification, no billing handling, no callbacks. Central already did all that — we're just persisting the fact locally.

### 2. ~~Wire EnsureInstalled into ProvisionFromClaims~~ DONE

**File:** `pkg/services/projects.go`

Add `InstalledAppService` as a dependency of `projectService`:

1. Add field to the `projectService` struct
2. Add parameter to `NewProjectService` constructor
3. Update the call site in `main.go`

After the user is ensured (line 758-760), iterate over `projectInfo.Applications` and call `EnsureInstalled` for each:

```go
// Install apps that central says should be in this project
for _, app := range projectInfo.Applications {
    if err := s.installedAppService.EnsureInstalled(ctxWithScope, projectID, app.Name); err != nil {
        s.logger.Error("Failed to install app from provision",
            zap.String("project_id", projectID.String()),
            zap.String("app_id", app.Name),
            zap.Error(err))
        // Don't return — app install failure shouldn't block provisioning
    }
}
```

**Important:** This must run after the tenant-scoped DB context is set up (line 737-743) since the repository needs tenant scope. Place it after the user ensure block but before the return.

**Idempotency:** `ProvisionFromClaims` is called on every admin authentication. On first call, apps get installed. On subsequent calls, `EnsureInstalled` sees they're already installed and is a no-op.

### 3. ~~Write tests~~ DONE

**Tests for EnsureInstalled:**
- App not installed → installs it locally
- App already installed → no-op (no error, no duplicate)
- Unknown app ID → skipped (no error)
- mcp-server → skipped (virtual app)
- Does NOT call central client (verify no central interaction)

**Tests for ProvisionFromClaims with applications:**
- Provision with applications in response → apps are installed locally
- Provision with empty applications → no install calls
- Provision with already-installed app → no error (idempotent)
- App install failure → provision still succeeds (error logged, not returned)

---

## Assumptions to Verify

1. **Central's ProvisionProject returns applications.** The `ProjectInfo` struct includes `Applications`, and the response is already being parsed (line 703-705 stores it in parameters). Need to confirm central actually populates this field when the project was created with pre-selected apps.

2. **Tenant scope is available.** The `EnsureInstalled` call needs tenant-scoped DB context. `ProvisionFromClaims` sets this up at line 737-743. Verify the repository's `Install` and `IsInstalled` methods work within this scope.

---

## Out of Scope

- **Syncing existing projects** — Central only supports pre-selected apps on new project creation. No sync of apps into existing projects.
- **Changes to SyncFromCentralAsync** — Not needed. Existing projects are unaffected.
- **Upsert / SyncApps infrastructure** — Not needed. Simple install-if-not-present is sufficient.
- **Central-side funnel** — Landing pages, demo UI, install flow, project creation with pre-selected apps. Covered in `ekaya-central/plans/DESIGN-app-marketplace-funnel.md`.
- **New app IDs** — Future apps (ai-drift-monitor, ai-data-guardian, ai-compliance-manager) depend on the WASM runtime. They will use this same `EnsureInstalled` path when they ship.
- **Billing reconciliation** — Billing is handled by central. The engine just persists the installation fact.
- **Periodic background polling** — Architecturally impossible today (engine does not persist admin JWT).
