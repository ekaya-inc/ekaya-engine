// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// MaxBatchUpdateSize is the maximum number of updates allowed per batch.
const MaxBatchUpdateSize = 50

// ColumnUpdate represents a single column update in a batch.
type ColumnUpdate struct {
	Table       string   `json:"table"`
	Column      string   `json:"column"`
	Description *string  `json:"description,omitempty"`
	EnumValues  []string `json:"enum_values,omitempty"`
	Entity      *string  `json:"entity,omitempty"`
	Role        *string  `json:"role,omitempty"`
}

// ColumnUpdateResult represents the result of a single column update.
type ColumnUpdateResult struct {
	Table   string `json:"table"`
	Column  string `json:"column"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Created bool   `json:"created,omitempty"`
}

// UpdateColumnsResponse is the response format for update_columns tool.
type UpdateColumnsResponse struct {
	Updated int                  `json:"updated"`
	Results []ColumnUpdateResult `json:"results"`
}

// RegisterBatchTools registers batch update MCP tools.
func RegisterBatchTools(s *server.MCPServer, deps *ColumnToolDeps) {
	registerUpdateColumnsTool(s, deps)
}

// registerUpdateColumnsTool adds the update_columns tool for batch updating multiple columns.
func registerUpdateColumnsTool(s *server.MCPServer, deps *ColumnToolDeps) {
	tool := mcp.NewTool(
		"update_columns",
		mcp.WithDescription(
			"Update metadata for multiple columns in a single call. "+
				"Useful for applying the same pattern across tables (e.g., soft delete, audit timestamps). "+
				"Each update specifies table, column, and the fields to update. "+
				fmt.Sprintf("Maximum %d updates per call. All updates are applied in a single transaction (all-or-nothing).", MaxBatchUpdateSize),
		),
		mcp.WithArray(
			"updates",
			mcp.Required(),
			mcp.Description("Array of column updates, each with table, column, and metadata fields (description, enum_values, entity, role)"),
			mcp.Items(map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table":       map[string]any{"type": "string", "description": "Table name"},
					"column":      map[string]any{"type": "string", "description": "Column name"},
					"description": map[string]any{"type": "string", "description": "Column description"},
					"entity":      map[string]any{"type": "string", "description": "Entity the column belongs to"},
					"role":        map[string]any{"type": "string", "description": "Column role: dimension, measure, identifier, attribute"},
					"enum_values": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Enum value labels"},
				},
				"required": []string{"table", "column"},
			}),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_columns")
		if err != nil {
			if result := AsToolAccessResult(err); result != nil {
				return result, nil
			}
			return nil, err
		}
		defer cleanup()

		// Parse updates array from request
		updates, err := parseColumnUpdates(req)
		if err != nil {
			return NewErrorResult("invalid_parameters", err.Error()), nil
		}

		// Validate batch size
		if len(updates) == 0 {
			return NewErrorResult("invalid_parameters", "updates array cannot be empty"), nil
		}
		if len(updates) > MaxBatchUpdateSize {
			return NewErrorResultWithDetails(
				"invalid_parameters",
				fmt.Sprintf("too many updates: maximum %d allowed per call, got %d", MaxBatchUpdateSize, len(updates)),
				map[string]any{
					"max_allowed": MaxBatchUpdateSize,
					"received":    len(updates),
				},
			), nil
		}

		// Get datasource ID for schema validation
		datasourceID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get datasource: %w", err)
		}

		// Phase 1: Validate all updates before applying any
		validationResults := make([]ColumnUpdateResult, len(updates))
		allValid := true
		for i, update := range updates {
			validationResults[i] = ColumnUpdateResult{
				Table:  update.Table,
				Column: update.Column,
			}

			if err := validateColumnUpdate(tenantCtx, deps, projectID, datasourceID, update); err != nil {
				validationResults[i].Status = "error"
				validationResults[i].Error = err.Error()
				allValid = false
			} else {
				validationResults[i].Status = "pending"
			}
		}

		// If any validation failed, return early with all results
		if !allValid {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(mustMarshal(UpdateColumnsResponse{
						Updated: 0,
						Results: validationResults,
					})),
				},
				IsError: true,
			}, nil
		}

		// Phase 2: Apply all updates via ColumnMetadataRepo
		results := make([]ColumnUpdateResult, len(updates))
		updatedCount := 0

		for i, update := range updates {
			results[i] = ColumnUpdateResult{
				Table:  update.Table,
				Column: update.Column,
			}

			// Check if metadata already exists to determine created vs updated
			schemaTable, _ := deps.SchemaRepo.FindTableByName(tenantCtx, projectID, datasourceID, update.Table)
			var isNew bool
			if schemaTable != nil {
				schemaColumn, _ := deps.SchemaRepo.GetColumnByName(tenantCtx, schemaTable.ID, update.Column)
				if schemaColumn != nil {
					existing, _ := deps.ColumnMetadataRepo.GetBySchemaColumnID(tenantCtx, schemaColumn.ID)
					isNew = existing == nil
				}
			}

			if err := trackColumnMetadata(tenantCtx, deps, projectID, datasourceID, update); err != nil {
				results[i].Status = "error"
				results[i].Error = fmt.Sprintf("failed to save: %v", err)
			} else {
				results[i].Status = "success"
				results[i].Created = isNew
				updatedCount++
			}
		}

		response := UpdateColumnsResponse{
			Updated: updatedCount,
			Results: results,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// parseColumnUpdates parses the updates array from the request.
func parseColumnUpdates(req mcp.CallToolRequest) ([]ColumnUpdate, error) {
	args, ok := req.Params.Arguments.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid request arguments")
	}

	updatesRaw, parseErr := extractArrayParam(args, "updates", nil)
	if parseErr != nil {
		return nil, parseErr
	}
	if updatesRaw == nil {
		return nil, fmt.Errorf("'updates' must be an array")
	}

	updates := make([]ColumnUpdate, 0, len(updatesRaw))
	for i, raw := range updatesRaw {
		updateMap, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("update at index %d must be an object", i)
		}

		update := ColumnUpdate{}

		// Required fields
		table, ok := updateMap["table"].(string)
		if !ok || table == "" {
			return nil, fmt.Errorf("update at index %d: 'table' is required and must be a non-empty string", i)
		}
		update.Table = trimString(table)
		if update.Table == "" {
			return nil, fmt.Errorf("update at index %d: 'table' cannot be empty", i)
		}

		column, ok := updateMap["column"].(string)
		if !ok || column == "" {
			return nil, fmt.Errorf("update at index %d: 'column' is required and must be a non-empty string", i)
		}
		update.Column = trimString(column)
		if update.Column == "" {
			return nil, fmt.Errorf("update at index %d: 'column' cannot be empty", i)
		}

		// Optional fields
		if desc, ok := updateMap["description"].(string); ok {
			update.Description = &desc
		}
		if entity, ok := updateMap["entity"].(string); ok {
			update.Entity = &entity
		}
		if role, ok := updateMap["role"].(string); ok {
			// Validate role
			validRoles := []string{"dimension", "measure", "identifier", "attribute"}
			isValidRole := false
			for _, validRole := range validRoles {
				if role == validRole {
					isValidRole = true
					break
				}
			}
			if !isValidRole {
				return nil, fmt.Errorf("update at index %d: 'role' must be one of: dimension, measure, identifier, attribute. Got: %q", i, role)
			}
			update.Role = &role
		}

		// Parse enum_values array
		enumRaw, enumErr := extractArrayParam(updateMap, "enum_values", nil)
		if enumErr != nil {
			return nil, fmt.Errorf("update at index %d: %v", i, enumErr)
		}
		if enumRaw != nil {
			enumValues := make([]string, 0, len(enumRaw))
			for j, ev := range enumRaw {
				evStr, ok := ev.(string)
				if !ok {
					return nil, fmt.Errorf("update at index %d: 'enum_values[%d]' must be a string, got %T", i, j, ev)
				}
				enumValues = append(enumValues, evStr)
			}
			update.EnumValues = enumValues
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// validateColumnUpdate validates a single column update.
func validateColumnUpdate(ctx context.Context, deps *ColumnToolDeps, projectID, datasourceID uuid.UUID, update ColumnUpdate) error {
	// Validate table exists
	schemaTable, err := deps.SchemaRepo.FindTableByName(ctx, projectID, datasourceID, update.Table)
	if err != nil {
		return fmt.Errorf("failed to lookup table: %w", err)
	}
	if schemaTable == nil {
		return fmt.Errorf("table %q not found in schema registry", update.Table)
	}

	// Validate column exists
	schemaColumn, err := deps.SchemaRepo.GetColumnByName(ctx, schemaTable.ID, update.Column)
	if err != nil {
		return fmt.Errorf("failed to lookup column: %w", err)
	}
	if schemaColumn == nil {
		return fmt.Errorf("column %q not found in table %q", update.Column, update.Table)
	}

	// Check precedence if metadata repo is available
	// Column metadata is now keyed by schema_column_id (FK to engine_schema_columns)
	if deps.ColumnMetadataRepo != nil && schemaColumn != nil {
		existing, err := deps.ColumnMetadataRepo.GetBySchemaColumnID(ctx, schemaColumn.ID)
		if err != nil {
			return fmt.Errorf("failed to check existing column metadata: %w", err)
		}
		if existing != nil {
			if !canModify(existing.Source, existing.LastEditSource, models.ProvenanceMCP) {
				effectiveSource := existing.Source
				if existing.LastEditSource != nil && *existing.LastEditSource != "" {
					effectiveSource = *existing.LastEditSource
				}
				return fmt.Errorf("precedence blocked (existing: %s, modifier: %s)", effectiveSource, models.ProvenanceMCP)
			}
		}
	}

	return nil
}

// trackColumnMetadata tracks column metadata with provenance.
// Column metadata is now keyed by schema_column_id (FK to engine_schema_columns).
func trackColumnMetadata(ctx context.Context, deps *ColumnToolDeps, projectID, datasourceID uuid.UUID, update ColumnUpdate) error {
	// Look up the schema column to get its ID
	schemaTable, err := deps.SchemaRepo.FindTableByName(ctx, projectID, datasourceID, update.Table)
	if err != nil || schemaTable == nil {
		return fmt.Errorf("table %q not found", update.Table)
	}
	schemaColumn, err := deps.SchemaRepo.GetColumnByName(ctx, schemaTable.ID, update.Column)
	if err != nil || schemaColumn == nil {
		return fmt.Errorf("column %q not found in table %q", update.Column, update.Table)
	}

	lastEditSource := models.ProvenanceMCP
	colMeta := &models.ColumnMetadata{
		ProjectID:      projectID,
		SchemaColumnID: schemaColumn.ID,
		Source:         models.ProvenanceMCP,
		LastEditSource: &lastEditSource,
	}
	if update.Description != nil {
		colMeta.Description = update.Description
	}
	// Entity is now stored in Features.IdentifierFeatures, handled by ontology extraction
	// Role is still a direct field
	if update.Role != nil {
		colMeta.Role = update.Role
	}
	// EnumValues are now stored in Features.EnumFeatures
	if update.EnumValues != nil {
		colMeta.Features.EnumFeatures = &models.EnumFeatures{
			Values: make([]models.ColumnEnumValue, len(update.EnumValues)),
		}
		for i, v := range update.EnumValues {
			colMeta.Features.EnumFeatures.Values[i] = models.ColumnEnumValue{Value: v}
		}
	}
	return deps.ColumnMetadataRepo.Upsert(ctx, colMeta)
}

// findUpdateIndex finds the index of an update in the original array.
func findUpdateIndex(updates []ColumnUpdate, table, column string, startFrom int) int {
	for i := startFrom; i < len(updates); i++ {
		if updates[i].Table == table && updates[i].Column == column {
			return i
		}
	}
	// Fallback: search from beginning
	for i := 0; i < startFrom; i++ {
		if updates[i].Table == table && updates[i].Column == column {
			return i
		}
	}
	return 0
}

// mustMarshal marshals to JSON, panicking on error (for internal use only).
func mustMarshal(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return string(data)
}
