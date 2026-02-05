// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// DevQueryToolDeps defines dependencies for dev query MCP tools.
type DevQueryToolDeps struct {
	BaseMCPToolDeps
	ProjectService services.ProjectService
	QueryService   services.QueryService
}

// RegisterDevQueryTools registers the dev query management tools with the MCP server.
// These tools are for administrators to manage query suggestions.
func RegisterDevQueryTools(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	registerListQuerySuggestionsTool(mcpServer, deps)
	registerApproveQuerySuggestionTool(mcpServer, deps)
	registerRejectQuerySuggestionTool(mcpServer, deps)
	registerCreateApprovedQueryTool(mcpServer, deps)
	registerUpdateApprovedQueryTool(mcpServer, deps)
	registerDeleteApprovedQueryTool(mcpServer, deps)
}

// querySuggestionInfo represents a single query suggestion in the response.
type querySuggestionInfo struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"` // "new" or "update"
	Name            string   `json:"name"`
	SQL             string   `json:"sql"`
	SuggestedBy     string   `json:"suggested_by"`
	CreatedAt       string   `json:"created_at"`
	Context         string   `json:"context,omitempty"`
	ParentQueryID   string   `json:"parent_query_id,omitempty"`
	ParentQueryName string   `json:"parent_query_name,omitempty"`
	Changes         []string `json:"changes,omitempty"` // For updates: which fields changed
	DatasourceID    string   `json:"datasource_id"`
	Status          string   `json:"status"`
}

// listQuerySuggestionsResponse is the response format for list_query_suggestions.
type listQuerySuggestionsResponse struct {
	Suggestions []querySuggestionInfo `json:"suggestions"`
	Count       int                   `json:"count"`
}

// registerListQuerySuggestionsTool registers the list_query_suggestions tool.
func registerListQuerySuggestionsTool(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	tool := mcp.NewTool(
		"list_query_suggestions",
		mcp.WithDescription(`List all pending query suggestions awaiting review.
Returns both new query suggestions and update suggestions for existing queries.
Use approve_query_suggestion or reject_query_suggestion to process suggestions.`),
		mcp.WithString("status",
			mcp.Description("Filter by status: pending, approved, rejected (default: pending)")),
		mcp.WithString("datasource_id",
			mcp.Description("Filter by datasource UUID")),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check tool access
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "list_query_suggestions")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse arguments
		args, _ := request.Params.Arguments.(map[string]any)

		// Parse optional status filter
		status := "pending" // default
		if statusVal, ok := args["status"].(string); ok && statusVal != "" {
			switch statusVal {
			case "pending", "approved", "rejected":
				status = statusVal
			default:
				return NewErrorResultWithDetails("invalid_parameters",
					"status must be one of: pending, approved, rejected",
					map[string]any{
						"parameter":    "status",
						"valid_values": []string{"pending", "approved", "rejected"},
						"actual_value": statusVal,
					}), nil
			}
		}

		// Parse optional datasource_id filter
		var datasourceID *uuid.UUID
		if dsIDVal, ok := args["datasource_id"].(string); ok && dsIDVal != "" {
			parsed, err := uuid.Parse(dsIDVal)
			if err != nil {
				return NewErrorResultWithDetails("invalid_parameters",
					"datasource_id is not a valid UUID",
					map[string]any{
						"parameter":    "datasource_id",
						"actual_value": dsIDVal,
					}), nil
			}
			datasourceID = &parsed
		}

		// For now, only "pending" status is directly supported by the service layer.
		// Other statuses would need additional repository methods.
		var queries []*models.Query
		if status == "pending" {
			queries, err = deps.QueryService.ListPending(tenantCtx, projectID)
			if err != nil {
				deps.Logger.Error("Failed to list pending queries",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
				return nil, fmt.Errorf("failed to list pending queries: %w", err)
			}
		} else {
			// For approved/rejected, we would need a different repository method.
			// For now, return empty list as those statuses are not yet implemented.
			queries = []*models.Query{}
		}

		// Filter by datasource_id if provided
		if datasourceID != nil {
			filtered := make([]*models.Query, 0)
			for _, q := range queries {
				if q.DatasourceID == *datasourceID {
					filtered = append(filtered, q)
				}
			}
			queries = filtered
		}

		// Build parent query lookup for update suggestions
		parentQueries := make(map[uuid.UUID]*models.Query)
		for _, q := range queries {
			if q.ParentQueryID != nil {
				// Fetch parent query if not already fetched
				if _, exists := parentQueries[*q.ParentQueryID]; !exists {
					parent, err := deps.QueryService.Get(tenantCtx, projectID, *q.ParentQueryID)
					if err == nil && parent != nil {
						parentQueries[*q.ParentQueryID] = parent
					}
				}
			}
		}

		// Build response
		suggestions := make([]querySuggestionInfo, 0, len(queries))
		for _, q := range queries {
			info := querySuggestionInfo{
				ID:           q.ID.String(),
				Name:         q.NaturalLanguagePrompt,
				SQL:          q.SQLQuery,
				CreatedAt:    q.CreatedAt.Format("2006-01-02T15:04:05Z"),
				DatasourceID: q.DatasourceID.String(),
				Status:       q.Status,
			}

			// Set suggested_by
			if q.SuggestedBy != nil {
				info.SuggestedBy = *q.SuggestedBy
			} else {
				info.SuggestedBy = "unknown"
			}

			// Extract context from suggestion_context
			if q.SuggestionContext != nil {
				if contextVal, ok := q.SuggestionContext["context"].(string); ok {
					info.Context = contextVal
				} else if reasonVal, ok := q.SuggestionContext["reason"].(string); ok {
					info.Context = reasonVal
				}
			}

			// Determine type and populate parent info for updates
			if q.ParentQueryID != nil {
				info.Type = "update"
				info.ParentQueryID = q.ParentQueryID.String()

				// Get parent query info
				if parent, exists := parentQueries[*q.ParentQueryID]; exists {
					info.ParentQueryName = parent.NaturalLanguagePrompt

					// Calculate what changed between parent and suggestion
					info.Changes = calculateChanges(parent, q)
				}
			} else {
				info.Type = "new"
			}

			suggestions = append(suggestions, info)
		}

		response := listQuerySuggestionsResponse{
			Suggestions: suggestions,
			Count:       len(suggestions),
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}

		return mcp.NewToolResultText(string(responseJSON)), nil
	}

	mcpServer.AddTool(tool, handler)
}

