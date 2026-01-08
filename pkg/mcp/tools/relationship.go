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

// RelationshipToolDeps contains dependencies for relationship MCP tools.
type RelationshipToolDeps struct {
	DB                     *database.DB
	MCPConfigService       services.MCPConfigService
	OntologyRepo           repositories.OntologyRepository
	OntologyEntityRepo     repositories.OntologyEntityRepository
	EntityRelationshipRepo repositories.EntityRelationshipRepository
	Logger                 *zap.Logger
}

// RegisterRelationshipTools registers relationship MCP tools.
func RegisterRelationshipTools(s *server.MCPServer, deps *RelationshipToolDeps) {
	registerUpdateRelationshipTool(s, deps)
	registerDeleteRelationshipTool(s, deps)
}

// checkRelationshipToolEnabled verifies a specific relationship tool is enabled for the project.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
func checkRelationshipToolEnabled(ctx context.Context, deps *RelationshipToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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

// registerUpdateRelationshipTool adds the update_relationship tool for creating/updating relationships.
func registerUpdateRelationshipTool(s *server.MCPServer, deps *RelationshipToolDeps) {
	tool := mcp.NewTool(
		"update_relationship",
		mcp.WithDescription(
			"Create or update a relationship between two entities with upsert semantics. "+
				"The from_entity and to_entity names together form the upsert key. "+
				"If a relationship between these entities already exists, it will be updated with the new values. "+
				"Optional parameters (description, label, cardinality) replace existing values when provided. "+
				"Omitted parameters preserve existing values. "+
				"Cardinality values: '1:1', '1:N', 'N:1', 'N:N', 'unknown' (default). "+
				"Example: update_relationship(from_entity='Account', to_entity='User', label='owns', cardinality='N:1', description='The user who owns this account')",
		),
		mcp.WithString(
			"from_entity",
			mcp.Required(),
			mcp.Description("Source entity name (upsert key). Required."),
		),
		mcp.WithString(
			"to_entity",
			mcp.Required(),
			mcp.Description("Target entity name (upsert key). Required."),
		),
		mcp.WithString(
			"description",
			mcp.Description("Optional - Relationship description explaining what this relationship means"),
		),
		mcp.WithString(
			"label",
			mcp.Description("Optional - Short semantic label for the relationship (e.g., 'owns', 'contains', 'placed_by')"),
		),
		mcp.WithString(
			"cardinality",
			mcp.Description("Optional - Cardinality of the relationship: '1:1', '1:N', 'N:1', 'N:N', or 'unknown'"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkRelationshipToolEnabled(ctx, deps, "update_relationship")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required parameters
		fromEntityName, err := req.RequireString("from_entity")
		if err != nil {
			return nil, err
		}

		toEntityName, err := req.RequireString("to_entity")
		if err != nil {
			return nil, err
		}

		// Get optional parameters
		description := getOptionalString(req, "description")
		label := getOptionalString(req, "label")
		cardinality := getOptionalString(req, "cardinality")

		// Validate cardinality if provided
		if cardinality != "" {
			validCardinalities := map[string]bool{
				"1:1": true, "1:N": true, "N:1": true, "N:N": true, "unknown": true,
			}
			if !validCardinalities[cardinality] {
				return nil, fmt.Errorf("invalid cardinality '%s': must be one of '1:1', '1:N', 'N:1', 'N:N', 'unknown'", cardinality)
			}
		}

		// Get active ontology
		ontology, err := deps.OntologyRepo.GetActive(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get active ontology: %w", err)
		}
		if ontology == nil {
			return nil, fmt.Errorf("no active ontology found for project")
		}

		// Get from entity
		fromEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, fromEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get from_entity: %w", err)
		}
		if fromEntity == nil {
			return nil, fmt.Errorf("from_entity '%s' not found", fromEntityName)
		}

		// Get to entity
		toEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, toEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get to_entity: %w", err)
		}
		if toEntity == nil {
			return nil, fmt.Errorf("to_entity '%s' not found", toEntityName)
		}

		// Check if relationship already exists
		existingRel, err := deps.EntityRelationshipRepo.GetByEntityPair(tenantCtx, ontology.ID, fromEntity.ID, toEntity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check for existing relationship: %w", err)
		}

		// Build or update relationship
		var rel *models.EntityRelationship
		isNew := existingRel == nil

		if isNew {
			// Create new relationship
			// We need column details for the unique constraint, but for agent-created relationships,
			// we'll use placeholder values since the actual column details aren't known
			rel = &models.EntityRelationship{
				OntologyID:         ontology.ID,
				SourceEntityID:     fromEntity.ID,
				TargetEntityID:     toEntity.ID,
				SourceColumnSchema: fromEntity.PrimarySchema,
				SourceColumnTable:  fromEntity.PrimaryTable,
				SourceColumnName:   fromEntity.PrimaryColumn,
				TargetColumnSchema: toEntity.PrimarySchema,
				TargetColumnTable:  toEntity.PrimaryTable,
				TargetColumnName:   toEntity.PrimaryColumn,
				DetectionMethod:    models.DetectionMethodManual,
				Confidence:         1.0,
				Status:             models.RelationshipStatusConfirmed,
				Cardinality:        "unknown",
			}
			if cardinality != "" {
				rel.Cardinality = cardinality
			}
			if description != "" {
				rel.Description = &description
			}
			if label != "" {
				rel.Association = &label
			}
		} else {
			// Update existing relationship
			rel = existingRel
			if description != "" {
				rel.Description = &description
			}
			if label != "" {
				rel.Association = &label
			}
			if cardinality != "" {
				rel.Cardinality = cardinality
			}
		}

		// Upsert the relationship
		if err := deps.EntityRelationshipRepo.Upsert(tenantCtx, rel); err != nil {
			return nil, fmt.Errorf("failed to upsert relationship: %w", err)
		}

		// Build response
		result := updateRelationshipResponse{
			FromEntity:  fromEntityName,
			ToEntity:    toEntityName,
			Description: description,
			Label:       label,
			Cardinality: rel.Cardinality,
			Created:     isNew,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerDeleteRelationshipTool adds the delete_relationship tool for removing relationships.
func registerDeleteRelationshipTool(s *server.MCPServer, deps *RelationshipToolDeps) {
	tool := mcp.NewTool(
		"delete_relationship",
		mcp.WithDescription(
			"Remove a relationship between two entities. "+
				"Use this when a relationship doesn't exist or was incorrectly identified. "+
				"This is a hard delete (not soft delete) since relationships can be re-created. "+
				"Use sparingly - only when a relationship is genuinely incorrect. "+
				"Example: delete_relationship(from_entity='Account', to_entity='InvalidEntity')",
		),
		mcp.WithString(
			"from_entity",
			mcp.Required(),
			mcp.Description("Source entity name"),
		),
		mcp.WithString(
			"to_entity",
			mcp.Required(),
			mcp.Description("Target entity name"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true), // Deleting the same relationship twice is idempotent
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkRelationshipToolEnabled(ctx, deps, "delete_relationship")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required parameters
		fromEntityName, err := req.RequireString("from_entity")
		if err != nil {
			return nil, err
		}

		toEntityName, err := req.RequireString("to_entity")
		if err != nil {
			return nil, err
		}

		// Get active ontology
		ontology, err := deps.OntologyRepo.GetActive(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get active ontology: %w", err)
		}
		if ontology == nil {
			return nil, fmt.Errorf("no active ontology found for project")
		}

		// Get from entity
		fromEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, fromEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get from_entity: %w", err)
		}
		if fromEntity == nil {
			return nil, fmt.Errorf("from_entity '%s' not found", fromEntityName)
		}

		// Get to entity
		toEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, toEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get to_entity: %w", err)
		}
		if toEntity == nil {
			return nil, fmt.Errorf("to_entity '%s' not found", toEntityName)
		}

		// Get relationship by entity pair
		rel, err := deps.EntityRelationshipRepo.GetByEntityPair(tenantCtx, ontology.ID, fromEntity.ID, toEntity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get relationship: %w", err)
		}
		if rel == nil {
			// Relationship doesn't exist - this is idempotent, so just return success
			result := deleteRelationshipResponse{
				FromEntity: fromEntityName,
				ToEntity:   toEntityName,
				Deleted:    false,
			}

			jsonResult, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %w", err)
			}

			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Delete the relationship
		if err := deps.EntityRelationshipRepo.Delete(tenantCtx, rel.ID); err != nil {
			return nil, fmt.Errorf("failed to delete relationship: %w", err)
		}

		// Build response
		result := deleteRelationshipResponse{
			FromEntity: fromEntityName,
			ToEntity:   toEntityName,
			Deleted:    true,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// updateRelationshipResponse is the response format for update_relationship tool.
type updateRelationshipResponse struct {
	FromEntity  string `json:"from_entity"`
	ToEntity    string `json:"to_entity"`
	Description string `json:"description,omitempty"`
	Label       string `json:"label,omitempty"`
	Cardinality string `json:"cardinality"`
	Created     bool   `json:"created"` // true if relationship was newly created, false if updated
}

// deleteRelationshipResponse is the response format for delete_relationship tool.
type deleteRelationshipResponse struct {
	FromEntity string `json:"from_entity"`
	ToEntity   string `json:"to_entity"`
	Deleted    bool   `json:"deleted"` // false if relationship didn't exist (idempotent)
}
