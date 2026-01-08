// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
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

// CheckToolAccess verifies that the specified tool is enabled for the current project.
// This is the shared implementation that all tool-specific check functions should use.
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

	// Check if caller is an agent (API key authentication)
	isAgent := claims.Subject == "agent"

	// Get tool groups state and check access using the unified checker
	state, err := mcpConfig.GetToolGroupsState(tenantCtx, projectID)
	if err != nil {
		scope.Close()
		logger.Error("Failed to get tool groups state",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	// Use the unified ToolAccessChecker for consistent access decisions
	checker := services.NewToolAccessChecker()
	if checker.IsToolAccessible(toolName, state, isAgent) {
		return &ToolAccessResult{
			ProjectID: projectID,
			TenantCtx: tenantCtx,
			Cleanup:   func() { scope.Close() },
		}, nil
	}

	scope.Close()
	return nil, fmt.Errorf("%s tool is not enabled for this project", toolName)
}

// CheckToolAccessWithLegacySignature is a helper that maintains the legacy 4-return-value signature
// for gradual migration. New code should use CheckToolAccess directly.
func CheckToolAccessWithLegacySignature(ctx context.Context, deps ToolAccessDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
	result, err := CheckToolAccess(ctx, deps, toolName)
	if err != nil {
		return uuid.Nil, nil, nil, err
	}
	return result.ProjectID, result.TenantCtx, result.Cleanup, nil
}
