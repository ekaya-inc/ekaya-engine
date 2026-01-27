// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ToolAccessDeps defines the common dependencies needed for tool access control.
// All tool dependency structs should embed or satisfy this interface.
type ToolAccessDeps interface {
	GetDB() *database.DB
	GetMCPConfigService() services.MCPConfigService
	GetLogger() *zap.Logger
}

// ToolAccessResult contains the result of a successful access check.
type ToolAccessResult struct {
	ProjectID uuid.UUID
	TenantCtx context.Context
	Cleanup   func()
}

// CheckToolAccess verifies that the specified tool is enabled for the current project and user role.
// This is the shared implementation that all tool-specific check functions should use.
// Tool access is determined by the JWT role:
// - Agents (API key auth) get ComputeAgentTools
// - Admin/Data/Developer roles get ComputeDeveloperTools
// - Regular users get ComputeUserTools
// Returns project ID, tenant-scoped context, cleanup function, and any error.
func CheckToolAccess(ctx context.Context, deps ToolAccessDeps, toolName string) (*ToolAccessResult, error) {
	db := deps.GetDB()
	mcpConfig := deps.GetMCPConfigService()
	logger := deps.GetLogger()

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
	scope, err := db.WithTenant(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire database connection: %w", err)
	}

	// Set tenant context for the query
	tenantCtx := database.SetTenantScope(ctx, scope)

	// Get tool groups state for configuration options
	state, err := mcpConfig.GetToolGroupsState(tenantCtx, projectID)
	if err != nil {
		scope.Close()
		logger.Error("Failed to get tool groups state",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	// Compute enabled tools based on JWT role
	enabledTools := computeToolsForRole(claims, state)

	// Check if the requested tool is in the enabled list
	if isToolInList(toolName, enabledTools) {
		return &ToolAccessResult{
			ProjectID: projectID,
			TenantCtx: tenantCtx,
			Cleanup:   func() { scope.Close() },
		}, nil
	}

	scope.Close()
	return nil, fmt.Errorf("%s tool is not enabled for this project", toolName)
}

// computeToolsForRole determines the tool set based on JWT claims.
// - Agents (Subject == "agent") get limited agent tools
// - Admin/Data/Developer roles get developer tools
// - Regular users get user tools
func computeToolsForRole(claims *auth.Claims, state map[string]*models.ToolGroupConfig) []services.ToolSpec {
	// Check if caller is an agent (API key authentication)
	if claims.Subject == "agent" {
		return services.ComputeAgentTools(state)
	}

	// Check if caller has admin/data/developer role
	if slices.Contains(claims.Roles, models.RoleAdmin) ||
		slices.Contains(claims.Roles, models.RoleData) ||
		slices.Contains(claims.Roles, "developer") {
		return services.ComputeDeveloperTools(state)
	}

	// Regular user gets user tools
	return services.ComputeUserTools(state)
}

// isToolInList checks if a tool name is in the enabled tools list.
func isToolInList(toolName string, tools []services.ToolSpec) bool {
	for _, tool := range tools {
		if tool.Name == toolName {
			return true
		}
	}
	return false
}

// AcquireToolAccess verifies tool access and sets up tenant context for tool execution.
// Also injects MCP provenance context for tracking who performed the operation.
// Returns project ID, tenant-scoped context, cleanup function, and any error.
func AcquireToolAccess(ctx context.Context, deps ToolAccessDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
	result, err := CheckToolAccess(ctx, deps, toolName)
	if err != nil {
		return uuid.Nil, nil, nil, err
	}

	// Inject MCP provenance context
	// Get user ID from claims (may be "agent" for API key auth)
	claims, _ := auth.GetClaims(ctx)
	userID := uuid.Nil
	if claims != nil && claims.Subject != "agent" {
		// Parse user ID for JWT-authenticated requests
		if parsed, err := uuid.Parse(claims.Subject); err == nil {
			userID = parsed
		}
	}

	// Add MCP provenance to the tenant context
	tenantCtxWithProvenance := models.WithMCPProvenance(result.TenantCtx, userID)

	return result.ProjectID, tenantCtxWithProvenance, result.Cleanup, nil
}

// AcquireToolAccessWithoutProvenance verifies tool access without injecting provenance context.
// Use this for read-only tools that don't modify ontology objects.
// Returns project ID, tenant-scoped context, cleanup function, and any error.
func AcquireToolAccessWithoutProvenance(ctx context.Context, deps ToolAccessDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
	result, err := CheckToolAccess(ctx, deps, toolName)
	if err != nil {
		return uuid.Nil, nil, nil, err
	}
	return result.ProjectID, result.TenantCtx, result.Cleanup, nil
}
