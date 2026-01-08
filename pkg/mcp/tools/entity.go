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

// EntityToolDeps contains dependencies for entity probe tools.
type EntityToolDeps struct {
	DB                     *database.DB
	MCPConfigService       services.MCPConfigService
	OntologyRepo           repositories.OntologyRepository
	OntologyEntityRepo     repositories.OntologyEntityRepository
	EntityRelationshipRepo repositories.EntityRelationshipRepository
	Logger                 *zap.Logger
}

// RegisterEntityTools registers entity probe MCP tools.
func RegisterEntityTools(s *server.MCPServer, deps *EntityToolDeps) {
	registerGetEntityTool(s, deps)
}

// checkEntityToolEnabled verifies a specific entity tool is enabled for the project.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
func checkEntityToolEnabled(ctx context.Context, deps *EntityToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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

// registerGetEntityTool adds the get_entity tool for retrieving full entity details.
func registerGetEntityTool(s *server.MCPServer, deps *EntityToolDeps) {
	tool := mcp.NewTool(
		"get_entity",
		mcp.WithDescription(
			"Retrieve full details about a specific entity including name, primary_table, description, "+
				"aliases, key_columns, occurrences (where it appears across tables/columns with roles), "+
				"and relationships (to/from other entities with labels and cardinality). "+
				"Use this before updating an entity to see its current state. "+
				"Example: get_entity(name='User') returns all metadata about the User entity.",
		),
		mcp.WithString(
			"name",
			mcp.Required(),
			mcp.Description("Name of the entity to retrieve (e.g., 'User', 'Account', 'Order')"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkEntityToolEnabled(ctx, deps, "get_entity")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required name parameter
		name, err := req.RequireString("name")
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

		// Get entity by name
		entity, err := deps.OntologyEntityRepo.GetByName(tenantCtx, ontology.ID, name)
		if err != nil {
			return nil, fmt.Errorf("failed to get entity: %w", err)
		}
		if entity == nil {
			return nil, fmt.Errorf("entity '%s' not found", name)
		}

		// Get aliases for this entity
		aliases, err := deps.OntologyEntityRepo.GetAliasesByEntity(tenantCtx, entity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get entity aliases: %w", err)
		}

		// Get key columns for this entity
		keyColumns, err := deps.OntologyEntityRepo.GetKeyColumnsByEntity(tenantCtx, entity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get entity key columns: %w", err)
		}

		// Get relationships where this entity is the source
		sourceRels, err := deps.EntityRelationshipRepo.GetByOntology(tenantCtx, ontology.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get entity relationships: %w", err)
		}

		// Get relationships where this entity is the target
		targetRels, err := deps.EntityRelationshipRepo.GetByTargetEntity(tenantCtx, entity.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get target entity relationships: %w", err)
		}

		// Get all entities for mapping IDs to names
		allEntities, err := deps.OntologyEntityRepo.GetByOntology(tenantCtx, ontology.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to get all entities: %w", err)
		}
		entityMap := make(map[uuid.UUID]string)
		for _, e := range allEntities {
			entityMap[e.ID] = e.Name
		}

		// Build response
		response := buildGetEntityResponse(entity, aliases, keyColumns, sourceRels, targetRels, entityMap)

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// buildGetEntityResponse constructs the response object for get_entity.
func buildGetEntityResponse(
	entity interface{},
	aliases interface{},
	keyColumns interface{},
	sourceRels interface{},
	targetRels interface{},
	entityMap map[uuid.UUID]string,
) getEntityResponse {
	// Type assertions
	e := entity.(*models.OntologyEntity)
	aliasesSlice := aliases.([]*models.OntologyEntityAlias)
	keyColumnsSlice := keyColumns.([]*models.OntologyEntityKeyColumn)
	sourceRelsSlice := sourceRels.([]*models.EntityRelationship)
	targetRelsSlice := targetRels.([]*models.EntityRelationship)

	// Extract aliases
	aliasNames := make([]string, 0, len(aliasesSlice))
	for _, a := range aliasesSlice {
		aliasNames = append(aliasNames, a.Alias)
	}

	// Extract key columns
	keyColumnNames := make([]string, 0, len(keyColumnsSlice))
	for _, kc := range keyColumnsSlice {
		keyColumnNames = append(keyColumnNames, kc.ColumnName)
	}

	// Build occurrences from relationships
	occurrences := make([]entityOccurrence, 0)
	seenOccurrences := make(map[string]bool) // Deduplicate occurrences

	// Add occurrences from source relationships (this entity appears in other tables)
	for _, rel := range sourceRelsSlice {
		if rel.SourceEntityID == e.ID {
			key := fmt.Sprintf("%s.%s", rel.SourceColumnTable, rel.SourceColumnName)
			if !seenOccurrences[key] {
				occ := entityOccurrence{
					Table:  rel.SourceColumnTable,
					Column: rel.SourceColumnName,
				}
				if rel.Association != nil {
					occ.Role = *rel.Association
				}
				occurrences = append(occurrences, occ)
				seenOccurrences[key] = true
			}
		}
	}

	// Add occurrences from target relationships (this entity is referenced from other tables)
	for _, rel := range targetRelsSlice {
		if rel.TargetEntityID == e.ID {
			key := fmt.Sprintf("%s.%s", rel.TargetColumnTable, rel.TargetColumnName)
			if !seenOccurrences[key] {
				occ := entityOccurrence{
					Table:  rel.TargetColumnTable,
					Column: rel.TargetColumnName,
				}
				if rel.Association != nil {
					occ.Role = *rel.Association
				}
				occurrences = append(occurrences, occ)
				seenOccurrences[key] = true
			}
		}
	}

	// Build relationships to other entities
	relationships := make([]entityRelationship, 0)

	// Relationships FROM this entity (this entity → other entities)
	for _, rel := range sourceRelsSlice {
		if rel.SourceEntityID == e.ID {
			toEntityName := entityMap[rel.TargetEntityID]
			relItem := entityRelationship{
				Direction: "to",
				Entity:    toEntityName,
				Columns:   fmt.Sprintf("%s.%s -> %s.%s", rel.SourceColumnTable, rel.SourceColumnName, rel.TargetColumnTable, rel.TargetColumnName),
			}
			if rel.Association != nil {
				relItem.Label = *rel.Association
			}
			relationships = append(relationships, relItem)
		}
	}

	// Relationships TO this entity (other entities → this entity)
	for _, rel := range targetRelsSlice {
		if rel.TargetEntityID == e.ID {
			fromEntityName := entityMap[rel.SourceEntityID]
			relItem := entityRelationship{
				Direction: "from",
				Entity:    fromEntityName,
				Columns:   fmt.Sprintf("%s.%s -> %s.%s", rel.SourceColumnTable, rel.SourceColumnName, rel.TargetColumnTable, rel.TargetColumnName),
			}
			if rel.Association != nil {
				relItem.Label = *rel.Association
			}
			relationships = append(relationships, relItem)
		}
	}

	return getEntityResponse{
		Name:          e.Name,
		PrimaryTable:  e.PrimaryTable,
		Description:   e.Description,
		Aliases:       aliasNames,
		KeyColumns:    keyColumnNames,
		Occurrences:   occurrences,
		Relationships: relationships,
	}
}

// getEntityResponse is the response format for get_entity tool.
type getEntityResponse struct {
	Name          string               `json:"name"`
	PrimaryTable  string               `json:"primary_table"`
	Description   string               `json:"description"`
	Aliases       []string             `json:"aliases,omitempty"`
	KeyColumns    []string             `json:"key_columns,omitempty"`
	Occurrences   []entityOccurrence   `json:"occurrences,omitempty"`
	Relationships []entityRelationship `json:"relationships,omitempty"`
}

// entityOccurrence represents where an entity appears in the schema.
type entityOccurrence struct {
	Table  string `json:"table"`
	Column string `json:"column"`
	Role   string `json:"role,omitempty"`
}

// entityRelationship represents a relationship to/from another entity.
type entityRelationship struct {
	Direction string `json:"direction"` // "to" or "from"
	Entity    string `json:"entity"`    // Name of the other entity
	Label     string `json:"label,omitempty"`
	Columns   string `json:"columns"` // e.g., "accounts.user_id -> users.user_id"
}
