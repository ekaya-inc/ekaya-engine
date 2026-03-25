package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// RelationshipToolDeps contains dependencies for relationship MCP tools.
type RelationshipToolDeps struct {
	BaseMCPToolDeps
	ProjectService services.ProjectService
	SchemaService  services.SchemaService
}

// RegisterRelationshipTools registers relationship management MCP tools.
func RegisterRelationshipTools(s *server.MCPServer, deps *RelationshipToolDeps) {
	registerListRelationshipsTool(s, deps)
	registerCreateRelationshipTool(s, deps)
	registerUpdateRelationshipTool(s, deps)
	registerDeleteRelationshipTool(s, deps)
}

func registerListRelationshipsTool(s *server.MCPServer, deps *RelationshipToolDeps) {
	tool := mcp.NewTool(
		"list_relationships",
		mcp.WithDescription(
			"List schema relationships for the default datasource so you can inspect the real join graph before editing it. "+
				"Returns semantic relationship_type separately from provenance (source, last_edit_source, effective_source).",
		),
		mcp.WithString(
			"table",
			mcp.Description("Optional exact table name filter. Matches either the source or target table."),
		),
		mcp.WithString(
			"type",
			mcp.Description("Optional relationship type filter: 'fk', 'inferred', 'manual', or 'review'."),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccessWithoutProvenance(ctx, deps, "list_relationships")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}

		tableFilter := trimString(getOptionalString(req, "table"))
		typeFilter := trimString(getOptionalString(req, "type"))
		if typeFilter != "" && !models.IsValidRelationshipType(typeFilter) {
			return NewErrorResult("invalid_parameters",
				"parameter 'type' must be one of 'fk', 'inferred', 'manual', or 'review'"), nil
		}

		relationshipsResponse, err := deps.SchemaService.GetRelationshipsResponse(tenantCtx, projectID, dsID)
		if err != nil {
			return nil, fmt.Errorf("failed to list relationships: %w", err)
		}

		filtered := make([]relationshipToolItem, 0, len(relationshipsResponse.Relationships))
		for _, rel := range relationshipsResponse.Relationships {
			if tableFilter != "" && rel.SourceTableName != tableFilter && rel.TargetTableName != tableFilter {
				continue
			}
			if typeFilter != "" && rel.RelationshipType != typeFilter {
				continue
			}
			filtered = append(filtered, relationshipToolItemFromDetail(rel))
		}

		payload, err := json.Marshal(listRelationshipsResponse{
			Relationships: filtered,
			TotalCount:    len(filtered),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerCreateRelationshipTool(s *server.MCPServer, deps *RelationshipToolDeps) {
	tool := mcp.NewTool(
		"create_relationship",
		mcp.WithDescription(
			"Create a manual relationship between two columns when inference missed a real join edge. "+
				"The relationship_type remains 'manual'; provenance is recorded as MCP.",
		),
		mcp.WithString("source_table", mcp.Required(), mcp.Description("Source table name")),
		mcp.WithString("source_column", mcp.Required(), mcp.Description("Source column name")),
		mcp.WithString("target_table", mcp.Required(), mcp.Description("Target table name")),
		mcp.WithString("target_column", mcp.Required(), mcp.Description("Target column name")),
		mcp.WithString(
			"cardinality",
			mcp.Description("Optional cardinality override: '1:1', '1:N', 'N:1', 'N:M', or 'unknown'."),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "create_relationship")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		sourceTable, err := req.RequireString("source_table")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}
		sourceColumn, err := req.RequireString("source_column")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}
		targetTable, err := req.RequireString("target_table")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}
		targetColumn, err := req.RequireString("target_column")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}

		dsID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get default datasource: %w", err)
		}

		reqModel := &models.AddRelationshipRequest{
			SourceTableName:  trimString(sourceTable),
			SourceColumnName: trimString(sourceColumn),
			TargetTableName:  trimString(targetTable),
			TargetColumnName: trimString(targetColumn),
			Cardinality:      trimString(getOptionalString(req, "cardinality")),
		}

		rel, err := deps.SchemaService.AddManualRelationship(tenantCtx, projectID, dsID, reqModel)
		if err != nil {
			if err == apperrors.ErrConflict {
				return NewErrorResult("relationship_exists", "a relationship between those columns already exists"), nil
			}
			return HandleServiceError(err, "create_relationship_failed")
		}

		item, err := getRelationshipToolItemByID(tenantCtx, deps, projectID, rel.ID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			item = relationshipToolItemFromModel(rel)
		}

		payload, err := json.Marshal(writeRelationshipResponse{
			Relationship: item,
			Created:      true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerUpdateRelationshipTool(s *server.MCPServer, deps *RelationshipToolDeps) {
	tool := mcp.NewTool(
		"update_relationship",
		mcp.WithDescription(
			"Update an existing relationship in place without changing its semantic relationship_type. "+
				"Use this to curate cardinality or approval state while preserving provenance.",
		),
		mcp.WithString("relationship_id", mcp.Required(), mcp.Description("Relationship UUID to update")),
		mcp.WithString(
			"cardinality",
			mcp.Description("Optional new cardinality: '1:1', '1:N', 'N:1', 'N:M', or 'unknown'."),
		),
		mcp.WithBoolean(
			"is_approved",
			mcp.Description("Optional approval state to persist on the relationship."),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_relationship")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		relationshipIDStr, err := req.RequireString("relationship_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}
		relationshipID, err := uuid.Parse(trimString(relationshipIDStr))
		if err != nil {
			return NewErrorResult("invalid_parameters", "parameter 'relationship_id' must be a valid UUID"), nil
		}

		var cardinality *string
		if value := trimString(getOptionalString(req, "cardinality")); value != "" {
			cardinality = &value
		}

		var isApproved *bool
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if value, exists := args["is_approved"]; exists {
				parsed, ok := value.(bool)
				if !ok {
					return NewErrorResult("invalid_parameters", "parameter 'is_approved' must be a boolean"), nil
				}
				isApproved = &parsed
			}
		}

		if cardinality == nil && isApproved == nil {
			return NewErrorResult("invalid_parameters", "at least one of 'cardinality' or 'is_approved' is required"), nil
		}

		existing, err := getRelationshipToolItemByID(tenantCtx, deps, projectID, relationshipID)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return NewErrorResult("relationship_not_found", "relationship not found"), nil
		}
		if !canModify(existing.Source, existing.LastEditSource, models.ProvenanceMCP) {
			return NewErrorResult("precedence_blocked",
				"Cannot modify relationship: precedence blocked by a higher-precedence manual edit"), nil
		}

		updated, err := deps.SchemaService.UpdateRelationship(tenantCtx, projectID, relationshipID, &models.UpdateRelationshipRequest{
			Cardinality: cardinality,
			IsApproved:  isApproved,
		})
		if err != nil {
			if err == apperrors.ErrNotFound {
				return NewErrorResult("relationship_not_found", "relationship not found"), nil
			}
			return HandleServiceError(err, "update_relationship_failed")
		}

		item, err := getRelationshipToolItemByID(tenantCtx, deps, projectID, updated.ID)
		if err != nil {
			return nil, err
		}
		if item == nil {
			item = relationshipToolItemFromModel(updated)
		}

		payload, err := json.Marshal(writeRelationshipResponse{
			Relationship: item,
			Updated:      true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

func registerDeleteRelationshipTool(s *server.MCPServer, deps *RelationshipToolDeps) {
	tool := mcp.NewTool(
		"delete_relationship",
		mcp.WithDescription(
			"Soft-delete a relationship so it remains suppressed across ordinary re-extraction. "+
				"A full ontology reset clears inferred/review tombstones so they can be rediscovered.",
		),
		mcp.WithString("relationship_id", mcp.Required(), mcp.Description("Relationship UUID to delete")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "delete_relationship")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		relationshipIDStr, err := req.RequireString("relationship_id")
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}
		relationshipID, err := uuid.Parse(trimString(relationshipIDStr))
		if err != nil {
			return NewErrorResult("invalid_parameters", "parameter 'relationship_id' must be a valid UUID"), nil
		}

		existing, err := getRelationshipToolItemByID(tenantCtx, deps, projectID, relationshipID)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return NewErrorResult("relationship_not_found", "relationship not found"), nil
		}
		if !canModify(existing.Source, existing.LastEditSource, models.ProvenanceMCP) {
			return NewErrorResult("precedence_blocked",
				"Cannot delete relationship: precedence blocked by a higher-precedence manual edit"), nil
		}

		if err := deps.SchemaService.RemoveRelationship(tenantCtx, projectID, relationshipID); err != nil {
			if err == apperrors.ErrNotFound {
				return NewErrorResult("relationship_not_found", "relationship not found"), nil
			}
			return HandleServiceError(err, "delete_relationship_failed")
		}

		payload, err := json.Marshal(deleteRelationshipResponse{
			RelationshipID: relationshipID.String(),
			Deleted:        true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(payload)), nil
	})
}

type relationshipToolItem struct {
	ID               string  `json:"id"`
	SourceTableName  string  `json:"source_table_name,omitempty"`
	SourceColumnName string  `json:"source_column_name,omitempty"`
	SourceColumnType string  `json:"source_column_type,omitempty"`
	TargetTableName  string  `json:"target_table_name,omitempty"`
	TargetColumnName string  `json:"target_column_name,omitempty"`
	TargetColumnType string  `json:"target_column_type,omitempty"`
	RelationshipType string  `json:"relationship_type"`
	Cardinality      string  `json:"cardinality,omitempty"`
	Confidence       float64 `json:"confidence"`
	InferenceMethod  *string `json:"inference_method,omitempty"`
	IsApproved       *bool   `json:"is_approved,omitempty"`
	Source           string  `json:"source"`
	LastEditSource   *string `json:"last_edit_source,omitempty"`
	EffectiveSource  string  `json:"effective_source"`
	CreatedBy        *string `json:"created_by,omitempty"`
	UpdatedBy        *string `json:"updated_by,omitempty"`
}

type listRelationshipsResponse struct {
	Relationships []relationshipToolItem `json:"relationships"`
	TotalCount    int                    `json:"total_count"`
}

type writeRelationshipResponse struct {
	Relationship *relationshipToolItem `json:"relationship,omitempty"`
	Created      bool                  `json:"created,omitempty"`
	Updated      bool                  `json:"updated,omitempty"`
}

type deleteRelationshipResponse struct {
	RelationshipID string `json:"relationship_id"`
	Deleted        bool   `json:"deleted"`
}

func getRelationshipToolItemByID(
	ctx context.Context,
	deps *RelationshipToolDeps,
	projectID, relationshipID uuid.UUID,
) (*relationshipToolItem, error) {
	relationshipsResponse, err := deps.SchemaService.GetRelationshipsResponse(ctx, projectID, uuid.Nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships: %w", err)
	}

	for _, rel := range relationshipsResponse.Relationships {
		if rel.ID == relationshipID {
			item := relationshipToolItemFromDetail(rel)
			return &item, nil
		}
	}

	return nil, nil
}

func relationshipToolItemFromDetail(rel *models.RelationshipDetail) relationshipToolItem {
	return relationshipToolItem{
		ID:               rel.ID.String(),
		SourceTableName:  rel.SourceTableName,
		SourceColumnName: rel.SourceColumnName,
		SourceColumnType: rel.SourceColumnType,
		TargetTableName:  rel.TargetTableName,
		TargetColumnName: rel.TargetColumnName,
		TargetColumnType: rel.TargetColumnType,
		RelationshipType: rel.RelationshipType,
		Cardinality:      rel.Cardinality,
		Confidence:       rel.Confidence,
		InferenceMethod:  rel.InferenceMethod,
		IsApproved:       rel.IsApproved,
		Source:           rel.Source,
		LastEditSource:   rel.LastEditSource,
		EffectiveSource:  rel.EffectiveSource,
		CreatedBy:        uuidPtrToString(rel.CreatedBy),
		UpdatedBy:        uuidPtrToString(rel.UpdatedBy),
	}
}

func relationshipToolItemFromModel(rel *models.SchemaRelationship) *relationshipToolItem {
	if rel == nil {
		return nil
	}

	return &relationshipToolItem{
		ID:               rel.ID.String(),
		RelationshipType: rel.RelationshipType,
		Cardinality:      rel.Cardinality,
		Confidence:       rel.Confidence,
		InferenceMethod:  rel.InferenceMethod,
		IsApproved:       rel.IsApproved,
		Source:           rel.Source,
		LastEditSource:   rel.LastEditSource,
		EffectiveSource:  rel.EffectiveSource(),
		CreatedBy:        uuidPtrToString(rel.CreatedBy),
		UpdatedBy:        uuidPtrToString(rel.UpdatedBy),
	}
}

func uuidPtrToString(id *uuid.UUID) *string {
	if id == nil {
		return nil
	}
	value := id.String()
	return &value
}
