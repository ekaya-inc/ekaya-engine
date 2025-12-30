# Plan: MCP Server Queries Configuration

## Goal

Add a "Queries" configuration section to the MCP Server page that allows administrators to:
1. Control access modes (force pre-approved queries only vs developer tools)
2. Allow/disallow MCP clients from suggesting new queries
3. Redesign MCP tool interface to use a single `available_queries()` tool instead of tool-per-query

---

## Part 1: UI Changes - MCP Server Page

### 1.0 Update MCP Server Logo

Replace the current generic Server icon with the official MCP logo in:
- **ProjectDashboard.tsx** - MCP Server tile in Applications section
- **MCPServerPage.tsx** - Page header icon

**Logo sources:**
- https://modelcontextprotocol.io/docs/getting-started/intro
- https://github.com/modelcontextprotocol

Download the official MCP logo SVG and add to `ui/src/assets/` or create as a React component in `ui/src/components/icons/MCPLogo.tsx`.

### 1.1 Queries Section States

**When no queries exist:**
```
┌─────────────────────────────────────────────────────────┐
│ ○ Queries                                               │
│   No pre-approved queries have been created.            │
│   → Create queries to enable this feature               │
└─────────────────────────────────────────────────────────┘
```
- Section is visually disabled (opacity-50)
- Link navigates to `/projects/{pid}/queries`

**When queries exist:**
```
┌─────────────────────────────────────────────────────────┐
│ Queries                                                 │
├─────────────────────────────────────────────────────────┤
│ ● FORCE all access through pre-approved queries  [OFF] │
│   This is the safest way to enable AI access to the    │
│   datasource. When enabled, MCP clients can only       │
│   execute pre-approved queries.                        │
│   ⚠️ Enabling this will disable Developer Tools.       │
├─────────────────────────────────────────────────────────┤
│ ○ Allow MCP Client to suggest queries            [ON]  │
│   Allow the client using this interface to suggest     │
│   queries that must be approved by an administrator.   │
│   Enabling this will expose the ontology, schema and   │
│   SQL of the pre-approved queries.                     │
└─────────────────────────────────────────────────────────┘
```

### 1.2 Interaction Rules

| Action | Result |
|--------|--------|
| Enable "FORCE pre-approved" | Auto-disable Developer Tools, show success toast |
| Try to enable Developer Tools while FORCE is ON | Show error toast: "Only Pre-Approved Queries are allowed" |
| Disable "FORCE pre-approved" | Developer Tools toggle becomes available again |
| Enable "Allow suggestions" | MCP clients get `suggest_query()` tool + ontology/schema exposure |

### 1.3 Component Changes

**File:** `ui/src/pages/MCPServerPage.tsx`

Add new state and API calls:
```typescript
const [queryCount, setQueryCount] = useState<number>(0);
const [pendingSuggestionCount, setPendingSuggestionCount] = useState<number>(0);

// Fetch query counts on mount
useEffect(() => {
  const fetchQueryStats = async () => {
    const response = await engineApi.getQueryStats(pid, datasourceId);
    if (response.success && response.data) {
      setQueryCount(response.data.total_count);
      setPendingSuggestionCount(response.data.pending_suggestion_count);
    }
  };
  fetchQueryStats();
}, [pid, datasourceId]);
```

Add toggle handlers with mutual exclusion logic:
```typescript
const handleToggleForceQueries = async (enabled: boolean) => {
  if (enabled && config?.toolGroups['developer']?.enabled) {
    // Auto-disable developer tools first
    await handleToggleToolGroup('developer', false);
  }
  await handleToggleToolGroup('approved_queries', enabled, { forceMode: true });
};

const handleToggleDevTools = async (enabled: boolean) => {
  if (enabled && config?.toolGroups['approved_queries']?.subOptions?.['forceMode']?.enabled) {
    toast({
      title: 'Not allowed',
      description: 'Only Pre-Approved Queries are allowed. Disable FORCE mode first.',
      variant: 'destructive',
    });
    return;
  }
  await handleToggleToolGroup('developer', enabled);
};
```

**New Component:** `ui/src/components/mcp/MCPQueriesConfig.tsx`

```typescript
interface MCPQueriesConfigProps {
  queryCount: number;
  pendingSuggestionCount: number;
  forceMode: boolean;
  allowSuggestions: boolean;
  onToggleForceMode: (enabled: boolean) => void;
  onToggleSuggestions: (enabled: boolean) => void;
  disabled?: boolean;
  projectId: string;
}
```

---

## Part 2: MCP Tool Interface Redesign

### 2.1 Current vs New Design

