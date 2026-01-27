# ISSUE: AI Data Liaison tools not appearing after app installation

## Summary

After installing the AI Data Liaison app, the query management tools (`approve_query_suggestion`, `reject_query_suggestion`) do not appear in the MCP server tool list, even though they show in the UI.

## Steps to Reproduce

1. Connect to ekaya-engine MCP server **without** AI Data Liaison installed
2. Suggest a pre-approved query:
   ```
   suggest_approved_query(
     name: "Most Active Hosts",
     description: "Hosts ranked by total billing engagements...",
     sql: "SELECT u.username, COUNT(*) as engagement_count..."
   )
   ```
   - This succeeds and creates a pending suggestion
3. Install the AI Data Liaison app from the UI
4. Refresh MCP tools (tried: `/mcp` reconnect, re-authenticate, restart Claude Code)
5. Attempt to approve the suggestion:
   ```
   approve_query_suggestion(suggestion_id: "...")
   ```

## Expected Behavior

After installing AI Data Liaison and refreshing:
- `approve_query_suggestion` tool should be available
- `reject_query_suggestion` tool should be available
- Other query management admin tools should appear

## Actual Behavior

- Developer tools show 43 tools in UI
- Query management tools appear in UI tool list
- MCP server does NOT expose `approve_query_suggestion` or `reject_query_suggestion`
- Error: `No such tool available: mcp__test_data__approve_query_suggestion`
- Tried multiple remediation steps with no change:
  - Re-authenticate (`/mcp` with auth)
  - Reconnect (`/mcp`)
  - Restart Claude Code entirely
  - Tool count remains at 43

## Investigation Notes

The issue may be in one of:
1. `pkg/mcp/tools/developer.go` - Tool filter not including query management tools
2. `pkg/services/mcp_tool_loadouts.go` - Loadout definitions
3. `pkg/services/mcp_config.go` - `filterAndConvertToolSpecs` or data liaison filtering logic
4. Tool registration - Tools may not be registered in the MCP server

## Related Files

- `pkg/mcp/tools/developer.go` - NewToolFilter
- `pkg/services/mcp_tool_loadouts.go` - LoadoutOntologyMaintenance includes query management tools
- `pkg/services/mcp_config.go` - buildResponse, filterAndConvertToolSpecs
- `pkg/services/mcp_tools_registry.go` - Tool registration

## Root Cause Analysis

The root cause was **#4: Tool registration** - the dev query tools were never registered with the MCP server.

The tools (`approve_query_suggestion`, `reject_query_suggestion`, `list_query_suggestions`, `create_approved_query`, `update_approved_query`, `delete_approved_query`) were:
1. Defined in `ToolRegistry` (pkg/services/mcp_tools_registry.go) - so UI showed them
2. Listed in `DataLiaisonTools` map - so filtering logic was correct
3. Implemented in `pkg/mcp/tools/dev_queries.go` via `RegisterDevQueryTools`

BUT `RegisterDevQueryTools` was never called in `main.go`. This meant the tools appeared in the UI's tool list (derived from ToolRegistry) but were never actually added to the MCP server.

## Fix

Added the missing `RegisterDevQueryTools` call in `main.go` after `RegisterApprovedQueriesTools`:

```go
// Register dev query tools for administrators to manage query suggestions
devQueryToolDeps := &mcptools.DevQueryToolDeps{
    DB:               db,
    MCPConfigService: mcpConfigService,
    ProjectService:   projectService,
    QueryService:     queryService,
    Logger:           logger,
}
mcptools.RegisterDevQueryTools(mcpServer.MCP(), devQueryToolDeps)
```
