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

	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// QueryToolDeps contains dependencies for approved queries tools.
type QueryToolDeps struct {
	BaseMCPToolDeps
	ProjectService      services.ProjectService
	QueryService        services.QueryService
	Auditor             *audit.SecurityAuditor
	QueryHistoryService services.QueryHistoryService // Optional: for query learning history
}

// QueryLoggingDeps defines the interface for dependencies needed to log query executions.
// Both QueryToolDeps and MCPToolDeps implement this interface.
type QueryLoggingDeps interface {
	GetLogger() *zap.Logger
	GetAuditor() *audit.SecurityAuditor
	GetDB() *database.DB
}

// GetAuditor implements QueryLoggingDeps.
func (d *QueryToolDeps) GetAuditor() *audit.SecurityAuditor { return d.Auditor }

// approvedQueriesToolNames lists all tools in the approved queries group.
var approvedQueriesToolNames = map[string]bool{
	"list_approved_queries":  true,
	"execute_approved_query": true,
	"suggest_approved_query": true,
	"suggest_query_update":   true,
	"get_query_history":      true,
	"record_query_feedback":  true,
}

// RegisterApprovedQueriesTools registers tools for executing Pre-Approved Queries.
func RegisterApprovedQueriesTools(s *server.MCPServer, deps *QueryToolDeps) {
	registerListApprovedQueriesTool(s, deps)
	registerExecuteApprovedQueryTool(s, deps)
	registerSuggestApprovedQueryTool(s, deps)
	registerSuggestQueryUpdateTool(s, deps)
	registerGetQueryHistoryTool(s, deps)
	registerRecordQueryFeedbackTool(s, deps)
}

// listApprovedQueriesResult is the response structure for list_approved_queries.
type listApprovedQueriesResult struct {
	Queries []approvedQueryInfo `json:"queries"`
}

