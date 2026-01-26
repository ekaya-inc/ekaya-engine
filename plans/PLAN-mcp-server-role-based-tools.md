# PLAN: MCP Server Role-Based Tool Configuration

## Problem Statement

The current MCP Server configuration page was designed with the assumption that admins would create separate projects per constituent for fine-grained tool control. However, the reality is that two roles coexist on the same project:

- **Business Users (role: `user`)** - Suggest queries and updates
- **Admins (role: `admin`, `data`, `developer`)** - Review/approve/reject suggestions, create Pre-Approved Queries, manage ontology

The current UI has:
- A single "Tools Enabled (N)" dropdown at the top showing all tools
- Toggle switches to enable/disable tool groups
- No role-based filtering of tools

**New Design**: Tools exposed via MCP depend on BOTH the project configuration AND the JWT role of the caller.

---

## UI Redesign Requirements

### Current Structure (to be replaced)
```
┌─────────────────────────────────────────────────┐
│ Your MCP Server URL                             │
│   [URL with copy button]                        │
│   MCP Setup Instructions link                   │
│   > Tools Enabled (49)  ← REMOVE THIS           │
├─────────────────────────────────────────────────┤
│ Tool Configuration  ← REMOVE THIS SECTION       │
│   [description text]                            │
├─────────────────────────────────────────────────┤
│ ○ Business User Tools  ← HAS TOGGLE             │
│   [description]                                 │
│   ☐ Allow Usage to Improve Ontology             │
├─────────────────────────────────────────────────┤
│ ○ Agent Tools  ← HAS TOGGLE                     │
│   [Agent API Key section]                       │
│   [JSON example]                                │
├─────────────────────────────────────────────────┤
│ ○ Developer Tools  ← HAS TOGGLE                 │
│   [warning text]                                │
│   ☐ Include schema exploration...              │
│   ☐ Include ontology management...             │
└─────────────────────────────────────────────────┘
```

### New Structure
```
┌─────────────────────────────────────────────────┐
│ Your MCP Server URL                             │
│   [URL with copy button]                        │
│   MCP Setup Instructions link                   │
│   (Note about changes requiring MCP restart)    │
│   ← NO TOOLS ENABLED DROPDOWN HERE              │
├─────────────────────────────────────────────────┤
│ User Tools  ← NO TOGGLE (always enabled)        │
│   The MCP Client can use Pre-Approved Queries   │
│   and the Ontology to craft read-only SQL for   │
│   ad-hoc requests. Any database-modifying SQL   │
│   statements will need to be in the form of a   │
│   Pre-Approved Query.                           │
│                                                 │
│   ☑ Allow Usage to Improve Ontology [RECOMMENDED]│
│     (ON by default)                             │
│                                                 │
│   > Tools Enabled (42)  ← COLLAPSED             │
├─────────────────────────────────────────────────┤
│ Developer Tools  ← NO TOGGLE (always enabled)   │
│   [description for admin/data/developer roles]  │
│                                                 │
│   ☑ Add Query Tools                             │
│   ☑ Add Ontology Maintenance                    │
│                                                 │
│   > Tools Enabled (49)  ← COLLAPSED             │
├─────────────────────────────────────────────────┤
│ Agent Tools  ← NO TOGGLE (always enabled)       │
│   [description]                                 │
│                                                 │
│   Agent API Key                                 │
│   [Generate/Regenerate/Revoke buttons]          │
│                                                 │
│   Agent Setup Example:                          │
│   {                                             │
│     "mcpServers": {                             │
│       "ekaya": {                                │
│         "type": "http",                         │
│         "url": "<server-url>",                  │
│         "headers": {                            │
│           "Authorization": "Bearer <key>"       │
│         }                                       │
│       }                                         │
│     }                                           │
│   }                                             │
│                                                 │
│   > Tools Enabled (3)  ← COLLAPSED              │
└─────────────────────────────────────────────────┘
```

---

## Role-to-Tool Mapping

| JWT Role | Tool Set | Description |
|----------|----------|-------------|
| `user` | User Tools (~42) | Pre-approved queries, ad-hoc read-only SQL, ontology (with optional ontology maintenance) |
| `admin` | Developer Tools (varies) | Schema, query management, ontology maintenance (configurable via sub-options) |
| `data` | Developer Tools (varies) | Same as admin - data analysts need full query tools |
| `developer` | Developer Tools (varies) | Same as admin - developers need debugging access |
| Agent API Key | Agent Tools (3) | `health`, `list_approved_queries`, `execute_approved_query` |

