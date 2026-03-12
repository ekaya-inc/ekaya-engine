# PLAN: Multiple Named AI Agents with Scoped Access

**Status:** COMPLETED
**Branch:** ddanieli/add-ai-agents

## Context

Currently there is ONE shared Agent API key per project, stored in `engine_mcp_config.agent_api_key_encrypted`. All agents using that key get access to ALL Pre-Approved Queries and ALL agent tools. If an admin needs multiple agents with different scopes, they have to create separate projects (and re-extract the Ontology, etc.). This is unacceptable.

### Current Architecture (What Exists Today)

**Single API Key Flow:**
1. Key is generated as 32 random bytes → 64 hex chars (`pkg/services/agent_api_key.go:53-78`)
2. Encrypted with AES-256-GCM via `CredentialEncryptor` and stored in `engine_mcp_config.agent_api_key_encrypted` (`pkg/repositories/mcp_config_repository.go:159-187`)
3. Auth middleware (`pkg/mcp/auth/middleware.go:117-201`) extracts API key from headers, validates against the single project key, then creates synthetic claims with `Subject: "agent"` and `ProjectID`
4. Tool filter (`pkg/mcp/tools/developer.go:168-255`) sees `Subject == "agent"` and returns limited query tools (`health`, `list_approved_queries`, `execute_approved_query`)
5. Audit logger (`pkg/mcp/audit.go:118-142`) logs `UserID: "agent"` and `UserEmail: nil` for agent requests

**Key files that will change:**

| File | Current Role |
|------|-------------|
| `pkg/services/agent_api_key.go` | Single key CRUD (GenerateKey, GetKey, ValidateKey) |
| `pkg/handlers/agent_api_key.go` | HTTP endpoints for single key (GET, POST regenerate) |
| `pkg/repositories/mcp_config_repository.go` | Stores encrypted key in `engine_mcp_config` |
| `pkg/mcp/auth/middleware.go` | Validates single key → synthetic claims `Subject:"agent"` |
| `pkg/mcp/audit.go` | Logs `UserID:"agent"`, `UserEmail:nil` |
| `pkg/mcp/tools/queries.go:115-207` | `list_approved_queries` returns ALL enabled queries |
| `pkg/mcp/tools/queries.go:239-293` | `execute_approved_query` checks if query is enabled, no agent scoping |
| `pkg/mcp/tools/access.go:112-167` | `CheckToolAccess` checks role, no per-agent query scoping |
| `pkg/mcp/tools/developer.go:168-255` | `NewToolFilter` filters tools by role, not per-agent |
| `ui/src/pages/AIAgentsPage.tsx` | Shows single key + setup checklist |
| `ui/src/components/mcp/AgentToolsSection.tsx` | Wraps single key display + setup example |
| `ui/src/components/mcp/AgentAPIKeyDisplay.tsx` | Single key reveal/copy/rotate UI |
| `ui/src/services/engineApi.ts:785-806` | `getAgentAPIKey`, `regenerateAgentAPIKey` |

## Goal

Support multiple named agents per project, each with their own API key, scoped pre-approved queries, and tool loadout. Agents are managed from the AI Agents admin page.

## Requirements

### UI: Agent Management

- Replace the current single-agent-key UI with a multi-agent list view.
- An **[+ Add Agent]** button opens a form with:
  1. **Name** (required) — used in logging/auditing to identify which agent accessed data.
  2. **Pre-Approved Query selection** — default is none selected. Admin can select all or a subset. At least one must be selected before saving.
  3. **[Save]** and **[Cancel]** buttons.
- After saving, the agent appears in the list with its auto-generated API key.
- Editing an existing agent allows:
  - Rotating the API key.
  - Changing which pre-approved queries it has access to.
  - Name is **read-only** after creation (cannot be changed).
- Deleting an agent requires typing `delete agent` in a confirmation dialog.

### Backend: Agent CRUD

- New database table for named agents (name, project_id, api_key_hash, created_at, etc.).
- New join table (or column) mapping agents to their allowed pre-approved query IDs.
- API endpoints: create agent, list agents, get agent, update agent queries, rotate key, delete agent.

### Auth: Scoped Access

