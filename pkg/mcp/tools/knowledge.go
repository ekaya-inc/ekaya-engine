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
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// KnowledgeToolDeps contains dependencies for project knowledge tools.
type KnowledgeToolDeps struct {
	DB                  *database.DB
	MCPConfigService    services.MCPConfigService
	KnowledgeRepository repositories.KnowledgeRepository
	Logger              *zap.Logger
}

// RegisterKnowledgeTools registers project knowledge MCP tools.
func RegisterKnowledgeTools(s *server.MCPServer, deps *KnowledgeToolDeps) {
	registerUpdateProjectKnowledgeTool(s, deps)
	registerDeleteProjectKnowledgeTool(s, deps)
}

// checkKnowledgeToolEnabled verifies a specific knowledge tool is enabled for the project.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
func checkKnowledgeToolEnabled(ctx context.Context, deps *KnowledgeToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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

	// Check if caller is an agent (API key authentication)
	isAgent := claims.Subject == "agent"

	// Get tool groups state and check access using the unified checker
	state, err := deps.MCPConfigService.GetToolGroupsState(tenantCtx, projectID)
	if err != nil {
		scope.Close()
		deps.Logger.Error("Failed to get tool groups state",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return uuid.Nil, nil, nil, fmt.Errorf("failed to check tool configuration: %w", err)
	}

	// Use the unified ToolAccessChecker for consistent access decisions
	checker := services.NewToolAccessChecker()
	if checker.IsToolAccessible(toolName, state, isAgent) {
		return projectID, tenantCtx, func() { scope.Close() }, nil
	}

	scope.Close()
	return uuid.Nil, nil, nil, fmt.Errorf("%s tool is not enabled for this project", toolName)
}

// registerUpdateProjectKnowledgeTool adds the update_project_knowledge tool for creating/updating domain facts.
func registerUpdateProjectKnowledgeTool(s *server.MCPServer, deps *KnowledgeToolDeps) {
	tool := mcp.NewTool(
		"update_project_knowledge",
		mcp.WithDescription(
			"Create or update a domain fact that persists across sessions. "+
				"Use this to capture business rules, terminology, enumerations, or conventions learned during analysis. "+
				"Facts are upserted by (category, fact) pair - the same fact can be updated with new context. "+
				"Categories: 'terminology' (domain-specific terms), 'business_rule' (validation rules, calculations), "+
				"'enumeration' (status values, type codes), 'convention' (naming patterns, soft deletes). "+
				"Example: fact='A tik represents 6 seconds of engagement', category='terminology', context='Inferred from billing_engagements table'",
		),
		mcp.WithString(
			"fact",
			mcp.Required(),
			mcp.Description("The domain fact or knowledge to store (e.g., 'Platform fees are ~33% of total_amount')"),
		),
		mcp.WithString(
			"fact_id",
			mcp.Description("Optional - UUID of existing fact to update. If omitted, upserts by (category, fact) match"),
		),
		mcp.WithString(
			"context",
			mcp.Description("Optional - How this fact was discovered (e.g., 'Found in user.go:45-67', 'Verified: tikr_share/total_amount ≈ 0.33')"),
		),
		mcp.WithString(
			"category",
			mcp.Description("Optional - Fact category: 'terminology', 'business_rule', 'enumeration', or 'convention'. Defaults to 'terminology'"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkKnowledgeToolEnabled(ctx, deps, "update_project_knowledge")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required fact parameter
		fact, err := req.RequireString("fact")
		if err != nil {
			return nil, err
		}

		// Get optional parameters
		factIDStr := getOptionalString(req, "fact_id")
		context := getOptionalString(req, "context")
		category := getOptionalString(req, "category")

		// Default category to terminology if not specified
		if category == "" {
			category = "terminology"
		}

		// Validate category
		validCategories := map[string]bool{
			"terminology":   true,
			"business_rule": true,
			"enumeration":   true,
			"convention":    true,
		}
		if !validCategories[category] {
			return nil, fmt.Errorf("invalid category: %s (must be one of: terminology, business_rule, enumeration, convention)", category)
		}

		// Build KnowledgeFact
		knowledgeFact := &models.KnowledgeFact{
			ProjectID: projectID,
			FactType:  category,
			Key:       fact, // Using fact as the key for upsert semantics
			Value:     fact,
			Context:   context,
		}

		// If fact_id provided, parse it and set ID for explicit update
		if factIDStr != "" {
			factID, err := uuid.Parse(factIDStr)
			if err != nil {
				return nil, fmt.Errorf("invalid fact_id: %w", err)
			}
			knowledgeFact.ID = factID
		}

		// Upsert the fact
		err = deps.KnowledgeRepository.Upsert(tenantCtx, knowledgeFact)
		if err != nil {
			return nil, fmt.Errorf("failed to upsert project knowledge: %w", err)
		}

		// Build response
		result := updateProjectKnowledgeResponse{
			FactID:    knowledgeFact.ID.String(),
			Fact:      fact,
			Category:  category,
			Context:   context,
			CreatedAt: knowledgeFact.CreatedAt,
			UpdatedAt: knowledgeFact.UpdatedAt,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerDeleteProjectKnowledgeTool adds the delete_project_knowledge tool for removing domain facts.
func registerDeleteProjectKnowledgeTool(s *server.MCPServer, deps *KnowledgeToolDeps) {
	tool := mcp.NewTool(
		"delete_project_knowledge",
		mcp.WithDescription(
			"Remove a domain fact that is incorrect or outdated. "+
				"Requires the fact_id (UUID) of the knowledge entry to delete. "+
				"Use this sparingly - only when a fact is wrong, not just when updating it (use update_project_knowledge for updates).",
		),
		mcp.WithString(
			"fact_id",
			mcp.Required(),
			mcp.Description("UUID of the knowledge fact to delete"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, tenantCtx, cleanup, err := checkKnowledgeToolEnabled(ctx, deps, "delete_project_knowledge")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required fact_id parameter
		factIDStr, err := req.RequireString("fact_id")
		if err != nil {
			return nil, err
		}

		factID, err := uuid.Parse(factIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid fact_id: %w", err)
		}

		// Delete the fact
		err = deps.KnowledgeRepository.Delete(tenantCtx, factID)
		if err != nil {
			return nil, fmt.Errorf("failed to delete project knowledge: %w", err)
		}

		// Build response
		result := deleteProjectKnowledgeResponse{
			FactID:  factIDStr,
			Deleted: true,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// updateProjectKnowledgeResponse is the response format for update_project_knowledge tool.
type updateProjectKnowledgeResponse struct {
	FactID    string `json:"fact_id"`
	Fact      string `json:"fact"`
	Category  string `json:"category"`
	Context   string `json:"context,omitempty"`
	CreatedAt any    `json:"created_at"`
	UpdatedAt any    `json:"updated_at"`
}

// deleteProjectKnowledgeResponse is the response format for delete_project_knowledge tool.
type deleteProjectKnowledgeResponse struct {
	FactID  string `json:"fact_id"`
	Deleted bool   `json:"deleted"`
}
