# Plan: MCP Server Queries Configuration

## Implementation Status Summary

| Component | Status | Notes |
|-----------|--------|-------|
| MCP Config Model | ❌ Missing | No ForceMode/AllowSuggestions fields |
| Query Model | ❌ Missing | No ApprovalStatus/SuggestedBy fields |
| Tool Filter for approved_queries | ❌ Missing | Only developer tools filtered |
| MCPConfigService.ShouldShowApprovedQueriesTools | ❌ Missing | Needed for tool filtering |
| Query Repository CountEnabledQueries | ❌ Missing | Needed for business logic |
| UI Pre-Approved Queries toggle | ✅ Partial | Shows toggle but state not persisted to backend |
| MCP Tools (list/execute_approved_query) | ✅ Done | Working but need filtering |

---

## Goal

Add proper tool filtering for Pre-Approved Queries so that:
1. When toggle is OFF → tools don't appear in MCP tool list
2. When no enabled queries exist → tools don't appear (business logic override)
3. UI state persists to backend

---

## Phase 1: Tool Filtering (PRIORITY - Current Bug Fix)

### 1.1 Export Tool Names Map

**File:** `pkg/mcp/tools/queries.go`

Add at top of file (similar to `developerToolNames` in developer.go):
```go
// ApprovedQueriesToolNames lists all tools in the approved_queries group.
// Exported for use by tool filter.
var ApprovedQueriesToolNames = map[string]bool{
    "list_approved_queries":   true,
    "execute_approved_query":  true,
}
```

### 1.2 Add CountEnabledQueries to Query Service

**File:** `pkg/services/query.go`

Add interface method and implementation:
```go
// In QueryService interface
CountEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (int, error)

// Implementation
func (s *queryService) CountEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (int, error) {
    queries, err := s.repo.ListEnabled(ctx, datasourceID)
    if err != nil {
        return 0, err
    }
    return len(queries), nil
}
```

### 1.3 Add ShouldShowApprovedQueriesTools to MCPConfigService

**File:** `pkg/services/mcp_config.go`

Add to interface:
```go
ShouldShowApprovedQueriesTools(ctx context.Context, projectID uuid.UUID) (bool, error)
```

Add implementation (requires injecting QueryService and ProjectService):
```go
func (s *mcpConfigService) ShouldShowApprovedQueriesTools(ctx context.Context, projectID uuid.UUID) (bool, error) {
    // Check if tool group is enabled
    enabled, err := s.IsToolGroupEnabled(ctx, projectID, "approved_queries")
    if err != nil || !enabled {
        return false, err
    }

    // Get default datasource for the project
    dsID, err := s.projectService.GetDefaultDatasourceID(ctx, projectID)
    if err != nil {
        return false, nil // No datasource = no queries
    }

    // Check if any enabled queries exist
    count, err := s.queryService.CountEnabledQueries(ctx, projectID, dsID)
    if err != nil {
        return false, err
    }

    return count > 0, nil
}
```

**Dependency injection change:** MCPConfigService needs QueryService and ProjectService:
```go
type mcpConfigService struct {
    repo           repositories.MCPConfigRepository
    queryService   QueryService      // Add
    projectService ProjectService    // Add
    serverURL      string
    logger         *zap.Logger
}
```

### 1.4 Extend NewToolFilter for approved_queries

**File:** `pkg/mcp/tools/developer.go`

Import the queries package tool names and extend filter:

```go
import (
    // ... existing imports
)

// At package level, reference queries tool names
// Note: Can't import from same package, so either:
// Option A: Move ApprovedQueriesToolNames to this file
// Option B: Create shared constants file

// Add to NewToolFilter function, after developer tools check:
func NewToolFilter(deps *DeveloperToolDeps) func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
    return func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
        // ... existing auth check code ...

        // Get developer tools config (existing)
        config, err := deps.MCPConfigService.GetToolGroupConfig(tenantCtx, projectID, developerToolGroup)

        // Get approved_queries visibility
        shouldShowQueries, _ := deps.MCPConfigService.ShouldShowApprovedQueriesTools(tenantCtx, projectID)

        var filtered []mcp.Tool
        for _, tool := range tools {
            // Filter developer tools (existing logic)
            if developerToolNames[tool.Name] {
                if config == nil || !config.Enabled {
                    continue
                }
                if tool.Name == "execute" && !config.EnableExecute {
                    continue
                }
            }

            // Filter approved_queries tools (NEW)
            if isApprovedQueriesToolName(tool.Name) && !shouldShowQueries {
                continue
            }

            filtered = append(filtered, tool)
        }
        return filtered
    }
}

// Helper function
func isApprovedQueriesToolName(name string) bool {
    return name == "list_approved_queries" || name == "execute_approved_query"
}
```

