package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// SchemaToolDeps contains dependencies for schema context tools.
type SchemaToolDeps struct {
	BaseMCPToolDeps
	ProjectService services.ProjectService
	SchemaService  services.SchemaService
}

// SchemaToolNames lists all tools in the schema group.
var SchemaToolNames = map[string]bool{
	"get_schema": true,
}

// RegisterSchemaTools registers tools for schema context with entity semantics.
func RegisterSchemaTools(s *server.MCPServer, deps *SchemaToolDeps) {
	registerGetSchemaContextTool(s, deps)
}

// registerGetSchemaContextTool exposes database schema with entity/role annotations for text2sql.
func registerGetSchemaContextTool(s *server.MCPServer, deps *SchemaToolDeps) {
	tool := mcp.NewTool(
		"get_schema",
		mcp.WithDescription(
			"Get database schema with entity/role semantic information for intelligent query generation. "+
				"This includes entity names (user, account, order) and roles (visitor, host, owner) for columns that represent domain entities. "+
				"For example, visits.host_id represents entity 'user' with role 'host' vs visits.visitor_id as 'user' with role 'visitor'. "+
				"Use this to understand the semantic meaning of foreign keys and generate accurate SQL joins.",
		),
		mcp.WithBoolean(
			"selected_only",
			mcp.Description("If true, only return selected tables/columns (default: false)"),
		),
		mcp.WithBoolean(
			"include_entities",
			mcp.Description("If true, include entity/role annotations (default: true). Set to false for standard schema without semantics."),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check if get_schema tool is enabled using unified access checker
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "get_schema")
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

		// Parse and validate options
		selectedOnly := false
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if val, exists := args["selected_only"]; exists {
				boolVal, ok := val.(bool)
				if !ok {
					return NewErrorResultWithDetails(
						"invalid_parameters",
						fmt.Sprintf("parameter 'selected_only' must be a boolean, got %T", val),
						map[string]any{
							"parameter":     "selected_only",
							"expected_type": "boolean",
							"actual_type":   fmt.Sprintf("%T", val),
						},
					), nil
				}
				selectedOnly = boolVal
			}
		}

		includeEntities := true
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if val, exists := args["include_entities"]; exists {
				boolVal, ok := val.(bool)
				if !ok {
					return NewErrorResultWithDetails(
						"invalid_parameters",
						fmt.Sprintf("parameter 'include_entities' must be a boolean, got %T", val),
						map[string]any{
							"parameter":     "include_entities",
							"expected_type": "boolean",
							"actual_type":   fmt.Sprintf("%T", val),
						},
					), nil
				}
				includeEntities = boolVal
			}
		}

		// Get schema context with or without entity enrichment
		var schemaContext string
		if includeEntities {
			schemaContext, err = deps.SchemaService.GetDatasourceSchemaWithEntities(tenantCtx, projectID, dsID, selectedOnly)
		} else {
			schemaContext, err = deps.SchemaService.GetDatasourceSchemaForPrompt(tenantCtx, projectID, dsID, selectedOnly)
		}
		if err != nil {
			// Check if this is a "no ontology" error - actionable
			if isOntologyNotFoundError(err) {
				return NewErrorResult(
					"ontology_not_found",
					"no active ontology found for project - cannot provide semantic annotations. Use include_entities=false for raw schema, or extract ontology first",
				), nil
			}
			// System errors (database connection, timeout, etc.) remain as Go errors
			return nil, fmt.Errorf("failed to get schema context: %w", err)
		}

		// Format response
		response := struct {
			SchemaContext    string `json:"schema_context"`
			ProjectID        string `json:"project_id"`
			DatasourceID     string `json:"datasource_id"`
			SelectedOnly     bool   `json:"selected_only"`
			IncludesEntities bool   `json:"includes_entities"`
		}{
			SchemaContext:    schemaContext,
			ProjectID:        projectID.String(),
			DatasourceID:     dsID.String(),
			SelectedOnly:     selectedOnly,
			IncludesEntities: includeEntities,
		}

		jsonResult, _ := json.Marshal(response)
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// getOptionalBool extracts an optional boolean parameter from the request.
func getOptionalBool(req mcp.CallToolRequest, key string) (bool, bool) {
	if args, ok := req.Params.Arguments.(map[string]any); ok {
		if val, ok := args[key].(bool); ok {
			return val, true
		}
	}
	return false, false
}

// isOntologyNotFoundError checks if an error is related to missing ontology.
func isOntologyNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "no active ontology") ||
		strings.Contains(errMsg, "ontology not found")
}
