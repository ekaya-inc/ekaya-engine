# PLAN: Show Enabled Tools in MCP Server Screen

## Overview

Add a "Tools Enabled" section at the bottom of the "Your MCP Server URL" card that displays a table of currently enabled MCP tools. This provides immediate visibility into which tools are exposed based on the current toggle configuration, serving as both a user feature and a visual verification that the state machine is producing correct output.

## Problem Statement

1. Users cannot see at-a-glance which MCP tools are enabled without connecting an MCP client
2. The state machine (`pkg/services/mcp_state.go`) produces output state, but there's no visual confirmation that the rules are being applied correctly
3. Tool visibility logic is duplicated between the state machine and the tool filter (`pkg/mcp/tools/developer.go:NewToolFilter`)
4. **Bug**: When rotating the Agent API Key, the Bearer token in the copyable JSON configuration is not updated

## Goals

1. Display a table showing tool name and one-line description for each enabled tool
2. Update the table dynamically as toggles change (no page refresh needed)
3. Couple the tool list with the state machine output to prevent translation drift
4. Fix the Agent Key rotation bug

## Architecture

### Tool Registry

Create a centralized tool registry that defines all tools with their metadata. This becomes the single source of truth for:
- Tool name (e.g., `get_schema`, `execute_approved_query`)
- Tool description (one-liner for display)
- Which tool group controls visibility (developer, approved_queries, agent_tools)
- Sub-option requirements (e.g., `execute` requires `enableExecute`)

**Location**: `pkg/services/mcp_tools_registry.go` (new file)

```go
// ToolDefinition describes an MCP tool for display purposes.
type ToolDefinition struct {
    Name        string
    Description string
    ToolGroup   string   // "developer", "approved_queries", "agent_tools"
    SubOption   string   // Optional: "enableExecute" for the execute tool
}

// ToolRegistry returns all tool definitions.
var ToolRegistry = []ToolDefinition{
    // Developer-only tools (require Developer Tools enabled)
    {Name: "echo", Description: "Echo back input message for testing", ToolGroup: "developer"},
    {Name: "execute", Description: "Execute DDL/DML statements", ToolGroup: "developer", SubOption: "enableExecute"},
    {Name: "get_schema", Description: "Get database schema with entity semantics", ToolGroup: "developer"},

    // Business user tools (approved_queries group)
    // These read-only query tools enable ad-hoc analysis when pre-approved queries don't match
    {Name: "query", Description: "Execute read-only SQL SELECT statements", ToolGroup: "approved_queries"},
    {Name: "sample", Description: "Quick data preview from a table", ToolGroup: "approved_queries"},
    {Name: "validate", Description: "Check SQL syntax without executing", ToolGroup: "approved_queries"},
    {Name: "get_ontology", Description: "Get business ontology for query generation", ToolGroup: "approved_queries"},
    {Name: "get_glossary", Description: "Get business glossary terms", ToolGroup: "approved_queries"},
    {Name: "list_approved_queries", Description: "List pre-approved SQL queries", ToolGroup: "approved_queries"},
    {Name: "execute_approved_query", Description: "Execute a pre-approved query by ID", ToolGroup: "approved_queries"},

    // Health is always available
    {Name: "health", Description: "Server health check", ToolGroup: "always"},
}
```

### Integrate with State Machine

Extend `MCPStateResult` to include enabled tools:

**Location**: `pkg/services/mcp_state.go`

```go
type MCPStateResult struct {
    State        map[string]*models.ToolGroupConfig
    EnabledTools []ToolDefinition  // NEW: Tools enabled based on state
    Error        *MCPStateError
}
```

The `Apply()` method already knows the final state—have it compute which tools are enabled based on that state. This keeps the logic in one place and prevents drift.

### API Response

Extend `MCPConfigResponse` to include enabled tools:

**Location**: `pkg/services/mcp_config.go`

```go
type MCPConfigResponse struct {
    ServerURL    string                             `json:"serverUrl"`
    ToolGroups   map[string]*models.ToolGroupConfig `json:"toolGroups"`
    EnabledTools []EnabledToolInfo                  `json:"enabledTools"`  // NEW
}

type EnabledToolInfo struct {
    Name        string `json:"name"`
    Description string `json:"description"`
}
```

### Frontend Component

Create a new component to display the tools table:

**Location**: `ui/src/components/mcp/MCPEnabledTools.tsx`