### 1.5 Update main.go Dependencies

**File:** `main.go`

Update MCPConfigService creation to include QueryService and ProjectService:
```go
mcpConfigService := services.NewMCPConfigService(
    repositories.NewMCPConfigRepository(),
    queryService,      // Add
    projectService,    // Add
    cfg.ServerURL,
    logger,
)
```

### 1.6 Tests

**File:** `pkg/mcp/tools/developer_filter_test.go`

Add test cases:
```go
func TestNewToolFilter_ApprovedQueriesToggleOff(t *testing.T)
func TestNewToolFilter_ApprovedQueriesNoQueriesExist(t *testing.T)
func TestNewToolFilter_ApprovedQueriesWithQueries(t *testing.T)
```

---

## Phase 2: Backend Model Updates (Future)

### 2.1 MCP Config Model

**File:** `pkg/models/mcp_config.go`

```go
type ToolGroupConfig struct {
    Enabled          bool `json:"enabled"`
    EnableExecute    bool `json:"enableExecute"`
    ForceMode        bool `json:"forceMode"`        // NEW
    AllowSuggestions bool `json:"allowSuggestions"` // NEW
}
```

### 2.2 Query Model (for suggest_query feature)

**File:** `pkg/models/query.go`

```go
type ApprovalStatus string

const (
    ApprovalStatusApproved ApprovalStatus = "approved"
    ApprovalStatusPending  ApprovalStatus = "pending"
    ApprovalStatusRejected ApprovalStatus = "rejected"
)

// Add to Query struct:
ApprovalStatus ApprovalStatus `json:"approval_status"`
SuggestedBy    *string        `json:"suggested_by,omitempty"`
SuggestedAt    *time.Time     `json:"suggested_at,omitempty"`
```

### 2.3 Database Migration

```sql
ALTER TABLE engine_queries
ADD COLUMN approval_status VARCHAR(20) NOT NULL DEFAULT 'approved';

ALTER TABLE engine_queries
ADD COLUMN suggested_by VARCHAR(255);

ALTER TABLE engine_queries
ADD COLUMN suggested_at TIMESTAMP WITH TIME ZONE;
```

---

## Phase 3: UI State Persistence (Future)

Currently the UI has local state for Pre-Approved Queries settings that isn't persisted:
```typescript
// MCPServerPage.tsx lines 26-28
const [forceMode, setForceMode] = useState(false);
const [allowSuggestions, setAllowSuggestions] = useState(true);
const [allowClientSuggestions, setAllowClientSuggestions] = useState(false);
```

Need to:
1. Read these from backend config response
2. Persist changes via `updateMCPConfig()` API call
3. Backend needs to store in `ToolGroupConfig.ForceMode` and `ToolGroupConfig.AllowSuggestions`

---

## Phase 4: New MCP Tools (Future)

Rename/add tools:
- `list_approved_queries` → `available_queries` (with conditional SQL exposure)
- `execute_approved_query` → `execute_query`
- NEW: `suggest_query` (creates pending query for admin approval)

---

## File Changes Summary

### Phase 1 (Priority)
| File | Change |
|------|--------|
| `pkg/mcp/tools/queries.go` | Export ApprovedQueriesToolNames map |
| `pkg/services/query.go` | Add CountEnabledQueries method |
| `pkg/services/mcp_config.go` | Add ShouldShowApprovedQueriesTools, inject dependencies |
| `pkg/mcp/tools/developer.go` | Extend NewToolFilter for approved_queries |
| `main.go` | Update MCPConfigService initialization |
| `pkg/mcp/tools/developer_filter_test.go` | Add approved_queries filter tests |

### Phase 2-4 (Future)
| File | Change |
|------|--------|
| `pkg/models/mcp_config.go` | Add ForceMode, AllowSuggestions fields |
| `pkg/models/query.go` | Add ApprovalStatus, SuggestedBy, SuggestedAt |
| `migrations/NNNN_*.sql` | Add approval columns |
| `pkg/mcp/tools/queries.go` | Rename tools, add suggest_query |
| `ui/src/pages/MCPServerPage.tsx` | Persist toggle state to backend |

---

## V1 Mode Matrix (Reference)

| FORCE Mode | Suggestions | Query SQL | suggest_query | Developer Tools |
|------------|-------------|-----------|---------------|-----------------|
| OFF | OFF | ❌ Hidden | ❌ | Depends on toggle |
| OFF | ON | ✅ Shown | ✅ | Depends on toggle |
| ON | OFF | ❌ Hidden | ❌ | ❌ Disabled |
| ON | ON | ✅ Shown | ✅ | ❌ Disabled |
