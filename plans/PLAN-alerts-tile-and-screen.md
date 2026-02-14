# PLAN: New Alerts Tile and Screen

**Status:** DRAFT
**Created:** 2026-02-14
**Route:** `/projects/{pid}/alerts`
**Product SKU:** TBD (new installed app, separate from `ai-data-liaison`)

## Background

The alert system is a security and governance feature that triggers alerts when MCP queries meet certain conditions (SQL injection, unusual volume, large exports, after-hours access, etc.). Currently, alerts are embedded inside the Audit page as one of six tabs, and alert configuration lives on the Settings page.

The goal is to promote Alerts to a first-class feature with its own dashboard tile, dedicated page, and separate product SKU — decoupling it from the AI Data Liaison product.

## Current Implementation Status

The alert system backend is **nearly complete**. The frontend exists but is **embedded inside the Audit page**, not standalone.

### Backend (Complete)

| Layer | Status | Files |
|-------|--------|-------|
| Models | Complete | `pkg/models/alert.go`, `pkg/models/alert_config.go` |
| Handlers | Complete | `pkg/handlers/alert_handler.go` (4 endpoints) |
| AlertService | Complete | `pkg/services/alert_service.go` (CRUD) |
| AlertTriggerService | 6/7 triggers | `pkg/services/alert_trigger_service.go` (SensitiveTable not implemented) |
| Repositories | Complete | `pkg/repositories/alert_repository.go`, `alert_trigger_repository.go` |
| Migrations | Complete | `027_audit_alerts.up.sql`, `028_alert_config.up.sql` |
| DI Wiring | Complete | `main.go` lines ~469-478 |
| Tests | Strong | ~1,400 lines across 3 test files |

**Existing API Endpoints:**
- `GET /api/projects/{pid}/audit/alerts` — list alerts with status/severity/pagination filters
- `POST /api/projects/{pid}/audit/alerts/{alert_id}/resolve` — resolve or dismiss an alert
- `GET /api/projects/{pid}/audit/alert-config` — get per-project alert configuration
- `PUT /api/projects/{pid}/audit/alert-config` — update per-project alert configuration

**7 Alert Types Defined (6 have triggers):**
1. `sql_injection_detected` — Triggers on security_level=critical or injection flag (trigger: implemented)
2. `unusual_query_volume` — User exceeds threshold_multiplier * 10 queries/hour (trigger: implemented)
3. `sensitive_table_access` — Query touches sensitive tables (trigger: **NOT implemented**)
4. `large_data_export` — Query returns > row_threshold rows (trigger: implemented)
5. `after_hours_access` — Access outside business hours (trigger: implemented)
6. `new_user_high_volume` — New user runs > query_threshold queries in first 24h (trigger: implemented)
7. `repeated_errors` — Same error repeats > error_count times in window_minutes (trigger: implemented)

**Alert config is stored as JSONB** in `engine_mcp_config.alert_config` column. This means alert config currently requires an `engine_mcp_config` row to exist for the project — which is created when MCP Server is configured. If MCP Server hasn't been configured, the `GET alert-config` call fails because there's no row in `engine_mcp_config`.

### Frontend (Exists but Embedded)

| Component | Status | Location |
|-----------|--------|----------|
| Alert list + resolution UI | Complete | `ui/src/pages/AuditPage.tsx` (lines 1181-1410, embedded as tab 6 of 6) |
| Alert config editor | Complete | `ui/src/components/AlertConfigSection.tsx` (404 lines) |
| Alert summary counts | Complete | AuditPage summary header shows open alerts by severity |
| TypeScript types | Complete | `ui/src/types/audit.ts` (AlertConfig, AuditAlert, AlertTypeSetting, ResolveAlertRequest) |
| API methods | Complete | `ui/src/services/engineApi.ts` (getAuditAlerts, resolveAuditAlert, getAlertConfig, updateAlertConfig) |
| Settings page integration | **Commented out** | `ui/src/pages/SettingsPage.tsx` — AlertConfigSection import and render are commented out (component preserved in `AlertConfigSection.tsx`) |
| Dashboard tile | None | Audit tile shows "security alerts" in description but no separate alerts tile |
| Dedicated route | None | No `/alerts` route exists |

### Product/SKU System

