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
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// DevQueryToolDeps defines dependencies for dev query MCP tools.
type DevQueryToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	ProjectService   services.ProjectService
	QueryService     services.QueryService
	Logger           *zap.Logger
}

// GetDB returns the database connection.
func (d *DevQueryToolDeps) GetDB() *database.DB {
	return d.DB
}

// GetMCPConfigService returns the MCP config service.
func (d *DevQueryToolDeps) GetMCPConfigService() services.MCPConfigService {
	return d.MCPConfigService
}

// GetLogger returns the logger.
func (d *DevQueryToolDeps) GetLogger() *zap.Logger {
	return d.Logger
}

// RegisterDevQueryTools registers the dev query management tools with the MCP server.
// These tools are for administrators to manage query suggestions.
func RegisterDevQueryTools(mcpServer *server.MCPServer, deps *DevQueryToolDeps) {
	registerListQuerySuggestionsTool(mcpServer, deps)
	registerApproveQuerySuggestionTool(mcpServer, deps)
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