- When an agent authenticates with its API key, the system identifies which named agent it is.
- The MCP server scopes the available tools and pre-approved queries to only those assigned to that agent.
- Listing queries returns only the agent's allowed queries.
- Executing a query is rejected if the agent doesn't have access to it.
- Changing the query list in the admin UI updates the agent's access in real time (no restart needed).

### Audit & Logging

- MCP Events and all logging/auditing uses the **agent name** instead of the user's email address when the request comes from an agent.

## Acceptance Criteria

- [x] I can click [+ Add Agent] and then [Cancel] and nothing happens.
- [x] I can click [+ Add Agent], fill in the name. I'm forced to select at least one Pre-Approved Query before I can save.
- [x] When I save an agent, I can come back and rotate keys and change which queries it has access to. I cannot change the name.
- [x] When I authenticate with that AI Agent's key, I see the limited set of tools and the MCP Client only lists the queries the agent has been given access to.
- [x] Changing the query list in the admin UI updates the MCP Client's visible queries in real time.
- [x] The AI Agent can only list and execute the pre-approved queries it has access to.
- [x] MCP Events and logging use the agent name instead of the user's email address.
- [x] I can delete an agent from the list. I have to type `delete agent` in the confirmation dialog.

---

## Implementation Tasks

### [x] Task 1: Database Migration — `017_named_agents.up.sql` / `017_named_agents.down.sql`

Create `migrations/017_named_agents.up.sql`:

```sql
-- Named AI agents with per-agent API keys and scoped query access
CREATE TABLE engine_agents (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      uuid NOT NULL REFERENCES engine_projects(id) ON DELETE CASCADE,
    name            varchar(255) NOT NULL,
    api_key_encrypted text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (project_id, name)
);

-- Agent-to-query access mapping
CREATE TABLE engine_agent_queries (
    agent_id    uuid NOT NULL REFERENCES engine_agents(id) ON DELETE CASCADE,
    query_id    uuid NOT NULL REFERENCES engine_queries(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, query_id)
);

-- Index for looking up agents by project
CREATE INDEX idx_engine_agents_project ON engine_agents(project_id);

-- Index for looking up queries by agent
CREATE INDEX idx_engine_agent_queries_agent ON engine_agent_queries(agent_id);

-- RLS policies (match existing patterns from migration 009)
ALTER TABLE engine_agents ENABLE ROW LEVEL SECURITY;
ALTER TABLE engine_agent_queries ENABLE ROW LEVEL SECURITY;
```

Down migration drops both tables.

**Design decisions:**
- `api_key_encrypted` stores AES-256-GCM encrypted key (same as `engine_mcp_config.agent_api_key_encrypted`). We do NOT store a hash — we encrypt because the `ValidateKey` flow needs constant-time comparison of the plaintext.
- `UNIQUE (project_id, name)` prevents duplicate names per project and makes name immutable (changing would break audit trail).
- `engine_agent_queries` is a simple join table. When admin unchecks a query, we delete the row. The MCP tools query this table at runtime, so changes take effect immediately with no restart.
- No `is_deleted` / soft-delete. Hard delete is fine since the confirmation dialog (`delete agent`) is the safety net.

**Migration removes** `engine_mcp_config.agent_api_key_encrypted` and its index. Named agents are the only supported agent-auth mechanism for this implementation; there is no rollout fallback.

---

### [x] Task 2: Models — `pkg/models/agent.go`

Create new model file:

```go
// Agent represents a named AI agent with scoped query access.
type Agent struct {
    ID              uuid.UUID   `json:"id"`
    ProjectID       uuid.UUID   `json:"project_id"`
    Name            string      `json:"name"`
    APIKeyEncrypted string      `json:"-"` // Never serialize
    CreatedAt       time.Time   `json:"created_at"`
    UpdatedAt       time.Time   `json:"updated_at"`
}
```

No model needed for the join table — it's just UUIDs handled by the repository.

---

### [x] Task 3: Repository — `pkg/repositories/agent_repository.go`

New repository interface and implementation:

