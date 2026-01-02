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

// HealthToolDeps contains dependencies for the health tool.
type HealthToolDeps struct {
	DB                *database.DB
	ProjectService    services.ProjectService
	DatasourceService services.DatasourceService
	Logger            *zap.Logger
}

type healthResult struct {
	Engine     string            `json:"engine"`
	Version    string            `json:"version"`
	Datasource *datasourceHealth `json:"datasource,omitempty"`
}

type datasourceHealth struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// RegisterHealthTool adds a health check tool to the MCP server.
// The tool returns server health status, version, and optionally datasource connectivity.
func RegisterHealthTool(s *server.MCPServer, version string, deps *HealthToolDeps) {
	tool := mcp.NewTool(
		"health",
		mcp.WithDescription("Returns server health status, version, and datasource connectivity"),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result := healthResult{
			Engine:  "healthy",
			Version: version,
		}

		// Check datasource health if deps are available
		if deps != nil && deps.ProjectService != nil && deps.DatasourceService != nil {
			dsHealth := checkDatasourceHealth(ctx, deps)
			if dsHealth != nil {
				result.Datasource = dsHealth
			}
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal health result: %w", err)
		}
		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// checkDatasourceHealth tests connectivity to the project's default datasource.
func checkDatasourceHealth(ctx context.Context, deps *HealthToolDeps) *datasourceHealth {
	// Get claims from context
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return &datasourceHealth{
			Status: "error",
			Error:  "authentication required",
		}
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return &datasourceHealth{
			Status: "error",
			Error:  fmt.Sprintf("invalid project ID: %v", err),
		}
	}

	// Acquire tenant-scoped database connection
	// The DB field is optional for backward compatibility with tests using mocks
	if deps.DB != nil {
		scope, err := deps.DB.WithTenant(ctx, projectID)
		if err != nil {
			deps.Logger.Error("Failed to acquire tenant connection",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return &datasourceHealth{
				Status: "error",
				Error:  fmt.Sprintf("failed to acquire database connection: %v", err),
			}
		}
		defer scope.Close()
		ctx = database.SetTenantScope(ctx, scope)
	}

	// Get default datasource ID for project
	datasourceID, err := deps.ProjectService.GetDefaultDatasourceID(ctx, projectID)
	if err != nil {
		deps.Logger.Error("Failed to get default datasource ID",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return &datasourceHealth{
			Status: "error",
			Error:  fmt.Sprintf("failed to get default datasource: %v", err),
		}
	}

	if datasourceID == uuid.Nil {
		return &datasourceHealth{
			Status: "not_configured",
			Error:  "no default datasource configured for project",
		}
	}

	// Get datasource details
	ds, err := deps.DatasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		deps.Logger.Error("Failed to get datasource",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return &datasourceHealth{
			Status: "error",
			Error:  fmt.Sprintf("failed to get datasource: %v", err),
		}
	}

	// Test connection
	err = deps.DatasourceService.TestConnection(ctx, ds.DatasourceType, ds.Config)
	if err != nil {
		deps.Logger.Warn("Datasource connection test failed",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.String("datasource_name", ds.Name),
			zap.Error(err))
		return &datasourceHealth{
			Name:   ds.Name,
			Type:   ds.DatasourceType,
			Status: "error",
			Error:  err.Error(),
		}
	}

	return &datasourceHealth{
		Name:   ds.Name,
		Type:   ds.DatasourceType,
		Status: "connected",
	}
}