**Current (tool-per-query):**
```
list_approved_queries()     → Returns list of queries
execute_approved_query(id)  → Runs specific query
```

**New (single tool with smart exposure):**
```
available_queries()         → Returns query metadata (varies by config)
execute_query(id, params)   → Runs specific query
suggest_query(...)          → Suggests new query (if allowed)
```

### 2.2 Tool: `available_queries()`

Returns different data based on configuration:

**Base response (always):**
```json
{
  "queries": [
    {
      "id": "uuid",
      "name": "Get customer orders",
      "description": "Returns all orders for a specific customer",
      "parameters": [
        {
          "name": "customer_id",
          "type": "uuid",
          "description": "The customer's unique identifier",
          "required": true
        }
      ]
    }
  ]
}
```

**When `allow_suggestions` is OFF:**
- DO NOT include `dialect` or `sql` fields
- MCP client sees what queries exist but not the implementation

**When `allow_suggestions` is ON:**
- Include `dialect` and `sql` fields
- Client can see query implementation to suggest similar queries

```json
{
  "queries": [...],
  "schema_available": true,
  "can_suggest": true
}
```

### 2.3 Tool: `execute_query(id, params)`

Same as current `execute_approved_query` - no changes needed except rename.

### 2.4 Tool: `suggest_query()` (New)

**Only available when `allow_suggestions` is enabled.**

```typescript
suggest_query({
  natural_language_prompt: string,  // Required
  additional_context?: string,
  sql_query: string,                // Required
  parameters?: QueryParameter[]
})
```

**Response:**
```json
{
  "success": true,
  "suggestion_id": "uuid",
  "message": "Query suggestion submitted for admin review",
  "status": "pending_approval"
}
```

### 2.5 Schema/Ontology Exposure

When `allow_suggestions` is ON, also expose:
- `get_schema()` - Returns table/column structure (like developer tools)
- `get_ontology()` - Returns entity summaries and relationships

This allows intelligent MCP clients to craft valid SQL suggestions.

---

## Part 3: Query Approval States

### 3.1 New State Model

Current model has `is_enabled` boolean. Add `approval_status` field:

```go
type ApprovalStatus string

const (
    ApprovalStatusApproved  ApprovalStatus = "approved"   // Admin created or approved
    ApprovalStatusPending   ApprovalStatus = "pending"    // Suggested, awaiting review
    ApprovalStatusRejected  ApprovalStatus = "rejected"   // Admin rejected suggestion
)
```

**State combinations:**
| approval_status | is_enabled | Meaning | Shows in available_queries() |
|-----------------|------------|---------|------------------------------|
| approved | true | Active query | Yes |
| approved | false | Disabled by admin | No |
| pending | - | Awaiting review | No |
| rejected | - | Rejected suggestion | No |

### 3.2 Database Migration

```sql
-- Add approval_status column with default for existing rows
ALTER TABLE engine_queries
ADD COLUMN approval_status VARCHAR(20) NOT NULL DEFAULT 'approved';

-- Add suggested_by for tracking who/what suggested
ALTER TABLE engine_queries
ADD COLUMN suggested_by VARCHAR(255);

-- Add suggested_at timestamp
ALTER TABLE engine_queries
ADD COLUMN suggested_at TIMESTAMP WITH TIME ZONE;

-- Index for filtering pending suggestions
CREATE INDEX idx_queries_approval_status
ON engine_queries(project_id, approval_status)
WHERE deleted_at IS NULL;
```

### 3.3 UI for Pending Suggestions

**QueriesView.tsx changes:**

Add filter tabs:
```
[All Queries] [Approved (5)] [Pending Review (3)] [Rejected (1)]
```

Pending queries show with distinct styling:
```
┌─────────────────────────────────────────────────────────┐
│ ⏳ Get monthly revenue by region                        │
│    pending • suggested by MCP client • 2 hours ago     │
│    [Approve] [Reject] [Edit & Approve]                 │
└─────────────────────────────────────────────────────────┘
```

---

## Part 4: Dashboard Badge

### 4.1 Queries Tile Badge

**File:** `ui/src/pages/ProjectDashboard.tsx`

Add pending suggestion count state and fetch:

```typescript
const [pendingQueryCount, setPendingQueryCount] = useState<number>(0);

useEffect(() => {
  const fetchPendingQueries = async () => {
    if (!pid || !isConnected) return;
    const response = await engineApi.getQueryStats(pid, datasourceId);
    if (response.success && response.data) {
      setPendingQueryCount(response.data.pending_suggestion_count);
    }
  };
  fetchPendingQueries();
}, [pid, isConnected, datasourceId]);
```

