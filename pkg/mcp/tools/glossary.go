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

// GlossaryToolDeps contains dependencies for glossary tools.
type GlossaryToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	GlossaryService  services.GlossaryService
	Logger           *zap.Logger
}

// RegisterGlossaryTools registers glossary-related MCP tools.
func RegisterGlossaryTools(s *server.MCPServer, deps *GlossaryToolDeps) {
	registerListGlossaryTool(s, deps)
	registerGetGlossarySQLTool(s, deps)
}

// checkGlossaryToolEnabled verifies a specific glossary tool is enabled for the project.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
func checkGlossaryToolEnabled(ctx context.Context, deps *GlossaryToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	// Use the unified ToolAccessChecker for consistent access decisions
	checker := services.NewToolAccessChecker()
	if checker.IsToolAccessible(toolName, state, isAgent) {
		return projectID, tenantCtx, func() { scope.Close() }, nil
	}

	scope.Close()
	return uuid.Nil, nil, nil, fmt.Errorf("%s tool is not enabled for this project", toolName)
}

// registerListGlossaryTool adds the list_glossary tool for discovering available business terms.
// Returns lightweight list of terms with definitions and aliases for discovery.
func registerListGlossaryTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"list_glossary",
		mcp.WithDescription(
			"List all business glossary terms for the project. "+
				"Returns term names, definitions, and aliases for discovery. "+
				"Use this to explore available business terms like 'Revenue', 'Active User', 'GMV', etc. "+
				"To get the SQL definition for a specific term, use get_glossary_sql.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkGlossaryToolEnabled(ctx, deps, "list_glossary")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get all glossary terms for the project
		terms, err := deps.GlossaryService.GetTerms(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get glossary terms: %w", err)
		}

		// Build lightweight response for discovery
		result := struct {
			Terms []listGlossaryResponse `json:"terms"`
			Count int                    `json:"count"`
		}{
			Terms: make([]listGlossaryResponse, 0, len(terms)),
			Count: len(terms),
		}

		for _, term := range terms {
			result.Terms = append(result.Terms, toListGlossaryResponse(term))
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerGetGlossarySQLTool adds the get_glossary_sql tool for retrieving SQL definitions.
// Accepts term name or alias and returns full entry with defining_sql for query composition.
func registerGetGlossarySQLTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"get_glossary_sql",
		mcp.WithDescription(
			"Get the SQL definition for a specific business term. "+
				"Accepts term name or alias (e.g., 'Revenue' or 'Total Revenue'). "+
				"Returns the complete SQL definition, output columns, and metadata needed for query composition. "+
				"Use list_glossary first to discover available terms.",
		),
		mcp.WithString(
			"term",
			mcp.Required(),
			mcp.Description("Business term name or alias to look up"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkGlossaryToolEnabled(ctx, deps, "get_glossary_sql")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get term parameter
		termName, err := req.RequireString("term")
		if err != nil {
			return nil, err
		}

		// Look up term by name or alias
		term, err := deps.GlossaryService.GetTermByName(tenantCtx, projectID, termName)
		if err != nil {
			return nil, fmt.Errorf("failed to get glossary term: %w", err)
		}

		// Handle not found case gracefully
		if term == nil {
			notFoundResult := struct {
				Error string `json:"error"`
				Term  string `json:"term"`
			}{
				Error: "Term not found",
				Term:  termName,
			}

			jsonResult, err := json.Marshal(notFoundResult)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %w", err)
			}

			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Build full response with SQL definition
		result := toGetGlossarySQLResponse(term)

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// listGlossaryResponse is the lightweight response format for list_glossary tool.
// Used for discovery - only includes term, definition, and aliases.
type listGlossaryResponse struct {
	Term       string   `json:"term"`
	Definition string   `json:"definition"`
	Aliases    []string `json:"aliases,omitempty"`
}

// getGlossarySQLResponse is the full response format for get_glossary_sql tool.
// Includes all fields needed for query composition.
type getGlossarySQLResponse struct {
	Term          string                `json:"term"`
	Definition    string                `json:"definition"`
	DefiningSQL   string                `json:"defining_sql"`
	BaseTable     string                `json:"base_table,omitempty"`
	OutputColumns []models.OutputColumn `json:"output_columns,omitempty"`
	Aliases       []string              `json:"aliases,omitempty"`
}

// toListGlossaryResponse converts a model to list_glossary response format.
func toListGlossaryResponse(term *models.BusinessGlossaryTerm) listGlossaryResponse {
	return listGlossaryResponse{
		Term:       term.Term,
		Definition: term.Definition,
		Aliases:    term.Aliases,
	}
}

// toGetGlossarySQLResponse converts a model to get_glossary_sql response format.
func toGetGlossarySQLResponse(term *models.BusinessGlossaryTerm) getGlossarySQLResponse {
	return getGlossarySQLResponse{
		Term:          term.Term,
		Definition:    term.Definition,
		DefiningSQL:   term.DefiningSQL,
		BaseTable:     term.BaseTable,
		OutputColumns: term.OutputColumns,
		Aliases:       term.Aliases,
	}
}
