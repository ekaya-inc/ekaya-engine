# PLAN: MCP Tools Should Default ON for New Projects

## Problem Statement

When a new project is created, the MCP Server exposes only the `health` tool regardless of the user's role (admin, user, or agent). The UI shows that Developer Tools are enabled with 42 tools, but the MCP server returns only 1 tool.

**Root Cause:** The tool filtering logic expects tools to be explicitly "enabled" in the database. Previously, admins had to explicitly enable tool loadouts (Business User Tools, Developer Tools, Agent Tools). The system was changed to always provide tools based on authentication, but the database defaults still require explicit opt-in.

**Desired Behavior:** New projects should have all tool sub-options defaulted to ON (opt-out model). Users get tools based on their role without requiring admin configuration:
- **Admin/Developer roles** → Full Developer Tools (query, ontology maintenance, schema tools)
- **Business User role** → Business User Tools (read-only query, schema viewing)
- **Agent auth (API key)** → Agent Tools (execute pre-approved queries only)

## Current State Investigation

### Key Files to Examine

1. **`pkg/services/mcp_config.go`** - `MCPConfigService` handles tool configuration
   - `GetConfig()` - Retrieves tool configuration for a project
   - `buildResponse()` - Builds the tool list based on config
   - `filterAndConvertToolSpecs()` - Filters tools based on enabled flags

2. **`pkg/services/mcp_tool_loadouts.go`** - Defines tool loadouts
   - `LoadoutDeveloperTools`, `LoadoutBusinessUserTools`, `LoadoutAgentTools`
   - Maps tool names to loadout categories

3. **`pkg/mcp/tools/developer.go`** - `NewToolFilter` creates the filter function
   - Determines which tools to expose based on config and role

4. **`pkg/repositories/mcp_config_repository.go`** - Database access for MCP config
   - Check how configs are created/retrieved for new projects

5. **`pkg/handlers/mcp_handler.go`** - MCP endpoint handler
   - How authentication context flows to tool filtering

### Database Tables

- `engine_mcp_configs` - Stores MCP configuration per project
- Check if new projects have rows created with default values

## Implementation Plan

### Phase 1: TDD RED - Write Failing Tests

Create integration tests that verify the expected behavior. These tests should FAIL initially (RED phase).

#### Test File: `pkg/mcp/tools/mcp_tools_integration_test.go`

```go
// Test structure:
// 1. Create a new project (no explicit MCP config)
// 2. Create users with different roles
// 3. Connect to MCP as each role
// 4. Verify tool counts match expectations

func TestMCPTools_NewProject_AdminGetsDevTools(t *testing.T)
func TestMCPTools_NewProject_UserGetsBusinessTools(t *testing.T)
func TestMCPTools_NewProject_AgentGetsAgentTools(t *testing.T)
func TestMCPTools_AllRolesGetHealthTool(t *testing.T)
```

#### Test Expectations

| Role | Expected Tool Categories | Approximate Tool Count |
|------|-------------------------|----------------------|
| Admin | Developer Tools (full) | 40+ tools |
| User (data role) | Business User Tools | 15-20 tools |
| Agent (API key) | Agent Tools only | 5-10 tools |
| All | Health tool | 1 (included in above) |

#### Test Implementation Details

1. Use `testhelpers.GetEngineDB(t)` for database access
2. Create project using `ProjectService.Create()`
3. Create users with specific roles using `UserService` or direct DB insert
4. Simulate MCP connection with role context
5. Call MCP `tools/list` and verify tool names/counts

### Phase 2: Fix - Default Tools ON

#### Option A: Database Default (Preferred)

Modify project creation to insert default MCP config with all options enabled:

**File: `pkg/services/project_service.go`** or **`pkg/repositories/project_repository.go`**

When creating a new project, also create the MCP config with defaults:

```go
// After project creation, initialize MCP config
defaultConfig := &models.MCPConfig{
    ProjectID: project.ID,
    // Developer Tools
    DevToolsEnabled: true,
    AddQueryTools: true,
    AddOntologyMaintenance: true,
    // Business User Tools
    BusinessToolsEnabled: true,
    AddReadOnlyQuery: true,
    // Agent Tools
    AgentToolsEnabled: true,
    // ... all sub-options ON
}
mcpConfigRepo.Create(ctx, defaultConfig)
```

#### Option B: Code Default (Fallback)

If no config exists, treat as "all enabled":

**File: `pkg/services/mcp_config.go`**

In `GetConfig()` or `buildResponse()`:

```go
func (s *mcpConfigService) GetConfig(ctx context.Context, projectID uuid.UUID) (*MCPConfigResponse, error) {
    config, err := s.repo.GetByProjectID(ctx, projectID)
    if err != nil || config == nil {
        // No config = return defaults with everything ON
        return s.defaultAllEnabledConfig(projectID), nil
    }
    // ... existing logic
}
```

#### Recommendation

Use **Option A** (database default) because:
1. Config is explicit and visible in DB
2. Admin changes are persisted correctly
3. Consistent with existing update flow
4. Easier to audit/debug

### Phase 3: TDD GREEN - Verify Tests Pass

After implementing the fix:
1. Run the integration tests
2. All role-based tests should now pass
3. Verify tool counts match expectations

### Phase 4: Manual Verification

1. Create a fresh project via UI
2. Navigate to MCP Server config page
3. Verify toggles show as ON by default
4. Connect Claude Code via `/mcp`
5. Verify admin gets 40+ tools (not just `health`)

## Files to Modify

| File | Change |
|------|--------|
| `pkg/mcp/tools/mcp_tools_integration_test.go` | NEW - Integration tests |
| `pkg/services/project_service.go` | Initialize MCP config on project creation |
| `pkg/repositories/mcp_config_repository.go` | Possibly add Create method if missing |
| `pkg/services/mcp_config.go` | Fallback defaults if no config exists |

## Testing Commands

```bash
# Run specific integration tests
go test -v ./pkg/mcp/tools/... -run TestMCPTools

# Run all checks
make check
```

## Success Criteria

1. [x] Integration tests exist for Admin, User, and Agent MCP access
2. [x] Tests initially fail (RED) showing current broken behavior
3. [ ] New projects automatically have MCP config with tools ON
4. [ ] Tests pass (GREEN) after fix
5. [ ] Manual verification: fresh project → admin gets 40+ tools via MCP

## Notes for Implementer

- The issue is NOT about tool registration (that was fixed in commit `06da772`)
- The issue is about **default configuration** for new projects
- Existing projects with explicit config should continue working as-is
- Focus on the "new project, no config yet" scenario
- Check if `engine_mcp_configs` table has any rows for the test project - likely empty
