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

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// QueryToolDeps contains dependencies for approved queries tools.
type QueryToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	ProjectService   services.ProjectService
	QueryService     services.QueryService
	Logger           *zap.Logger
}

const approvedQueriesToolGroup = "approved_queries"

// approvedQueriesToolNames lists all tools in the approved queries group.
var approvedQueriesToolNames = map[string]bool{
	"list_approved_queries":  true,
	"execute_approved_query": true,
	"suggest_approved_query": true,
	"get_query_history":      true,
}

// RegisterApprovedQueriesTools registers tools for executing Pre-Approved Queries.
func RegisterApprovedQueriesTools(s *server.MCPServer, deps *QueryToolDeps) {
	registerListApprovedQueriesTool(s, deps)
	registerExecuteApprovedQueryTool(s, deps)
	registerSuggestApprovedQueryTool(s, deps)
	registerGetQueryHistoryTool(s, deps)
}

// checkApprovedQueriesEnabled verifies the caller is authorized to use approved queries tools.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
// Returns the project ID and a tenant-scoped context if authorized, or an error if not.
func checkApprovedQueriesEnabled(ctx context.Context, deps *QueryToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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

	// Get tool groups state and check access using the unified checker
	state, err := deps.MCPConfigService.GetToolGroupsState(tenantCtx, projectID)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to get tool groups state",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool group configuration: %w", err)
	}

	// Use the unified ToolAccessChecker for consistent access decisions
	checker := services.NewToolAccessChecker()
	if checker.IsToolAccessible(toolName, state, isAgent) {
		return projectID, tenantCtx, func() { scope.Close() }, nil
	}

	scope.Close()
	return uuid.Nil, nil, nil, fmt.Errorf("approved queries tools are not enabled for this project")
}

// listApprovedQueriesResult is the response structure for list_approved_queries.
type listApprovedQueriesResult struct {
	Queries []approvedQueryInfo `json:"queries"`
}