// calculateChanges determines which fields changed between the parent query and the suggestion.
func calculateChanges(parent, suggestion *models.Query) []string {
	var changes []string

	if parent.SQLQuery != suggestion.SQLQuery {
		changes = append(changes, "sql")
	}
	if parent.NaturalLanguagePrompt != suggestion.NaturalLanguagePrompt {
		changes = append(changes, "name")
	}
	if !stringPtrEqual(parent.AdditionalContext, suggestion.AdditionalContext) {
		changes = append(changes, "description")
	}
	if !parametersEqual(parent.Parameters, suggestion.Parameters) {
		changes = append(changes, "parameters")
	}
	if !outputColumnsEqual(parent.OutputColumns, suggestion.OutputColumns) {
		changes = append(changes, "output_columns")
	}
	if !stringPtrEqual(parent.Constraints, suggestion.Constraints) {
		changes = append(changes, "constraints")
	}
	if !tagsEqual(parent.Tags, suggestion.Tags) {
		changes = append(changes, "tags")
	}
	if parent.AllowsModification != suggestion.AllowsModification {
		changes = append(changes, "allows_modification")
	}

	return changes
}

// stringPtrEqual compares two string pointers for equality.
func stringPtrEqual(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// parametersEqual compares two parameter slices for equality.
func parametersEqual(a, b []models.QueryParameter) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Type != b[i].Type ||
			a[i].Description != b[i].Description ||
			a[i].Required != b[i].Required {
			return false
		}
		// Note: We don't compare Default as it can be any type
	}
	return true
}

// outputColumnsEqual compares two output column slices for equality.
func outputColumnsEqual(a, b []models.OutputColumn) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name ||
			a[i].Type != b[i].Type ||
			a[i].Description != b[i].Description {
			return false
		}
	}
	return true
}

