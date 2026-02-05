// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ToolAccessError represents an actionable error that should be returned as a
// JSON success response to the MCP client, not as a Go error. Per CLAUDE.md
// Rule #6, actionable errors (authentication, invalid parameters, tool not
// enabled) must be returned as JSON so the LLM can see and act on them.
type ToolAccessError struct {
	Code    string
	Message string
	// MCPResult contains the pre-built MCP response for this error
	MCPResult *mcp.CallToolResult
}

func (e *ToolAccessError) Error() string {
	return e.Message
}

// IsToolAccessError checks if an error is a ToolAccessError and returns it.
// Use this in tool handlers to convert actionable errors to JSON responses:
//
//	projectID, ctx, cleanup, err := AcquireToolAccess(ctx, deps, "my_tool")
//	if err != nil {
//	    if result := AsToolAccessResult(err); result != nil {
//	        return result, nil
//	    }
//	    return nil, err
//	}
func AsToolAccessResult(err error) *mcp.CallToolResult {
	var accessErr *ToolAccessError
	if errors.As(err, &accessErr) {
		return accessErr.MCPResult
	}
	return nil
}

// newToolAccessError creates a ToolAccessError with the given code and message.
func newToolAccessError(code, message string) *ToolAccessError {
	return &ToolAccessError{
		Code:      code,
		Message:   message,
		MCPResult: NewErrorResult(code, message),
	}
}

// ToolAccessDeps defines the common dependencies needed for tool access control.
// All tool dependency structs should embed or satisfy this interface.
type ToolAccessDeps interface {
	GetDB() *database.DB
	GetMCPConfigService() services.MCPConfigService
	GetLogger() *zap.Logger
}

// BaseMCPToolDeps provides the common dependencies that all MCP tools need.
// Tool-specific *Deps structs should embed this to avoid repeating the
// GetDB/GetMCPConfigService/GetLogger method implementations.
type BaseMCPToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	Logger           *zap.Logger
}

// GetDB implements ToolAccessDeps.
func (d *BaseMCPToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *BaseMCPToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }

// GetLogger implements ToolAccessDeps.
func (d *BaseMCPToolDeps) GetLogger() *zap.Logger { return d.Logger }

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
//
// Returns project ID, tenant-scoped context, cleanup function, and any error.
// Per CLAUDE.md Rule #6, actionable errors (authentication, invalid parameters, tool not enabled)
// are returned as ToolAccessError which can be converted to JSON MCP responses using AsToolAccessResult.
// System errors (database failures) are returned as regular Go errors.
func CheckToolAccess(ctx context.Context, deps ToolAccessDeps, toolName string) (*ToolAccessResult, error) {
	db := deps.GetDB()
	mcpConfig := deps.GetMCPConfigService()
	logger := deps.GetLogger()

	// Get claims from context
	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return nil, newToolAccessError("authentication_required", "authentication required")
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return nil, newToolAccessError("invalid_project_id", fmt.Sprintf("invalid project ID: %v", err))
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
	return nil, newToolAccessError("tool_not_enabled", fmt.Sprintf("%s tool is not enabled for this project", toolName))
}

// computeToolsForRole determines the tool set based on JWT claims.
// Uses ComputeEnabledToolsFromConfig to ensure consistency with NewToolFilter's listing behavior.
func computeToolsForRole(claims *auth.Claims, state map[string]*models.ToolGroupConfig) []services.ToolSpec {
	// Check if caller is an agent (API key authentication)
	isAgent := claims.Subject == "agent"

	// Use the same function as the tool filter to ensure listing and calling are consistent
	return services.ComputeEnabledToolsFromConfig(state, isAgent)
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