type approvedQueryInfo struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`        // natural_language_prompt
	Description        string             `json:"description"` // additional_context
	SQL                string             `json:"sql"`         // The SQL template
	Parameters         []parameterInfo    `json:"parameters"`
	OutputColumns      []outputColumnInfo `json:"output_columns,omitempty"`
	Constraints        string             `json:"constraints,omitempty"` // Limitations and assumptions
	Tags               []string           `json:"tags,omitempty"`        // Tags for organizing queries
	AllowsModification bool               `json:"allows_modification"`   // Can execute INSERT/UPDATE/DELETE/CALL
	Dialect            string             `json:"dialect"`
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
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "list_approved_queries")
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

		// Parse and validate optional tags filter
		var tags []string
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if tagsVal, exists := args["tags"]; exists {
				// Validate that tags is an array
				tagsArray, ok := tagsVal.([]any)
				if !ok {
					return NewErrorResultWithDetails("invalid_parameters",
						"parameter 'tags' must be an array",
						map[string]any{
							"parameter":     "tags",
							"expected_type": "array",
							"actual_type":   fmt.Sprintf("%T", tagsVal),
						}), nil
				}
				// Validate that each element is a string
				for i, tag := range tagsArray {
					str, ok := tag.(string)
					if !ok {
						return NewErrorResultWithDetails("invalid_parameters",
							"all tag elements must be strings",
							map[string]any{
								"parameter":             "tags",
								"invalid_element_index": i,
								"invalid_element_type":  fmt.Sprintf("%T", tag),
							}), nil
					}
					tags = append(tags, str)
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
				ID:                 q.ID.String(),
				Name:               q.NaturalLanguagePrompt,
				Description:        desc,
				SQL:                q.SQLQuery,
				Parameters:         params,
				OutputColumns:      outputCols,
				Constraints:        constraints,
				Tags:               q.Tags,
				AllowsModification: q.AllowsModification,
				Dialect:            q.Dialect,
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
		mcp.WithReadOnlyHintAnnotation(false),    // Some queries may modify data (INSERT/UPDATE/DELETE)
		mcp.WithDestructiveHintAnnotation(false), // Individual queries may be destructive
		mcp.WithIdempotentHintAnnotation(false),  // Modifying queries are not idempotent
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "execute_approved_query")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get query_id parameter
		queryIDStr, err := req.RequireString("query_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", "query_id parameter is required"), nil
		}
		queryID, err := uuid.Parse(queryIDStr)
		if err != nil {
			return NewErrorResult("invalid_parameters",
				fmt.Sprintf("invalid query_id format: %q is not a valid UUID", queryIDStr)), nil
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
			// Check if this is a "not found" error
			errMsg := err.Error()
			if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "no rows") {
				return NewErrorResult("QUERY_NOT_FOUND",
					fmt.Sprintf("query with ID %q not found. Use list_approved_queries to see available queries.", queryID)), nil
			}
			// Database/system errors remain as Go errors
			return nil, fmt.Errorf("failed to get query metadata: %w", err)
		}

		// Check if query is enabled (approved)
		if !query.IsEnabled {
			return NewErrorResult("QUERY_NOT_APPROVED",
				fmt.Sprintf("query %q (ID: %s) is not enabled. Only enabled queries can be executed.", query.NaturalLanguagePrompt, queryID)), nil
		}

		// Detect SQL statement type
		sqlType := services.DetectSQLType(query.SQLQuery)
		isModifying := services.IsModifyingStatement(sqlType)

		// Validate: modifying statements require allows_modification flag
		if isModifying && !query.AllowsModification {
			return NewErrorResult("QUERY_NOT_AUTHORIZED",
				fmt.Sprintf("query %q is a %s statement but is not authorized for data modification",
					query.NaturalLanguagePrompt, sqlType)), nil
		}

		startTime := time.Now()

		// Route to appropriate execution path based on SQL type
		if isModifying {
			// Execute as modifying query (no row limit, returns rows_affected)
			modifyResult, err := deps.QueryService.ExecuteModifyingWithParameters(
				tenantCtx, projectID, queryID, params)
			executionTimeMs := time.Since(startTime).Milliseconds()
			if err != nil {
				// Log failed execution for audit
				go logQueryExecution(tenantCtx, deps, QueryExecutionLog{
					ProjectID:       projectID,
					QueryID:         queryID,
					QueryName:       query.NaturalLanguagePrompt,
					SQL:             query.SQLQuery,
					SQLType:         string(sqlType),
					Params:          params,
					RowCount:        0,
					RowsAffected:    0,
					ExecutionTimeMs: int(executionTimeMs),
					IsModifying:     true,
					Success:         false,
					ErrorMessage:    err.Error(),
				})
				return convertQueryExecutionError(err, query.NaturalLanguagePrompt, queryID.String())
			}

			// Log successful execution to history
			go logQueryExecution(tenantCtx, deps, QueryExecutionLog{
				ProjectID:       projectID,
				QueryID:         queryID,
				QueryName:       query.NaturalLanguagePrompt,
				SQL:             query.SQLQuery,
				SQLType:         string(sqlType),
				Params:          params,
				RowCount:        modifyResult.RowCount,
				RowsAffected:    modifyResult.RowsAffected,
				ExecutionTimeMs: int(executionTimeMs),
				IsModifying:     true,
				Success:         true,
			})

			// Format response for modifying query
			response := struct {
				QueryName       string           `json:"query_name"`
				ParametersUsed  map[string]any   `json:"parameters_used"`
				Columns         []string         `json:"columns,omitempty"`
				Rows            []map[string]any `json:"rows,omitempty"`
				RowCount        int              `json:"row_count"`
				RowsAffected    int64            `json:"rows_affected"`
				ModifiedData    bool             `json:"modified_data"`
				ExecutionTimeMs int64            `json:"execution_time_ms"`
			}{
				QueryName:       query.NaturalLanguagePrompt,
				ParametersUsed:  params,
				Columns:         modifyResult.Columns,
				Rows:            modifyResult.Rows,
				RowCount:        modifyResult.RowCount,
				RowsAffected:    modifyResult.RowsAffected,
				ModifiedData:    true,
				ExecutionTimeMs: executionTimeMs,
			}

			jsonResult, _ := json.Marshal(response)
			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Execute as read-only query (with row limit)
		execReq := &services.ExecuteQueryRequest{Limit: limit}
		result, err := deps.QueryService.ExecuteWithParameters(
			tenantCtx, projectID, queryID, params, execReq)
		executionTimeMs := time.Since(startTime).Milliseconds()
		if err != nil {
			// Convert actionable errors to error results
			return convertQueryExecutionError(err, query.NaturalLanguagePrompt, queryID.String())
		}

		// Log execution to history (best effort - don't fail request if logging fails)
		go logQueryExecution(tenantCtx, deps, QueryExecutionLog{
			ProjectID:       projectID,
			QueryID:         queryID,
			QueryName:       query.NaturalLanguagePrompt,
			SQL:             query.SQLQuery,
			SQLType:         string(sqlType),
			Params:          params,
			RowCount:        len(result.Rows),
			RowsAffected:    0,
			ExecutionTimeMs: int(executionTimeMs),
			IsModifying:     false,
			Success:         true,
		})

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

// convertQueryExecutionError converts actionable query execution errors to error results.
// System errors (database connection failures, internal errors) remain as Go errors.
func convertQueryExecutionError(err error, queryName, queryID string) (*mcp.CallToolResult, error) {
	if err == nil {
		return nil, nil
	}

	errMsg := err.Error()

	// SQL injection detection - security violation (actionable)
	if containsAny(errMsg, []string{"injection", "SQL injection"}) {
		return NewErrorResultWithDetails("security_violation",
			fmt.Sprintf("SQL injection attempt detected in query %q", queryName),
			map[string]any{
				"query_id":    queryID,
				"query_name":  queryName,
				"error":       errMsg,
				"remediation": "Do not attempt to inject SQL. Use parameterized queries only.",
			}), nil
	}

	// Parameter validation errors (actionable - Claude can fix parameters)
	if containsAny(errMsg, []string{"required parameter", "missing parameter", "unknown parameter"}) {
		return NewErrorResult("parameter_validation",
			fmt.Sprintf("parameter validation failed for query %q: %s. Use list_approved_queries to see required parameters.", queryName, errMsg)), nil
	}

	// Type conversion errors (actionable - Claude can provide correct types)
	if containsAny(errMsg, []string{"cannot convert", "invalid format", "type mismatch"}) {
		return NewErrorResult("type_validation",
			fmt.Sprintf("parameter type mismatch in query %q: %s. Check parameter types in list_approved_queries.", queryName, errMsg)), nil
	}

	// SQL syntax errors (actionable - indicates query definition issue)
	if containsAny(errMsg, []string{"syntax error", "invalid syntax", "parse error"}) {
		return NewErrorResult("query_error",
			fmt.Sprintf("SQL syntax error in query %q: %s. This query may need to be updated.", queryName, errMsg)), nil
	}

	// Database connection failures and system errors remain as Go errors
	if containsAny(errMsg, []string{"connection", "timeout", "context", "deadlock"}) {
		return nil, fmt.Errorf("[system_error] query %q (ID: %s): %w", queryName, queryID, err)
	}

	// Default: treat as actionable query execution error
	return NewErrorResult("query_error",
		fmt.Sprintf("query execution failed for %q: %s", queryName, errMsg)), nil
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
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "suggest_approved_query")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
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
					return NewErrorResult("invalid_parameters",
						fmt.Sprintf("invalid parameters: %s", err.Error())), nil
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
			return NewErrorResult("validation_error",
				fmt.Sprintf("validation failed: %s", err.Error())), nil
		}

		// Merge output column descriptions with detected columns
		outputColumns := buildOutputColumns(validationResult.Columns, outputColDescs)

		// Detect if this is a modifying statement (INSERT/UPDATE/DELETE/CALL)
		sqlType := services.DetectSQLType(sqlQuery)
		isModifying := services.IsModifyingStatement(sqlType)

		// Build suggestion context
		suggestionContext := map[string]any{
			"validation": map[string]any{
				"sql_valid":       validationResult.SQLValid,
				"dry_run_rows":    validationResult.DryRunRows,
				"parameters_used": validationResult.ParametersUsed,
			},
			"sql_type":     string(sqlType),
			"is_modifying": isModifying,
		}

		// Create query with status='pending' and suggested_by='agent'
		// Auto-set allows_modification for INSERT/UPDATE/DELETE/CALL statements
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
			AllowsModification:    isModifying,
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

// registerSuggestQueryUpdateTool - AI agents suggest updates to existing queries for approval.
func registerSuggestQueryUpdateTool(s *server.MCPServer, deps *QueryToolDeps) {
	tool := mcp.NewTool(
		"suggest_query_update",
		mcp.WithDescription(
			"Suggest an update to an existing pre-approved query. "+
				"The suggestion will be reviewed by an administrator before being applied. "+
				"The original query remains active until the update is approved.",
		),
		mcp.WithString(
			"query_id",
			mcp.Required(),
			mcp.Description("UUID of the existing query to update"),
		),
		mcp.WithString(
			"sql",
			mcp.Description("Updated SQL query"),
		),
		mcp.WithString(
			"name",
			mcp.Description("Updated name"),
		),
		mcp.WithString(
			"description",
			mcp.Description("Updated description"),
		),
		mcp.WithArray(
			"parameters",
			mcp.Description("Updated parameter definitions"),
		),
		mcp.WithObject(
			"output_column_descriptions",
			mcp.Description("Updated output column descriptions"),
		),
		mcp.WithArray(
			"tags",
			mcp.Description("Updated tags for organizing queries"),
		),
		mcp.WithString(
			"context",
			mcp.Required(),
			mcp.Description("Explanation of why this update is needed"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "suggest_query_update")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Extract required parameters
		queryIDStr, err := req.RequireString("query_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", "query_id parameter is required"), nil
		}
		queryID, err := uuid.Parse(queryIDStr)
		if err != nil {
			return NewErrorResult("invalid_parameters",
				fmt.Sprintf("invalid query_id format: %q is not a valid UUID", queryIDStr)), nil
		}

		contextReason, err := req.RequireString("context")
		if err != nil {
			return NewErrorResult("invalid_parameters", "context parameter is required"), nil
		}

		// First, fetch the original query to validate it exists and get its datasource
		originalQuery, err := deps.QueryService.Get(tenantCtx, projectID, queryID)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "no rows") {
				return NewErrorResult("QUERY_NOT_FOUND",
					fmt.Sprintf("query with ID %q not found. Use list_approved_queries to see available queries.", queryID)), nil
			}
			return nil, fmt.Errorf("failed to get original query: %w", err)
		}

		// Build the update request
		updateReq := &services.SuggestUpdateRequest{
			QueryID: queryID,
			SuggestionContext: map[string]any{
				"reason": contextReason,
			},
		}

		// Parse optional fields from request
		args, _ := req.Params.Arguments.(map[string]any)

		// Track what fields are being updated for the response
		var updatedFields []string

		// Parse optional SQL update
		if sqlVal, ok := args["sql"].(string); ok && sqlVal != "" {
			updateReq.SQLQuery = &sqlVal
			updatedFields = append(updatedFields, "sql")

			// Validate the new SQL
			validationRes, err := deps.QueryService.Validate(tenantCtx, projectID, originalQuery.DatasourceID, sqlVal)
			if err != nil {
				return NewErrorResult("validation_error",
					fmt.Sprintf("failed to validate SQL: %s", err.Error())), nil
			}
			if !validationRes.Valid {
				return NewErrorResult("invalid_sql",
					fmt.Sprintf("invalid SQL: %s", validationRes.Message)), nil
			}

			// Add validation info to suggestion context
			updateReq.SuggestionContext["sql_validated"] = true
		}

		// Parse optional name update
		if nameVal, ok := args["name"].(string); ok && nameVal != "" {
			updateReq.NaturalLanguagePrompt = &nameVal
			updatedFields = append(updatedFields, "name")
		}

		// Parse optional description update
		if descVal, ok := args["description"].(string); ok && descVal != "" {
			updateReq.AdditionalContext = &descVal
			updatedFields = append(updatedFields, "description")
		}

		// Parse optional parameters update
		if paramsArray, ok := args["parameters"].([]any); ok && len(paramsArray) > 0 {
			paramDefs, err := parseParameterDefinitions(paramsArray)
			if err != nil {
				return NewErrorResult("invalid_parameters",
					fmt.Sprintf("invalid parameters: %s", err.Error())), nil
			}
			updateReq.Parameters = &paramDefs
			updatedFields = append(updatedFields, "parameters")
		}

		// Parse optional output column descriptions
		if descs, ok := args["output_column_descriptions"].(map[string]any); ok && len(descs) > 0 {
			outputColDescs := make(map[string]string)
			for k, v := range descs {
				if str, ok := v.(string); ok {
					outputColDescs[k] = str
				}
			}
			updateReq.OutputColumnDescriptions = outputColDescs
			updatedFields = append(updatedFields, "output_column_descriptions")
		}

		// Parse optional tags update
		if tagsArray, ok := args["tags"].([]any); ok {
			var tags []string
			for _, tag := range tagsArray {
				if str, ok := tag.(string); ok {
					tags = append(tags, str)
				}
			}
			updateReq.Tags = &tags
			updatedFields = append(updatedFields, "tags")
		}

		// Require at least one field to update (besides context)
		if len(updatedFields) == 0 {
			return NewErrorResult("invalid_parameters",
				"at least one update field (sql, name, description, parameters, output_column_descriptions, or tags) is required"), nil
		}

		// Add updated fields to context
		updateReq.SuggestionContext["updated_fields"] = updatedFields

		// Create the suggestion via the service
		suggestion, err := deps.QueryService.SuggestUpdate(tenantCtx, projectID, updateReq)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "not found") {
				return NewErrorResult("QUERY_NOT_FOUND",
					fmt.Sprintf("query with ID %q not found", queryID)), nil
			}
			return nil, fmt.Errorf("failed to create update suggestion: %w", err)
		}

		// Format response
		response := struct {
			SuggestionID    string   `json:"suggestion_id"`
			Status          string   `json:"status"`
			ParentQueryID   string   `json:"parent_query_id"`
			ParentQueryName string   `json:"parent_query_name"`
			UpdatedFields   []string `json:"updated_fields"`
			Message         string   `json:"message"`
		}{
			SuggestionID:    suggestion.ID.String(),
			Status:          suggestion.Status,
			ParentQueryID:   queryID.String(),
			ParentQueryName: originalQuery.NaturalLanguagePrompt,
			UpdatedFields:   updatedFields,
			Message: fmt.Sprintf("Update suggestion created for query %q. "+
				"An administrator will review and approve or reject this suggestion. "+
				"The original query remains active until the update is approved.",
				originalQuery.NaturalLanguagePrompt),
		}

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
// For modifying statements (INSERT/UPDATE/DELETE/CALL), uses EXPLAIN validation
// instead of actually executing the query.
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

	// Detect SQL type to determine validation approach
	sqlType := services.DetectSQLType(sqlQuery)

	// For modifying statements, use EXPLAIN validation instead of executing
	if services.IsModifyingStatement(sqlType) {
		return validateModifyingQuery(ctx, deps, projectID, dsID, sqlQuery, paramDefs, paramValues)
	}

	// For SELECT statements, execute with dry-run to detect output columns
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

// validateModifyingQuery validates INSERT/UPDATE/DELETE/CALL statements using EXPLAIN.
// These cannot be wrapped in SELECT, so we validate syntax instead of executing.
func validateModifyingQuery(ctx context.Context, deps *QueryToolDeps, projectID, dsID uuid.UUID, sqlQuery string, paramDefs []models.QueryParameter, paramValues map[string]any) (*validationResult, error) {
	// Use the service's Validate method (which uses EXPLAIN)
	validationRes, err := deps.QueryService.Validate(ctx, projectID, dsID, sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("query validation failed: %w", err)
	}

	if !validationRes.Valid {
		return nil, fmt.Errorf("invalid SQL: %s", validationRes.Message)
	}

	// For modifying queries, we can't easily detect output columns without executing.
	// Output columns would come from RETURNING clause, but EXPLAIN doesn't return them.
	// Return empty columns - the user can specify them manually if needed.
	return &validationResult{
		SQLValid:       true,
		DryRunRows:     0, // No dry-run for modifying queries
		Columns:        []columnDetail{},
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

// QueryExecutionLog contains all fields needed to log a query execution.
type QueryExecutionLog struct {
	ProjectID       uuid.UUID
	QueryID         uuid.UUID
	QueryName       string
	SQL             string
	SQLType         string // SELECT, INSERT, UPDATE, DELETE, CALL
	Params          map[string]any
	RowCount        int
	RowsAffected    int64
	ExecutionTimeMs int
	IsModifying     bool
	Success         bool
	ErrorMessage    string
}

// logQueryExecution logs a query execution to the history table.
// This runs in a goroutine and uses best-effort logging - failures are logged but don't affect the caller.
// The deps parameter must implement QueryLoggingDeps interface.
// IMPORTANT: This function acquires its own database connection because it runs asynchronously
// and the caller's connection may be released before this goroutine executes.
func logQueryExecution(ctx context.Context, deps QueryLoggingDeps, log QueryExecutionLog) {
	logger := deps.GetLogger()

	// Get user ID from context if available
	userID := auth.GetUserIDFromContext(ctx)

	// Acquire a fresh database connection for this goroutine.
	// We cannot use the connection from context because it may be released
	// by the time this goroutine runs (race condition).
	db := deps.GetDB()
	if db == nil {
		logger.Warn("Failed to log query execution: database not available")
		return
	}

	// Use a background context with timeout since the original context may be cancelled
	execCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scope, err := db.WithTenant(execCtx, log.ProjectID)
	if err != nil {
		logger.Warn("Failed to log query execution: could not acquire tenant scope",
			zap.Error(err),
			zap.String("project_id", log.ProjectID.String()))
		return
	}
	defer scope.Close()

	// Marshal parameters to JSON
	var paramsJSON []byte
	if len(log.Params) > 0 {
		paramsJSON, err = json.Marshal(log.Params)
		if err != nil {
			logger.Warn("Failed to marshal parameters for query execution log",
				zap.Error(err),
				zap.String("query_id", log.QueryID.String()))
			paramsJSON = nil
		}
	}

	// Insert execution record with enhanced audit fields
	query := `
		INSERT INTO engine_query_executions
			(project_id, query_id, sql, row_count, execution_time_ms, parameters, user_id, source,
			 is_modifying, rows_affected, success, error_message)
		VALUES
			($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	// Convert rows_affected to *int64 for nullable column
	var rowsAffected *int64
	if log.IsModifying {
		rowsAffected = &log.RowsAffected
	}

	// Convert error_message to *string for nullable column
	var errorMessage *string
	if log.ErrorMessage != "" {
		errorMessage = &log.ErrorMessage
	}

	_, err = scope.Conn.Exec(execCtx, query,
		log.ProjectID,
		log.QueryID,
		log.SQL,
		log.RowCount,
		log.ExecutionTimeMs,
		paramsJSON,
		userID,
		"mcp",
		log.IsModifying,
		rowsAffected,
		log.Success,
		errorMessage,
	)

	if err != nil {
		logger.Error("Failed to log query execution",
			zap.Error(err),
			zap.String("project_id", log.ProjectID.String()),
			zap.String("query_id", log.QueryID.String()))
	}

	// For modifying queries, also log to SIEM audit trail
	auditor := deps.GetAuditor()
	if log.IsModifying && auditor != nil {
		auditor.LogModifyingQueryExecution(ctx, log.ProjectID, log.QueryID,
			audit.ModifyingQueryDetails{
				QueryName:       log.QueryName,
				SQLType:         log.SQLType,
				SQL:             log.SQL,
				Parameters:      log.Params,
				RowsAffected:    log.RowsAffected,
				RowCount:        log.RowCount,
				Success:         log.Success,
				ErrorMessage:    log.ErrorMessage,
				ExecutionTimeMs: int64(log.ExecutionTimeMs),
			},
			"", // Client IP not available in MCP context
		)
	}
}