Render badge on Queries tile (similar to Ontology tile pattern):

```typescript
{isQueriesTile && pendingQueryCount > 0 && !tile.disabled && (
  <div className="absolute -top-1 -right-1 flex items-center gap-1 bg-amber-500 text-white text-xs font-medium px-2 py-0.5 rounded-full">
    <Clock className="h-3 w-3" />
    {pendingQueryCount}
  </div>
)}
```

---

## Part 5: Backend Changes

### 5.1 MCP Config Model Updates

**File:** `pkg/models/mcp_config.go`

```go
type ToolGroupConfig struct {
    Enabled          bool `json:"enabled"`
    EnableExecute    bool `json:"enableExecute"`
    ForceMode        bool `json:"forceMode"`        // New: FORCE pre-approved only
    AllowSuggestions bool `json:"allowSuggestions"` // New: Allow suggest_query
}

func DefaultMCPConfig(projectID uuid.UUID) *MCPConfig {
    return &MCPConfig{
        ProjectID: projectID,
        ToolGroups: map[string]*ToolGroupConfig{
            "developer":        {Enabled: false},
            "approved_queries": {Enabled: true, ForceMode: false, AllowSuggestions: true},
        },
    }
}
```

### 5.2 Query Model Updates

**File:** `pkg/models/query.go`

```go
type ApprovalStatus string

const (
    ApprovalStatusApproved ApprovalStatus = "approved"
    ApprovalStatusPending  ApprovalStatus = "pending"
    ApprovalStatusRejected ApprovalStatus = "rejected"
)

type Query struct {
    // ... existing fields ...
    ApprovalStatus ApprovalStatus `json:"approval_status"`
    SuggestedBy    *string        `json:"suggested_by,omitempty"`
    SuggestedAt    *time.Time     `json:"suggested_at,omitempty"`
}
```

### 5.3 New API Endpoints

**Query Stats:**
```
GET /api/projects/{pid}/datasources/{did}/queries/stats

Response:
{
  "total_count": 8,
  "approved_count": 5,
  "pending_suggestion_count": 3,
  "rejected_count": 0
}
```

**Approve/Reject Suggestion:**
```
POST /api/projects/{pid}/datasources/{did}/queries/{qid}/approve
POST /api/projects/{pid}/datasources/{did}/queries/{qid}/reject
```

### 5.4 MCP Tools Updates

**File:** `pkg/mcp/tools/queries.go`

Replace `RegisterApprovedQueriesTools` with:

```go
func RegisterQueryTools(s *server.MCPServer, deps *QueryToolDeps) {
    registerAvailableQueriesTool(s, deps)
    registerExecuteQueryTool(s, deps)
    registerSuggestQueryTool(s, deps)  // Conditional registration
}

func registerAvailableQueriesTool(s *server.MCPServer, deps *QueryToolDeps) {
    // Returns query metadata
    // Conditionally includes SQL/dialect based on allow_suggestions config
}

func registerSuggestQueryTool(s *server.MCPServer, deps *QueryToolDeps) {
    // Creates query with approval_status = "pending"
    // Only callable when allow_suggestions = true
}
```

### 5.5 Config Validation Service

**File:** `pkg/services/mcp_config.go`

Add validation in Update method:

```go
func (s *mcpConfigService) Update(ctx context.Context, projectID uuid.UUID, req *UpdateMCPConfigRequest) (*MCPConfigResponse, error) {
    // Validate: If enabling developer tools, check force mode isn't on
    if req.ToolGroups["developer"] != nil && req.ToolGroups["developer"].Enabled {
        current, _ := s.Get(ctx, projectID)
        if current.ToolGroups["approved_queries"] != nil &&
           current.ToolGroups["approved_queries"].SubOptions["forceMode"].Enabled {
            return nil, fmt.Errorf("cannot enable developer tools while FORCE pre-approved queries mode is active")
        }
    }
    // ... continue with update
}
```

---

## Part 6: Frontend Type Updates

**File:** `ui/src/types/mcp.ts`

```typescript
export interface ToolGroupInfo {
  enabled: boolean;
  name: string;
  description: string;
  warning?: string;
  subOptions?: Record<string, SubOptionInfo>;
}

export interface SubOptionInfo {
  enabled: boolean;
  name: string;
  description?: string;
  warning?: string;
}

// Extended config response
export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: Record<string, ToolGroupInfo>;
  queryStats?: {
    totalCount: number;
    pendingSuggestionCount: number;
  };
}
```

**File:** `ui/src/types/query.ts`

