// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// OntologyToolDeps contains dependencies for ontology tools.
type OntologyToolDeps struct {
	BaseMCPToolDeps
	ProjectService         services.ProjectService
	OntologyContextService services.OntologyContextService
	OntologyRepo           repositories.OntologyRepository
	SchemaRepo             repositories.SchemaRepository
}

// RegisterOntologyTools registers ontology-related MCP tools.
func RegisterOntologyTools(s *server.MCPServer, deps *OntologyToolDeps) {
	registerGetOntologyTool(s, deps)
}

// getStringSlice extracts a slice of strings from request arguments.
// Returns (nil, nil) if the key is absent; returns an error if the value is malformed.
func getStringSlice(req mcp.CallToolRequest, key string) ([]string, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return nil, nil
	}
	return extractStringSlice(args, key, nil)
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
			mcp.WithStringItems(),
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
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "get_ontology")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Parse required depth parameter
		depth, err := req.RequireString("depth")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}

		// Validate depth value
		validDepths := map[string]bool{
			"domain":   true,
			"entities": true,
			"tables":   true,
			"columns":  true,
		}
		if !validDepths[depth] {
			return NewErrorResult(
				"invalid_parameters",
				fmt.Sprintf("invalid depth '%s': must be one of 'domain', 'entities', 'tables', 'columns'", depth),
			), nil
		}

		// Parse optional parameters
		tables, parseErr := getStringSlice(req, "tables")
		if parseErr != nil {
			return NewErrorResult("invalid_parameters", parseErr.Error()), nil
		}
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
			result, err = handleDomainDepth(tenantCtx, deps, projectID, includeRelationships)
		case "entities":
			result, err = handleEntitiesDepth(tenantCtx, deps, projectID, includeRelationships)
		case "tables":
			result, err = handleTablesDepth(tenantCtx, deps, projectID, tables, includeRelationships)
		case "columns":
			result, err = handleColumnsDepth(tenantCtx, deps, projectID, tables)
		default:
			return NewErrorResult("invalid_parameters",
				fmt.Sprintf("unexpected depth value: %s; valid values are: domain, entities, tables, columns", depth)), nil
		}

		if err != nil {
			return nil, fmt.Errorf("failed to get ontology at depth '%s': %w", depth, err)
		}

		// Check if result is already a CallToolResult (error result from handleColumnsDepth)
		if toolResult, ok := result.(*mcp.CallToolResult); ok {
			return toolResult, nil
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// handleDomainDepth returns high-level domain context.
// Note: Relationships have been removed for v1.0 entity simplification.
func handleDomainDepth(ctx context.Context, deps *OntologyToolDeps, projectID uuid.UUID, includeRelationships bool) (any, error) {
	result, err := deps.OntologyContextService.GetDomainContext(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain context: %w", err)
	}

	return result, nil
}

// handleEntitiesDepth returns entity summaries with occurrences.
// Note: Entity-level depth has been removed for v1.0 entity simplification.
// This function now returns domain-level context as a fallback.
func handleEntitiesDepth(ctx context.Context, deps *OntologyToolDeps, projectID uuid.UUID, includeRelationships bool) (any, error) {
	// Entity functionality removed - fall back to domain context
	result, err := deps.OntologyContextService.GetDomainContext(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get domain context: %w", err)
	}

	return result, nil
}

// handleTablesDepth returns table-level summaries with column overview.
func handleTablesDepth(ctx context.Context, deps *OntologyToolDeps, projectID uuid.UUID, tables []string, includeRelationships bool) (any, error) {
	result, err := deps.OntologyContextService.GetTablesContext(ctx, projectID, tables)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables context: %w", err)
	}

	return result, nil
}

// handleColumnsDepth returns full column details for specified tables.
func handleColumnsDepth(ctx context.Context, deps *OntologyToolDeps, projectID uuid.UUID, tables []string) (any, error) {
	// Check for actionable parameter errors before calling service
	if len(tables) == 0 {
		return NewErrorResult(
			"invalid_parameters",
			"table names required for columns depth - please specify which tables to retrieve column details for",
		), nil
	}
	if len(tables) > services.MaxColumnsDepthTables {
		return NewErrorResultWithDetails(
			"invalid_parameters",
			fmt.Sprintf("too many tables requested: maximum %d tables allowed for columns depth", services.MaxColumnsDepthTables),
			map[string]any{
				"requested_count": len(tables),
				"max_allowed":     services.MaxColumnsDepthTables,
			},
		), nil
	}

	result, err := deps.OntologyContextService.GetColumnsContext(ctx, projectID, tables)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns context: %w", err)
	}

	return result, nil
}
