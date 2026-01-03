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
	registerGetGlossaryTool(s, deps)
}

// checkGlossaryToolsEnabled verifies glossary tools are enabled for the project.
// Glossary tools share visibility with approved_queries/ontology tools.
func checkGlossaryToolsEnabled(ctx context.Context, deps *GlossaryToolDeps) (uuid.UUID, context.Context, func(), error) {
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

	// Check if approved_queries tool group is enabled (glossary tools share this visibility)
	showGlossaryTools, err := deps.MCPConfigService.ShouldShowApprovedQueriesTools(tenantCtx, projectID)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to check glossary tools visibility",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	if !showGlossaryTools {
		scope.Close()
		return uuid.Nil, nil, nil, fmt.Errorf("glossary tools are not enabled for this project")
	}

	return projectID, tenantCtx, func() { scope.Close() }, nil
}

// registerGetGlossaryTool adds the get_glossary tool for business term lookup.
func registerGetGlossaryTool(s *server.MCPServer, deps *GlossaryToolDeps) {
	tool := mcp.NewTool(
		"get_glossary",
		mcp.WithDescription(
			"Get business glossary terms for the project. "+
				"Returns metric definitions, calculations, and SQL patterns. "+
				"Use this to understand business terms like 'Revenue', 'Active User', 'GMV', etc. "+
				"Each term includes the SQL pattern and filters needed to calculate it.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkGlossaryToolsEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get all glossary terms for the project
		terms, err := deps.GlossaryService.GetTerms(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get glossary terms: %w", err)
		}

		// Build response
		result := struct {
			Terms []glossaryTermResponse `json:"terms"`
			Count int                    `json:"count"`
		}{
			Terms: make([]glossaryTermResponse, 0, len(terms)),
			Count: len(terms),
		}

		for _, term := range terms {
			result.Terms = append(result.Terms, toGlossaryTermResponse(term))
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// glossaryTermResponse is the MCP response format for a glossary term.
// Excludes internal fields like ID, ProjectID, CreatedBy, timestamps.
type glossaryTermResponse struct {
	Term        string           `json:"term"`
	Definition  string           `json:"definition"`
	SQLPattern  string           `json:"sql_pattern,omitempty"`
	BaseTable   string           `json:"base_table,omitempty"`
	ColumnsUsed []string         `json:"columns_used,omitempty"`
	Filters     []filterResponse `json:"filters,omitempty"`
	Aggregation string           `json:"aggregation,omitempty"`
	Source      string           `json:"source"`
}

// filterResponse is the MCP response format for a filter condition.
type filterResponse struct {
	Column   string   `json:"column"`
	Operator string   `json:"operator"`
	Values   []string `json:"values,omitempty"`
}

// toGlossaryTermResponse converts a model to MCP response format.
func toGlossaryTermResponse(term *models.BusinessGlossaryTerm) glossaryTermResponse {
	resp := glossaryTermResponse{
		Term:        term.Term,
		Definition:  term.Definition,
		SQLPattern:  term.SQLPattern,
		BaseTable:   term.BaseTable,
		ColumnsUsed: term.ColumnsUsed,
		Aggregation: term.Aggregation,
		Source:      term.Source,
	}

	if len(term.Filters) > 0 {
		resp.Filters = make([]filterResponse, 0, len(term.Filters))
		for _, f := range term.Filters {
			resp.Filters = append(resp.Filters, filterResponse{
				Column:   f.Column,
				Operator: f.Operator,
				Values:   f.Values,
			})
		}
	}

	return resp
}