**Sub-options affecting tool counts:**
- User Tools: `allowOntologyMaintenance` adds/removes ontology maintenance tools
- Developer Tools: `addQueryTools` and `addOntologyMaintenance` control tool availability

---

## Implementation Tasks

### Task 1: Backend - Add Role-Based Tool Filtering [x]

**File: `pkg/mcp/tools/access.go`**

Currently `CheckToolAccess` computes enabled tools from project config only. Update to also consider JWT role:

```go
func CheckToolAccess(ctx context.Context, toolName string) (bool, error) {
    claims := auth.ClaimsFromContext(ctx)

    // Determine effective tool set based on role
    isAgent := claims.Subject == "agent"
    isAdmin := slices.Contains(claims.Roles, models.RoleAdmin) ||
               slices.Contains(claims.Roles, models.RoleData) ||
               slices.Contains(claims.Roles, "developer")

    // Get project config
    config := ... // existing logic

    // Compute tools based on role
    var enabledTools []ToolSpec
    if isAgent {
        enabledTools = ComputeAgentTools(config)  // Limited set
    } else if isAdmin {
        enabledTools = ComputeDeveloperTools(config)  // Full set
    } else {
        enabledTools = ComputeUserTools(config)  // Business user set
    }

    return contains(enabledTools, toolName), nil
}
```

**File: `pkg/services/mcp_tool_loadouts.go`**

Add new functions for role-based tool computation:

```go
func ComputeUserTools(state map[string]*models.ToolGroupConfig) []ToolSpec {
    // Returns tools for business users:
    // - health (always)
    // - Pre-approved query tools
    // - Ad-hoc read-only SQL tools (ontology-guided)
    // - Ontology maintenance tools IF allowOntologyMaintenance is true
}

func ComputeDeveloperTools(state map[string]*models.ToolGroupConfig) []ToolSpec {
    // Returns full tool set:
    // - All user tools
    // - Schema exploration tools
    // - Query management tools (create, update, delete approved queries)
    // - Full ontology maintenance
    // - Developer debugging tools
}

func ComputeAgentTools(state map[string]*models.ToolGroupConfig) []ToolSpec {
    // Returns limited set:
    // - health
    // - list_approved_queries
    // - execute_approved_query
}
```

### Task 2: Backend - Update MCP Config Response [x]

**File: `pkg/services/mcp_config.go`**

Update `MCPConfigResponse` to include per-role enabled tools:

```go
type MCPConfigResponse struct {
    ServerURL         string                             `json:"serverUrl"`
    ToolGroups        map[string]*models.ToolGroupConfig `json:"toolGroups"`
    UserTools         []EnabledToolInfo                  `json:"userTools"`      // NEW
    DeveloperTools    []EnabledToolInfo                  `json:"developerTools"` // NEW
    AgentTools        []EnabledToolInfo                  `json:"agentTools"`     // NEW
    EnabledTools      []EnabledToolInfo                  `json:"enabledTools"`   // DEPRECATED - keep for backward compat
}
```

Update `buildResponse()` to compute all three tool lists.

### Task 3: Backend - Update Tool Group Config [x]

Since tool groups no longer have enable/disable toggles but retain sub-options:

**File: `pkg/models/mcp_config.go`**

The `ToolGroupConfig` struct keeps the sub-options:

```go
type ToolGroupConfig struct {
    // User Tools options
    AllowOntologyMaintenance bool `json:"allowOntologyMaintenance,omitempty"`

    // Developer Tools options (kept as-is)
    AddQueryTools          bool `json:"addQueryTools,omitempty"`
    AddOntologyMaintenance bool `json:"addOntologyMaintenance,omitempty"`

    // Legacy fields - can be removed in future cleanup
    Enabled     bool     `json:"enabled,omitempty"`  // No longer used for selection
    CustomTools []string `json:"customTools,omitempty"`
}
```

### Task 4: Frontend - Update Types

**File: `ui/src/types/mcp.ts`**

```typescript
export interface MCPConfigResponse {
  serverUrl: string;
  toolGroups: Record<string, ToolGroupState>;
  userTools: EnabledToolInfo[];      // NEW
  developerTools: EnabledToolInfo[]; // NEW
  agentTools: EnabledToolInfo[];     // NEW
  enabledTools: EnabledToolInfo[];   // DEPRECATED - for backward compat
}
```