The project uses an **installed apps** model (not traditional SKUs):
- Apps are defined as constants: `mcp-server`, `ai-data-liaison`, `ai-agents`
- Backend: `pkg/models/installed_app.go` has `KnownAppIDs` map for validation
- Frontend: `ui/src/types/installedApp.ts` has `APP_ID_*` constants
- Dashboard tiles are conditionally shown via `installedApps.some(app => app.app_id === APP_ID_X)`
- Installation flow goes through ekaya-central for billing/activation
- DB table: `engine_installed_apps` with `app_id`, `project_id`, `settings` JSONB, `activated_at`

## Implementation Plan

### Phase 1: New Product SKU

- [ ] Add `AppIDAlerts = "alerts"` constant to `pkg/models/installed_app.go`
- [ ] Add `"alerts"` to `KnownAppIDs` map in same file
- [ ] Add `APP_ID_ALERTS = 'alerts'` constant to `ui/src/types/installedApp.ts`
- [ ] Add Alerts app entry to `ui/src/pages/ApplicationsPage.tsx` in the applications list (title: "Alerts & Monitoring", icon: Bell, color: red/amber, installable: true)

### Phase 2: Decouple Alert Config Storage

Currently alert config is stored in `engine_mcp_config.alert_config` JSONB column, which requires an MCP config row to exist. This is the root cause of the "Failed to load alert configuration" error on the Settings page — if MCP Server hasn't been configured, there's no `engine_mcp_config` row.

- [ ] Create migration to add `alert_config` JSONB column to `engine_installed_apps` table (or create a dedicated `engine_alert_config` table with project_id + config JSONB)
- [ ] Update `GetAlertConfig` / `SetAlertConfig` repository methods to read/write from the new location instead of `engine_mcp_config`
- [ ] Ensure `GetAlertConfig` returns `DefaultAlertConfig()` when no config exists (already handled in handler, but repo should also handle gracefully)
- [ ] Add down migration to preserve the `engine_mcp_config.alert_config` data during transition

**Decision needed:** Store alert config in `engine_installed_apps.settings` JSONB for the alerts app, or create a dedicated table? The `engine_installed_apps.settings` approach is simpler and follows the existing pattern. A dedicated table gives more query flexibility.

### Phase 3: New Alerts Page

Create a dedicated `/projects/{pid}/alerts` page that combines the alert list and alert configuration into a single view.

- [ ] Create `ui/src/pages/AlertsPage.tsx` with two sections/tabs:
  1. **Active Alerts** — Extract and enhance the alerts tab from AuditPage (lines 1181-1410)
     - Status filter (open/resolved/dismissed/all)
     - Severity filter (all/critical/warning/info)
     - Alert type filter (new — not in current UI)
     - Alert list table with resolution workflow
     - Summary header showing open alert counts by severity
  2. **Configuration** — Move AlertConfigSection here (currently in SettingsPage)
     - Master enable/disable toggle
     - Per-alert-type settings (enable, severity, thresholds)
- [ ] Add route `<Route path="alerts" element={<AlertsPage />} />` in `ui/src/App.tsx` under the project routes
- [ ] Remove AlertConfigSection from `ui/src/pages/SettingsPage.tsx`

### Phase 4: Dashboard Tile

- [ ] Add Alerts tile to `ui/src/pages/ProjectDashboard.tsx` in the Intelligence section, gated by `APP_ID_ALERTS`:
  ```
  Title: "Alerts & Monitoring"
  Description: "Security alerts, anomaly detection, and governance monitoring"
  Icon: Bell
  Path: /projects/{pid}/alerts
  Color: amber or red
  ```
- [ ] Show alert count badge on tile if there are open alerts (optional enhancement — requires fetching alert summary on dashboard load)

### Phase 5: Clean Up Audit Page

After alerts have their own page:

- [ ] Remove the Alerts tab (tab 6) from `ui/src/pages/AuditPage.tsx`
- [ ] Remove alert-related summary counts from AuditPage summary header (or keep a minimal "X open alerts" link that navigates to `/alerts`)
- [ ] Remove unused alert imports from AuditPage

### Phase 6: Backend Route Consideration (Optional)

The existing API routes are under `/api/projects/{pid}/audit/alerts` and `/api/projects/{pid}/audit/alert-config`. These still work fine regardless of the frontend route change. No backend route changes are strictly needed, but for consistency:

- [ ] Consider adding route aliases at `/api/projects/{pid}/alerts` and `/api/projects/{pid}/alerts/config` that map to the same handlers
- [ ] Or leave the existing routes as-is (simpler, no breaking changes)

