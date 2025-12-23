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
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// DeveloperToolDeps contains dependencies for developer tools.
type DeveloperToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	Logger           *zap.Logger
}

const developerToolGroup = "developer"

// RegisterDeveloperTools registers the developer tool group tools.
// These tools are only accessible when the developer tool group is enabled.
func RegisterDeveloperTools(s *server.MCPServer, deps *DeveloperToolDeps) {
	registerEchoTool(s, deps)
}

// checkDeveloperEnabled verifies the developer tool group is enabled for the project.
// Returns the project ID and a tenant-scoped context if enabled, or an error if not.
func checkDeveloperEnabled(ctx context.Context, deps *DeveloperToolDeps) (uuid.UUID, context.Context, func(), error) {
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

	// Check if developer tool group is enabled
	enabled, err := deps.MCPConfigService.IsToolGroupEnabled(tenantCtx, projectID, developerToolGroup)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to check developer tool group",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool group configuration: %w", err)
	}

	if !enabled {
		scope.Close()
		return uuid.Nil, nil, nil, fmt.Errorf("developer tools are not enabled for this project")
	}

	return projectID, tenantCtx, func() { scope.Close() }, nil
}

// registerEchoTool adds a simple echo tool for testing the developer tool group.
// This tool verifies that authentication and tool group configuration work correctly.
func registerEchoTool(s *server.MCPServer, deps *DeveloperToolDeps) {
	tool := mcp.NewTool(
		"echo",
		mcp.WithDescription("Echo back the input message (developer tool for testing)"),
		mcp.WithString(
			"message",
			mcp.Required(),
			mcp.Description("The message to echo back"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Check if developer tools are enabled
		_, _, cleanup, err := checkDeveloperEnabled(ctx, deps)
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get the message argument
		message, err := req.RequireString("message")
		if err != nil {
			return nil, err
		}

		result, err := json.Marshal(map[string]string{
			"echo":   message,
			"status": "developer tools enabled",
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(result)), nil
	})
}