type approvedQueryInfo struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`        // natural_language_prompt
	Description   string             `json:"description"` // additional_context
	SQL           string             `json:"sql"`         // The SQL template
	Parameters    []parameterInfo    `json:"parameters"`
	OutputColumns []outputColumnInfo `json:"output_columns,omitempty"`
	Constraints   string             `json:"constraints,omitempty"` // Limitations and assumptions
	Tags          []string           `json:"tags,omitempty"`        // Tags for organizing queries
	Dialect       string             `json:"dialect"`
}

type parameterInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

type outputColumnInfo struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// registerListApprovedQueriesTool - Lists all enabled parameterized queries with metadata.
func registerListApprovedQueriesTool(s *server.MCPServer, deps *QueryToolDeps) {
	tool := mcp.NewTool(
		"list_approved_queries",
		mcp.WithDescription(
			"List all pre-approved SQL queries available for execution. "+
				"Returns query metadata including parameters needed for execution. "+
				"Optionally filter by tags to find queries in specific categories. "+
				"Use execute_approved_query to run a specific query with parameters.",
		),
		mcp.WithArray(
			"tags",
			mcp.Description("Optional: Filter queries by tags. Returns queries matching ANY of the provided tags (e.g., [\"billing\", \"category:analytics\"])"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, deps, "list_approved_queries")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get default datasource
		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}

		// Parse optional tags filter
		var tags []string
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if tagsArray, ok := args["tags"].([]any); ok {
				for _, tag := range tagsArray {
					if str, ok := tag.(string); ok {
						tags = append(tags, str)
					}
				}
			}
		}

		// List enabled queries (filtered by tags if provided)
		var queries []*models.Query
		if len(tags) > 0 {
			queries, err = deps.QueryService.ListEnabledByTags(tenantCtx, projectID, dsID, tags)
		} else {
			queries, err = deps.QueryService.ListEnabled(tenantCtx, projectID, dsID)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to list queries: %w", err)
		}

		result := listApprovedQueriesResult{
			Queries: make([]approvedQueryInfo, len(queries)),
		}

		for i, q := range queries {
			params := make([]parameterInfo, len(q.Parameters))
			for j, p := range q.Parameters {
				params[j] = parameterInfo{
					Name:        p.Name,
					Type:        p.Type,
					Description: p.Description,
					Required:    p.Required,
					Default:     p.Default,
				}
			}

			// Use manually specified output_columns if available
			// Output columns come from query execution results, not SQL parsing
			var outputCols []outputColumnInfo
			if len(q.OutputColumns) > 0 {
				outputCols = make([]outputColumnInfo, len(q.OutputColumns))
				for j, oc := range q.OutputColumns {
					outputCols[j] = outputColumnInfo{
						Name:        oc.Name,
						Type:        oc.Type,
						Description: oc.Description,
					}
				}
			}

			desc := ""
			if q.AdditionalContext != nil {
				desc = *q.AdditionalContext
			}

			constraints := ""
			if q.Constraints != nil {
				constraints = *q.Constraints
			}

			result.Queries[i] = approvedQueryInfo{
				ID:            q.ID.String(),
				Name:          q.NaturalLanguagePrompt,
				Description:   desc,
				SQL:           q.SQLQuery,
				Parameters:    params,
				OutputColumns: outputCols,
				Constraints:   constraints,
				Tags:          q.Tags,
				Dialect:       q.Dialect,
			}
		}

		jsonResult, _ := json.Marshal(result)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerExecuteApprovedQueryTool - Executes a pre-approved query with parameters, includes injection detection.
func registerExecuteApprovedQueryTool(s *server.MCPServer, deps *QueryToolDeps) {
	tool := mcp.NewTool(
		"execute_approved_query",
		mcp.WithDescription(
			"Execute a pre-approved SQL query by ID with optional parameters. "+
				"Use list_approved_queries first to see available queries and required parameters. "+
				"Parameters are type-checked and validated before execution. "+
				"SQL injection attempts are detected and logged.",
		),
		mcp.WithString(
			"query_id",
			mcp.Required(),
			mcp.Description("The ID of the approved query to execute (from list_approved_queries)"),
		),
		mcp.WithObject(
			"parameters",
			mcp.Description("Parameter values as key-value pairs matching the query's parameter definitions"),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Max rows to return (default: 100, max: 1000)"),
		),
		mcp.WithReadOnlyHintAnnotation(true), // Approved queries should be SELECT only
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, deps, "execute_approved_query")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get query_id parameter
		queryIDStr, err := req.RequireString("query_id")
		if err != nil {
			return nil, err
		}
		queryID, err := uuid.Parse(queryIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid query_id format: %w", err)
		}

		// Get optional parameters
		params := make(map[string]any)
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if p, ok := args["parameters"].(map[string]any); ok {
				params = p
			}
		}

		// Get limit
		limit := 100
		if limitVal, ok := getOptionalFloat(req, "limit"); ok {
			limit = int(limitVal)
		}
		if limit > 1000 {
			limit = 1000
		}

		// Get query metadata before execution
		query, err := deps.QueryService.Get(tenantCtx, projectID, queryID)
		if err != nil {
			return nil, fmt.Errorf("failed to get query metadata: %w", err)
		}

		// Execute with parameters (includes injection detection)
		execReq := &services.ExecuteQueryRequest{Limit: limit}
		startTime := time.Now()
		result, err := deps.QueryService.ExecuteWithParameters(
			tenantCtx, projectID, queryID, params, execReq)
		executionTimeMs := time.Since(startTime).Milliseconds()
		if err != nil {
			// Enhance error message with query context
			return nil, enhanceErrorWithContext(err, query.NaturalLanguagePrompt)
		}

		// Log execution to history (best effort - don't fail request if logging fails)
		go logQueryExecution(tenantCtx, deps, projectID, queryID, query.SQLQuery, params, len(result.Rows), int(executionTimeMs))

		// Format response
		truncated := len(result.Rows) > limit
		rows := result.Rows
		if truncated {
			rows = rows[:limit]
		}

		// Convert column info for response
		type columnInfo struct {
			Name string `json:"name"`
			Type string `json:"type"`
		}
		columns := make([]columnInfo, len(result.Columns))
		for i, col := range result.Columns {
			columns[i] = columnInfo{Name: col.Name, Type: col.Type}
		}

		response := struct {
			QueryName       string           `json:"query_name"`
			ParametersUsed  map[string]any   `json:"parameters_used"`
			Columns         []columnInfo     `json:"columns"`
			Rows            []map[string]any `json:"rows"`
			RowCount        int              `json:"row_count"`
			Truncated       bool             `json:"truncated"`
			ExecutionTimeMs int64            `json:"execution_time_ms"`
		}{
			QueryName:       query.NaturalLanguagePrompt,
			ParametersUsed:  params,
			Columns:         columns,
			Rows:            rows,
			RowCount:        len(rows),
			Truncated:       truncated,
			ExecutionTimeMs: executionTimeMs,
		}

		jsonResult, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// enhanceErrorWithContext wraps an error with query context and categorizes the error type.
func enhanceErrorWithContext(err error, queryName string) error {
	if err == nil {
		return nil
	}

	errMsg := err.Error()
	errorType := categorizeError(errMsg)

	// Format: [error_type] query "Query Name": original error message
	return fmt.Errorf("[%s] query %q: %w", errorType, queryName, err)
}

// categorizeError determines the error type based on error message content.
// More specific checks come first to avoid false matches.
func categorizeError(errMsg string) string {
	// Check for SQL injection detection (most specific)
	if containsAny(errMsg, []string{"injection", "SQL injection"}) {
		return "security_violation"
	}

	// Check for type conversion errors (before general parameter check)
	if containsAny(errMsg, []string{"cannot convert", "invalid format"}) {
		return "type_validation"
	}

	// Check for parameter-related errors (validation, missing, unknown)
	if containsAny(errMsg, []string{"required", "missing", "unknown parameter"}) {
		return "parameter_validation"
	}

	// Check for execution errors
	if containsAny(errMsg, []string{"execute", "execution", "query failed"}) {
		return "execution_error"
	}

	// Default category
	return "query_error"
}

// containsAny checks if a string contains any of the given substrings (case-insensitive).
func containsAny(s string, substrs []string) bool {
	lowerS := strings.ToLower(s)
	for _, substr := range substrs {
		if strings.Contains(lowerS, strings.ToLower(substr)) {
			return true
		}
	}
	return false
}

// registerSuggestApprovedQueryTool - AI agents suggest reusable queries for approval.
func registerSuggestApprovedQueryTool(s *server.MCPServer, deps *QueryToolDeps) {
	tool := mcp.NewTool(
		"suggest_approved_query",
		mcp.WithDescription(
			"Suggest a reusable parameterized query for approval. "+
				"After validation, the query is stored with status='pending' (or 'approved' if auto-approve is enabled). "+
				"Use this when you discover a useful query pattern that should be saved for future use.",
		),
		mcp.WithString(
			"name",
			mcp.Required(),
			mcp.Description("Human-readable name for the query"),
		),
		mcp.WithString(
			"description",
			mcp.Required(),
			mcp.Description("What business question this query answers"),
		),
		mcp.WithString(
			"sql",
			mcp.Required(),
			mcp.Description("SQL query with {{parameter}} placeholders"),
		),
		mcp.WithArray(
			"parameters",
			mcp.Description("Parameter definitions (inferred from SQL if omitted)"),
		),
		mcp.WithObject(
			"output_column_descriptions",
			mcp.Description("Optional descriptions for output columns (e.g., {\"total_earned_usd\": \"Total earnings in USD\"})"),
		),
		mcp.WithArray(
			"tags",
			mcp.Description("Optional tags for organizing queries (e.g., [\"billing\", \"category:analytics\", \"reporting\"])"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, deps, "suggest_approved_query")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Extract parameters
		name, err := req.RequireString("name")
		if err != nil {
			return nil, err
		}
		description, err := req.RequireString("description")
		if err != nil {
			return nil, err
		}
		sqlQuery, err := req.RequireString("sql")
		if err != nil {
			return nil, err
		}

		// Get default datasource
		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}

		// Parse optional parameters
		var paramDefs []models.QueryParameter
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if paramsArray, ok := args["parameters"].([]any); ok {
				paramDefs, err = parseParameterDefinitions(paramsArray)
				if err != nil {
					return nil, fmt.Errorf("invalid parameters: %w", err)
				}
			}
		}

		// Parse optional output column descriptions
		var outputColDescs map[string]string
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if descs, ok := args["output_column_descriptions"].(map[string]any); ok {
				outputColDescs = make(map[string]string)
				for k, v := range descs {
					if str, ok := v.(string); ok {
						outputColDescs[k] = str
					}
				}
			}
		}

		// Parse optional tags
		var tags []string
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if tagsArray, ok := args["tags"].([]any); ok {
				for _, tag := range tagsArray {
					if str, ok := tag.(string); ok {
						tags = append(tags, str)
					}
				}
			}
		}

		// Validate SQL and parameters with dry-run execution
		validationResult, err := validateAndTestQuery(tenantCtx, deps, projectID, dsID, sqlQuery, paramDefs)
		if err != nil {
			return nil, fmt.Errorf("validation failed: %w", err)
		}

		// Merge output column descriptions with detected columns
		outputColumns := buildOutputColumns(validationResult.Columns, outputColDescs)

		// Build suggestion context
		suggestionContext := map[string]any{
			"validation": map[string]any{
				"sql_valid":       validationResult.SQLValid,
				"dry_run_rows":    validationResult.DryRunRows,
				"parameters_used": validationResult.ParametersUsed,
			},
		}

		// Create query with status='pending' and suggested_by='agent'
		createReq := &services.CreateQueryRequest{
			NaturalLanguagePrompt: name,
			AdditionalContext:     description,
			SQLQuery:              sqlQuery,
			IsEnabled:             false, // Start disabled until approved
			Parameters:            paramDefs,
			OutputColumns:         outputColumns,
			Tags:                  tags,
			Status:                "pending",
			SuggestedBy:           "agent",
			SuggestionContext:     suggestionContext,
		}

		query, err := deps.QueryService.Create(tenantCtx, projectID, dsID, createReq)
		if err != nil {
			return nil, fmt.Errorf("failed to create query suggestion: %w", err)
		}

		// Format response
		response := struct {
			SuggestionID  string             `json:"suggestion_id"`
			Status        string             `json:"status"`
			Validation    validationResponse `json:"validation"`
			ApprovedQuery *approvedQueryInfo `json:"approved_query,omitempty"`
		}{
			SuggestionID: query.ID.String(),
			Status:       query.Status,
			Validation: validationResponse{
				SQLValid:              validationResult.SQLValid,
				DryRunRows:            validationResult.DryRunRows,
				DetectedOutputColumns: buildColumnInfo(validationResult.Columns),
			},
		}

		// If auto-approve is enabled, include the approved query
		// For now, always return pending status (auto-approve can be implemented later)

		jsonResult, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// validationResult holds the results of SQL validation and dry-run execution.
type validationResult struct {
	SQLValid       bool
	DryRunRows     int
	Columns        []columnDetail
	ParametersUsed map[string]any
}

type columnDetail struct {
	Name string
	Type string
}

type validationResponse struct {
	SQLValid              bool           `json:"sql_valid"`
	DryRunRows            int            `json:"dry_run_rows"`
	DetectedOutputColumns []columnDetail `json:"detected_output_columns"`
}

// parseParameterDefinitions converts MCP parameter array to QueryParameter slice.
func parseParameterDefinitions(paramsArray []any) ([]models.QueryParameter, error) {
	params := make([]models.QueryParameter, 0, len(paramsArray))

	for i, p := range paramsArray {
		paramMap, ok := p.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("parameter %d is not an object", i)
		}

		name, ok := paramMap["name"].(string)
		if !ok || name == "" {
			return nil, fmt.Errorf("parameter %d missing required 'name' field", i)
		}

		paramType, ok := paramMap["type"].(string)
		if !ok || paramType == "" {
			return nil, fmt.Errorf("parameter %d missing required 'type' field", i)
		}

		param := models.QueryParameter{
			Name:     name,
			Type:     paramType,
			Required: true, // Default to required
		}

		if desc, ok := paramMap["description"].(string); ok {
			param.Description = desc
		}

		if required, ok := paramMap["required"].(bool); ok {
			param.Required = required
		}

		if example, ok := paramMap["example"]; ok {
			param.Default = example
		}

		params = append(params, param)
	}

	return params, nil
}

// validateAndTestQuery validates SQL and runs a dry-run with example parameters.
func validateAndTestQuery(ctx context.Context, deps *QueryToolDeps, projectID, dsID uuid.UUID, sqlQuery string, paramDefs []models.QueryParameter) (*validationResult, error) {
	// Build parameter values from examples
	paramValues := make(map[string]any)
	for _, p := range paramDefs {
		if p.Default != nil {
			paramValues[p.Name] = p.Default
		} else if p.Required {
			return nil, fmt.Errorf("parameter %q is required but has no example value", p.Name)
		}
	}

	// Test the query with a limit of 1 to detect output columns
	testReq := &services.TestQueryRequest{
		SQLQuery:             sqlQuery,
		Limit:                1,
		ParameterDefinitions: paramDefs,
		ParameterValues:      paramValues,
	}

	result, err := deps.QueryService.Test(ctx, projectID, dsID, testReq)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	// Build validation result
	columns := make([]columnDetail, len(result.Columns))
	for i, col := range result.Columns {
		columns[i] = columnDetail{
			Name: col.Name,
			Type: col.Type,
		}
	}

	return &validationResult{
		SQLValid:       true,
		DryRunRows:     len(result.Rows),
		Columns:        columns,
		ParametersUsed: paramValues,
	}, nil
}

// buildOutputColumns merges detected columns with provided descriptions.
func buildOutputColumns(columns []columnDetail, descriptions map[string]string) []models.OutputColumn {
	outputCols := make([]models.OutputColumn, len(columns))
	for i, col := range columns {
		outputCols[i] = models.OutputColumn{
			Name:        col.Name,
			Type:        col.Type,
			Description: descriptions[col.Name],
		}
	}
	return outputCols
}

// buildColumnInfo converts columnDetail to response format.
func buildColumnInfo(columns []columnDetail) []columnDetail {
	return columns
}

// logQueryExecution logs a query execution to the history table.
// This runs in a goroutine and uses best-effort logging - failures are logged but don't affect the caller.
func logQueryExecution(ctx context.Context, deps *QueryToolDeps, projectID, queryID uuid.UUID, sql string, params map[string]any, rowCount, executionTimeMs int) {
	// Get user ID from context if available
	userID := auth.GetUserIDFromContext(ctx)

	// Get tenant scope
	scope, ok := database.GetTenantScope(ctx)
	if !ok || scope == nil {
		deps.Logger.Warn("Failed to log query execution: tenant scope not found")
		return
	}

	// Marshal parameters to JSON
	var paramsJSON []byte
	if len(params) > 0 {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			deps.Logger.Warn("Failed to marshal parameters for query execution log",
				zap.Error(err),
				zap.String("query_id", queryID.String()))
			paramsJSON = nil
		}
	}

	// Insert execution record
	query := `
		INSERT INTO engine_query_executions
			(project_id, query_id, sql, row_count, execution_time_ms, parameters, user_id, source)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := scope.Conn.Exec(ctx, query,
		projectID,
		queryID,
		sql,
		rowCount,
		executionTimeMs,
		paramsJSON,
		userID,
		"mcp",
	)

	if err != nil {
		deps.Logger.Error("Failed to log query execution",
			zap.Error(err),
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()))
	}
}

