// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/jsonutil"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

const projectOverviewFactType = "project_overview"

var writableProjectKnowledgeCategories = []string{
	models.FactTypeTerminology,
	models.FactTypeBusinessRule,
	models.FactTypeEnumeration,
	models.FactTypeConvention,
}

var writableProjectKnowledgeCategoryMap = map[string]bool{
	models.FactTypeTerminology:  true,
	models.FactTypeBusinessRule: true,
	models.FactTypeEnumeration:  true,
	models.FactTypeConvention:   true,
}

// KnowledgeToolDeps contains dependencies for project knowledge tools.
type KnowledgeToolDeps struct {
	BaseMCPToolDeps
	KnowledgeRepository repositories.KnowledgeRepository
}

// RegisterKnowledgeTools registers project knowledge MCP tools.
func RegisterKnowledgeTools(s *server.MCPServer, deps *KnowledgeToolDeps) {
	registerListProjectKnowledgeTool(s, deps)
	registerUpdateProjectKnowledgeTool(s, deps)
	registerDeleteProjectKnowledgeTool(s, deps)
}

// registerListProjectKnowledgeTool adds the list_project_knowledge tool for discovering project knowledge fact IDs.
func registerListProjectKnowledgeTool(s *server.MCPServer, deps *KnowledgeToolDeps) {
	tool := mcp.NewTool(
		"list_project_knowledge",
		mcp.WithDescription(
			"List all project knowledge facts for the current project so an MCP client can discover stable fact_id values before updating or deleting existing facts. "+
				"Returns fact_id, category, fact, optional context, provenance fields, and timestamps for maintenance workflows. "+
				"This can include read-only project_overview entries managed through the ontology extraction flow.",
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "list_project_knowledge")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		facts, err := deps.KnowledgeRepository.GetByProject(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to list project knowledge: %w", err)
		}

		result := listProjectKnowledgeResponse{
			Facts: make([]projectKnowledgeListItem, 0, len(facts)),
			Count: len(facts),
		}
		for _, fact := range facts {
			result.Facts = append(result.Facts, toProjectKnowledgeListItem(fact))
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerUpdateProjectKnowledgeTool adds the update_project_knowledge tool for creating/updating domain facts.
func registerUpdateProjectKnowledgeTool(s *server.MCPServer, deps *KnowledgeToolDeps) {
	tool := mcp.NewTool(
		"update_project_knowledge",
		mcp.WithDescription(
			"Create or update a domain fact that persists across sessions. "+
				"Use this to capture business rules, terminology, enumerations, or conventions learned during analysis. "+
				"Provide fact_id to update an existing fact; if fact_id is omitted, this creates a new fact. "+
				"Use list_project_knowledge first when you need to discover an existing fact_id from an earlier session. "+
				"Categories: 'terminology' (domain-specific terms), 'business_rule' (validation rules, calculations), "+
				"'enumeration' (status values, type codes), 'convention' (naming patterns, soft deletes). "+
				"'project_overview' may appear in list_project_knowledge but is read-only here and must be managed through the ontology extraction project_overview input. "+
				"For table-specific metadata (ephemeral tables, usage notes, preferred alternatives), use update_table instead. "+
				"Example: fact='A tik represents 6 seconds of engagement', category='terminology', context='Inferred from billing_engagements table'",
		),
		mcp.WithString(
			"fact",
			mcp.Required(),
			mcp.Description("The domain fact or knowledge to store (e.g., 'Platform fees are ~33% of total_amount'). Maximum 255 characters."),
		),
		mcp.WithString(
			"fact_id",
			mcp.Description("Optional - UUID of existing fact to update. If omitted, a new fact is created"),
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
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_project_knowledge")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get required fact parameter
		fact, err := req.RequireString("fact")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}

		// Validate fact is not empty after trimming
		fact = trimString(fact)
		if fact == "" {
			return NewErrorResult("invalid_parameters", "parameter 'fact' cannot be empty"), nil
		}

		// Get optional parameters
		factIDStr := getOptionalString(req, "fact_id")
		context := getOptionalString(req, "context")
		category := getOptionalString(req, "category")

		// Default category to terminology if not specified
		if category == "" {
			category = models.FactTypeTerminology
		}

		if category == projectOverviewFactType {
			return newProjectOverviewReadOnlyResult("update", ""), nil
		}

		// Validate category
		if !writableProjectKnowledgeCategoryMap[category] {
			return NewErrorResultWithDetails(
				"invalid_parameters",
				"invalid category value",
				map[string]any{
					"parameter": "category",
					"expected":  writableProjectKnowledgeCategories,
					"actual":    category,
				},
			), nil
		}

		// Build KnowledgeFact (project-lifecycle scope, no ontology association)
		knowledgeFact := &models.KnowledgeFact{
			ProjectID: projectID,
			FactType:  category,
			Value:     fact,
			Context:   context,
		}

		// If fact_id provided, parse it and update existing fact
		if factIDStr != "" {
			factIDStr = trimString(factIDStr)
			factID, err := uuid.Parse(factIDStr)
			if err != nil {
				return NewErrorResult(
					"invalid_parameters",
					fmt.Sprintf("invalid fact_id format: %q is not a valid UUID", factIDStr),
				), nil
			}
			knowledgeFact.ID = factID
			// Update existing fact
			err = deps.KnowledgeRepository.Update(tenantCtx, knowledgeFact)
			if err != nil {
				return HandleServiceError(err, "update_knowledge_failed")
			}
		} else {
			// Create new fact
			err = deps.KnowledgeRepository.Create(tenantCtx, knowledgeFact)
			if err != nil {
				return HandleServiceError(err, "create_knowledge_failed")
			}
		}

		// Build response
		result := updateProjectKnowledgeResponse{
			FactID:    knowledgeFact.ID.String(),
			Fact:      fact,
			Category:  category,
			Context:   context,
			CreatedAt: jsonutil.FormatUTCTime(knowledgeFact.CreatedAt),
			UpdatedAt: jsonutil.FormatUTCTime(knowledgeFact.UpdatedAt),
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
				"Use list_project_knowledge first to discover existing fact_id values. "+
				"project_overview entries may appear in list_project_knowledge but are read-only here and cannot be deleted through this tool. "+
				"Use this sparingly - only when a fact is wrong, not just when updating it (use update_project_knowledge for updates).",
		),
		mcp.WithString(
			"fact_id",
			mcp.Required(),
			mcp.Description("UUID of the knowledge fact to delete"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "delete_project_knowledge")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Get required fact_id parameter
		factIDStr, err := req.RequireString("fact_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}

		// Validate fact_id is not empty after trimming
		factIDStr = trimString(factIDStr)
		if factIDStr == "" {
			return NewErrorResult("invalid_parameters", "parameter 'fact_id' cannot be empty"), nil
		}

		// Validate UUID format
		factID, err := uuid.Parse(factIDStr)
		if err != nil {
			return NewErrorResult(
				"invalid_parameters",
				fmt.Sprintf("invalid fact_id format: %q is not a valid UUID", factIDStr),
			), nil
		}

		fact, err := deps.KnowledgeRepository.GetByID(tenantCtx, factID)
		if err != nil {
			if err == apperrors.ErrNotFound {
				return NewErrorResult("FACT_NOT_FOUND", fmt.Sprintf("fact %q not found", factIDStr)), nil
			}
			return HandleServiceError(err, "get_knowledge_failed")
		}

		if fact.FactType == projectOverviewFactType {
			return newProjectOverviewReadOnlyResult("delete", factIDStr), nil
		}

		// Delete the fact
		err = deps.KnowledgeRepository.Delete(tenantCtx, factID)
		if err != nil {
			if err == apperrors.ErrNotFound {
				return NewErrorResult("FACT_NOT_FOUND", fmt.Sprintf("fact %q not found", factIDStr)), nil
			}
			return HandleServiceError(err, "delete_knowledge_failed")
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

func toProjectKnowledgeListItem(fact *models.KnowledgeFact) projectKnowledgeListItem {
	item := projectKnowledgeListItem{
		FactID:    fact.ID.String(),
		Category:  fact.FactType,
		Fact:      fact.Value,
		Context:   fact.Context,
		Source:    fact.Source,
		CreatedAt: jsonutil.FormatUTCTime(fact.CreatedAt),
		UpdatedAt: jsonutil.FormatUTCTime(fact.UpdatedAt),
	}
	if fact.LastEditSource != nil {
		item.LastEditSource = *fact.LastEditSource
	}
	return item
}

// listProjectKnowledgeResponse is the response format for list_project_knowledge tool.
type listProjectKnowledgeResponse struct {
	Facts []projectKnowledgeListItem `json:"facts"`
	Count int                        `json:"count"`
}

// projectKnowledgeListItem is the MCP-friendly response item for a project knowledge fact.
type projectKnowledgeListItem struct {
	FactID         string `json:"fact_id"`
	Category       string `json:"category"`
	Fact           string `json:"fact"`
	Context        string `json:"context,omitempty"`
	Source         string `json:"source,omitempty"`
	LastEditSource string `json:"last_edit_source,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// updateProjectKnowledgeResponse is the response format for update_project_knowledge tool.
type updateProjectKnowledgeResponse struct {
	FactID    string `json:"fact_id"`
	Fact      string `json:"fact"`
	Category  string `json:"category"`
	Context   string `json:"context,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// deleteProjectKnowledgeResponse is the response format for delete_project_knowledge tool.
type deleteProjectKnowledgeResponse struct {
	FactID  string `json:"fact_id"`
	Deleted bool   `json:"deleted"`
}

func newProjectOverviewReadOnlyResult(action, factID string) *mcp.CallToolResult {
	details := map[string]any{
		"action":     action,
		"category":   projectOverviewFactType,
		"manage_via": "ontology extraction project_overview input",
	}
	if factID != "" {
		details["fact_id"] = factID
	}

	return NewErrorResultWithDetails(
		"invalid_parameters",
		"project_overview is read-only in project knowledge tools; manage it through the ontology extraction project_overview input",
		details,
	)
}