```go
type AgentRepository interface {
    Create(ctx context.Context, agent *models.Agent) error
    GetByID(ctx context.Context, projectID, agentID uuid.UUID) (*models.Agent, error)
    ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error)
    UpdateAPIKey(ctx context.Context, agentID uuid.UUID, encryptedKey string) error
    Delete(ctx context.Context, projectID, agentID uuid.UUID) error

    // Query access management
    SetQueryAccess(ctx context.Context, agentID uuid.UUID, queryIDs []uuid.UUID) error
    GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error)
    HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error)

    // Auth: find agent by iterating encrypted keys (see design note below)
    FindByAPIKey(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error)
}
```

**Key implementation details:**

- `SetQueryAccess`: Deletes all existing rows for the agent, then inserts the new set in a single transaction. This is the simplest approach and handles both add/remove in one call.
- `FindByAPIKey`: Returns all agents for a project. The service layer decrypts each key and does constant-time comparison. With <100 agents per project, this is fast enough. Alternative: store a key prefix/hash column for filtering, but that adds complexity we don't need yet.
- All queries use tenant scoping via `database.GetTenantScope(ctx)`.

---

### [x] Task 4: Service — `pkg/services/agent_service.go`

New service interface:

```go
type AgentService interface {
    Create(ctx context.Context, projectID uuid.UUID, name string, queryIDs []uuid.UUID) (*models.Agent, string, error)
    // Returns (agent, plaintext_key, error) — key is only returned on create

    List(ctx context.Context, projectID uuid.UUID) ([]*AgentWithQueries, error)
    Get(ctx context.Context, projectID, agentID uuid.UUID) (*AgentWithQueries, error)

    UpdateQueryAccess(ctx context.Context, projectID, agentID uuid.UUID, queryIDs []uuid.UUID) error
    RotateKey(ctx context.Context, projectID, agentID uuid.UUID) (string, error)
    // Returns new plaintext key

    Delete(ctx context.Context, projectID, agentID uuid.UUID) error

    // Auth: validate key and return the matching agent
    ValidateKey(ctx context.Context, projectID uuid.UUID, key string) (*models.Agent, error)
    // Returns nil if no agent matches (not an error, just no match)
}

type AgentWithQueries struct {
    models.Agent
    QueryIDs []uuid.UUID `json:"query_ids"`
}
```

**Key implementation details:**

- `Create`: Generates 32 random bytes → 64 hex key (same as current `agent_api_key.go:53-78`), encrypts with `CredentialEncryptor`, stores agent + query access in a transaction. Validates `len(queryIDs) >= 1`.
- `ValidateKey`: Calls `repo.FindByAPIKey(ctx, projectID)` to get all agents for the project, decrypts each key, uses `subtle.ConstantTimeCompare`. Returns the matched agent or nil. This replaces the current `AgentAPIKeyService.ValidateKey`.
- `RotateKey`: Same key generation as Create, overwrites `api_key_encrypted`.
- `List`: Returns agents with their query IDs for the UI list view.

---

### [x] Task 5: Handler — `pkg/handlers/agent_handler.go`

New REST handler with routes:

```
POST   /api/projects/{pid}/agents          → Create agent (name, query_ids[])
GET    /api/projects/{pid}/agents          → List agents
GET    /api/projects/{pid}/agents/{aid}    → Get agent with query access
PATCH  /api/projects/{pid}/agents/{aid}    → Update query access (query_ids[])
POST   /api/projects/{pid}/agents/{aid}/rotate-key → Rotate API key
DELETE /api/projects/{pid}/agents/{aid}    → Delete agent
```

All routes require auth + admin role (same as current agent key endpoints at `pkg/handlers/agent_api_key.go:43-49`).

**Response shapes:**

```json
// POST create → returns agent with plaintext key (only time key is shown)
{
  "id": "uuid",
  "name": "sales-bot",
  "api_key": "64hexchars",  // plaintext, only on create
  "query_ids": ["uuid1", "uuid2"],
  "created_at": "..."
}

// GET list → returns agents with masked keys
{
  "agents": [
    {
      "id": "uuid",
      "name": "sales-bot",
      "query_ids": ["uuid1", "uuid2"],
      "created_at": "..."
    }
  ]
}

// GET single → same as list item
// PATCH update → same as list item
// POST rotate-key → { "api_key": "new64hexchars" }
// DELETE → 204 No Content
```

