// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// MCPToolDeps contains dependencies for MCP tools.
// This includes developer tools, business user tools, and approved query tools.
type MCPToolDeps struct {
	DB                *database.DB
	MCPConfigService  services.MCPConfigService
	DatasourceService services.DatasourceService
	SchemaService     services.SchemaService
	ProjectService    services.ProjectService
	AdapterFactory    datasource.DatasourceAdapterFactory
	Logger            *zap.Logger
}

const developerToolGroup = "developer"

// getOptionalString extracts an optional string argument from the request.
func getOptionalString(req mcp.CallToolRequest, key string) string {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return ""
	}
	val, ok := args[key].(string)
	if !ok {
		return ""
	}
	return val
}

// getOptionalFloat extracts an optional float argument from the request.
func getOptionalFloat(req mcp.CallToolRequest, key string) (float64, bool) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return 0, false
	}
	val, ok := args[key].(float64)
	return val, ok
}

// truncateSQL truncates SQL for logging, adding ellipsis if truncated.
func truncateSQL(sql string, maxLen int) string {
	if len(sql) <= maxLen {
		return sql
	}
	return sql[:maxLen] + "..."
}

// Filter maps are generated from ToolRegistry to prevent drift.
// These are populated at init time from the single source of truth.
var (
	// developerToolNames lists all tools in the developer group.
	developerToolNames map[string]bool

	// businessUserToolNames lists read-only query tools that are part of the approved_queries group.
	// These tools enable business users to answer ad-hoc questions when pre-approved queries don't match.
	businessUserToolNames map[string]bool

	// ontologyToolNames lists all ontology tools (grouped with approved_queries visibility).
	// This includes get_glossary since business glossary is part of semantic context.
	ontologyToolNames map[string]bool

	// agentToolNames lists tools available to agents when agent_tools is enabled.
	// Agents can access echo for testing and approved_queries tools for data access.
	agentToolNames map[string]bool
)

func init() {
	// Build filter maps from ToolRegistry to ensure consistency
	developerToolNames = buildToolNameMap(services.ToolGroupDeveloper)
	businessUserToolNames = buildBusinessUserToolMap()
	ontologyToolNames = buildOntologyToolMap()
	agentToolNames = buildAgentToolMap()
}

// buildToolNameMap creates a map of tool names for a given tool group from the registry.
func buildToolNameMap(group string) map[string]bool {
	m := make(map[string]bool)
	for _, t := range services.ToolRegistry {
		if t.ToolGroup == group {
			m[t.Name] = true
		}
	}
	return m
}

// buildBusinessUserToolMap returns tools from approved_queries group that are business user tools.
// These are the read-only query tools (query, sample, validate) as opposed to
// the approved query tools (list_approved_queries, execute_approved_query) or ontology tools.
func buildBusinessUserToolMap() map[string]bool {
	return map[string]bool{
		"query":    true,
		"sample":   true,
		"validate": true,
	}
}

// buildOntologyToolMap returns ontology-related tools from the approved_queries group.
// This includes glossary tools since business glossary is part of semantic context.
func buildOntologyToolMap() map[string]bool {
	return map[string]bool{
		"get_ontology":     true,
		"list_glossary":    true,
		"get_glossary_sql": true,
	}
}

// buildAgentToolMap returns tools available to agents (from services.agentAllowedTools).
// Note: health is handled separately in filterAgentTools.
func buildAgentToolMap() map[string]bool {
	return map[string]bool{
		"echo":                   true,
		"list_approved_queries":  true,
		"execute_approved_query": true,
	}
}

// RegisterMCPTools registers all MCP tools (developer, business user, and query tools).
// Tool visibility is controlled by the tool filter based on project configuration.
func RegisterMCPTools(s *server.MCPServer, deps *MCPToolDeps) {
	registerEchoTool(s, deps)
	registerQueryTool(s, deps)
	registerSampleTool(s, deps)
	registerExecuteTool(s, deps)
	registerValidateTool(s, deps)
}