// tagsEqual compares two tag slices for equality.
func tagsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// approveQuerySuggestionResponse is the response format for approve_query_suggestion.
type approveQuerySuggestionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	QueryID string `json:"query_id"`
	Type    string `json:"type"` // "new" or "update"
}

// registerApproveQuerySuggestionTool registers the approve_query_suggestion tool.
func registerApproveQuerySuggestionTool(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	tool := mcp.NewTool(
		"approve_query_suggestion",
		mcp.WithDescription(`Approve a pending query suggestion.
For new queries: Sets status to approved and enables the query for execution.
For update suggestions: Applies the changes to the original query and removes the pending record.
Use list_query_suggestions first to see pending suggestions.`),
		mcp.WithString("suggestion_id",
			mcp.Required(),
			mcp.Description("UUID of the pending suggestion to approve")),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check tool access
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "approve_query_suggestion")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse arguments
		args, _ := request.Params.Arguments.(map[string]any)

		// Parse required suggestion_id
		suggestionIDStr, ok := args["suggestion_id"].(string)
		if !ok || suggestionIDStr == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"suggestion_id is required",
				map[string]any{
					"parameter": "suggestion_id",
				}), nil
		}

		suggestionID, err := uuid.Parse(suggestionIDStr)
		if err != nil {
			return NewErrorResultWithDetails("invalid_parameters",
				"suggestion_id is not a valid UUID",
				map[string]any{
					"parameter":    "suggestion_id",
					"actual_value": suggestionIDStr,
				}), nil
		}

		// Get the suggestion to determine its type before approval
		suggestion, err := deps.QueryService.Get(tenantCtx, projectID, suggestionID)
		if err != nil {
			deps.Logger.Error("Failed to get query suggestion",
				zap.String("project_id", projectID.String()),
				zap.String("suggestion_id", suggestionID.String()),
				zap.Error(err))
			return NewErrorResultWithDetails("not_found",
				"suggestion not found",
				map[string]any{
					"suggestion_id": suggestionIDStr,
				}), nil
		}

		// Verify it's a pending suggestion
		if suggestion.Status != "pending" {
			return NewErrorResultWithDetails("invalid_state",
				fmt.Sprintf("suggestion is not pending (status: %s)", suggestion.Status),
				map[string]any{
					"suggestion_id": suggestionIDStr,
					"status":        suggestion.Status,
				}), nil
		}

		// Determine suggestion type
		suggestionType := "new"
		if suggestion.ParentQueryID != nil {
			suggestionType = "update"
		}

		// Get reviewer ID from context
		reviewerID := auth.GetUserIDFromContext(ctx)
		if reviewerID == "" {
			reviewerID = "mcp-session" // Fallback for MCP sessions without user context
		}

		// Approve the query
		err = deps.QueryService.ApproveQuery(tenantCtx, projectID, suggestionID, reviewerID)
		if err != nil {
			deps.Logger.Error("Failed to approve query suggestion",
				zap.String("project_id", projectID.String()),
				zap.String("suggestion_id", suggestionID.String()),
				zap.String("reviewer_id", reviewerID),
				zap.Error(err))
			return nil, fmt.Errorf("failed to approve query: %w", err)
		}

		// Build response message based on type
		var message string
		var queryID string
		if suggestionType == "update" {
			queryID = suggestion.ParentQueryID.String()
			message = fmt.Sprintf("Update suggestion approved. Changes applied to query %s.", queryID)
		} else {
			queryID = suggestionID.String()
			message = "Query approved and enabled for execution."
		}

		deps.Logger.Info("Approved query suggestion",
			zap.String("project_id", projectID.String()),
			zap.String("suggestion_id", suggestionID.String()),
			zap.String("type", suggestionType),
			zap.String("reviewer_id", reviewerID),
		)

		response := approveQuerySuggestionResponse{
			Success: true,
			Message: message,
			QueryID: queryID,
			Type:    suggestionType,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}

		return mcp.NewToolResultText(string(responseJSON)), nil
	}

	mcpServer.AddTool(tool, handler)
}

// rejectQuerySuggestionResponse is the response format for reject_query_suggestion.
type rejectQuerySuggestionResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	SuggestionID string `json:"suggestion_id"`
	Reason       string `json:"reason"`
}

