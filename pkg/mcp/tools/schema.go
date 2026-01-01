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
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// SchemaToolDeps contains dependencies for schema context tools.
type SchemaToolDeps struct {
	DB             *database.DB
	ProjectService services.ProjectService
	SchemaService  services.SchemaService
	Logger         *zap.Logger
}

const schemaToolGroup = "schema"

// schemaToolNames lists all tools in the schema group.
var schemaToolNames = map[string]bool{
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
		// Get claims from context
		claims, ok := auth.GetClaims(ctx)
		if !ok {
			return nil, fmt.Errorf("authentication required")
		}

		projectID, err := uuid.Parse(claims.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("invalid project ID: %w", err)
		}

		// Acquire connection with tenant scope
		scope, err := deps.DB.WithTenant(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to acquire database connection: %w", err)
		}
		defer scope.Close()

		// Set tenant context
		tenantCtx := database.SetTenantScope(ctx, scope)

		// Get default datasource
		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}

		// Parse options
		selectedOnly := false
		if val, ok := getOptionalBool(req, "selected_only"); ok {
			selectedOnly = val
		}

		includeEntities := true
		if val, ok := getOptionalBool(req, "include_entities"); ok {
			includeEntities = val
		}

		// Get schema context with or without entity enrichment
		var schemaContext string
		if includeEntities {
			schemaContext, err = deps.SchemaService.GetDatasourceSchemaWithEntities(tenantCtx, projectID, dsID, selectedOnly)
		} else {
			schemaContext, err = deps.SchemaService.GetDatasourceSchemaForPrompt(tenantCtx, projectID, dsID, selectedOnly)
		}
		if err != nil {
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
