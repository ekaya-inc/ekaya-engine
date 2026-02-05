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
	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// MCPToolDeps contains dependencies for MCP tools.
// This includes developer tools, business user tools, and approved query tools.
type MCPToolDeps struct {
	BaseMCPToolDeps
	DatasourceService            services.DatasourceService
	SchemaService                services.SchemaService
	ProjectService               services.ProjectService
	AdapterFactory               datasource.DatasourceAdapterFactory
	SchemaChangeDetectionService services.SchemaChangeDetectionService
	DataChangeDetectionService   services.DataChangeDetectionService
	ChangeReviewService          services.ChangeReviewService
	PendingChangeRepo            repositories.PendingChangeRepository
	InstalledAppService          services.InstalledAppService
	Auditor                      *audit.SecurityAuditor // Optional: for modifying query SIEM logging
}

// dataLiaisonTools is a reference to the shared list in services.DataLiaisonTools.
// These tools require the AI Data Liaison app to be installed.
var dataLiaisonTools = services.DataLiaisonTools

// GetAuditor implements QueryLoggingDeps.
func (d *MCPToolDeps) GetAuditor() *audit.SecurityAuditor { return d.Auditor }

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

// buildAgentToolMap returns tools available to agents (Limited Query loadout).
// Agents get only approved query tools - echo is a developer testing tool.
// Note: health is handled separately in filterAgentTools.
func buildAgentToolMap() map[string]bool {
	return map[string]bool{
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
	registerExplainQueryTool(s, deps)
	registerRefreshSchemaTool(s, deps)
	registerScanDataChangesTool(s, deps)
	registerListPendingChangesTool(s, deps)
	registerApproveChangeTool(s, deps)
	registerRejectChangeTool(s, deps)
	registerApproveAllChangesTool(s, deps)
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

		// Check if project has a datasource configured
		// Without a datasource, tools like query, execute, sample are useless
		hasDatasource := false
		if deps.ProjectService != nil {
			datasourceID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
			if err != nil {
				deps.Logger.Debug("Tool filter: failed to check default datasource",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
				// On error, assume no datasource (safer)
			} else if datasourceID != uuid.Nil {
				hasDatasource = true
			}
		}

		// If no datasource is configured, only expose health tool
		if !hasDatasource {
			deps.Logger.Debug("Tool filter: no datasource configured, returning only health",
				zap.String("project_id", projectID.String()))
			return filterToHealthOnly(tools)
		}

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

		// Check if AI Data Liaison app is installed - if not, remove data liaison tools
		dataLiaisonInstalled := false
		if deps.InstalledAppService != nil {
			installed, err := deps.InstalledAppService.IsInstalled(tenantCtx, projectID, models.AppIDAIDataLiaison)
			if err != nil {
				deps.Logger.Warn("Tool filter: failed to check AI Data Liaison app installation",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
				// On error, default to not installed (safer)
			} else {
				dataLiaisonInstalled = installed
			}
		}

		// Remove data liaison tools if app not installed
		if !dataLiaisonInstalled {
			for toolName := range dataLiaisonTools {
				delete(enabledNames, toolName)
			}
		}

		deps.Logger.Debug("Tool filter: filtering based on GetEnabledTools",
			zap.String("project_id", projectID.String()),
			zap.Int("enabled_tool_count", len(enabledNames)),
			zap.Bool("data_liaison_installed", dataLiaisonInstalled))

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
		// Check if echo tool is enabled using unified access checker
		_, _, cleanup, err := AcquireToolAccess(ctx, deps, "echo")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "query")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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
			return NewErrorResult("invalid_sql_type",
				"query tool only accepts SELECT statements; use execute tool for DDL/DML"), nil
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

		// Track execution time for logging
		startTime := time.Now()

		// Execute with limit + 1 to detect truncation
		queryResult, err := executor.Query(tenantCtx, sql, limit+1)
		executionTimeMs := time.Since(startTime).Milliseconds()
		if err != nil {
			// Check if this is a SQL user error (syntax, missing table, etc.)
			if errResult := NewSQLErrorResult(err); errResult != nil {
				return errResult, nil
			}
			return nil, fmt.Errorf("query execution failed: %w", err)
		}

		// Check if truncated
		truncated := len(queryResult.Rows) > limit
		rows := queryResult.Rows
		if truncated {
			rows = rows[:limit]
		}

		// Log execution to history (best effort - don't fail request if logging fails)
		// Note: QueryID is nil for ad-hoc queries executed via the query tool
		go logQueryExecution(tenantCtx, deps, QueryExecutionLog{
			ProjectID:       projectID,
			QueryID:         uuid.Nil, // Ad-hoc query, no associated approved query
			SQL:             sql,
			SQLType:         "SELECT",
			RowCount:        len(rows),
			ExecutionTimeMs: int(executionTimeMs),
			IsModifying:     false,
			Success:         true,
		})

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
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "sample")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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
		sql := fmt.Sprintf(`SELECT * FROM %s.%s`, quotedSchema, quotedTable)

		// Execute query - adapter handles dialect-specific limit (LIMIT for PostgreSQL, TOP for SQL Server)
		queryResult, err := executor.Query(tenantCtx, sql, limit)
		if err != nil {
			// Check if this is a SQL user error (table not found, etc.)
			if errResult := NewSQLErrorResult(err); errResult != nil {
				return errResult, nil
			}
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
		// Check if execute tool is enabled using unified access checker
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "execute")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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

		// Detect SQL type for logging
		sqlType := services.DetectSQLType(sql)

		// Track execution time for logging
		startTime := time.Now()

		// Execute statement with timeout to prevent long-running operations
		execCtx, cancel := context.WithTimeout(tenantCtx, 30*time.Second)
		defer cancel()

		execResult, err := executor.Execute(execCtx, sql)
		executionTimeMs := time.Since(startTime).Milliseconds()
		if err != nil {
			// Log failed execution
			go logQueryExecution(tenantCtx, deps, QueryExecutionLog{
				ProjectID:       projectID,
				QueryID:         uuid.Nil, // Ad-hoc query, no associated approved query
				SQL:             sql,
				SQLType:         string(sqlType),
				RowCount:        0,
				RowsAffected:    0,
				ExecutionTimeMs: int(executionTimeMs),
				IsModifying:     true,
				Success:         false,
				ErrorMessage:    err.Error(),
			})

			// Check if this is a SQL user error (syntax, constraint, missing table, etc.)
			// These should be returned as JSON errors, not MCP protocol errors
			if errResult := NewSQLErrorResult(err); errResult != nil {
				deps.Logger.Debug("DDL/DML execution failed (user error)",
					zap.String("project_id", projectID.String()),
					zap.String("sql_preview", truncateSQL(sql, 200)),
					zap.Error(err),
				)
				return errResult, nil
			}

			// Server error - return as MCP protocol error
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

		// Log successful execution
		go logQueryExecution(tenantCtx, deps, QueryExecutionLog{
			ProjectID:       projectID,
			QueryID:         uuid.Nil, // Ad-hoc query, no associated approved query
			SQL:             sql,
			SQLType:         string(sqlType),
			RowCount:        execResult.RowCount,
			RowsAffected:    execResult.RowsAffected,
			ExecutionTimeMs: int(executionTimeMs),
			IsModifying:     true,
			Success:         true,
		})

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
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "validate")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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

// registerExplainQueryTool adds the explain_query tool for query performance analysis.
func registerExplainQueryTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"explain_query",
		mcp.WithDescription(
			"Analyze SQL query performance using EXPLAIN ANALYZE. "+
				"Returns execution plan, timing information, and optimization hints. "+
				"Note: This executes the query to gather actual performance data.",
		),
		mcp.WithString(
			"sql",
			mcp.Required(),
			mcp.Description("SQL query to analyze (typically a SELECT statement)"),
		),
		mcp.WithReadOnlyHintAnnotation(false), // EXPLAIN ANALYZE executes the query
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "explain_query")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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

		executor, err := deps.AdapterFactory.NewQueryExecutor(tenantCtx, dsType, dsConfig, projectID, uuid.Nil, "")
		if err != nil {
			return nil, fmt.Errorf("failed to create query executor: %w", err)
		}
		defer executor.Close()

		// Execute EXPLAIN ANALYZE
		explainResult, err := executor.ExplainQuery(tenantCtx, sql)
		if err != nil {
			// Check if this is a SQL user error (syntax, missing table, etc.)
			if errResult := NewSQLErrorResult(err); errResult != nil {
				return errResult, nil
			}
			return nil, fmt.Errorf("EXPLAIN ANALYZE failed: %w", err)
		}

		// Format response
		result := struct {
			Plan             string   `json:"plan"`
			ExecutionTimeMs  float64  `json:"execution_time_ms"`
			PlanningTimeMs   float64  `json:"planning_time_ms"`
			PerformanceHints []string `json:"performance_hints"`
		}{
			Plan:             explainResult.Plan,
			ExecutionTimeMs:  explainResult.ExecutionTimeMs,
			PlanningTimeMs:   explainResult.PlanningTimeMs,
			PerformanceHints: explainResult.PerformanceHints,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// getOptionalBoolWithDefaultDev extracts an optional boolean argument with a default value.
func getOptionalBoolWithDefaultDev(req mcp.CallToolRequest, key string, defaultVal bool) bool {
	if args, ok := req.Params.Arguments.(map[string]any); ok {
		if val, ok := args[key].(bool); ok {
			return val
		}
	}
	return defaultVal
}

// registerRefreshSchemaTool adds the refresh_schema tool for syncing schema from datasource.
func registerRefreshSchemaTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"refresh_schema",
		mcp.WithDescription(
			"Refresh schema from datasource and auto-select new tables/columns. "+
				"Use after execute() to make new tables visible to other tools. "+
				"Returns summary: tables added/removed, columns added, relationships discovered.",
		),
		mcp.WithBoolean(
			"auto_select",
			mcp.Description("Automatically select all new tables/columns (default: true)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "refresh_schema")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get default datasource
		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}
		if dsID == uuid.Nil {
			return nil, fmt.Errorf("no default datasource configured for project")
		}

		autoSelect := getOptionalBoolWithDefaultDev(req, "auto_select", true)

		// Refresh schema - autoSelect causes new tables/columns to be marked as selected at creation time
		result, err := deps.SchemaService.RefreshDatasourceSchema(tenantCtx, projectID, dsID, autoSelect)
		if err != nil {
			deps.Logger.Error("Schema refresh failed",
				zap.String("project_id", projectID.String()),
				zap.String("datasource_id", dsID.String()),
				zap.Error(err),
			)
			return nil, fmt.Errorf("schema refresh failed: %w", err)
		}

		// Auto-select was applied if new tables were discovered and autoSelect was true
		autoSelectApplied := autoSelect && len(result.NewTableNames) > 0

		// Get relationships for response (uses enriched response with table/column names)
		relsResp, _ := deps.SchemaService.GetRelationshipsResponse(tenantCtx, projectID, dsID)

		relPairs := make([]map[string]string, 0)
		if relsResp != nil {
			for _, r := range relsResp.Relationships {
				if r.IsApproved != nil && !*r.IsApproved {
					continue // Skip removed relationships
				}
				relPairs = append(relPairs, map[string]string{
					"from": r.SourceTableName + "." + r.SourceColumnName,
					"to":   r.TargetTableName + "." + r.TargetColumnName,
				})
			}
		}

		// Detect changes and queue for review (if change detection service is configured)
		var pendingChangesCreated int
		if deps.SchemaChangeDetectionService != nil {
			changes, err := deps.SchemaChangeDetectionService.DetectChanges(tenantCtx, projectID, result)
			if err != nil {
				// Log but don't fail - schema refresh succeeded
				deps.Logger.Warn("Change detection failed",
					zap.String("project_id", projectID.String()),
					zap.Error(err),
				)
			} else {
				pendingChangesCreated = len(changes)
			}
		}

		deps.Logger.Info("Schema refresh completed via MCP",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", dsID.String()),
			zap.Int("tables_upserted", result.TablesUpserted),
			zap.Int64("tables_deleted", result.TablesDeleted),
			zap.Int("columns_upserted", result.ColumnsUpserted),
			zap.Int("relationships_created", result.RelationshipsCreated),
			zap.Int("pending_changes_created", pendingChangesCreated),
		)

		response := struct {
			TablesAdded           []string            `json:"tables_added"`
			TablesRemoved         []string            `json:"tables_removed"`
			ColumnsAdded          int                 `json:"columns_added"`
			RelationshipsFound    int                 `json:"relationships_found"`
			Relationships         []map[string]string `json:"relationships,omitempty"`
			AutoSelectApplied     bool                `json:"auto_select_applied"`
			PendingChangesCreated int                 `json:"pending_changes_created"`
		}{
			TablesAdded:           result.NewTableNames,
			TablesRemoved:         result.RemovedTableNames,
			ColumnsAdded:          len(result.NewColumns),
			RelationshipsFound:    len(relPairs),
			Relationships:         relPairs,
			AutoSelectApplied:     autoSelectApplied,
			PendingChangesCreated: pendingChangesCreated,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerListPendingChangesTool adds the list_pending_changes tool for viewing pending ontology changes.
func registerListPendingChangesTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"list_pending_changes",
		mcp.WithDescription(
			"List pending ontology changes detected from schema or data analysis. "+
				"Review these changes and approve/reject them to update the ontology.",
		),
		mcp.WithString(
			"status",
			mcp.Description("Filter by status: pending, approved, rejected, auto_applied (default: pending)"),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Max changes to return (default: 50, max: 500)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "list_pending_changes")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Check if pending change repo is configured
		if deps.PendingChangeRepo == nil {
			return nil, fmt.Errorf("pending changes not available: repository not configured")
		}

		// Get parameters
		status := getOptionalString(req, "status")
		if status == "" {
			status = "pending"
		}
		limit := 50
		if limitVal, ok := getOptionalFloat(req, "limit"); ok {
			limit = int(limitVal)
		}
		if limit > 500 {
			limit = 500
		}
		if limit < 1 {
			limit = 50
		}

		// List changes
		changes, err := deps.PendingChangeRepo.List(tenantCtx, projectID, status, limit)
		if err != nil {
			deps.Logger.Error("Failed to list pending changes",
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
			return nil, fmt.Errorf("failed to list pending changes: %w", err)
		}

		// Get counts by status
		counts, err := deps.PendingChangeRepo.CountByStatus(tenantCtx, projectID)
		if err != nil {
			deps.Logger.Warn("Failed to get pending change counts",
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
			// Continue without counts
		}

		response := struct {
			Changes []any          `json:"changes"`
			Count   int            `json:"count"`
			Counts  map[string]int `json:"counts,omitempty"`
		}{
			Changes: make([]any, len(changes)),
			Count:   len(changes),
			Counts:  counts,
		}

		// Convert changes to response format
		for i, c := range changes {
			response.Changes[i] = map[string]any{
				"id":                c.ID.String(),
				"change_type":       c.ChangeType,
				"change_source":     c.ChangeSource,
				"table_name":        c.TableName,
				"column_name":       c.ColumnName,
				"old_value":         c.OldValue,
				"new_value":         c.NewValue,
				"suggested_action":  c.SuggestedAction,
				"suggested_payload": c.SuggestedPayload,
				"status":            c.Status,
				"created_at":        c.CreatedAt,
			}
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerApproveChangeTool adds the approve_change tool for approving pending ontology changes.
func registerApproveChangeTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"approve_change",
		mcp.WithDescription(
			"Approve a pending ontology change and apply it. "+
				"The change will be applied to the ontology with 'mcp' provenance. "+
				"Precedence rules apply: Admin > MCP > Inference changes.",
		),
		mcp.WithString(
			"change_id",
			mcp.Required(),
			mcp.Description("UUID of the pending change to approve"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "approve_change")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Check if pending change repo is configured
		if deps.PendingChangeRepo == nil {
			return nil, fmt.Errorf("pending changes not available: repository not configured")
		}
		if deps.ChangeReviewService == nil {
			return nil, fmt.Errorf("change review service not available")
		}

		// Get change ID
		changeIDStr, err := req.RequireString("change_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", "change_id is required"), nil
		}

		changeID, err := uuid.Parse(changeIDStr)
		if err != nil {
			return NewErrorResult("invalid_parameters", fmt.Sprintf("invalid change_id: %v", err)), nil
		}

		// Verify change belongs to this project
		change, err := deps.PendingChangeRepo.GetByID(tenantCtx, changeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get pending change: %w", err)
		}
		if change == nil {
			return NewErrorResult("not_found", "pending change not found"), nil
		}
		if change.ProjectID != projectID {
			return NewErrorResult("not_found", "pending change not found"), nil
		}

		// Approve the change
		approvedChange, err := deps.ChangeReviewService.ApproveChange(tenantCtx, changeID, models.ProvenanceMCP)
		if err != nil {
			deps.Logger.Error("Failed to approve change",
				zap.String("change_id", changeIDStr),
				zap.Error(err),
			)
			return NewErrorResult("approval_failed", err.Error()), nil
		}

		response := map[string]any{
			"id":               approvedChange.ID.String(),
			"status":           approvedChange.Status,
			"change_type":      approvedChange.ChangeType,
			"table_name":       approvedChange.TableName,
			"column_name":      approvedChange.ColumnName,
			"suggested_action": approvedChange.SuggestedAction,
			"message":          "Change approved and applied successfully",
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerRejectChangeTool adds the reject_change tool for rejecting pending ontology changes.
func registerRejectChangeTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"reject_change",
		mcp.WithDescription(
			"Reject a pending ontology change without applying it. "+
				"Use this when a suggested change is not appropriate.",
		),
		mcp.WithString(
			"change_id",
			mcp.Required(),
			mcp.Description("UUID of the pending change to reject"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "reject_change")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Check if pending change repo is configured
		if deps.PendingChangeRepo == nil {
			return nil, fmt.Errorf("pending changes not available: repository not configured")
		}
		if deps.ChangeReviewService == nil {
			return nil, fmt.Errorf("change review service not available")
		}

		// Get change ID
		changeIDStr, err := req.RequireString("change_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", "change_id is required"), nil
		}

		changeID, err := uuid.Parse(changeIDStr)
		if err != nil {
			return NewErrorResult("invalid_parameters", fmt.Sprintf("invalid change_id: %v", err)), nil
		}

		// Verify change belongs to this project
		change, err := deps.PendingChangeRepo.GetByID(tenantCtx, changeID)
		if err != nil {
			return nil, fmt.Errorf("failed to get pending change: %w", err)
		}
		if change == nil {
			return NewErrorResult("not_found", "pending change not found"), nil
		}
		if change.ProjectID != projectID {
			return NewErrorResult("not_found", "pending change not found"), nil
		}

		// Reject the change
		rejectedChange, err := deps.ChangeReviewService.RejectChange(tenantCtx, changeID, models.ProvenanceMCP)
		if err != nil {
			deps.Logger.Error("Failed to reject change",
				zap.String("change_id", changeIDStr),
				zap.Error(err),
			)
			return NewErrorResult("rejection_failed", err.Error()), nil
		}

		response := map[string]any{
			"id":          rejectedChange.ID.String(),
			"status":      rejectedChange.Status,
			"change_type": rejectedChange.ChangeType,
			"table_name":  rejectedChange.TableName,
			"column_name": rejectedChange.ColumnName,
			"message":     "Change rejected successfully",
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerApproveAllChangesTool adds the approve_all_changes tool for bulk approving pending changes.
func registerApproveAllChangesTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"approve_all_changes",
		mcp.WithDescription(
			"Approve all pending ontology changes that can be applied. "+
				"Changes blocked by precedence rules will be skipped. "+
				"Returns summary of approved and skipped changes.",
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "approve_all_changes")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Check if services are configured
		if deps.PendingChangeRepo == nil {
			return nil, fmt.Errorf("pending changes not available: repository not configured")
		}
		if deps.ChangeReviewService == nil {
			return nil, fmt.Errorf("change review service not available")
		}

		// Approve all changes
		result, err := deps.ChangeReviewService.ApproveAllChanges(tenantCtx, projectID, models.ProvenanceMCP)
		if err != nil {
			deps.Logger.Error("Failed to approve all changes",
				zap.String("project_id", projectID.String()),
				zap.Error(err),
			)
			return nil, fmt.Errorf("failed to approve all changes: %w", err)
		}

		response := map[string]any{
			"approved":       result.Approved,
			"skipped":        result.Skipped,
			"skipped_reason": result.SkippedReason,
			"message":        fmt.Sprintf("Approved %d changes, skipped %d", result.Approved, result.Skipped),
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerScanDataChangesTool adds the scan_data_changes tool for detecting data-level changes.
func registerScanDataChangesTool(s *server.MCPServer, deps *MCPToolDeps) {
	tool := mcp.NewTool(
		"scan_data_changes",
		mcp.WithDescription(
			"Scan data for changes like new enum values and potential FK patterns. "+
				"Creates pending changes for review. Use after data updates to keep ontology current.",
		),
		mcp.WithString(
			"tables",
			mcp.Description("Comma-separated list of table names to scan (default: all selected tables)"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "scan_data_changes")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Check if data change detection service is configured
		if deps.DataChangeDetectionService == nil {
			return nil, fmt.Errorf("data change detection not available: service not configured")
		}

		// Get default datasource
		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}
		if dsID == uuid.Nil {
			return nil, fmt.Errorf("no default datasource configured for project")
		}

		// Get optional tables parameter
		tablesStr := getOptionalString(req, "tables")

		var changes []*models.PendingChange
		if tablesStr != "" {
			// Parse comma-separated table names
			tableNames := strings.Split(tablesStr, ",")
			for i, t := range tableNames {
				tableNames[i] = strings.TrimSpace(t)
			}
			changes, err = deps.DataChangeDetectionService.ScanTables(tenantCtx, projectID, dsID, tableNames)
		} else {
			changes, err = deps.DataChangeDetectionService.ScanForChanges(tenantCtx, projectID, dsID)
		}

		if err != nil {
			deps.Logger.Error("Data change scan failed",
				zap.String("project_id", projectID.String()),
				zap.String("datasource_id", dsID.String()),
				zap.Error(err),
			)
			return nil, fmt.Errorf("data change scan failed: %w", err)
		}

		deps.Logger.Info("Data change scan completed via MCP",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", dsID.String()),
			zap.Int("changes_found", len(changes)),
		)

		// Build response summary
		changesByType := make(map[string]int)
		for _, c := range changes {
			changesByType[c.ChangeType]++
		}

		response := struct {
			TotalChanges  int            `json:"total_changes"`
			ChangesByType map[string]int `json:"changes_by_type"`
			Changes       []any          `json:"changes"`
		}{
			TotalChanges:  len(changes),
			ChangesByType: changesByType,
			Changes:       make([]any, len(changes)),
		}

		for i, c := range changes {
			response.Changes[i] = map[string]any{
				"id":               c.ID.String(),
				"change_type":      c.ChangeType,
				"table_name":       c.TableName,
				"column_name":      c.ColumnName,
				"new_value":        c.NewValue,
				"suggested_action": c.SuggestedAction,
			}
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}