// registerRejectQuerySuggestionTool registers the reject_query_suggestion tool.
func registerRejectQuerySuggestionTool(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	tool := mcp.NewTool(
		"reject_query_suggestion",
		mcp.WithDescription(`Reject a pending query suggestion with a reason.
The suggestion will be marked as rejected and the reason will be recorded.
Use list_query_suggestions first to see pending suggestions.`),
		mcp.WithString("suggestion_id",
			mcp.Required(),
			mcp.Description("UUID of the pending suggestion to reject")),
		mcp.WithString("reason",
			mcp.Required(),
			mcp.Description("Explanation for why the suggestion was rejected")),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check tool access
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "reject_query_suggestion")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse arguments
		args, _ := request.Params.Arguments.(map[string]any)

		// Parse required suggestion_id
		suggestionIDStr, ok := args["suggestion_id"].(string)
		if !ok || suggestionIDStr == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"suggestion_id is required",
				map[string]any{
					"parameter": "suggestion_id",
				}), nil
		}

		suggestionID, err := uuid.Parse(suggestionIDStr)
		if err != nil {
			return NewErrorResultWithDetails("invalid_parameters",
				"suggestion_id is not a valid UUID",
				map[string]any{
					"parameter":    "suggestion_id",
					"actual_value": suggestionIDStr,
				}), nil
		}

		// Parse required reason
		reason, ok := args["reason"].(string)
		if !ok || reason == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"reason is required",
				map[string]any{
					"parameter": "reason",
				}), nil
		}

		// Get the suggestion to verify it exists and is pending
		suggestion, err := deps.QueryService.Get(tenantCtx, projectID, suggestionID)
		if err != nil {
			deps.Logger.Error("Failed to get query suggestion",
				zap.String("project_id", projectID.String()),
				zap.String("suggestion_id", suggestionID.String()),
				zap.Error(err))
			return NewErrorResultWithDetails("not_found",
				"suggestion not found",
				map[string]any{
					"suggestion_id": suggestionIDStr,
				}), nil
		}

		// Verify it's a pending suggestion
		if suggestion.Status != "pending" {
			return NewErrorResultWithDetails("invalid_state",
				fmt.Sprintf("suggestion is not pending (status: %s)", suggestion.Status),
				map[string]any{
					"suggestion_id": suggestionIDStr,
					"status":        suggestion.Status,
				}), nil
		}

		// Get reviewer ID from context
		reviewerID := auth.GetUserIDFromContext(ctx)
		if reviewerID == "" {
			reviewerID = "mcp-session" // Fallback for MCP sessions without user context
		}

		// Reject the query
		err = deps.QueryService.RejectQuery(tenantCtx, projectID, suggestionID, reviewerID, reason)
		if err != nil {
			deps.Logger.Error("Failed to reject query suggestion",
				zap.String("project_id", projectID.String()),
				zap.String("suggestion_id", suggestionID.String()),
				zap.String("reviewer_id", reviewerID),
				zap.Error(err))
			return nil, fmt.Errorf("failed to reject query: %w", err)
		}

		deps.Logger.Info("Rejected query suggestion",
			zap.String("project_id", projectID.String()),
			zap.String("suggestion_id", suggestionID.String()),
			zap.String("reviewer_id", reviewerID),
			zap.String("reason", reason),
		)

		response := rejectQuerySuggestionResponse{
			Success:      true,
			Message:      "Suggestion rejected.",
			SuggestionID: suggestionID.String(),
			Reason:       reason,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}

		return mcp.NewToolResultText(string(responseJSON)), nil
	}

	mcpServer.AddTool(tool, handler)
}

// createApprovedQueryResponse is the response format for create_approved_query.
type createApprovedQueryResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	QueryID string `json:"query_id"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}

// registerCreateApprovedQueryTool registers the create_approved_query tool.
func registerCreateApprovedQueryTool(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	tool := mcp.NewTool(
		"create_approved_query",
		mcp.WithDescription(`Create a new pre-approved query directly (no review required).