```tsx
interface MCPEnabledToolsProps {
    tools: Array<{name: string; description: string}>;
}

export default function MCPEnabledTools({ tools }: MCPEnabledToolsProps) {
    if (tools.length === 0) {
        return (
            <div className="text-sm text-text-secondary italic">
                No tools enabled. Enable a tool group above.
            </div>
        );
    }

    return (
        <div className="mt-4 border-t border-border-light pt-4">
            <h4 className="text-sm font-medium text-text-primary mb-2">Tools Enabled</h4>
            <table className="w-full text-sm">
                <tbody>
                    {tools.map(tool => (
                        <tr key={tool.name} className="border-b border-border-light last:border-0">
                            <td className="py-1.5 font-mono text-text-primary">{tool.name}</td>
                            <td className="py-1.5 text-text-secondary">{tool.description}</td>
                        </tr>
                    ))}
                </tbody>
            </table>
        </div>
    );
}
```

### Update MCPServerURL Component

**Location**: `ui/src/components/mcp/MCPServerURL.tsx`

Add `enabledTools` prop and render `MCPEnabledTools` at the bottom of the card.

## Bug Fix: Agent Key Rotation

### Problem

In `AgentAPIKeyDisplay.tsx`, when the key is rotated via `handleRegenerate()`, the local `key` state is updated. However, `MCPServerURL.tsx` receives `agentApiKey` as a prop from `MCPServerPage.tsx`, which fetches it separately. These are not synchronized.

### Current Flow

1. `MCPServerPage.tsx` fetches agent key via `useEffect` when `isAgentToolsEnabled` changes
2. Passes `agentApiKey` to `MCPServerURL.tsx` for the JSON config display
3. `AgentAPIKeyDisplay.tsx` has its own key state and rotation logic
4. When key is rotated in `AgentAPIKeyDisplay`, `MCPServerPage.tsx` doesn't know about it

### Fix

Option A (Recommended): Lift key state to `MCPServerPage.tsx` and pass a callback to `AgentAPIKeyDisplay`:

```tsx
// MCPServerPage.tsx
const [agentApiKey, setAgentApiKey] = useState<string>('');

// Pass setter to AgentAPIKeyDisplay
<AgentAPIKeyDisplay
    projectId={pid!}
    onKeyChange={setAgentApiKey}  // NEW
/>
```

```tsx
// AgentAPIKeyDisplay.tsx
interface AgentAPIKeyDisplayProps {
    projectId: string;
    onKeyChange?: (key: string) => void;  // NEW
}

const handleRegenerate = async () => {
    // ... existing code ...
    if (response.success && response.data) {
        setKey(response.data.key);
        setMasked(false);
        onKeyChange?.(response.data.key);  // Notify parent
        // ...
    }
};
```

Option B: Use React context or state management for agent key (heavier solution).

## Implementation Steps

1. [x] **Create tool registry** (`pkg/services/mcp_tools_registry.go`) ✅ COMPLETED
   - Created `ToolDefinition` struct with Name, Description, ToolGroup, SubOption fields
   - Created `ToolRegistry` slice with all 11 tools using CURRENT tool groupings (query/sample/validate in developer group)
   - Implemented `GetEnabledTools(state map[string]*models.ToolGroupConfig) []ToolDefinition` function
   - Added `agentAllowedTools` map for agent mode filtering
   - Created comprehensive test suite in `mcp_tools_registry_test.go` (13 tests passing)
   - Uses existing constants from `mcp_state.go`: `ToolGroupDeveloper`, `ToolGroupApprovedQueries`, `ToolGroupAgentTools`
   - **Key behavior**: agent_tools mode overrides all other settings; forceMode hides developer tools
   - **Files created**: `pkg/services/mcp_tools_registry.go`, `pkg/services/mcp_tools_registry_test.go`
   - **NOTE for next session**: The registry currently reflects CURRENT tool groupings. Step 2 will update both the registry AND the MCP tool filter to move query/sample/validate to approved_queries group.

2. [ ] **Move read-only query tools to approved_queries group** (`pkg/mcp/tools/developer.go`, `pkg/mcp/tools/queries.go`)
   - Move `query`, `sample`, `validate` from `developerToolNames` to `approvedQueriesToolNames` (or create a new `businessUserToolNames` map)
   - Update `checkDeveloperEnabled` → `checkApprovedQueriesEnabled` for these tools (they should check approved_queries group, not developer)
   - Update `filterTools()` to show these tools when approved_queries is enabled
   - **Rationale**: These are read-only tools that enable business users to answer ad-hoc questions when pre-approved queries don't match. They align with the "Business User Tools" intent: "The MCP Client will use the Pre-Approved Queries if they match the Business User's request and fall back on using the Ontology to craft new SQL queries to answer their questions."
   - Update tests in `pkg/mcp/tools/developer_filter_test.go`