### Task 5: Frontend - Update MCPServerPage

**File: `ui/src/pages/MCPServerPage.tsx`**

Major restructure:

1. **Remove** the top-level "Tools Enabled" dropdown from the URL section
2. **Remove** the "Tool Configuration" header section
3. **Restructure** to show three always-visible sections:

```tsx
<div className="space-y-6">
  {/* URL Section - simplified */}
  <Card>
    <CardHeader>
      <CardTitle>Your MCP Server URL</CardTitle>
    </CardHeader>
    <CardContent>
      <CopyableUrl url={config.serverUrl} />
      <Link to="...">MCP Setup Instructions</Link>
      <p className="text-sm text-muted-foreground">
        Note: Changes to configuration will take effect after restarting the MCP Client.
      </p>
    </CardContent>
  </Card>

  {/* User Tools Section */}
  <UserToolsSection
    allowOntologyMaintenance={config.toolGroups.user?.allowOntologyMaintenance ?? true}
    onAllowOntologyMaintenanceChange={handleAllowOntologyMaintenanceChange}
    enabledTools={config.userTools}
  />

  {/* Developer Tools Section */}
  <DeveloperToolsSection
    addQueryTools={config.toolGroups.developer?.addQueryTools ?? true}
    onAddQueryToolsChange={handleAddQueryToolsChange}
    addOntologyMaintenance={config.toolGroups.developer?.addOntologyMaintenance ?? true}
    onAddOntologyMaintenanceChange={handleAddOntologyMaintenanceChange}
    enabledTools={config.developerTools}
  />

  {/* Agent Tools Section */}
  <AgentToolsSection
    serverUrl={config.serverUrl}
    apiKey={agentApiKey}
    onGenerateKey={handleGenerateKey}
    onRevokeKey={handleRevokeKey}
    enabledTools={config.agentTools}
  />
</div>
```

### Task 6: Frontend - Create UserToolsSection Component

**File: `ui/src/components/mcp/UserToolsSection.tsx`**

```tsx
interface UserToolsSectionProps {
  allowOntologyMaintenance: boolean;
  onAllowOntologyMaintenanceChange: (value: boolean) => void;
  enabledTools: EnabledToolInfo[];
}

export function UserToolsSection({ ... }: UserToolsSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>User Tools</CardTitle>
        <CardDescription>
          The MCP Client can use{' '}
          <Link to="./queries" className="text-primary hover:underline">
            Pre-Approved Queries
          </Link>{' '}
          and the Ontology to craft read-only SQL for ad-hoc requests.
          Any database-modifying SQL statements will need to be in the form of
          a Pre-Approved Query.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Ontology improvement toggle */}
        <div className="flex items-center justify-between">
          <div>
            <Label>Allow Usage to Improve Ontology</Label>
            <p className="text-sm text-muted-foreground">RECOMMENDED</p>
          </div>
          <Switch
            checked={allowOntologyMaintenance}
            onCheckedChange={onAllowOntologyMaintenanceChange}
          />
        </div>

        {/* Collapsible tools list */}
        <MCPEnabledTools
          tools={enabledTools}
          title={`Tools Enabled (${enabledTools.length})`}
        />
      </CardContent>
    </Card>
  );
}
```

### Task 7: Frontend - Create DeveloperToolsSection Component

**File: `ui/src/components/mcp/DeveloperToolsSection.tsx`**

Includes the existing sub-options for Developer Tools:

```tsx
interface DeveloperToolsSectionProps {
  addQueryTools: boolean;
  onAddQueryToolsChange: (value: boolean) => void;
  addOntologyMaintenance: boolean;
  onAddOntologyMaintenanceChange: (value: boolean) => void;
  enabledTools: EnabledToolInfo[];
}

export function DeveloperToolsSection({
  addQueryTools,
  onAddQueryToolsChange,
  addOntologyMaintenance,
  onAddOntologyMaintenanceChange,
  enabledTools,
}: DeveloperToolsSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Developer Tools</CardTitle>
        <CardDescription>
          Full access to schema exploration, query management, and ontology maintenance.
          Available to users with admin, data, or developer roles.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {/* Add Query Tools toggle */}
        <div className="flex items-center justify-between">
          <div>
            <Label>Add Query Tools</Label>
            <p className="text-sm text-muted-foreground">
              Include schema exploration and query execution tools
            </p>
          </div>
          <Switch
            checked={addQueryTools}
            onCheckedChange={onAddQueryToolsChange}
          />
        </div>

        {/* Add Ontology Maintenance toggle */}
        <div className="flex items-center justify-between">
          <div>
            <Label>Add Ontology Maintenance</Label>
            <p className="text-sm text-muted-foreground">
              Include ontology management and refinement tools
            </p>
          </div>
          <Switch
            checked={addOntologyMaintenance}
            onCheckedChange={onAddOntologyMaintenanceChange}
          />
        </div>

        {/* Collapsible tools list */}
        <MCPEnabledTools
          tools={enabledTools}
          title={`Tools Enabled (${enabledTools.length})`}
        />
      </CardContent>
    </Card>
  );
}
```

### Task 8: Frontend - Update AgentToolsSection

**File: `ui/src/components/mcp/AgentToolsSection.tsx`** (or update existing in MCPServerPage)

Move the Agent API Key management and JSON example INTO this section:

```tsx
interface AgentToolsSectionProps {
  serverUrl: string;
  apiKey: string | null;
  onGenerateKey: () => void;
  onRevokeKey: () => void;
  enabledTools: EnabledToolInfo[];
}

export function AgentToolsSection({ serverUrl, apiKey, ... }: AgentToolsSectionProps) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Agent Tools</CardTitle>
        <CardDescription>
          Limited tool access for AI agents using API key authentication.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        {/* API Key Management */}
        <div>
          <Label>AI Agent API Key</Label>
          {apiKey ? (
            <div className="flex gap-2">
              <Input value={apiKey} readOnly className="font-mono" />
              <Button variant="destructive" onClick={onRevokeKey}>Revoke</Button>
            </div>
          ) : (
            <Button onClick={onGenerateKey}>Generate API Key</Button>
          )}
        </div>

        {/* Setup Example */}
        {apiKey && (
          <div>
            <Label>Agent Setup Example</Label>
            <pre className="bg-muted p-4 rounded-md text-sm overflow-x-auto">
{JSON.stringify({
  mcpServers: {
    ekaya: {
      type: "http",
      url: serverUrl,
      headers: {
        Authorization: `Bearer ${apiKey}`
      }
    }
  }
}, null, 2)}
            </pre>
          </div>
        )}

        {/* Tools list */}
        <MCPEnabledTools
          tools={enabledTools}
          title={`Tools Enabled (${enabledTools.length})`}
        />
      </CardContent>
    </Card>
  );
}
```

### Task 9: Frontend - Update Metadata Constants

**File: `ui/src/constants/mcpToolMetadata.tsx`**

Update or remove the toggle-related metadata since groups no longer have enable/disable toggles:

```typescript
export const TOOL_GROUP_IDS = {
  USER: 'user',           // Renamed from APPROVED_QUERIES
  DEVELOPER: 'developer',
  AGENT: 'agent_tools',
} as const;

// Remove or simplify TOOL_GROUP_METADATA since we no longer need
// toggle descriptions, warnings, etc.
```

### Task 10: Backend - Update API Handler

**File: `pkg/handlers/mcp_config.go`**

The PATCH endpoint handles sub-options for both User Tools and Developer Tools:

```go
func (h *MCPConfigHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
    var req struct {
        // User Tools sub-option
        AllowOntologyMaintenance *bool `json:"allowOntologyMaintenance,omitempty"`
        // Developer Tools sub-options
        AddQueryTools          *bool `json:"addQueryTools,omitempty"`
        AddOntologyMaintenance *bool `json:"addOntologyMaintenance,omitempty"`
    }
    // ...
}
```

### Task 11: Update Tool Registry

**File: `pkg/services/mcp_tools_registry.go`**

Review and update `ToolRegistry` to assign correct `ToolGroup` values:
- Tools for business users only → `user`
- Tools for admins only → `developer`
- Agent-only tools → `agent`
- Always available → `always`

### Task 12: Database Migration (optional)

Since we're ignoring old `enabled` flags and using defaults for missing sub-options, a migration is optional. However, for cleanliness, consider:

**File: `migrations/0XX_mcp_config_defaults.up.sql`**