The query will be immediately available for execution with status='approved'.
SQL syntax is validated before creation. Use this for admin-created queries that bypass the suggestion workflow.
If datasource_id is not provided, the project's default datasource will be used.`),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Human-readable name for the query")),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("What business question this query answers")),
		mcp.WithString("sql",
			mcp.Required(),
			mcp.Description("SQL query with {{parameter}} placeholders")),
		mcp.WithString("datasource_id",
			mcp.Description("UUID of the datasource (optional, defaults to project's default datasource)")),
		mcp.WithArray("parameters",
			mcp.Description("Parameter definitions (array of objects with name, type, description, required, example)")),
		mcp.WithObject("output_column_descriptions",
			mcp.Description("Descriptions for output columns (e.g., {\"total\": \"Total amount in USD\"})")),
		mcp.WithArray("tags",
			mcp.Description("Tags for organizing queries (e.g., [\"billing\", \"reporting\"])")),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check tool access
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "create_approved_query")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse arguments
		args, _ := request.Params.Arguments.(map[string]any)

		// Parse required name
		name, ok := args["name"].(string)
		if !ok || name == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"name is required",
				map[string]any{
					"parameter": "name",
				}), nil
		}

		// Parse required description
		description, ok := args["description"].(string)
		if !ok || description == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"description is required",
				map[string]any{
					"parameter": "description",
				}), nil
		}

		// Parse required sql
		sqlQuery, ok := args["sql"].(string)
		if !ok || sqlQuery == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"sql is required",
				map[string]any{
					"parameter": "sql",
				}), nil
		}

		// Parse optional datasource_id (auto-detect if not provided)
		var datasourceID uuid.UUID
		datasourceIDStr, ok := args["datasource_id"].(string)
		if ok && datasourceIDStr != "" {
			datasourceID, err = uuid.Parse(datasourceIDStr)
			if err != nil {
				return NewErrorResultWithDetails("invalid_parameters",
					"datasource_id is not a valid UUID",
					map[string]any{
						"parameter":    "datasource_id",
						"actual_value": datasourceIDStr,
					}), nil
			}
		} else {
			// Auto-detect default datasource (same pattern as suggest_approved_query)
			datasourceID, err = deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
			if err != nil {
				return nil, fmt.Errorf("failed to get default datasource: %w", err)
			}
		}

		// Validate SQL syntax before creation
		validationRes, err := deps.QueryService.Validate(tenantCtx, projectID, datasourceID, sqlQuery)
		if err != nil {
			// Use DEBUG for input errors (SQL validation, etc.), ERROR for server errors
			if IsInputError(err) {
				deps.Logger.Debug("Failed to validate SQL (input error)",
					zap.String("project_id", projectID.String()),
					zap.String("error", err.Error()))
			} else {
				deps.Logger.Error("Failed to validate SQL",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
			}
			return NewErrorResultWithDetails("validation_error",
				fmt.Sprintf("failed to validate SQL: %s", err.Error()),
				map[string]any{
					"sql": sqlQuery,
				}), nil
		}

		if !validationRes.Valid {
			return NewErrorResultWithDetails("invalid_sql",
				fmt.Sprintf("invalid SQL: %s", validationRes.Message),
				map[string]any{
					"sql":   sqlQuery,
					"error": validationRes.Message,
				}), nil
		}

		// Parse optional parameters
		var paramDefs []models.QueryParameter
		if paramsArray, ok := args["parameters"].([]any); ok && len(paramsArray) > 0 {
			paramDefs, err = parseDevQueryParameterDefinitions(paramsArray)
			if err != nil {
				return NewErrorResultWithDetails("invalid_parameters",
					fmt.Sprintf("invalid parameters: %s", err.Error()),
					map[string]any{
						"parameter": "parameters",
					}), nil
			}
		}

		// Parse optional output column descriptions
		var outputColumns []models.OutputColumn
		if descs, ok := args["output_column_descriptions"].(map[string]any); ok && len(descs) > 0 {
			for colName, colDesc := range descs {
				if descStr, ok := colDesc.(string); ok {
					outputColumns = append(outputColumns, models.OutputColumn{
						Name:        colName,
						Description: descStr,
					})
				}
			}
		}

		// Parse optional tags
		var tags []string
		if tagsArray, ok := args["tags"].([]any); ok {
			for _, tag := range tagsArray {
				if str, ok := tag.(string); ok {
					tags = append(tags, str)
				}
			}
		}

		// Detect if this is a modifying statement (INSERT/UPDATE/DELETE/CALL)
		sqlType := services.DetectSQLType(sqlQuery)
		isModifying := services.IsModifyingStatement(sqlType)

		// Create the query via DirectCreate (status="approved", suggested_by="admin")
		createReq := &services.CreateQueryRequest{
			NaturalLanguagePrompt: name,
			AdditionalContext:     description,
			SQLQuery:              sqlQuery,
			Parameters:            paramDefs,
			OutputColumns:         outputColumns,
			Tags:                  tags,
			AllowsModification:    isModifying,
		}

		query, err := deps.QueryService.DirectCreate(tenantCtx, projectID, datasourceID, createReq)
		if err != nil {
			// Use DEBUG for input errors (validation failures, etc.), ERROR for server errors
			if IsInputError(err) {
				deps.Logger.Debug("Failed to create approved query (input error)",
					zap.String("project_id", projectID.String()),
					zap.String("datasource_id", datasourceID.String()),
					zap.String("error", err.Error()))
			} else {
				deps.Logger.Error("Failed to create approved query",
					zap.String("project_id", projectID.String()),
					zap.String("datasource_id", datasourceID.String()),
					zap.Error(err))
			}
			return nil, fmt.Errorf("failed to create query: %w", err)
		}

		deps.Logger.Info("Created approved query",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", query.ID.String()),
			zap.String("name", name),
		)

		response := createApprovedQueryResponse{
			Success: true,
			Message: "Query created and approved. It is now available for execution.",
			QueryID: query.ID.String(),
			Name:    name,
			Status:  query.Status,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}

		return mcp.NewToolResultText(string(responseJSON)), nil
	}

	mcpServer.AddTool(tool, handler)
}