```typescript
export type ApprovalStatus = 'approved' | 'pending' | 'rejected';

export interface Query {
  // ... existing fields ...
  approval_status: ApprovalStatus;
  suggested_by?: string;
  suggested_at?: string;
}

export interface QueryStatsResponse {
  total_count: number;
  approved_count: number;
  pending_suggestion_count: number;
  rejected_count: number;
}
```

---

## Part 7: Test Plan

### 7.1 Backend Tests

**File:** `pkg/handlers/mcp_config_test.go`

```go
func TestMCPConfig_ForceModePreventsDevTools(t *testing.T)
func TestMCPConfig_DisableForceAllowsDevTools(t *testing.T)
func TestMCPConfig_AllowSuggestionsToggle(t *testing.T)
```

**File:** `pkg/mcp/tools/queries_test.go`

```go
func TestAvailableQueries_HidesSQLWhenSuggestionsDisabled(t *testing.T)
func TestAvailableQueries_ExposesSQLWhenSuggestionsEnabled(t *testing.T)
func TestSuggestQuery_CreatesWithPendingStatus(t *testing.T)
func TestSuggestQuery_FailsWhenSuggestionsDisabled(t *testing.T)
func TestExecuteQuery_OnlyRunsApprovedEnabled(t *testing.T)
```

**File:** `pkg/handlers/queries_test.go`

```go
func TestQueryStats_ReturnsCounts(t *testing.T)
func TestApproveQuery_ChangesStatus(t *testing.T)
func TestRejectQuery_ChangesStatus(t *testing.T)
func TestListQueries_FiltersByApprovalStatus(t *testing.T)
```

### 7.2 Frontend Tests

**File:** `ui/src/pages/__tests__/MCPServerPage.test.tsx`

```typescript
describe('MCPServerPage', () => {
  it('shows disabled Queries section when no queries exist')
  it('shows Queries config options when queries exist')
  it('disables Developer Tools toggle when FORCE mode is on')
  it('shows error toast when trying to enable DevTools with FORCE on')
  it('re-enables Developer Tools toggle when FORCE mode is off')
  it('toggles allow suggestions option')
})
```

**File:** `ui/src/components/__tests__/QueriesView.test.tsx`

```typescript
describe('QueriesView', () => {
  it('shows filter tabs for approval status')
  it('displays pending queries with approve/reject buttons')
  it('shows pending badge count correctly')
  it('calls approve endpoint when Approve clicked')
  it('calls reject endpoint when Reject clicked')
})
```

**File:** `ui/src/pages/__tests__/ProjectDashboard.test.tsx`

```typescript
describe('ProjectDashboard', () => {
  it('shows pending query badge on Queries tile')
  it('hides badge when no pending queries')
})
```

---

## File Changes Summary

| File | Change Type |
|------|-------------|
| `pkg/models/query.go` | Add ApprovalStatus, SuggestedBy, SuggestedAt fields |
| `pkg/models/mcp_config.go` | Add ForceMode, AllowSuggestions to ToolGroupConfig |
| `pkg/repositories/query_repository.go` | Add methods for stats, filter by status |
| `pkg/services/query.go` | Add SuggestQuery, ApproveQuery, RejectQuery, GetStats |
| `pkg/services/mcp_config.go` | Add validation for force mode vs dev tools |
| `pkg/handlers/queries.go` | Add Stats, Approve, Reject endpoints |
| `pkg/mcp/tools/queries.go` | Rewrite with available_queries, suggest_query |
| `migrations/NNNN_add_query_approval.sql` | Add approval_status, suggested_by columns |
| `ui/src/types/query.ts` | Add ApprovalStatus type, QueryStatsResponse |
| `ui/src/types/mcp.ts` | Extend ToolGroupInfo with new sub-options |
| `ui/src/services/engineApi.ts` | Add getQueryStats, approveQuery, rejectQuery |
| `ui/src/pages/MCPServerPage.tsx` | Add Queries config section, force mode logic |
| `ui/src/components/mcp/MCPQueriesConfig.tsx` | New component for queries config UI |
| `ui/src/components/QueriesView.tsx` | Add approval status filters, approve/reject UI |
| `ui/src/pages/ProjectDashboard.tsx` | Add pending query badge to Queries tile |
| `ui/src/components/mcp/MCPToolGroup.tsx` | Support error state for disabled toggles |
| `pkg/handlers/queries_test.go` | Tests for new endpoints |
| `pkg/mcp/tools/queries_test.go` | Tests for redesigned tools |
| `ui/src/pages/__tests__/MCPServerPage.test.tsx` | Tests for force mode, suggestions toggle |
| `ui/src/components/__tests__/QueriesView.test.tsx` | Tests for approval workflow |
