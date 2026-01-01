// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// DeveloperToolDeps contains dependencies for developer tools.
type DeveloperToolDeps struct {
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

// developerToolNames lists all tools in the developer group.
var developerToolNames = map[string]bool{
	"echo":     true,
	"query":    true,
	"sample":   true,
	"execute":  true,
	"validate": true,
}

// RegisterDeveloperTools registers the developer tool group tools.
// These tools are only accessible when the developer tool group is enabled.
func RegisterDeveloperTools(s *server.MCPServer, deps *DeveloperToolDeps) {
	registerEchoTool(s, deps)
	registerQueryTool(s, deps)
	registerSampleTool(s, deps)
	registerExecuteTool(s, deps)
	registerValidateTool(s, deps)
}

// NewToolFilter creates a ToolFilterFunc that filters tools based on MCP configuration.
// It filters out developer tools when the developer group is disabled, and filters out
// the execute tool when EnableExecute is false.
func NewToolFilter(deps *DeveloperToolDeps) func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
	return func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
		// Get claims from context
		claims, ok := auth.GetClaims(ctx)
		if !ok {
			deps.Logger.Debug("Tool filter: no auth context, filtering developer tools")
			return filterOutDeveloperTools(tools, true)
		}

		projectID, err := uuid.Parse(claims.ProjectID)
		if err != nil {
			deps.Logger.Error("Tool filter: invalid project ID in claims",
				zap.String("project_id", claims.ProjectID),
				zap.Error(err))
			return filterOutDeveloperTools(tools, true)
		}

		// Acquire tenant scope for database access
		scope, err := deps.DB.WithTenant(ctx, projectID)
		if err != nil {
			deps.Logger.Error("Tool filter: failed to acquire tenant scope",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return filterOutDeveloperTools(tools, true)
		}
		defer scope.Close()

		// Set tenant context for the config query
		tenantCtx := database.SetTenantScope(ctx, scope)

		// Get tool group config
		config, err := deps.MCPConfigService.GetToolGroupConfig(tenantCtx, projectID, developerToolGroup)
		if err != nil {
			deps.Logger.Error("Tool filter: failed to get tool group config",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return filterOutDeveloperTools(tools, true)
		}

		// If no config or disabled, filter out all developer tools
		if config == nil {
			deps.Logger.Debug("Tool filter: no config found, filtering developer tools",
				zap.String("project_id", projectID.String()))
			return filterOutDeveloperTools(tools, true)
		}

		if !config.Enabled {
			deps.Logger.Debug("Tool filter: developer tools disabled",
				zap.String("project_id", projectID.String()))
			return filterOutDeveloperTools(tools, true)
		}

		// Developer tools enabled - check if execute should be filtered
		if !config.EnableExecute {
			deps.Logger.Debug("Tool filter: execute tool disabled",
				zap.String("project_id", projectID.String()))
			return filterOutExecuteTool(tools)
		}

		deps.Logger.Debug("Tool filter: all developer tools enabled",
			zap.String("project_id", projectID.String()))
		return tools
	}
}

// filterOutDeveloperTools removes all developer tools from the list.
func filterOutDeveloperTools(tools []mcp.Tool, _ bool) []mcp.Tool {
	filtered := make([]mcp.Tool, 0, len(tools))
	for _, tool := range tools {
		if !developerToolNames[tool.Name] {
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
func checkDeveloperEnabled(ctx context.Context, deps *DeveloperToolDeps) (uuid.UUID, context.Context, func(), error) {
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
func checkExecuteEnabled(ctx context.Context, deps *DeveloperToolDeps) (uuid.UUID, context.Context, func(), error) {
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

// registerEchoTool adds a simple echo tool for testing the developer tool group.
// This tool verifies that authentication and tool group configuration work correctly.
func registerEchoTool(s *server.MCPServer, deps *DeveloperToolDeps) {
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
		// Check if developer tools are enabled
		_, _, cleanup, err := checkDeveloperEnabled(ctx, deps)
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
			"echo":   message,
			"status": "developer tools enabled",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}

// getDefaultDatasourceConfig returns the default datasource type and config for a project.
func getDefaultDatasourceConfig(ctx context.Context, deps *DeveloperToolDeps, projectID uuid.UUID) (string, map[string]any, error) {
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
func registerQueryTool(s *server.MCPServer, deps *DeveloperToolDeps) {
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
		projectID, tenantCtx, cleanup, err := checkDeveloperEnabled(ctx, deps)
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

		result := struct {
			Columns   []string         `json:"columns"`
			Rows      []map[string]any `json:"rows"`
			RowCount  int              `json:"row_count"`
			Truncated bool             `json:"truncated"`
		}{
			Columns:   queryResult.Columns,
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
func registerSampleTool(s *server.MCPServer, deps *DeveloperToolDeps) {
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
		projectID, tenantCtx, cleanup, err := checkDeveloperEnabled(ctx, deps)
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

		// Build query with properly sanitized identifiers to prevent SQL injection
		quotedSchema := pgx.Identifier{schemaName}.Sanitize()
		quotedTable := pgx.Identifier{tableName}.Sanitize()
		sql := fmt.Sprintf(`SELECT * FROM %s.%s LIMIT %d`, quotedSchema, quotedTable, limit)

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

		// Execute query
		queryResult, err := executor.ExecuteQuery(tenantCtx, sql, 0)
		if err != nil {
			return nil, fmt.Errorf("sample query failed: %w", err)
		}

		result := struct {
			Columns   []string         `json:"columns"`
			Rows      []map[string]any `json:"rows"`
			RowCount  int              `json:"row_count"`
			Truncated bool             `json:"truncated"`
		}{
			Columns:   queryResult.Columns,
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
func registerExecuteTool(s *server.MCPServer, deps *DeveloperToolDeps) {
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
func registerValidateTool(s *server.MCPServer, deps *DeveloperToolDeps) {
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
		projectID, tenantCtx, cleanup, err := checkDeveloperEnabled(ctx, deps)
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