// parseDevQueryParameterDefinitions converts MCP parameter array to QueryParameter slice.
// This is a local version for dev_queries.go to avoid import cycles.
func parseDevQueryParameterDefinitions(paramsArray []any) ([]models.QueryParameter, error) {
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

// updateApprovedQueryResponse is the response format for update_approved_query.
type updateApprovedQueryResponse struct {
	Success bool     `json:"success"`
	Message string   `json:"message"`
	QueryID string   `json:"query_id"`
	Name    string   `json:"name"`
	Updated []string `json:"updated"` // List of fields that were updated
}

// registerUpdateApprovedQueryTool registers the update_approved_query tool.
func registerUpdateApprovedQueryTool(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	tool := mcp.NewTool(
		"update_approved_query",
		mcp.WithDescription(`Update an existing pre-approved query directly (no review required).
Changes are applied immediately. SQL syntax is validated if a new SQL query is provided.
Use this for admin-initiated updates that bypass the suggestion workflow.`),
		mcp.WithString("query_id",
			mcp.Required(),
			mcp.Description("UUID of the query to update")),
		mcp.WithString("sql",
			mcp.Description("Updated SQL query with {{parameter}} placeholders")),
		mcp.WithString("name",
			mcp.Description("Updated human-readable name for the query")),
		mcp.WithString("description",
			mcp.Description("Updated description of what business question this query answers")),
		mcp.WithArray("parameters",
			mcp.Description("Updated parameter definitions (array of objects with name, type, description, required, example)")),
		mcp.WithObject("output_column_descriptions",
			mcp.Description("Updated descriptions for output columns (e.g., {\"total\": \"Total amount in USD\"})")),
		mcp.WithArray("tags",
			mcp.Description("Updated tags for organizing queries (e.g., [\"billing\", \"reporting\"])")),
		mcp.WithBoolean("is_enabled",
			mcp.Description("Enable or disable the query")),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check tool access
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_approved_query")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse arguments
		args, _ := request.Params.Arguments.(map[string]any)

		// Parse required query_id
		queryIDStr, ok := args["query_id"].(string)
		if !ok || queryIDStr == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"query_id is required",
				map[string]any{
					"parameter": "query_id",
				}), nil
		}

		queryID, err := uuid.Parse(queryIDStr)
		if err != nil {
			return NewErrorResultWithDetails("invalid_parameters",
				"query_id is not a valid UUID",
				map[string]any{
					"parameter":    "query_id",
					"actual_value": queryIDStr,
				}), nil
		}

		// Get the existing query to verify it exists and get its datasource_id
		existingQuery, err := deps.QueryService.Get(tenantCtx, projectID, queryID)
		if err != nil {
			deps.Logger.Error("Failed to get query for update",
				zap.String("project_id", projectID.String()),
				zap.String("query_id", queryID.String()),
				zap.Error(err))
			return NewErrorResultWithDetails("not_found",
				"query not found",
				map[string]any{
					"query_id": queryIDStr,
				}), nil
		}

		// Build update request - only include fields that were provided
		updateReq := &services.UpdateQueryRequest{}
		var updatedFields []string

		// Parse optional sql
		if sqlQuery, ok := args["sql"].(string); ok && sqlQuery != "" {
			// Validate SQL syntax before update
			validationRes, err := deps.QueryService.Validate(tenantCtx, projectID, existingQuery.DatasourceID, sqlQuery)
			if err != nil {
				deps.Logger.Error("Failed to validate SQL",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
				return NewErrorResultWithDetails("validation_error",
					fmt.Sprintf("failed to validate SQL: %s", err.Error()),
					map[string]any{
						"sql": sqlQuery,
					}), nil
			}

			if !validationRes.Valid {
				return NewErrorResultWithDetails("invalid_sql",
					fmt.Sprintf("invalid SQL: %s", validationRes.Message),
					map[string]any{
						"sql":   sqlQuery,
						"error": validationRes.Message,
					}), nil
			}

			updateReq.SQLQuery = &sqlQuery
			updatedFields = append(updatedFields, "sql")

			// Detect if this is a modifying statement and update allows_modification
			sqlType := services.DetectSQLType(sqlQuery)
			isModifying := services.IsModifyingStatement(sqlType)
			updateReq.AllowsModification = &isModifying
		}

		// Parse optional name
		if name, ok := args["name"].(string); ok && name != "" {
			updateReq.NaturalLanguagePrompt = &name
			updatedFields = append(updatedFields, "name")
		}

		// Parse optional description
		if description, ok := args["description"].(string); ok && description != "" {
			updateReq.AdditionalContext = &description
			updatedFields = append(updatedFields, "description")
		}

		// Parse optional is_enabled
		if isEnabled, ok := args["is_enabled"].(bool); ok {
			updateReq.IsEnabled = &isEnabled
			updatedFields = append(updatedFields, "is_enabled")
		}

		// Parse optional parameters
		if paramsArray, ok := args["parameters"].([]any); ok && len(paramsArray) > 0 {
			paramDefs, err := parseDevQueryParameterDefinitions(paramsArray)
			if err != nil {
				return NewErrorResultWithDetails("invalid_parameters",
					fmt.Sprintf("invalid parameters: %s", err.Error()),
					map[string]any{
						"parameter": "parameters",
					}), nil
			}
			updateReq.Parameters = &paramDefs
			updatedFields = append(updatedFields, "parameters")
		}

		// Parse optional output column descriptions
		if descs, ok := args["output_column_descriptions"].(map[string]any); ok && len(descs) > 0 {
			outputColumns := make([]models.OutputColumn, 0, len(descs))
			for colName, colDesc := range descs {
				if descStr, ok := colDesc.(string); ok {
					outputColumns = append(outputColumns, models.OutputColumn{
						Name:        colName,
						Description: descStr,
					})
				}
			}
			updateReq.OutputColumns = &outputColumns
			updatedFields = append(updatedFields, "output_columns")
		}

		// Parse optional tags
		if tagsArray, ok := args["tags"].([]any); ok {
			tags := make([]string, 0, len(tagsArray))
			for _, tag := range tagsArray {
				if str, ok := tag.(string); ok {
					tags = append(tags, str)
				}
			}
			updateReq.Tags = &tags
			updatedFields = append(updatedFields, "tags")
		}

		// Check if any fields were actually provided for update
		if len(updatedFields) == 0 {
			return NewErrorResultWithDetails("invalid_parameters",
				"at least one field must be provided for update",
				map[string]any{
					"available_fields": []string{"sql", "name", "description", "parameters", "output_column_descriptions", "tags", "is_enabled"},
				}), nil
		}

		// Perform the update via DirectUpdate (no pending record)
		updatedQuery, err := deps.QueryService.DirectUpdate(tenantCtx, projectID, queryID, updateReq)
		if err != nil {
			deps.Logger.Error("Failed to update approved query",
				zap.String("project_id", projectID.String()),
				zap.String("query_id", queryID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("failed to update query: %w", err)
		}

		deps.Logger.Info("Updated approved query",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()),
			zap.Strings("updated_fields", updatedFields),
		)

		response := updateApprovedQueryResponse{
			Success: true,
			Message: fmt.Sprintf("Query updated successfully. Changed fields: %v", updatedFields),
			QueryID: queryID.String(),
			Name:    updatedQuery.NaturalLanguagePrompt,
			Updated: updatedFields,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}

		return mcp.NewToolResultText(string(responseJSON)), nil
	}

	mcpServer.AddTool(tool, handler)
}

// deleteApprovedQueryResponse is the response format for delete_approved_query.
type deleteApprovedQueryResponse struct {
	Success                  bool   `json:"success"`
	Message                  string `json:"message"`
	QueryID                  string `json:"query_id"`
	RejectedSuggestionsCount int    `json:"rejected_suggestions_count"`
}

// registerDeleteApprovedQueryTool registers the delete_approved_query tool.
func registerDeleteApprovedQueryTool(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	tool := mcp.NewTool(
		"delete_approved_query",
		mcp.WithDescription(`Delete a pre-approved query.
The query will be soft-deleted and no longer available for execution.
Any pending update suggestions for this query will be automatically rejected with reason "Original query was deleted".`),
		mcp.WithString("query_id",
			mcp.Required(),
			mcp.Description("UUID of the query to delete")),
	)

	handler := func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check tool access
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "delete_approved_query")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse arguments
		args, _ := request.Params.Arguments.(map[string]any)

		// Parse required query_id
		queryIDStr, ok := args["query_id"].(string)
		if !ok || queryIDStr == "" {
			return NewErrorResultWithDetails("invalid_parameters",
				"query_id is required",
				map[string]any{
					"parameter": "query_id",
				}), nil
		}

		queryID, err := uuid.Parse(queryIDStr)
		if err != nil {
			return NewErrorResultWithDetails("invalid_parameters",
				"query_id is not a valid UUID",
				map[string]any{
					"parameter":    "query_id",
					"actual_value": queryIDStr,
				}), nil
		}

		// Get reviewer ID from context
		reviewerID := auth.GetUserIDFromContext(ctx)
		if reviewerID == "" {
			reviewerID = "mcp-session" // Fallback for MCP sessions without user context
		}

		// Delete the query and auto-reject pending suggestions
		rejectedCount, err := deps.QueryService.DeleteWithPendingRejection(tenantCtx, projectID, queryID, reviewerID)
		if err != nil {
			// Use DEBUG for input errors (not found, etc.), ERROR for server errors
			if IsInputError(err) {
				deps.Logger.Debug("Failed to delete approved query (input error)",
					zap.String("project_id", projectID.String()),
					zap.String("query_id", queryID.String()),
					zap.String("error", err.Error()))
			} else {
				deps.Logger.Error("Failed to delete approved query",
					zap.String("project_id", projectID.String()),
					zap.String("query_id", queryID.String()),
					zap.Error(err))
			}
			return NewErrorResultWithDetails("not_found",
				"query not found",
				map[string]any{
					"query_id": queryIDStr,
				}), nil
		}

		// Build response message
		message := "Query deleted successfully."
		if rejectedCount > 0 {
			message = fmt.Sprintf("Query deleted successfully. %d pending update suggestion(s) were auto-rejected.", rejectedCount)
		}

		deps.Logger.Info("Deleted approved query",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()),
			zap.Int("rejected_suggestions", rejectedCount),
		)

		response := deleteApprovedQueryResponse{
			Success:                  true,
			Message:                  message,
			QueryID:                  queryID.String(),
			RejectedSuggestionsCount: rejectedCount,
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal response: %w", err)
		}

		return mcp.NewToolResultText(string(responseJSON)), nil
	}

	mcpServer.AddTool(tool, handler)
}