**Important:** The create response is the ONLY time the plaintext key is returned. After that, the key cannot be retrieved — only rotated. This is the same pattern as the current single-key flow (`AgentAPIKeyDisplay` reveals the stored key, but with named agents we'll store encrypted and never return it again after creation).

**Wait — reconsidering:** The current single-key UI lets you reveal the key on focus (`AgentAPIKeyDisplay.tsx:60-77`). For named agents, we should keep the same reveal capability via a separate endpoint. This means we need:

```
GET /api/projects/{pid}/agents/{aid}/key?reveal=true → { "key": "...", "masked": bool }
```

This mirrors the existing `GET /api/projects/{pid}/mcp/agent-key?reveal=true` pattern.

---

### [x] Task 6: Auth Middleware — Update `pkg/mcp/auth/middleware.go`

**Current flow (line 171):** `m.agentKeyService.ValidateKey(tenantCtx, projectID, apiKey)` returns `bool`.

**New flow:** Replace `agentKeyService` dependency with `agentService` (new service from Task 4). The middleware calls `agentService.ValidateKey(tenantCtx, projectID, apiKey)` which returns `*models.Agent` or nil.

**Changes to synthetic claims (line 189-192):**

```go
// Before:
claims := &auth.Claims{ProjectID: projectID.String()}
claims.Subject = "agent"

// After:
claims := &auth.Claims{
    ProjectID: projectID.String(),
    Email:     agent.Name,  // Agent name goes into Email for audit trail
}
claims.Subject = "agent:" + agent.ID.String()  // "agent:<uuid>" format
```

This design:
- `Subject` starts with `"agent:"` so `strings.HasPrefix(claims.Subject, "agent")` can still detect agent auth (update all checks from `== "agent"` to `strings.HasPrefix(..., "agent")`)
- `Subject` contains the agent UUID for correlation in audit logs
- `Email` contains the agent name for display in audit logs and MCP events

**Agent ID in context:** Add the agent ID to context so downstream code (tool filter, query tools) can access it:

```go
// New context key in pkg/auth/claims.go
const AgentIDKey contextKey = "agent_id"

// In middleware after successful auth:
ctx = context.WithValue(ctx, auth.AgentIDKey, agent.ID)
```

**Files to update:**
- `pkg/mcp/auth/middleware.go` — Change `agentKeyService` to `agentService`, update `handleAgentKeyAuth`
- `pkg/auth/claims.go` — Add `AgentIDKey` context key, add `GetAgentID(ctx)` helper
- All places that check `claims.Subject == "agent"` must change to `strings.HasPrefix(claims.Subject, "agent")`

**Locations of `claims.Subject == "agent"` checks (grep results):**
- `pkg/mcp/tools/access.go:173` — `computeToolsForRole`
- `pkg/mcp/tools/access.go:238` — `checkAppInstallation`
- `pkg/mcp/tools/access.go:284` — `AcquireToolAccess`
- `pkg/mcp/tools/developer.go:221` — `NewToolFilter`

---

### [x] Task 7: MCP Query Scoping — Update `pkg/mcp/tools/queries.go`

This is the core scoping logic. Two tools need changes:

**`list_approved_queries` (line 115-207):**

Currently calls `deps.QueryService.ListEnabled(tenantCtx, projectID, dsID)` which returns ALL enabled queries.

New logic:
1. Check if caller is an agent: `agentID, isAgent := auth.GetAgentID(ctx)`
2. If agent: get allowed query IDs from `agentService.GetQueryAccess(ctx, agentID)`, then filter the query list to only include those IDs
3. If not agent (JWT user): return all enabled queries as before

Implementation option A (filter in-memory): List all enabled queries, then filter to the agent's allowed set. Simple, works for reasonable query counts.

Implementation option B (filter in SQL): Add a new repository method `ListEnabledForAgent(ctx, projectID, dsID, queryIDs []uuid.UUID)` that adds `AND id = ANY($4)` to the WHERE clause.

**Recommendation: Option A** for simplicity. Most projects will have <100 pre-approved queries. If performance becomes an issue, we can switch to Option B later.

**`execute_approved_query` (line 239-293):**

Currently validates the query exists and is enabled (line 278-293). Add an agent access check:

```go
// After getting query metadata (line 278)
query, err := deps.QueryService.Get(tenantCtx, projectID, queryID)
// ... existing error handling ...

// NEW: Check agent access
if agentID, isAgent := auth.GetAgentID(ctx); isAgent {
    hasAccess, err := deps.AgentService.HasQueryAccess(tenantCtx, agentID, queryID)
    if err != nil {
        return nil, fmt.Errorf("failed to check agent query access: %w", err)
    }
    if !hasAccess {
        return NewErrorResult("QUERY_ACCESS_DENIED",
            fmt.Sprintf("agent does not have access to query %q", queryID)), nil
    }
}
```

**Dependencies update:** `QueryToolDeps` struct needs a new field:

```go
type QueryToolDeps struct {
    BaseMCPToolDeps
    ProjectService      services.ProjectService
    QueryService        services.QueryService
    AgentService        services.AgentService  // NEW
    Auditor             *audit.SecurityAuditor
    QueryHistoryService services.QueryHistoryService
}
```

This also requires updating the wiring in `main.go` where `QueryToolDeps` is constructed.

---

### [x] Task 8: Audit Logging — Update `pkg/mcp/audit.go`

**Current `buildEvent` (line 118-142):**

```go
if claims, ok := auth.GetClaims(ctx); ok {
    event.UserID = claims.Subject       // "agent"
    if claims.Email != "" {
        event.UserEmail = &claims.Email  // nil for agents
    }
}
```

**After Task 6 changes:** No code change needed in audit.go! The middleware now sets:
- `claims.Subject = "agent:<uuid>"` → `event.UserID` will be `"agent:<uuid>"`
- `claims.Email = agent.Name` → `event.UserEmail` will be the agent name

The audit log will naturally contain the agent name in `user_email` and the agent UUID in `user_id`. This satisfies the "MCP Events and logging use the agent name" acceptance criterion without any changes to audit.go.

---

### [x] Task 9: Tool Filter — Update `pkg/mcp/tools/developer.go`

**Current agent filter (line 220-255):** Returns `filterAgentTools(tools, effectiveEnabled)` which gives health + limited query tools.

**No change needed to tool filter logic.** Named agents still get the same tool set (health, list_approved_queries, execute_approved_query). The scoping happens at the query level inside the tools themselves (Task 7), not at the tool level.

The `isAgent` check at line 221 (`claims.Subject == "agent"`) needs to change to `strings.HasPrefix(claims.Subject, "agent")` (part of Task 6).

---

### [x] Task 10: UI — Replace Single-Key UI with Agent List

**Replace `AgentToolsSection` and `AgentAPIKeyDisplay` usage on `AIAgentsPage`.**

The current page at `ui/src/pages/AIAgentsPage.tsx` shows:
1. Setup checklist (keep as-is)
2. `AgentToolsSection` with single key display (replace)
3. Danger zone / uninstall (keep as-is)

**New UI structure:**

```
AIAgentsPage
├── AppPageHeader (keep)
├── SetupChecklist (keep — items unchanged)
├── AgentListSection (NEW — replaces AgentToolsSection)
│   ├── Header: "AI Agents" + [+ Add Agent] button
│   ├── Agent table/list:
│   │   └── Row per agent: Name | Created | Key (masked/reveal) | Actions (edit, rotate, delete)
│   └── Empty state: "No agents yet. Click + Add Agent to get started."
├── AddAgentDialog (NEW — modal form)
│   ├── Name input (required)
│   ├── Pre-Approved Query multi-select (checkboxes)
│   │   └── "Select All" toggle + individual checkboxes per query
│   ├── Validation: at least 1 query must be selected
│   └── [Save] [Cancel]
├── EditAgentDialog (NEW — modal for editing existing agent)
│   ├── Name (read-only, displayed but not editable)
│   ├── Pre-Approved Query multi-select (same as add, but pre-populated)
│   ├── API Key section: display + rotate button
│   └── [Save Changes] [Cancel]
├── DeleteAgentDialog (NEW — confirmation modal)
│   ├── "Type 'delete agent' to confirm"
│   └── [Delete] [Cancel]
└── Danger Zone (keep)
```

**New API client methods in `ui/src/services/engineApi.ts`:**

```typescript
// Agent CRUD
createAgent(projectId: string, name: string, queryIds: string[]): Promise<ApiResponse<AgentCreateResponse>>
listAgents(projectId: string): Promise<ApiResponse<AgentListResponse>>
getAgent(projectId: string, agentId: string): Promise<ApiResponse<AgentResponse>>
updateAgentQueries(projectId: string, agentId: string, queryIds: string[]): Promise<ApiResponse<AgentResponse>>
rotateAgentKey(projectId: string, agentId: string): Promise<ApiResponse<{api_key: string}>>
getAgentKey(projectId: string, agentId: string, reveal: boolean): Promise<ApiResponse<{key: string, masked: boolean}>>
deleteAgent(projectId: string, agentId: string): Promise<ApiResponse<void>>
```

**New types in `ui/src/types/`:**

```typescript
interface Agent {
  id: string;
  name: string;
  query_ids: string[];
  created_at: string;
}

interface AgentCreateResponse extends Agent {
  api_key: string;  // plaintext, shown once
}

interface AgentListResponse {
  agents: Agent[];
}
```

**Key UX decisions:**
- After creating an agent, show the API key in a modal with a copy button and a warning: "This is the only time this key will be shown. Copy it now." (Same pattern as GitHub personal access tokens.)
- Actually, reconsider: the current flow lets users reveal keys on focus. We should keep that capability. The create response shows the key, and users can also reveal it later via the `GET .../key?reveal=true` endpoint.
- When editing, pre-populate the query checkboxes with the agent's current access.
- The query list for the checkboxes comes from `engineApi.listQueries(pid, dsId)` filtered to `status === 'approved'`.

---

### [x] Task 11: Wiring — Update `main.go`

Wire the new `AgentService` and `AgentRepository` into the dependency injection. Key changes:

1. Create `AgentRepository` with the DB connection
2. Create `AgentService` with the repository + encryptor
3. Pass `AgentService` to:
   - `AgentHandler` (new handler for REST endpoints)
   - `mcpauth.Middleware` (replaces `agentKeyService`)
   - `QueryToolDeps` (for query scoping in MCP tools)
4. Register new routes on the HTTP mux

**Current wiring location:** `main.go` around line 383-429 where MCP dependencies are set up.

---

### [x] Task 12: Remove Legacy Single-Key Support

**Decision:** Do not carry forward the old shared project key in `engine_mcp_config.agent_api_key_encrypted`.

Implementation requirements:

1. Remove the runtime fallback that validated against `engine_mcp_config.agent_api_key_encrypted`
2. Remove the old single-key UI and API surface
3. Update migration `017` to drop `engine_mcp_config.agent_api_key_encrypted` and its index

This avoids transitional code paths and technical debt. Named agents are the only supported mechanism immediately.

---

## Task Ordering and Dependencies

```
Task 1 (Migration) ─── no deps, do first
    │
Task 2 (Models) ─── depends on Task 1 (knows the schema)
    │
Task 3 (Repository) ─── depends on Task 2
    │
Task 4 (Service) ─── depends on Task 3
    │
    ├── Task 5 (Handler) ─── depends on Task 4
    │
    ├── Task 6 (Auth Middleware) ─── depends on Task 4
    │       │
    │       ├── Task 7 (MCP Query Scoping) ─── depends on Task 6
    │       │
    │       └── Task 8 (Audit) ─── free after Task 6 (may need no changes)
    │
    └── Task 9 (Tool Filter) ─── minor change, depends on Task 6

Task 10 (UI) ─── depends on Task 5 (needs API endpoints)

Task 11 (Wiring) ─── depends on Tasks 4, 5, 6, 7

Task 12 (Backward Compat) ─── integrated into Task 4 and Task 6
```

**Recommended implementation order:**
1. Task 1 → 2 → 3 → 4 (database through service — foundational)
2. Task 6 (auth middleware — critical path)
3. Task 5 (handler — enables UI work)
4. Task 7 (query scoping — core feature)
5. Task 11 (wiring — connects everything)
6. Task 9 (tool filter — small change)
7. Task 8 (audit — verify, likely no changes needed)
8. Task 10 (UI — can proceed in parallel once Task 5 is done)
9. Task 12 (backward compat — baked into Tasks 4 and 6)