// registerGetQueryHistoryTool - Returns recent query execution history to avoid rewriting queries.
func registerGetQueryHistoryTool(s *server.MCPServer, deps *QueryToolDeps) {
	tool := mcp.NewTool(
		"get_query_history",
		mcp.WithDescription(
			"Get recent query execution history to see what queries have been run recently. "+
				"Helps avoid rewriting the same queries repeatedly across sessions. "+
				"Returns SQL, execution time, row count, and parameters for recent executions.",
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Maximum number of query executions to return (default: 20, max: 100)"),
		),
		mcp.WithNumber(
			"hours_back",
			mcp.Description("How many hours back to look for query history (default: 24, max: 168)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, deps, "get_query_history")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get optional limit parameter (default 20, max 100)
		limit := 20
		if limitVal, ok := getOptionalFloat(req, "limit"); ok {
			limit = int(limitVal)
			if limit > 100 {
				limit = 100
			}
			if limit < 1 {
				limit = 1
			}
		}

		// Get optional hours_back parameter (default 24, max 168 = 1 week)
		hoursBack := 24
		if hoursVal, ok := getOptionalFloat(req, "hours_back"); ok {
			hoursBack = int(hoursVal)
			if hoursBack > 168 {
				hoursBack = 168
			}
			if hoursBack < 1 {
				hoursBack = 1
			}
		}

		// Calculate cutoff time
		cutoffTime := time.Now().Add(-time.Duration(hoursBack) * time.Hour)

		// Query execution history
		scope, ok := database.GetTenantScope(tenantCtx)
		if !ok || scope == nil {
			return nil, fmt.Errorf("tenant scope not found in context")
		}

		query := `
			SELECT
				qe.sql,
				qe.executed_at,
				qe.row_count,
				qe.execution_time_ms,
				qe.parameters,
				q.natural_language_prompt as query_name
			FROM engine_query_executions qe
			LEFT JOIN engine_queries q ON qe.query_id = q.id
			WHERE qe.project_id = $1
			  AND qe.executed_at >= $2
			ORDER BY qe.executed_at DESC
			LIMIT $3
		`

		rows, err := scope.Conn.Query(tenantCtx, query, projectID, cutoffTime, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to query execution history: %w", err)
		}
		defer rows.Close()

		type queryExecution struct {
			SQL             string         `json:"sql"`
			ExecutedAt      string         `json:"executed_at"`
			RowCount        int            `json:"row_count"`
			ExecutionTimeMs int            `json:"execution_time_ms"`
			Parameters      map[string]any `json:"parameters,omitempty"`
			QueryName       *string        `json:"query_name,omitempty"`
		}

		var executions []queryExecution

		for rows.Next() {
			var exec queryExecution
			var executedAt time.Time
			var paramsJSON []byte
			var queryName *string

			err := rows.Scan(
				&exec.SQL,
				&executedAt,
				&exec.RowCount,
				&exec.ExecutionTimeMs,
				&paramsJSON,
				&queryName,
			)
			if err != nil {
				return nil, fmt.Errorf("failed to scan execution row: %w", err)
			}

			exec.ExecutedAt = executedAt.Format(time.RFC3339)
			exec.QueryName = queryName

			// Parse parameters JSON if present
			if len(paramsJSON) > 0 {
				var params map[string]any
				if err := json.Unmarshal(paramsJSON, &params); err == nil {
					exec.Parameters = params
				}
			}

			executions = append(executions, exec)
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error iterating execution rows: %w", err)
		}

		response := struct {
			RecentQueries []queryExecution `json:"recent_queries"`
			Count         int              `json:"count"`
			HoursBack     int              `json:"hours_back"`
		}{
			RecentQueries: executions,
			Count:         len(executions),
			HoursBack:     hoursBack,
		}

		jsonResult, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}
