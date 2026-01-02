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
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// OntologyToolDeps contains dependencies for ontology tools.
type OntologyToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	ProjectService   services.ProjectService
	OntologyRepo     repositories.OntologyRepository
	EntityRepo       repositories.OntologyEntityRepository
	SchemaRepo       repositories.SchemaRepository
	Logger           *zap.Logger
}

const ontologyToolGroup = "approved_queries" // Ontology tools share visibility with approved_queries

// RegisterOntologyTools registers ontology-related MCP tools.
func RegisterOntologyTools(s *server.MCPServer, deps *OntologyToolDeps) {
	registerGetOntologyTool(s, deps)
}

// checkOntologyToolsEnabled verifies ontology tools are enabled for the project.
// Ontology tools share visibility with approved_queries tools.
func checkOntologyToolsEnabled(ctx context.Context, deps *OntologyToolDeps) (uuid.UUID, context.Context, func(), error) {
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

	// Check if approved_queries tool group is enabled (ontology tools share this visibility)
	showOntologyTools, err := deps.MCPConfigService.ShouldShowApprovedQueriesTools(tenantCtx, projectID)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to check ontology tools visibility",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	if !showOntologyTools {
		scope.Close()
		return uuid.Nil, nil, nil, fmt.Errorf("ontology tools are not enabled for this project")
	}

	return projectID, tenantCtx, func() { scope.Close() }, nil
}

// getStringSlice extracts a slice of strings from request arguments.
func getStringSlice(req mcp.CallToolRequest, key string) []string {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return nil
	}
	val, ok := args[key].([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(val))
	for _, item := range val {
		if str, ok := item.(string); ok {
			result = append(result, str)
		}
	}
	return result
}

// getOptionalBoolWithDefault extracts an optional boolean argument with a default value.
func getOptionalBoolWithDefault(req mcp.CallToolRequest, key string, defaultVal bool) bool {
	if val, ok := getOptionalBool(req, key); ok {
		return val
	}
	return defaultVal
}

// registerGetOntologyTool adds the get_ontology tool for structured ontology exploration.
func registerGetOntologyTool(s *server.MCPServer, deps *OntologyToolDeps) {
	tool := mcp.NewTool(
		"get_ontology",
		mcp.WithDescription(
			"Get structured ontology information at configurable depth levels. "+
				"Progressive disclosure: start with 'domain' for high-level context, "+
				"then drill down to 'entities', 'tables', or 'columns' as needed. "+
				"Use 'tables' parameter to filter specific tables at 'tables' or 'columns' depth.",
		),
		mcp.WithString(
			"depth",
			mcp.Required(),
			mcp.Description("Depth level: 'domain' (high-level), 'entities' (entity summaries), 'tables' (table details), or 'columns' (full column details)"),
		),
		mcp.WithArray(
			"tables",
			mcp.Description("Optional: filter to specific tables (for 'tables' or 'columns' depth)"),
		),
		mcp.WithBoolean(
			"include_relationships",
			mcp.Description("Include relationship graph (default: true)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkOntologyToolsEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse required depth parameter
		depth, err := req.RequireString("depth")
		if err != nil {
			return nil, err
		}

		// Validate depth value
		validDepths := map[string]bool{
			"domain":  true,
			"entities": true,
			"tables":  true,
			"columns": true,
		}
		if !validDepths[depth] {
			return nil, fmt.Errorf("invalid depth: must be one of 'domain', 'entities', 'tables', 'columns'")
		}

		// Parse optional parameters
		tables := getStringSlice(req, "tables")
		includeRelationships := getOptionalBoolWithDefault(req, "include_relationships", true)

		// Get the active ontology
		ontology, err := deps.OntologyRepo.GetActive(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get ontology: %w", err)
		}

		// Handle case where no ontology has been extracted yet
		if ontology == nil {
			result := map[string]any{
				"error":        nil,
				"has_ontology": false,
				"message":      "Ontology not yet extracted. Use get_schema for raw schema information.",
				"domain":       nil,
				"entities":     []any{},
			}
			jsonResult, _ := json.Marshal(result)
			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Route to appropriate handler based on depth
		var result any
		switch depth {
		case "domain":
			result, err = handleDomainDepth(tenantCtx, deps, ontology, includeRelationships)
		case "entities":
			result, err = handleEntitiesDepth(tenantCtx, deps, ontology, includeRelationships)
		case "tables":
			result, err = handleTablesDepth(tenantCtx, deps, ontology, tables, includeRelationships)
		case "columns":
			result, err = handleColumnsDepth(tenantCtx, deps, ontology, tables)
		default:
			return nil, fmt.Errorf("unexpected depth value: %s", depth)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to get ontology at depth '%s': %w", depth, err)
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// handleDomainDepth returns high-level domain context.
func handleDomainDepth(ctx context.Context, deps *OntologyToolDeps, ontology any, includeRelationships bool) (any, error) {
	// TODO: Implement domain depth handler (will be implemented in subsequent steps)
	return map[string]any{
		"message": "Domain depth handler not yet implemented",
	}, nil
}

// handleEntitiesDepth returns entity summaries with occurrences.
func handleEntitiesDepth(ctx context.Context, deps *OntologyToolDeps, ontology any, includeRelationships bool) (any, error) {
	// TODO: Implement entities depth handler (will be implemented in subsequent steps)
	return map[string]any{
		"message": "Entities depth handler not yet implemented",
	}, nil
}

// handleTablesDepth returns table-level summaries with column overview.
func handleTablesDepth(ctx context.Context, deps *OntologyToolDeps, ontology any, tables []string, includeRelationships bool) (any, error) {
	// TODO: Implement tables depth handler (will be implemented in subsequent steps)
	return map[string]any{
		"message": "Tables depth handler not yet implemented",
	}, nil
}

// handleColumnsDepth returns full column details for specified tables.
func handleColumnsDepth(ctx context.Context, deps *OntologyToolDeps, ontology any, tables []string) (any, error) {
	// TODO: Implement columns depth handler (will be implemented in subsequent steps)
	return map[string]any{
		"message": "Columns depth handler not yet implemented",
	}, nil
}