// NewToolFilter creates a ToolFilterFunc that filters tools based on MCP configuration.
// It uses GetEnabledTools from the services package to ensure consistency with the UI.
// For agent authentication, it restricts access to only agent-allowed tools.
func NewToolFilter(deps *MCPToolDeps) func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
	return func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
		// Get claims from context
		claims, ok := auth.GetClaims(ctx)
		if !ok {
			deps.Logger.Debug("Tool filter: no auth context, returning only health")
			return filterToHealthOnly(tools)
		}

		projectID, err := uuid.Parse(claims.ProjectID)
		if err != nil {
			deps.Logger.Error("Tool filter: invalid project ID in claims",
				zap.String("project_id", claims.ProjectID),
				zap.Error(err))
			return filterToHealthOnly(tools)
		}

		// Acquire tenant scope for database access
		scope, err := deps.DB.WithTenant(ctx, projectID)
		if err != nil {
			deps.Logger.Error("Tool filter: failed to acquire tenant scope",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return filterToHealthOnly(tools)
		}
		defer scope.Close()

		// Set tenant context for the config query
		tenantCtx := database.SetTenantScope(ctx, scope)

		// Check if this is agent authentication (Subject = "agent" set by MCP auth middleware)
		isAgent := claims.Subject == "agent"

		if isAgent {
			// Agent authentication - only allow agent tools when agent_tools is enabled
			agentEnabled, err := deps.MCPConfigService.IsToolGroupEnabled(tenantCtx, projectID, services.ToolGroupAgentTools)
			if err != nil {
				deps.Logger.Error("Tool filter: failed to check agent_tools config",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
				return filterAgentTools(tools, false)
			}

			deps.Logger.Debug("Tool filter: agent authentication, filtering to agent tools only",
				zap.String("project_id", projectID.String()),
				zap.Bool("agent_tools_enabled", agentEnabled))

			return filterAgentTools(tools, agentEnabled)
		}

		// User authentication - use GetEnabledTools for consistent filtering with UI
		state, err := deps.MCPConfigService.GetToolGroupsState(tenantCtx, projectID)
		if err != nil {
			deps.Logger.Error("Tool filter: failed to get tool groups state",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return filterToHealthOnly(tools)
		}

		// Get enabled tools using the same function as the UI
		enabledToolDefs := services.GetEnabledTools(state)

		// Build a set of enabled tool names
		enabledNames := make(map[string]bool, len(enabledToolDefs))
		for _, td := range enabledToolDefs {
			enabledNames[td.Name] = true
		}

		deps.Logger.Debug("Tool filter: filtering based on GetEnabledTools",
			zap.String("project_id", projectID.String()),
			zap.Int("enabled_tool_count", len(enabledNames)))

		// Filter MCP tools to only include enabled ones
		return filterByEnabledNames(tools, enabledNames)
	}
}

// filterTools filters tools based on visibility flags for each tool group.
func filterTools(tools []mcp.Tool, showDeveloper, showExecute, showApprovedQueries bool) []mcp.Tool {
	filtered := make([]mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		// Check developer tools
		if developerToolNames[tool.Name] {
			if !showDeveloper {
				continue
			}
			if tool.Name == "execute" && !showExecute {
				continue
			}
		}

		// Check schema tools - tied to developer tools visibility
		if SchemaToolNames[tool.Name] && !showDeveloper {
			continue
		}

		// Check business user tools (query, sample, validate) - tied to approved_queries visibility
		if businessUserToolNames[tool.Name] && !showApprovedQueries {
			continue
		}

		// Check approved_queries tools
		if approvedQueriesToolNames[tool.Name] && !showApprovedQueries {
			continue
		}

		// Check ontology tools - tied to approved_queries visibility
		if ontologyToolNames[tool.Name] && !showApprovedQueries {
			continue
		}

		filtered = append(filtered, tool)
	}
	return filtered
}

// filterAgentTools filters tools for agent authentication.
// When agent_tools is enabled, only approved_queries tools (list_approved_queries, execute_approved_query) are allowed.
// When disabled, no tools are available (except health which is always available).
func filterAgentTools(tools []mcp.Tool, agentToolsEnabled bool) []mcp.Tool {
	filtered := make([]mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		// Health is always available
		if tool.Name == "health" {
			filtered = append(filtered, tool)
			continue
		}

		// When agent_tools disabled, filter out everything except health
		if !agentToolsEnabled {
			continue
		}

		// When agent_tools enabled, only allow approved_queries tools
		if agentToolNames[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// filterToHealthOnly returns only the health tool from the list.
// Used when authentication fails or config cannot be loaded.
func filterToHealthOnly(tools []mcp.Tool) []mcp.Tool {
	filtered := make([]mcp.Tool, 0, 1)
	for _, tool := range tools {
		if tool.Name == "health" {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// filterByEnabledNames filters tools to only include those in the enabled names set.
// This is used with GetEnabledTools to ensure MCP and UI show the same tools.
func filterByEnabledNames(tools []mcp.Tool, enabledNames map[string]bool) []mcp.Tool {
	filtered := make([]mcp.Tool, 0, len(enabledNames))
	for _, tool := range tools {
		if enabledNames[tool.Name] {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// filterOutExecuteTool removes only the execute tool from the list.
func filterOutExecuteTool(tools []mcp.Tool) []mcp.Tool {
	filtered := make([]mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool.Name != "execute" {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

// checkDeveloperEnabled verifies the developer tool group is enabled for the project.
// Returns the project ID and a tenant-scoped context if enabled, or an error if not.
func checkDeveloperEnabled(ctx context.Context, deps *MCPToolDeps) (uuid.UUID, context.Context, func(), error) {
	// Get claims from context
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return uuid.Nil, nil, nil, fmt.Errorf("authentication required")
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("invalid project ID: %w", err)
	}

	// Acquire connection with tenant scope
	scope, err := deps.DB.WithTenant(ctx, projectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("failed to acquire database connection: %w", err)
	}

	// Set tenant context for the query
	tenantCtx := database.SetTenantScope(ctx, scope)

	// Check if developer tool group is enabled
	enabled, err := deps.MCPConfigService.IsToolGroupEnabled(tenantCtx, projectID, developerToolGroup)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to check developer tool group",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool group configuration: %w", err)
	}

	if !enabled {
		scope.Close()
		return uuid.Nil, nil, nil, fmt.Errorf("developer tools are not enabled for this project")
	}

	return projectID, tenantCtx, func() { scope.Close() }, nil
}

// checkExecuteEnabled verifies the execute tool is enabled for the project.
// This checks both the developer tool group and the EnableExecute sub-option.
func checkExecuteEnabled(ctx context.Context, deps *MCPToolDeps) (uuid.UUID, context.Context, func(), error) {
	// First check if developer tools are enabled
	projectID, tenantCtx, cleanup, err := checkDeveloperEnabled(ctx, deps)
	if err != nil {
		return uuid.Nil, nil, nil, err
	}

	// Check if EnableExecute is set
	config, err := deps.MCPConfigService.GetToolGroupConfig(tenantCtx, projectID, developerToolGroup)
	if err != nil {
		cleanup()
		deps.Logger.Error("Failed to get developer tool config",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	if config == nil || !config.EnableExecute {
		cleanup()
		return uuid.Nil, nil, nil, fmt.Errorf("execute tool is not enabled for this project")
	}

	return projectID, tenantCtx, cleanup, nil
}

// checkBusinessUserToolsEnabled verifies the caller is authorized to use business user tools (query, sample, validate).
// Authorization is granted if approved_queries tool group is enabled.
// These are read-only query tools that enable business users to answer ad-hoc questions.
// Returns the project ID and a tenant-scoped context if authorized, or an error if not.
func checkBusinessUserToolsEnabled(ctx context.Context, deps *MCPToolDeps) (uuid.UUID, context.Context, func(), error) {
	// Get claims from context
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return uuid.Nil, nil, nil, fmt.Errorf("authentication required")
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("invalid project ID: %w", err)
	}

	// Acquire connection with tenant scope
	scope, err := deps.DB.WithTenant(ctx, projectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("failed to acquire database connection: %w", err)
	}

	// Set tenant context for the query
	tenantCtx := database.SetTenantScope(ctx, scope)

	// Check if approved_queries tool group is enabled
	enabled, err := deps.MCPConfigService.IsToolGroupEnabled(tenantCtx, projectID, "approved_queries")
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to check approved_queries tool group",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool group configuration: %w", err)
	}

	if !enabled {
		scope.Close()
		return uuid.Nil, nil, nil, fmt.Errorf("business user tools are not enabled for this project")
	}

	return projectID, tenantCtx, func() { scope.Close() }, nil
}

// checkEchoEnabled verifies the caller is authorized to use the echo tool.
// Authorization is granted if:
//   - developer tool group is enabled (for user authentication), OR
//   - agent_tools is enabled AND caller is an agent (claims.Subject == "agent")
//
// Returns the project ID and a tenant-scoped context if authorized, or an error if not.
func checkEchoEnabled(ctx context.Context, deps *MCPToolDeps) (uuid.UUID, context.Context, func(), error) {
	// Get claims from context
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return uuid.Nil, nil, nil, fmt.Errorf("authentication required")
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("invalid project ID: %w", err)
	}

	// Acquire connection with tenant scope
	scope, err := deps.DB.WithTenant(ctx, projectID)
	if err != nil {
		return uuid.Nil, nil, nil, fmt.Errorf("failed to acquire database connection: %w", err)
	}

	// Set tenant context for the query
	tenantCtx := database.SetTenantScope(ctx, scope)

	// Check if caller is an agent (API key authentication)
	isAgent := claims.Subject == "agent"

	// Check if developer tool group is enabled (for users)
	developerEnabled, err := deps.MCPConfigService.IsToolGroupEnabled(tenantCtx, projectID, developerToolGroup)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to check developer tool group",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool group configuration: %w", err)
	}

	// If developer tools is enabled, allow access (for both users and agents)
	if developerEnabled {
		return projectID, tenantCtx, func() { scope.Close() }, nil
	}

	// For agents, also check if agent_tools is enabled
	if isAgent {
		agentToolsEnabled, err := deps.MCPConfigService.IsToolGroupEnabled(tenantCtx, projectID, services.ToolGroupAgentTools)
		if err != nil {
			scope.Close()
			deps.Logger.Error("Failed to check agent_tools tool group",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool group configuration: %w", err)
		}

		if agentToolsEnabled {
			return projectID, tenantCtx, func() { scope.Close() }, nil
		}
	}

	// Neither developer tools nor agent_tools (for agents) is enabled
	scope.Close()
	return uuid.Nil, nil, nil, fmt.Errorf("echo tool is not enabled for this project")
}

// registerEchoTool adds a simple echo tool for testing the developer tool group.
// This tool verifies that authentication and tool group configuration work correctly.
func registerEchoTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"echo",
		mcp.WithDescription("Echo back the input message (developer tool for testing)"),
		mcp.WithString(
			"message",
			mcp.Required(),
			mcp.Description("The message to echo back"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check if echo tool is enabled (developer tools OR agent_tools for agents)
		_, _, cleanup, err := checkEchoEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get the message argument
		message, err := req.RequireString("message")
		if err != nil {
			return nil, err
		}

		result, err := json.Marshal(map[string]string{
			"echo": message,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

// getDefaultDatasourceConfig returns the default datasource type and config for a project.
func getDefaultDatasourceConfig(ctx context.Context, deps *MCPToolDeps, projectID uuid.UUID) (string, map[string]any, error) {
	dsID, err := deps.ProjectService.GetDefaultDatasourceID(ctx, projectID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get default datasource: %w", err)
	}
	if dsID == uuid.Nil {
		return "", nil, fmt.Errorf("no default datasource configured for project")
	}

	ds, err := deps.DatasourceService.Get(ctx, projectID, dsID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	return ds.DatasourceType, ds.Config, nil
}

// registerQueryTool adds the query tool for executing read-only SQL.
func registerQueryTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"query",
		mcp.WithDescription("Execute read-only SQL SELECT statements for data analysis."),
		mcp.WithString(
			"sql",
			mcp.Required(),
			mcp.Description("SQL SELECT statement to execute"),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Max rows to return (default: 100, max: 1000)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkBusinessUserToolsEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get SQL parameter
		sql, err := req.RequireString("sql")
		if err != nil {
			return nil, err
		}

		// Validate it's a SELECT statement
		sqlUpper := strings.ToUpper(strings.TrimSpace(sql))
		if !strings.HasPrefix(sqlUpper, "SELECT") && !strings.HasPrefix(sqlUpper, "WITH") {
			return nil, fmt.Errorf("query tool only accepts SELECT statements; use execute tool for DDL/DML")
		}

		// Get limit parameter
		limit := 100
		if limitVal, ok := getOptionalFloat(req, "limit"); ok {
			limit = int(limitVal)
		}
		if limit > 1000 {
			limit = 1000
		}
		if limit < 1 {
			limit = 100
		}

		// Get datasource config and create executor
		dsType, dsConfig, err := getDefaultDatasourceConfig(tenantCtx, deps, projectID)
		if err != nil {
			return nil, err
		}

		// TODO: Extract userID from context and datasourceID from getDefaultDatasourceConfig when step 8 is implemented
		executor, err := deps.AdapterFactory.NewQueryExecutor(tenantCtx, dsType, dsConfig, projectID, uuid.Nil, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create query executor: %w", err)
		}
		defer executor.Close()

		// Execute with limit + 1 to detect truncation
		queryResult, err := executor.ExecuteQuery(tenantCtx, sql, limit+1)
		if err != nil {
			return nil, fmt.Errorf("query execution failed: %w", err)
		}

		// Check if truncated
		truncated := len(queryResult.Rows) > limit
		rows := queryResult.Rows
		if truncated {
			rows = rows[:limit]
		}

		// Extract column names from ColumnInfo for response
		columnNames := make([]string, len(queryResult.Columns))
		for i, col := range queryResult.Columns {
			columnNames[i] = col.Name
		}

		result := struct {
			Columns   []string         `json:"columns"`
			Rows      []map[string]any `json:"rows"`
			RowCount  int              `json:"row_count"`
			Truncated bool             `json:"truncated"`
		}{
			Columns:   columnNames,
			Rows:      rows,
			RowCount:  len(rows),
			Truncated: truncated,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerSampleTool adds the sample tool for quick data preview.
func registerSampleTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"sample",
		mcp.WithDescription("Quick data preview from a table without writing SQL."),
		mcp.WithString(
			"table",
			mcp.Required(),
			mcp.Description("Table name (format: schema.table or just table)"),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Number of rows to return (default: 10, max: 100)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkBusinessUserToolsEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get table parameter
		table, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}

		// Parse schema.table format
		schemaName := "public"
		tableName := table
		if idx := strings.Index(table, "."); idx != -1 {
			schemaName = table[:idx]
			tableName = table[idx+1:]
		}

		// Get limit parameter
		limit := 10
		if limitVal, ok := getOptionalFloat(req, "limit"); ok {
			limit = int(limitVal)
		}
		if limit > 100 {
			limit = 100
		}
		if limit < 1 {
			limit = 10
		}

		// Get datasource config and create executor
		dsType, dsConfig, err := getDefaultDatasourceConfig(tenantCtx, deps, projectID)
		if err != nil {
			return nil, err
		}

		// TODO: Extract userID from context and datasourceID from getDefaultDatasourceConfig when step 8 is implemented
		executor, err := deps.AdapterFactory.NewQueryExecutor(tenantCtx, dsType, dsConfig, projectID, uuid.Nil, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create query executor: %w", err)
		}
		defer executor.Close()

		// Build query with properly sanitized identifiers to prevent SQL injection
		// Use adapter's QuoteIdentifier for database-agnostic quoting
		quotedSchema := executor.QuoteIdentifier(schemaName)
		quotedTable := executor.QuoteIdentifier(tableName)
		sql := fmt.Sprintf(`SELECT * FROM %s.%s LIMIT %d`, quotedSchema, quotedTable, limit)

		// Execute query
		queryResult, err := executor.ExecuteQuery(tenantCtx, sql, 0)
		if err != nil {
			return nil, fmt.Errorf("sample query failed: %w", err)
		}

		// Extract column names from ColumnInfo for response
		columnNames := make([]string, len(queryResult.Columns))
		for i, col := range queryResult.Columns {
			columnNames[i] = col.Name
		}

		result := struct {
			Columns   []string         `json:"columns"`
			Rows      []map[string]any `json:"rows"`
			RowCount  int              `json:"row_count"`
			Truncated bool             `json:"truncated"`
		}{
			Columns:   columnNames,
			Rows:      queryResult.Rows,
			RowCount:  queryResult.RowCount,
			Truncated: false,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerExecuteTool adds the execute tool for DDL/DML statements.
func registerExecuteTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"execute",
		mcp.WithDescription("Execute DDL/DML statements (CREATE, INSERT, UPDATE, DELETE, etc.)"),
		mcp.WithString(
			"sql",
			mcp.Required(),
			mcp.Description("SQL statement to execute"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check if execute tool is specifically enabled (not just developer tools)
		projectID, tenantCtx, cleanup, err := checkExecuteEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get SQL parameter
		sql, err := req.RequireString("sql")
		if err != nil {
			return nil, err
		}

		// Log execution for audit trail
		deps.Logger.Info("Executing DDL/DML statement via MCP",
			zap.String("project_id", projectID.String()),
			zap.String("sql_preview", truncateSQL(sql, 200)),
		)

		// Get datasource config and create executor
		dsType, dsConfig, err := getDefaultDatasourceConfig(tenantCtx, deps, projectID)
		if err != nil {
			return nil, err
		}

		// TODO: Extract userID from context and datasourceID from getDefaultDatasourceConfig when step 8 is implemented
		executor, err := deps.AdapterFactory.NewQueryExecutor(tenantCtx, dsType, dsConfig, projectID, uuid.Nil, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create query executor: %w", err)
		}
		defer executor.Close()

		// Execute statement with timeout to prevent long-running operations
		execCtx, cancel := context.WithTimeout(tenantCtx, 30*time.Second)
		defer cancel()

		execResult, err := executor.Execute(execCtx, sql)
		if err != nil {
			deps.Logger.Error("DDL/DML execution failed",
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
			return nil, fmt.Errorf("execution failed: %w", err)
		}

		deps.Logger.Info("DDL/DML execution completed",
			zap.String("project_id", projectID.String()),
			zap.Int64("rows_affected", execResult.RowsAffected),
		)

		// Build response based on whether rows were returned
		var result any
		if len(execResult.Columns) > 0 {
			// Statement returned rows (RETURNING clause)
			result = struct {
				Columns      []string         `json:"columns"`
				Rows         []map[string]any `json:"rows"`
				RowCount     int              `json:"row_count"`
				RowsAffected int64            `json:"rows_affected"`
			}{
				Columns:      execResult.Columns,
				Rows:         execResult.Rows,
				RowCount:     execResult.RowCount,
				RowsAffected: execResult.RowsAffected,
			}
		} else {
			// No rows returned
			result = struct {
				RowsAffected int64  `json:"rows_affected"`
				Message      string `json:"message"`
			}{
				RowsAffected: execResult.RowsAffected,
				Message:      fmt.Sprintf("%d rows affected", execResult.RowsAffected),
			}
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerValidateTool adds the validate tool for SQL syntax checking.
func registerValidateTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"validate",
		mcp.WithDescription("Check SQL syntax without executing. Uses EXPLAIN for validation. Note: DDL statements (CREATE, ALTER, DROP) cannot be validated this way."),
		mcp.WithString(
			"sql",
			mcp.Required(),
			mcp.Description("SQL statement to validate"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkBusinessUserToolsEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get SQL parameter
		sql, err := req.RequireString("sql")
		if err != nil {
			return nil, err
		}

		// Get datasource config and create executor
		dsType, dsConfig, err := getDefaultDatasourceConfig(tenantCtx, deps, projectID)
		if err != nil {
			return nil, err
		}

		// TODO: Extract userID from context and datasourceID from getDefaultDatasourceConfig when step 8 is implemented
		executor, err := deps.AdapterFactory.NewQueryExecutor(tenantCtx, dsType, dsConfig, projectID, uuid.Nil, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create query executor: %w", err)
		}
		defer executor.Close()

		// Validate SQL
		validationErr := executor.ValidateQuery(tenantCtx, sql)

		var result any
		if validationErr == nil {
			result = struct {
				Valid bool `json:"valid"`
			}{
				Valid: true,
			}
		} else {
			result = struct {
				Valid bool   `json:"valid"`
				Error string `json:"error"`
			}{
				Valid: false,
				Error: validationErr.Error(),
			}
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}