3. **Extend state machine** (`pkg/services/mcp_state.go`)
   - Add `EnabledTools` field to `MCPStateResult`
   - Update `Apply()` to compute enabled tools from final state
   - Add tests for tool visibility based on state

3. **Update API response** (`pkg/services/mcp_config.go`)
   - Add `EnabledTools` to `MCPConfigResponse`
   - Update `buildResponse()` to include enabled tools from state result

4. **Create frontend component** (`ui/src/components/mcp/MCPEnabledTools.tsx`)
   - Simple table display component

5. **Update MCPServerURL** (`ui/src/components/mcp/MCPServerURL.tsx`)
   - Add `enabledTools` prop
   - Render `MCPEnabledTools` inside the card

6. **Update MCPServerPage** (`ui/src/pages/MCPServerPage.tsx`)
   - Pass `enabledTools` from config response to `MCPServerURL`

7. **Fix agent key rotation bug** (`ui/src/components/mcp/AgentAPIKeyDisplay.tsx`, `ui/src/pages/MCPServerPage.tsx`)
   - Add `onKeyChange` callback prop
   - Lift key state to parent

8. **Update types** (`ui/src/types/index.ts`)
   - Add `EnabledToolInfo` type
   - Update `MCPConfigResponse` type

## Testing

1. **Unit tests for tool registry**
   - Verify each tool group state produces correct tools
   - Test sub-option requirements (e.g., execute requires enableExecute)
   - Test mutual exclusivity (agent_tools disables others)

2. **State machine tests**
   - Extend existing tests to verify `EnabledTools` in result

3. **Manual testing**
   - Toggle each tool group, verify tools table updates
   - Enable agent_tools, verify only agent-allowed tools shown
   - Rotate agent key, verify JSON config updates immediately

## Files to Modify

### Backend
- `pkg/services/mcp_tools_registry.go` (new)
- `pkg/mcp/tools/developer.go` (step 2: move query/sample/validate to approved_queries)
- `pkg/mcp/tools/queries.go` (step 2: add authorization checks for new tools)
- `pkg/mcp/tools/developer_filter_test.go` (step 2: update filter tests)
- `pkg/services/mcp_state.go`
- `pkg/services/mcp_state_test.go`
- `pkg/services/mcp_config.go`
- `pkg/services/mcp_config_test.go`

### Frontend
- `ui/src/components/mcp/MCPEnabledTools.tsx` (new)
- `ui/src/components/mcp/MCPServerURL.tsx`
- `ui/src/components/mcp/AgentAPIKeyDisplay.tsx`
- `ui/src/pages/MCPServerPage.tsx`
- `ui/src/types/index.ts`

## Tool Visibility Rules

### After Step 2 (New Rules)

| Tool | Tool Group | Additional Requirements |
|------|------------|------------------------|
| echo | developer OR agent_tools (for agents) | - |
| execute | developer | `enableExecute` sub-option |
| get_schema | developer | - |
| query | approved_queries | - |
| sample | approved_queries | - |
| validate | approved_queries | - |
| get_ontology | approved_queries | - |
| get_glossary | approved_queries | - |
| list_approved_queries | approved_queries OR agent_tools (for agents) | - |
| execute_approved_query | approved_queries OR agent_tools (for agents) | - |
| health | always | - |

**Business User Tools rationale**: `query`, `sample`, and `validate` are read-only tools that enable business users to answer ad-hoc questions when pre-approved queries don't match their request. Combined with `get_ontology` and `get_glossary`, the MCP client can craft SQL queries using semantic context. This offers flexibility while maintaining the safety of read-only access.

When `agent_tools` is enabled (for agent authentication), only `echo`, `list_approved_queries`, and `execute_approved_query` are available.

When `forceMode` is enabled on `approved_queries`, developer tools are hidden.

### Before Step 2 (Current Rules)

| Tool | Tool Group | Additional Requirements |
|------|------------|------------------------|
| echo | developer OR agent_tools (for agents) | - |
| query | developer | - |
| sample | developer | - |
| validate | developer | - |
| execute | developer | `enableExecute` sub-option |
| get_schema | developer | - |
| get_ontology | approved_queries | - |
| get_glossary | approved_queries | - |
| list_approved_queries | approved_queries OR agent_tools (for agents) | - |
| execute_approved_query | approved_queries OR agent_tools (for agents) | - |
| health | always | - |