// logQueryHistory records a successful query to the learning history table.
// This runs in a goroutine and uses best-effort logging - failures are logged but don't affect the caller.
func logQueryHistory(ctx context.Context, db *database.DB, logger *zap.Logger, svc services.QueryHistoryService, entry *models.QueryHistoryEntry) {
	if db == nil || svc == nil {
		return
	}

	// Use a background context with timeout since the original context may be cancelled
	execCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scope, err := db.WithTenant(execCtx, entry.ProjectID)
	if err != nil {
		logger.Warn("Failed to record query history: could not acquire tenant scope",
			zap.Error(err),
			zap.String("project_id", entry.ProjectID.String()))
		return
	}
	defer scope.Close()

	tenantCtx := database.SetTenantScope(execCtx, scope)

	if err := svc.Record(tenantCtx, entry); err != nil {
		logger.Warn("Failed to record query history entry",
			zap.Error(err),
			zap.String("project_id", entry.ProjectID.String()))
	}
}

// registerGetQueryHistoryTool - Returns the user's recent successful query history for learning.
// Queries the engine_mcp_query_history table which contains queries with natural language context.
func registerGetQueryHistoryTool(s *server.MCPServer, deps *QueryToolDeps) {
	tool := mcp.NewTool(
		"get_query_history",
		mcp.WithDescription(
			"Get your recent successful query history with natural language context. "+
				"Use this to find previous queries that answered similar questions and reuse their SQL patterns. "+
				"Only shows your own queries (private to your account). "+
				"Returns the natural language question, SQL, execution details, and optional feedback.",
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Maximum number of queries to return (default: 20, max: 100)"),
		),
		mcp.WithString(
			"since",
			mcp.Description("Only return queries after this date (ISO 8601 format, e.g., '2024-01-01')"),
		),
		mcp.WithString(
			"tables",
			mcp.Description("Comma-separated list of table names to filter by (e.g., 'orders,users')"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "get_query_history")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		if deps.QueryHistoryService == nil {
			return NewErrorResult("service_unavailable", "query history service is not available"), nil
		}

		userID := auth.GetUserIDFromContext(tenantCtx)
		if userID == "" {
			return NewErrorResult("authentication_required", "user identity required for query history"), nil
		}

		// Parse filters
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

		filters := models.QueryHistoryFilters{
			UserID: userID,
			Limit:  limit,
		}

		if sinceStr := getOptionalString(req, "since"); sinceStr != "" {
			t, err := time.Parse(time.RFC3339, sinceStr)
			if err != nil {
				// Try date-only format
				t, err = time.Parse("2006-01-02", sinceStr)
				if err != nil {
					return NewErrorResult("invalid_parameter", fmt.Sprintf("invalid 'since' date format: %s (use ISO 8601, e.g., '2024-01-01')", sinceStr)), nil
				}
			}
			filters.Since = &t
		}

		if tablesStr := getOptionalString(req, "tables"); tablesStr != "" {
			tables := strings.Split(tablesStr, ",")
			for i := range tables {
				tables[i] = strings.TrimSpace(tables[i])
			}
			filters.TablesUsed = tables
		}

		entries, totalCount, err := deps.QueryHistoryService.List(tenantCtx, projectID, filters)
		if err != nil {
			return nil, fmt.Errorf("failed to query history: %w", err)
		}

		// Build response
		type historyEntry struct {
			ID                  string   `json:"id"`
			NaturalLanguage     string   `json:"natural_language"`
			SQL                 string   `json:"sql"`
			ExecutedAt          string   `json:"executed_at"`
			RowCount            *int     `json:"row_count,omitempty"`
			ExecutionDurationMs *int     `json:"execution_duration_ms,omitempty"`
			TablesUsed          []string `json:"tables_used,omitempty"`
			QueryType           *string  `json:"query_type,omitempty"`
			UserFeedback        *string  `json:"user_feedback,omitempty"`
		}

		queries := make([]historyEntry, len(entries))
		for i, e := range entries {
			queries[i] = historyEntry{
				ID:                  e.ID.String(),
				NaturalLanguage:     e.NaturalLanguage,
				SQL:                 e.SQL,
				ExecutedAt:          e.ExecutedAt.Format(time.RFC3339),
				RowCount:            e.RowCount,
				ExecutionDurationMs: e.ExecutionDurationMs,
				TablesUsed:          e.TablesUsed,
				QueryType:           e.QueryType,
				UserFeedback:        e.UserFeedback,
			}
		}

		response := struct {
			Queries    []historyEntry `json:"queries"`
			TotalCount int            `json:"total_count"`
			HasMore    bool           `json:"has_more"`
		}{
			Queries:    queries,
			TotalCount: totalCount,
			HasMore:    totalCount > len(queries),
		}

		jsonResult, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerRecordQueryFeedbackTool - Records user feedback on a query history entry.
func registerRecordQueryFeedbackTool(s *server.MCPServer, deps *QueryToolDeps) {
	tool := mcp.NewTool(
		"record_query_feedback",
		mcp.WithDescription(
			"Record whether a previously generated query was helpful. "+
				"This feedback improves future query suggestions. "+
				"Use the query_id from get_query_history or from the query tool's response.",
		),
		mcp.WithString(
			"query_id",
			mcp.Required(),
			mcp.Description("UUID of the query history entry to provide feedback on"),
		),
		mcp.WithString(
			"feedback",
			mcp.Required(),
			mcp.Description("Feedback value: 'helpful' or 'not_helpful'"),
		),
		mcp.WithString(
			"comment",
			mcp.Description("Optional comment explaining the feedback (e.g., 'Had to add a date filter')"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "record_query_feedback")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		if deps.QueryHistoryService == nil {
			return NewErrorResult("service_unavailable", "query history service is not available"), nil
		}

		userID := auth.GetUserIDFromContext(tenantCtx)
		if userID == "" {
			return NewErrorResult("authentication_required", "user identity required for feedback"), nil
		}

		// Parse required parameters
		queryIDStr, err := req.RequireString("query_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", "query_id is required"), nil
		}
		queryID, err := uuid.Parse(queryIDStr)
		if err != nil {
			return NewErrorResult("invalid_parameters", fmt.Sprintf("invalid query_id format: %s", queryIDStr)), nil
		}

		feedback, err := req.RequireString("feedback")
		if err != nil {
			return NewErrorResult("invalid_parameters", "feedback is required"), nil
		}
		if feedback != "helpful" && feedback != "not_helpful" {
			return NewErrorResult("invalid_parameters", "feedback must be 'helpful' or 'not_helpful'"), nil
		}

		// Optional comment
		var comment *string
		if c := getOptionalString(req, "comment"); c != "" {
			comment = &c
		}

		err = deps.QueryHistoryService.RecordFeedback(tenantCtx, projectID, queryID, userID, feedback, comment)
		if err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "not owned") {
				return NewErrorResult("not_found", "query history entry not found or not accessible"), nil
			}
			return nil, fmt.Errorf("failed to record feedback: %w", err)
		}

		response := struct {
			Recorded bool   `json:"recorded"`
			Message  string `json:"message"`
		}{
			Recorded: true,
			Message:  "Feedback recorded. Thank you for helping improve query suggestions.",
		}

		jsonResult, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}
