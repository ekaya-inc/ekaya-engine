// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	GetInstalledAppService() services.InstalledAppService
}

// BaseMCPToolDeps provides the common dependencies that all MCP tools need.
// Tool-specific *Deps structs should embed this to avoid repeating the
// GetDB/GetMCPConfigService/GetLogger method implementations.
type BaseMCPToolDeps struct {
	DB                  *database.DB
	MCPConfigService    services.MCPConfigService
	Logger              *zap.Logger
	InstalledAppService services.InstalledAppService
}

// GetDB implements ToolAccessDeps.
func (d *BaseMCPToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *BaseMCPToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }

// GetLogger implements ToolAccessDeps.
func (d *BaseMCPToolDeps) GetLogger() *zap.Logger { return d.Logger }

// GetInstalledAppService implements ToolAccessDeps.
func (d *BaseMCPToolDeps) GetInstalledAppService() services.InstalledAppService {
	return d.InstalledAppService
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
// - Admin/Data roles get the union of enabled developer and user toggles
// - Regular users get only user-toggle tools
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
	if !isToolInList(toolName, enabledTools) {
		scope.Close()
		return nil, newToolAccessError("tool_not_enabled", fmt.Sprintf("%s tool is not enabled for this project", toolName))
	}

	// Check if the required app is installed for this tool
	if err := checkAppInstallation(tenantCtx, deps, claims, toolName, projectID, state); err != nil {
		scope.Close()
		return nil, err
	}

	return &ToolAccessResult{
		ProjectID: projectID,
		TenantCtx: tenantCtx,
		Cleanup:   func() { scope.Close() },
	}, nil
}

// computeToolsForRole determines the tool set based on JWT claims.
// Agent auth gets agent tools, user role gets limited tools, admin/data get full developer tools.
func computeToolsForRole(claims *auth.Claims, state map[string]*models.ToolGroupConfig) []services.ToolSpec {
	// Agent authentication (API key)
	if auth.IsAgentClaims(claims) {
		return services.ComputeEnabledToolsFromConfig(state, true)
	}

	// Determine the user's effective role
	role := effectiveRole(claims)

	// User role: only tools exposed by user-facing toggles.
	if role == models.RoleUser {
		return services.ComputeUserTools(state)
	}

	// Admin and Data roles get the union of enabled developer and user toggles.
	return services.ComputeEnabledToolsFromConfig(state, false)
}

// effectiveRole returns the highest-privilege role from the user's JWT claims.
// Privilege order: admin > data > user.
// Falls back to "user" (least privilege) if no roles are present.
// This ensures that if a JWT contains multiple roles (e.g., ["user", "admin"]),
// the user gets the access level of their most privileged role — consistent
// with how RequireRole checks any matching role in the HTTP middleware.
func effectiveRole(claims *auth.Claims) string {
	if len(claims.Roles) == 0 {
		return models.RoleUser
	}

	// Privilege ranking: admin > data > user
	rolePriority := map[string]int{
		models.RoleAdmin: 2,
		models.RoleData:  1,
		models.RoleUser:  0,
	}

	best := models.RoleUser
	bestPriority := 0
	for _, role := range claims.Roles {
		if p, ok := rolePriority[role]; ok && p > bestPriority {
			best = role
			bestPriority = p
		}
	}
	return best
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

// checkAppInstallation verifies that at least one enabled ownership path for
// the tool is installed. This keeps execution-time app gating aligned with the
// same toggle registry used by the UI/tool inventory and preserves shared tools
// such as query, which can be exposed by more than one app path.
// Returns nil if access is allowed, or a ToolAccessError if the app is not installed.
// If InstalledAppService is nil (e.g. in tests), access is allowed (backward compat).
func checkAppInstallation(
	ctx context.Context,
	deps ToolAccessDeps,
	claims *auth.Claims,
	toolName string,
	projectID uuid.UUID,
	state map[string]*models.ToolGroupConfig,
) error {
	appService := deps.GetInstalledAppService()
	if appService == nil {
		return nil // No app service available, allow access (backward compat for tests)
	}

	// Agent auth requires ai-agents app to be installed
	if auth.IsAgentClaims(claims) {
		installed, err := appService.IsInstalled(ctx, projectID, models.AppIDAIAgents)
		if err != nil {
			deps.GetLogger().Error("Failed to check ai-agents app installation",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Fail closed: deny access on error
			return newToolAccessError("app_not_installed", "ai-agents app installation check failed")
		}
		if !installed {
			return newToolAccessError("app_not_installed", "ai-agents app is not installed for this project")
		}
		return nil
	}

	requiredAppIDs := services.GetEnabledToolOwningAppIDs(toolName, state)
	if len(requiredAppIDs) == 0 {
		requiredAppIDs = services.GetToolOwningAppIDs(toolName)
	}

	if len(requiredAppIDs) == 0 {
		return nil
	}

	requiredAppNames := make([]string, 0, len(requiredAppIDs))
	hasInstalledOwner := false
	for _, appID := range requiredAppIDs {
		if appID == models.AppIDMCPServer {
			hasInstalledOwner = true
			continue
		}

		appName := services.AppDisplayNames[appID]
		if appName == "" {
			appName = appID
		}
		requiredAppNames = append(requiredAppNames, appName)

		installed, err := appService.IsInstalled(ctx, projectID, appID)
		if err != nil {
			deps.GetLogger().Error("Failed to check tool owner app installation",
				zap.String("project_id", projectID.String()),
				zap.String("tool_name", toolName),
				zap.String("app_id", appID),
				zap.Error(err))
			return newToolAccessError("app_not_installed", fmt.Sprintf("%s app installation check failed", appName))
		}
		if installed {
			hasInstalledOwner = true
		}
	}

	if hasInstalledOwner {
		return nil
	}

	if len(requiredAppNames) == 0 {
		return nil
	}
	if len(requiredAppNames) == 1 {
		return newToolAccessError("app_not_installed", fmt.Sprintf("%s app is not installed for this project", requiredAppNames[0]))
	}

	return newToolAccessError("app_not_installed",
		fmt.Sprintf("required tool owner apps are not installed for this project: %s", strings.Join(requiredAppNames, ", ")))
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
	// Get user ID from claims (may be agent auth, which has no user UUID provenance)
	claims, _ := auth.GetClaims(ctx)
	userID := uuid.Nil
	if claims != nil && !auth.IsAgentClaims(claims) {
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
