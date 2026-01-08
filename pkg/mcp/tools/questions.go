// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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

// QuestionToolDeps contains dependencies for ontology question tools.
type QuestionToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	QuestionRepo     repositories.OntologyQuestionRepository
	Logger           *zap.Logger
}

// RegisterQuestionTools registers ontology question MCP tools.
func RegisterQuestionTools(s *server.MCPServer, deps *QuestionToolDeps) {
	registerListOntologyQuestionsTool(s, deps)
	registerResolveOntologyQuestionTool(s, deps)
}

// checkQuestionToolEnabled verifies a specific question tool is enabled for the project.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
func checkQuestionToolEnabled(ctx context.Context, deps *QuestionToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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

// registerListOntologyQuestionsTool adds the list_ontology_questions tool for listing questions with filters.
func registerListOntologyQuestionsTool(s *server.MCPServer, deps *QuestionToolDeps) {
	tool := mcp.NewTool(
		"list_ontology_questions",
		mcp.WithDescription(
			"List ontology questions generated during schema extraction with flexible filtering and pagination. "+
				"Filter by status (pending/skipped/answered/deleted), category (business_rules/relationship/terminology/enumeration/temporal/data_quality), "+
				"entity (affected entity name), or priority (1-5, where 1=highest). "+
				"Returns questions with id, text, category, priority, context, created_at, and counts_by_status for dashboard display. "+
				"Use this to batch-process pending questions or review answered questions. "+
				"Example: list_ontology_questions(status='pending', priority=1, limit=20) returns high-priority unanswered questions.",
		),
		mcp.WithString(
			"status",
			mcp.Description("Optional - Filter by status: 'pending', 'skipped', 'answered', or 'deleted'"),
		),
		mcp.WithString(
			"category",
			mcp.Description("Optional - Filter by category: 'business_rules', 'relationship', 'terminology', 'enumeration', 'temporal', or 'data_quality'"),
		),
		mcp.WithString(
			"entity",
			mcp.Description("Optional - Filter by affected entity name (searches in question context)"),
		),
		mcp.WithNumber(
			"priority",
			mcp.Description("Optional - Filter by priority level (1=highest, 5=lowest)"),
		),
		mcp.WithNumber(
			"limit",
			mcp.Description("Optional - Maximum number of questions to return (default 20, max 100)"),
		),
		mcp.WithNumber(
			"offset",
			mcp.Description("Optional - Offset for pagination (default 0)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkQuestionToolEnabled(ctx, deps, "list_ontology_questions")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse optional filters
		var filters repositories.QuestionListFilters

		// Status filter
		if statusStr := getOptionalString(req, "status"); statusStr != "" {
			status := models.QuestionStatus(statusStr)
			// Validate status
			if !models.IsValidQuestionStatus(status) {
				return nil, fmt.Errorf("invalid status: %s (must be one of: pending, skipped, answered, deleted)", statusStr)
			}
			filters.Status = &status
		}

		// Category filter
		if categoryStr := getOptionalString(req, "category"); categoryStr != "" {
			// Validate category
			validCategory := false
			for _, vc := range models.ValidQuestionCategories {
				if vc == categoryStr {
					validCategory = true
					break
				}
			}
			if !validCategory {
				return nil, fmt.Errorf("invalid category: %s (must be one of: %v)", categoryStr, models.ValidQuestionCategories)
			}
			filters.Category = &categoryStr
		}

		// Entity filter
		if entityStr := getOptionalString(req, "entity"); entityStr != "" {
			filters.Entity = &entityStr
		}

		// Priority filter
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if priorityVal, ok := args["priority"]; ok {
				priorityFloat, ok := priorityVal.(float64)
				if !ok {
					// Try string conversion
					priorityStr, ok := priorityVal.(string)
					if !ok {
						return nil, fmt.Errorf("priority must be a number")
					}
					parsed, err := strconv.Atoi(priorityStr)
					if err != nil {
						return nil, fmt.Errorf("priority must be a number: %w", err)
					}
					priorityInt := int(parsed)
					filters.Priority = &priorityInt
				} else {
					priorityInt := int(priorityFloat)
					filters.Priority = &priorityInt
				}

				// Validate priority range
				if *filters.Priority < 1 || *filters.Priority > 5 {
					return nil, fmt.Errorf("priority must be between 1 and 5")
				}
			}
		}

		// Limit (default 20, max 100)
		filters.Limit = 20
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if limitVal, ok := args["limit"]; ok {
				limitFloat, ok := limitVal.(float64)
				if !ok {
					// Try string conversion
					limitStr, ok := limitVal.(string)
					if !ok {
						return nil, fmt.Errorf("limit must be a number")
					}
					parsed, err := strconv.Atoi(limitStr)
					if err != nil {
						return nil, fmt.Errorf("limit must be a number: %w", err)
					}
					filters.Limit = int(parsed)
				} else {
					filters.Limit = int(limitFloat)
				}

				if filters.Limit <= 0 {
					filters.Limit = 20
				} else if filters.Limit > 100 {
					filters.Limit = 100
				}
			}
		}

		// Offset (default 0)
		filters.Offset = 0
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if offsetVal, ok := args["offset"]; ok {
				offsetFloat, ok := offsetVal.(float64)
				if !ok {
					// Try string conversion
					offsetStr, ok := offsetVal.(string)
					if !ok {
						return nil, fmt.Errorf("offset must be a number")
					}
					parsed, err := strconv.Atoi(offsetStr)
					if err != nil {
						return nil, fmt.Errorf("offset must be a number: %w", err)
					}
					filters.Offset = int(parsed)
				} else {
					filters.Offset = int(offsetFloat)
				}

				if filters.Offset < 0 {
					filters.Offset = 0
				}
			}
		}

		// Call repository
		result, err := deps.QuestionRepo.List(tenantCtx, projectID, filters)
		if err != nil {
			deps.Logger.Error("Failed to list questions",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("failed to list questions: %w", err)
		}

		// Build response
		questions := make([]map[string]interface{}, 0, len(result.Questions))
		for _, q := range result.Questions {
			questionInfo := map[string]interface{}{
				"id":         q.ID.String(),
				"question":   q.Text,
				"category":   q.Category,
				"priority":   q.Priority,
				"status":     string(q.Status),
				"created_at": q.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			}

			// Add optional fields if present
			if q.Reasoning != "" {
				questionInfo["reasoning"] = q.Reasoning
			}
			if q.Affects != nil {
				questionInfo["context"] = map[string]interface{}{
					"tables":  q.Affects.Tables,
					"columns": q.Affects.Columns,
				}
			}
			if q.IsRequired {
				questionInfo["is_required"] = true
			}
			if q.Answer != "" {
				questionInfo["answer"] = q.Answer
			}
			if q.AnsweredAt != nil {
				questionInfo["answered_at"] = q.AnsweredAt.Format("2006-01-02T15:04:05Z07:00")
			}

			questions = append(questions, questionInfo)
		}

		// Build counts_by_status
		countsByStatus := make(map[string]int)
		for status, count := range result.CountsByStatus {
			countsByStatus[string(status)] = count
		}

		response := map[string]interface{}{
			"questions":        questions,
			"total_count":      result.TotalCount,
			"counts_by_status": countsByStatus,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerResolveOntologyQuestionTool adds the resolve_ontology_question tool for marking questions as resolved.
func registerResolveOntologyQuestionTool(s *server.MCPServer, deps *QuestionToolDeps) {
	tool := mcp.NewTool(
		"resolve_ontology_question",
		mcp.WithDescription(
			"Mark an ontology question as resolved after researching and updating the ontology. "+
				"Use this after you've used other update tools (update_entity, update_column, update_glossary_term, etc.) "+
				"to capture the knowledge you learned while answering the question. "+
				"This transitions the question status from 'pending' to 'answered' and sets the answered_at timestamp. "+
				"Example workflow: 1) Research code/docs to answer question, 2) Update ontology with learned knowledge via update tools, "+
				"3) Call resolve_ontology_question with optional resolution_notes explaining how you found the answer.",
		),
		mcp.WithString(
			"question_id",
			mcp.Required(),
			mcp.Description("Required - The UUID of the question to mark as resolved"),
		),
		mcp.WithString(
			"resolution_notes",
			mcp.Description("Optional - Notes explaining how the answer was found (e.g., 'Found in user.go:45-67', 'Inferred from FK constraints')"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		_, tenantCtx, cleanup, err := checkQuestionToolEnabled(ctx, deps, "resolve_ontology_question")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Extract question_id (required)
		questionIDStr := getOptionalString(req, "question_id")
		if questionIDStr == "" {
			return nil, fmt.Errorf("question_id is required")
		}

		questionID, err := uuid.Parse(questionIDStr)
		if err != nil {
			return nil, fmt.Errorf("invalid question_id format: %w", err)
		}

		// Extract resolution_notes (optional)
		resolutionNotes := getOptionalString(req, "resolution_notes")

		// Get the question to verify it exists and is pending
		question, err := deps.QuestionRepo.GetByID(tenantCtx, questionID)
		if err != nil {
			deps.Logger.Error("Failed to get question",
				zap.String("question_id", questionID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("failed to get question: %w", err)
		}

		if question == nil {
			return nil, fmt.Errorf("question not found: %s", questionID)
		}

		// Mark question as answered with optional resolution notes
		// Use nil for answered_by since this is an agent action (not a specific user)
		if resolutionNotes != "" {
			err = deps.QuestionRepo.SubmitAnswer(tenantCtx, questionID, resolutionNotes, nil)
		} else {
			// If no notes provided, submit a default message
			err = deps.QuestionRepo.SubmitAnswer(tenantCtx, questionID, "Resolved by AI agent", nil)
		}

		if err != nil {
			deps.Logger.Error("Failed to resolve question",
				zap.String("question_id", questionID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("failed to resolve question: %w", err)
		}

		// Build response
		response := map[string]interface{}{
			"question_id": questionID.String(),
			"status":      "answered",
			"resolved_at": time.Now().Format("2006-01-02T15:04:05Z07:00"),
		}

		if resolutionNotes != "" {
			response["resolution_notes"] = resolutionNotes
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}
