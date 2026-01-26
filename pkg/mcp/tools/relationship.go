// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

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

// GetDB implements ToolAccessDeps.
func (d *RelationshipToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *RelationshipToolDeps) GetMCPConfigService() services.MCPConfigService {
	return d.MCPConfigService
}

// GetLogger implements ToolAccessDeps.
func (d *RelationshipToolDeps) GetLogger() *zap.Logger { return d.Logger }

// RegisterRelationshipTools registers relationship MCP tools.
func RegisterRelationshipTools(s *server.MCPServer, deps *RelationshipToolDeps) {
	registerUpdateRelationshipTool(s, deps)
	registerDeleteRelationshipTool(s, deps)
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
				"Cardinality values: '1:1', '1:N', 'N:1', 'N:M', 'unknown' (default)."+
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
			mcp.Description("Optional - Cardinality of the relationship: '1:1', '1:N', 'N:1', 'N:M', or 'unknown'"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_relationship")
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

		// Validate from_entity and to_entity are not empty after trimming
		fromEntityName = strings.TrimSpace(fromEntityName)
		if fromEntityName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'from_entity' cannot be empty"), nil
		}

		toEntityName = strings.TrimSpace(toEntityName)
		if toEntityName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'to_entity' cannot be empty"), nil
		}

		// Get optional parameters
		description := getOptionalString(req, "description")
		label := getOptionalString(req, "label")
		cardinality := getOptionalString(req, "cardinality")

		// Validate cardinality if provided
		if cardinality != "" {
			validCardinalities := []string{"1:1", "1:N", "N:1", "N:M", "unknown"}
			validCardinalitiesMap := map[string]bool{
				"1:1": true, "1:N": true, "N:1": true, "N:M": true, "unknown": true,
			}
			if !validCardinalitiesMap[cardinality] {
				return NewErrorResultWithDetails(
					"invalid_parameters",
					fmt.Sprintf("invalid cardinality value: %q", cardinality),
					map[string]any{
						"parameter": "cardinality",
						"expected":  validCardinalities,
						"actual":    cardinality,
					},
				), nil
			}
		}

		// Get or create active ontology (enables immediate use without extraction)
		ontology, err := ensureOntologyExists(tenantCtx, deps.OntologyRepo, projectID)
		if err != nil {
			return NewErrorResult("ontology_error", err.Error()), nil
		}

		// Get from entity
		fromEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, fromEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get from_entity: %w", err)
		}
		if fromEntity == nil {
			return NewErrorResult("ENTITY_NOT_FOUND", fmt.Sprintf("from_entity %q not found", fromEntityName)), nil
		}

		// Get to entity
		toEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, toEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get to_entity: %w", err)
		}
		if toEntity == nil {
			return NewErrorResult("ENTITY_NOT_FOUND", fmt.Sprintf("to_entity %q not found", toEntityName)), nil
		}

		// Check if relationship already exists
		existingRel, err := deps.EntityRelationshipRepo.GetByEntityPair(tenantCtx, ontology.ID, fromEntity.ID, toEntity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to check for existing relationship: %w", err)
		}

		// Build or update relationship
		var rel *models.EntityRelationship
		isNew := existingRel == nil

		// If relationship exists, check precedence before allowing update
		if !isNew {
			// Check precedence: can MCP modify this relationship?
			// Use Source and LastEditSource (method tracking) not CreatedBy/UpdatedBy (user UUIDs)
			if !canModifyRelationship(existingRel.Source, existingRel.LastEditSource, models.ProvenanceMCP) {
				effectiveSource := existingRel.Source
				if existingRel.LastEditSource != nil && *existingRel.LastEditSource != "" {
					effectiveSource = *existingRel.LastEditSource
				}
				return NewErrorResult("precedence_blocked",
					fmt.Sprintf("Cannot modify relationship: precedence blocked (existing: %s, modifier: %s). "+
						"Manual changes cannot be overridden by MCP. Use the UI to modify or delete this relationship.",
						effectiveSource, models.ProvenanceMCP)), nil
			}
		}

		if isNew {
			// Create new relationship
			// Note: Source and CreatedBy are set by the repository from provenance context
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
			// Note: LastEditSource and UpdatedBy are set by the repository from provenance context
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
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "delete_relationship")
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

		// Validate from_entity is not empty after trimming
		fromEntityName = strings.TrimSpace(fromEntityName)
		if fromEntityName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'from_entity' cannot be empty"), nil
		}

		// Validate to_entity is not empty after trimming
		toEntityName = strings.TrimSpace(toEntityName)
		if toEntityName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'to_entity' cannot be empty"), nil
		}

		// Get or create active ontology (enables immediate use without extraction)
		ontology, err := ensureOntologyExists(tenantCtx, deps.OntologyRepo, projectID)
		if err != nil {
			return NewErrorResult("ontology_error", err.Error()), nil
		}

		// Get from entity
		fromEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, fromEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get from_entity: %w", err)
		}
		if fromEntity == nil {
			return NewErrorResult("ENTITY_NOT_FOUND", fmt.Sprintf("from_entity %q not found", fromEntityName)), nil
		}

		// Get to entity
		toEntity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, toEntityName)
		if err != nil {
			return nil, fmt.Errorf("failed to get to_entity: %w", err)
		}
		if toEntity == nil {
			return NewErrorResult("ENTITY_NOT_FOUND", fmt.Sprintf("to_entity %q not found", toEntityName)), nil
		}

		// Get relationship by entity pair
		rel, err := deps.EntityRelationshipRepo.GetByEntityPair(tenantCtx, ontology.ID, fromEntity.ID, toEntity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get relationship: %w", err)
		}
		if rel == nil {
			return NewErrorResult("RELATIONSHIP_NOT_FOUND", fmt.Sprintf("relationship from %q to %q not found", fromEntityName, toEntityName)), nil
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

// canModifyRelationship checks if a source can modify a relationship based on precedence.
// Precedence hierarchy: Manual (3) > MCP (2) > Inference (1)
// Returns true if the modification is allowed, false if blocked by higher precedence.
func canModifyRelationship(elementCreatedBy string, elementUpdatedBy *string, modifierSource string) bool {
	modifierLevel := precedenceLevelRelationship(modifierSource)

	// Check against updated_by if present, otherwise check created_by
	var existingSource string
	if elementUpdatedBy != nil && *elementUpdatedBy != "" {
		existingSource = *elementUpdatedBy
	} else {
		existingSource = elementCreatedBy
	}

	existingLevel := precedenceLevelRelationship(existingSource)

	// Modifier can change if their level is >= existing level
	return modifierLevel >= existingLevel
}

// precedenceLevelRelationship returns the numeric precedence level for a source.
// Higher number = higher precedence.
func precedenceLevelRelationship(source string) int {
	switch source {
	case models.ProvenanceManual:
		return 3
	case models.ProvenanceMCP:
		return 2
	case models.ProvenanceInference:
		return 1
	default:
		return 0 // Unknown source has lowest precedence
	}
}
