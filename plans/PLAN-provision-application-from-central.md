# Plan: Provision Applications From ekaya-central Response

**Status:** IMPLEMENTED

**Goal:** When ekaya-engine provisions a project, read the `applications` array from ekaya-central's provision response and use it to determine what to provision. Today this is always MCP Server; the change makes engine respond to central's directive rather than hardcoding.

**Design principle:** ekaya-central is the source of truth for which applications a project has. ekaya-engine provisions what it's told to. Backward-compatible: if `applications` is absent (old central), engine falls back to current behavior.

**Depends on:** ekaya-central plan `PLAN-provision-mcp-server-application.md` (adds `applications` to provision response)

---

## Context

### Current State

**Central client** (`pkg/central/client.go:20-34`):
```go
type ProjectInfo struct {
    ID          string      `json:"id"`
    Name        string      `json:"name"`
    Description string      `json:"description,omitempty"`
    URLs        ProjectURLs `json:"urls,omitempty"`
}
```

Parses the provision response from ekaya-central but has no `Applications` field. Unknown JSON fields are silently ignored.

**Provision flow** (`pkg/services/projects.go:524-558`):
1. `ProvisionFromClaims()` calls `centralClient.ProvisionProject()` → gets `ProjectInfo`
2. Extracts URLs from `ProjectInfo` into `params` map
3. Calls `Provision()` which always runs:
   - `createEmptyOntology()` (line 170)
   - `createDefaultMCPConfig()` (line 175)
   - `generateAgentAPIKey()` (line 180)

These three steps are hardcoded — no check for what applications to provision.

**Project model** (`pkg/models/project.go:15-23`):
- Has `Parameters map[string]interface{}` (generic key-value store)
- No `Applications` field

### New provision response from ekaya-central

```json
{
  "project": {
    ...existing fields...
    "applications": [
      {
        "name": "mcp-server",
        "billing": {
          "status": "dormant",
          "freeSeatsLimit": 2
        }
      }
    ]
  }
}
```

---

## Tasks

### 1. Add application types to central client

**File:** `pkg/central/client.go`

- [x] Add structs for application info:

```go
// ApplicationInfo describes an application assigned to a project by ekaya-central.
type ApplicationInfo struct {
    Name    string       `json:"name"`
    Billing *BillingInfo `json:"billing,omitempty"`
}

// BillingInfo contains billing status from ekaya-central.
type BillingInfo struct {
    Status         string `json:"status"`
    FreeSeatsLimit int    `json:"freeSeatsLimit"`
}
```

- [x] Add `Applications` to `ProjectInfo`:

```go
type ProjectInfo struct {
    ID           string            `json:"id"`
    Name         string            `json:"name"`
    Description  string            `json:"description,omitempty"`
    Applications []ApplicationInfo `json:"applications,omitempty"`
    URLs         ProjectURLs       `json:"urls,omitempty"`
}
```

This is backward-compatible: if central doesn't send `applications`, the field will be `nil`.

### 2. Define application name constants

**File:** `pkg/central/client.go` (or a new `pkg/central/applications.go` if cleaner)

- [x] Add constants for known application names:

```go
const (
    AppMCPServer      = "mcp-server"
    AppAIDataLiaison  = "ai-data-liaison"
)
```

### 3. Pass applications through to Provision

**File:** `pkg/services/projects.go`

In `ProvisionFromClaims()` (line 535):

- [x] After getting `projectInfo`, store the applications list in `params`:

```go
if len(projectInfo.Applications) > 0 {
    params["applications"] = projectInfo.Applications
}
```

In `Provision()` (line 110):

- [x] Extract applications from params (with fallback):

```go
// Determine which applications to provision
applications, _ := params["applications"].([]central.ApplicationInfo)
hasMCPServer := len(applications) == 0 // default: provision MCP if no apps specified (backward compat)
for _, app := range applications {
    if app.Name == central.AppMCPServer {
        hasMCPServer = true
    }
}
```

- [x] Gate the existing MCP provisioning steps on `hasMCPServer`:

```go
if hasMCPServer {
    s.createEmptyOntology(ctx, projectID)
    s.createDefaultMCPConfig(ctx, projectID)
    s.generateAgentAPIKey(ctx, projectID)
}
```

Today this changes nothing functionally (central always sends `mcp-server`), but it sets up the pattern for when ADL or other apps are added.

### 4. Add applications to ProvisionResult

**File:** `pkg/services/projects.go`

- [x] Add `Applications` to `ProvisionResult`:

```go
type ProvisionResult struct {
    ProjectID       uuid.UUID
    Name            string
    Applications    []central.ApplicationInfo // applications assigned by central
    PAPIURL         string
    ProjectsPageURL string
    ProjectPageURL  string
    Created         bool
}
```

- [x] Populate it from `projectInfo.Applications` when creating a new project
- [x] For existing projects, read from stored `params["applications"]` if present

### 5. Log applications during provisioning

**File:** `pkg/services/projects.go`

- [x] Add application names to the provision log line (line 550):

```go
appNames := make([]string, len(projectInfo.Applications))
for i, app := range projectInfo.Applications {
    appNames[i] = app.Name
}
s.logger.Info("Provisioning project from claims",
    zap.String("project_id", claims.ProjectID),
    zap.String("project_name", projectInfo.Name),
    zap.Strings("applications", appNames),
    ...
)
```

### 6. Update tests

**File:** `pkg/central/client_test.go` (if exists, or create)

- [x] Test that `ProjectInfo` correctly unmarshals the `applications` field
- [x] Test backward compatibility: response without `applications` results in `nil` slice

**File:** `pkg/services/projects_test.go` or relevant test

- [x] Test that provision with `applications: [{"name": "mcp-server"}]` runs MCP setup
- [x] Test that provision with empty `applications` falls back to MCP setup (backward compat)
- [x] Test that provision with `applications: [{"name": "ai-data-liaison"}]` (future) does NOT run MCP setup

---

## Companion Plan

The ekaya-central side of this change is documented in:
`/ekaya-central/plans/PLAN-provision-mcp-server-application.md`

ekaya-central must be deployed first (or simultaneously) since it produces the `applications` field. ekaya-engine's changes are backward-compatible and safe to deploy in any order.