**Recommendation:** Leave existing API routes as-is. The frontend already knows the API paths — changing them adds risk for no user-facing benefit.

## HARD STOP — Open Questions Must Be Resolved Before Implementation

> **IMPLEMENTER: DO NOT BEGIN ANY PHASE OF THIS PLAN UNTIL ALL FOUR QUESTIONS BELOW HAVE WRITTEN ANSWERS.**
>
> These questions affect architectural decisions across multiple phases. Implementing without answers will result in rework. If you are an automated agent, escalate to the user and wait for responses. If answers are not recorded below, ask the user before proceeding.

### Question 1: Product SKU name and billing
**Status:** UNANSWERED

The `alerts` app ID needs to be registered in ekaya-central's app catalog for install/activate/uninstall flows. What is the pricing model? Is this a separate paid product or included with another tier?

**Why this blocks:** Phase 1 (new product SKU) cannot be completed without knowing the app ID string, display name, and whether central needs changes first.

**Answer:**
> _(record answer here)_

---

### Question 2: Alert config storage
**Status:** UNANSWERED

Should alert config move to `engine_installed_apps.settings` JSONB (simpler, follows existing pattern) or a dedicated `engine_alert_config` table (more query flexibility)? The current storage in `engine_mcp_config.alert_config` will break if alerts are decoupled from MCP Server — if no `engine_mcp_config` row exists for the project, the GET endpoint fails (this is the "Failed to load alert configuration" bug visible in the Settings page today).

**Why this blocks:** Phase 2 (decouple alert config storage) requires knowing the target schema before writing migrations or updating repository code.

**Answer:**
> _(record answer here)_

---

### Question 3: Dependency on MCP audit events
**Status:** UNANSWERED

Alert triggers currently fire from `pkg/mcp/audit.go` when MCP tool calls complete. If a project has Alerts installed but NOT AI Data Liaison (which gates the MCP server and audit pipeline), no audit events will fire and no alerts will ever trigger. Should Alerts require AI Data Liaison as a prerequisite app, or should the trigger pipeline work independently of the MCP audit flow?

**Why this blocks:** This determines whether the Alerts app can be installed standalone or must enforce a dependency. It affects Phase 1 (app definition — does it declare a prerequisite?) and Phase 4 (dashboard tile — should it show a warning if the prerequisite isn't met?).

**Answer:**
> _(record answer here)_

---

### Question 4: SensitiveTable trigger scope
**Status:** UNANSWERED

The `sensitive_table_access` alert type is defined in models and shown in the config UI, but has no trigger implementation in `alert_trigger_service.go`. Should implementing this trigger be included in this plan's scope, or deferred to a future plan?

**Why this blocks:** If in scope, it adds work to Phase 3 (the alerts page would show a non-functional alert type without it) and requires a new task for the trigger implementation. If deferred, the UI should either hide the type or show it as "coming soon."

**Answer:**
> _(record answer here)_

## Files That Will Be Modified

| File | Change |
|------|--------|
| `pkg/models/installed_app.go` | Add `AppIDAlerts` constant and known app entry |
| `ui/src/types/installedApp.ts` | Add `APP_ID_ALERTS` constant |
| `ui/src/pages/ApplicationsPage.tsx` | Add Alerts app to marketplace |
| `ui/src/pages/AlertsPage.tsx` | **New file** — dedicated alerts page |
| `ui/src/App.tsx` | Add `/alerts` route |
| `ui/src/pages/ProjectDashboard.tsx` | Add Alerts tile gated by installed app |
| `ui/src/pages/SettingsPage.tsx` | Remove AlertConfigSection |
| `ui/src/pages/AuditPage.tsx` | Remove Alerts tab |
| New migration file | Alert config storage migration (if decoupling from engine_mcp_config) |
| `pkg/repositories/mcp_config_repository.go` | Update alert config methods (if changing storage) |

## Files That Will NOT Change

| File | Reason |
|------|--------|
| `pkg/handlers/alert_handler.go` | API routes stay the same |
| `pkg/services/alert_service.go` | Business logic unchanged |
| `pkg/services/alert_trigger_service.go` | Trigger pipeline unchanged |
| `pkg/repositories/alert_repository.go` | Alert data access unchanged |
| `pkg/mcp/audit.go` | Audit event pipeline unchanged |
| `ui/src/components/AlertConfigSection.tsx` | Reused as-is in new page |
| `ui/src/services/engineApi.ts` | API methods already exist |
| `ui/src/types/audit.ts` | Types already defined |