```sql
-- Set default sub-option values for existing configs (optional cleanup)
UPDATE engine_mcp_config
SET tool_groups = tool_groups || '{
  "allowOntologyMaintenance": true,
  "addQueryTools": true,
  "addOntologyMaintenance": true
}'::jsonb
WHERE NOT (tool_groups ? 'allowOntologyMaintenance')
   OR NOT (tool_groups ? 'addQueryTools')
   OR NOT (tool_groups ? 'addOntologyMaintenance');
```

**Note:** The backend should handle missing keys with defaults, so this migration is purely for data consistency.

---

## Testing Plan

### Unit Tests

1. **`pkg/services/mcp_tool_loadouts_test.go`**
   - Test `ComputeUserTools()` returns correct tool set
   - Test `ComputeDeveloperTools()` returns correct tool set
   - Test `ComputeAgentTools()` returns correct tool set

2. **`pkg/mcp/tools/access_test.go`**
   - Test tool access with JWT role `user` → only user tools accessible
   - Test tool access with JWT role `admin` → all developer tools accessible
   - Test tool access with JWT role `data` → all developer tools accessible
   - Test tool access with agent API key → only agent tools accessible

3. **`pkg/services/mcp_config_test.go`**
   - Test `buildResponse()` includes all three tool lists
   - Test `allowOntologyMaintenance` toggle affects user tools

### Integration Tests

1. **MCP Tool Execution**
   - Create test with user JWT → verify can only call user tools
   - Create test with admin JWT → verify can call all developer tools
   - Create test with agent API key → verify can only call agent tools

### Manual Testing

1. Load MCP Server page → verify new UI layout
2. Toggle "Allow Usage to Improve Ontology" → verify user tools list updates
3. Connect MCP client with user JWT → verify limited tools available
4. Connect MCP client with admin JWT → verify full tools available
5. Connect MCP client with agent API key → verify agent tools only

---

## Files to Modify

### Backend
| File | Changes |
|------|---------|
| `pkg/mcp/tools/access.go` | Add role-based tool filtering |
| `pkg/services/mcp_tool_loadouts.go` | Add `ComputeUserTools`, `ComputeDeveloperTools`, `ComputeAgentTools` |
| `pkg/services/mcp_config.go` | Update response to include per-role tool lists |
| `pkg/handlers/mcp_config.go` | Simplify update handler |
| `pkg/models/mcp_config.go` | Simplify ToolGroupConfig |
| `pkg/services/mcp_tools_registry.go` | Update tool group assignments |
| `migrations/0XX_*.sql` | Set default allowOntologyMaintenance |

### Frontend
| File | Changes |
|------|---------|
| `ui/src/pages/MCPServerPage.tsx` | Major restructure |
| `ui/src/components/mcp/UserToolsSection.tsx` | New component |
| `ui/src/components/mcp/DeveloperToolsSection.tsx` | New component |
| `ui/src/components/mcp/AgentToolsSection.tsx` | New or updated component |
| `ui/src/types/mcp.ts` | Add per-role tool arrays |
| `ui/src/constants/mcpToolMetadata.tsx` | Update/simplify |

### Tests
| File | Changes |
|------|---------|
| `pkg/services/mcp_tool_loadouts_test.go` | New tests for role-based computation |
| `pkg/mcp/tools/access_test.go` | New tests for role-based access |
| `pkg/services/mcp_config_test.go` | Update for new response format |

---

## Migration Notes

- **Old `enabled` flags are ignored** - All projects immediately get role-based tool filtering
- Sub-options are preserved: `allowOntologyMaintenance`, `addQueryTools`, `addOntologyMaintenance`
- Default values for sub-options: all `true` (maximally permissive)
- No breaking changes to MCP protocol - tools are filtered server-side
- MCP clients will see different tools based on their JWT role automatically
- No migration script needed - old configs work, just with new interpretation

---

## Design Decisions

The following decisions were made during planning:

1. **`allowOntologyMaintenance` scope**: Only affects User Tools. Developers always have full ontology access regardless of this toggle.

2. **Developer Tools sub-options**: Keep the existing sub-options ("Add Query Tools" and "Add Ontology Maintenance") to allow admins to control which developer tools are available.

3. **Migration strategy**: Ignore old settings. All projects get the new role-based behavior immediately. Old `enabled` flags on tool groups are ignored.
