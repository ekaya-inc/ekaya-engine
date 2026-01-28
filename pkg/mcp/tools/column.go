// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ColumnToolDeps contains dependencies for column metadata tools.
type ColumnToolDeps struct {
	DB                 *database.DB
	MCPConfigService   services.MCPConfigService
	OntologyRepo       repositories.OntologyRepository
	SchemaRepo         repositories.SchemaRepository
	ColumnMetadataRepo repositories.ColumnMetadataRepository
	ProjectService     services.ProjectService
	Logger             *zap.Logger
}

// GetDB implements ToolAccessDeps.
func (d *ColumnToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *ColumnToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }

// GetLogger implements ToolAccessDeps.
func (d *ColumnToolDeps) GetLogger() *zap.Logger { return d.Logger }

// RegisterColumnTools registers column metadata MCP tools.
func RegisterColumnTools(s *server.MCPServer, deps *ColumnToolDeps) {
	registerGetColumnMetadataTool(s, deps)
	registerUpdateColumnTool(s, deps)
	registerDeleteColumnMetadataTool(s, deps)
}

// registerGetColumnMetadataTool adds the get_column_metadata tool for inspecting current column metadata.
func registerGetColumnMetadataTool(s *server.MCPServer, deps *ColumnToolDeps) {
	tool := mcp.NewTool(
		"get_column_metadata",
		mcp.WithDescription(
			"Get current ontology metadata for a specific column. "+
				"Returns description, semantic_type, enum_values, entity, role, and schema info (data_type, is_nullable, is_primary_key). "+
				"Use this before update_column to see what's already documented. "+
				"Example: get_column_metadata(table='users', column='status') returns current metadata for the status column.",
		),
		mcp.WithString(
			"table",
			mcp.Required(),
			mcp.Description("Table name containing the column"),
		),
		mcp.WithString(
			"column",
			mcp.Required(),
			mcp.Description("Column name to inspect"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccessWithoutProvenance(ctx, deps, "get_column_metadata")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required parameters
		table, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}
		table = trimString(table)
		if table == "" {
			return NewErrorResult("invalid_parameters", "parameter 'table' cannot be empty"), nil
		}

		column, err := req.RequireString("column")
		if err != nil {
			return nil, err
		}
		column = trimString(column)
		if column == "" {
			return NewErrorResult("invalid_parameters", "parameter 'column' cannot be empty"), nil
		}

		// Get datasource ID
		datasourceID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get datasource: %w", err)
		}

		// Validate table exists in schema registry
		schemaTable, err := deps.SchemaRepo.FindTableByName(tenantCtx, projectID, datasourceID, table)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup table: %w", err)
		}
		if schemaTable == nil {
			return NewErrorResult("TABLE_NOT_FOUND",
				fmt.Sprintf("table %q not found in schema registry", table)), nil
		}

		// Validate column exists in table
		schemaColumn, err := deps.SchemaRepo.GetColumnByName(tenantCtx, schemaTable.ID, column)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup column: %w", err)
		}
		if schemaColumn == nil {
			return NewErrorResult("COLUMN_NOT_FOUND",
				fmt.Sprintf("column %q not found in table %q", column, table)), nil
		}

		// Build response with schema info
		response := getColumnMetadataResponse{
			Table:  table,
			Column: column,
			Schema: columnSchemaInfo{
				DataType:     schemaColumn.DataType,
				IsNullable:   schemaColumn.IsNullable,
				IsPrimaryKey: schemaColumn.IsPrimaryKey,
			},
		}

		// Get ontology metadata if available
		ontology, err := deps.OntologyRepo.GetActive(tenantCtx, projectID)
		if err != nil {
			deps.Logger.Warn("Failed to get active ontology for column metadata",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
		}

		if ontology != nil {
			// Get column details from ontology
			columnDetails := ontology.GetColumnDetails(table)
			for _, colDetail := range columnDetails {
				if colDetail.Name == column {
					response.Metadata = &columnMetadataInfo{
						Description:  colDetail.Description,
						SemanticType: colDetail.SemanticType,
						Entity:       colDetail.SemanticType, // Entity is stored in SemanticType
						Role:         colDetail.Role,
					}

					// Format enum values
					if len(colDetail.EnumValues) > 0 {
						response.Metadata.EnumValues = formatEnumValues(colDetail.EnumValues)
					}
					break
				}
			}
		}

		// Fallback: check engine_column_metadata for additional metadata
		if deps.ColumnMetadataRepo != nil {
			columnMeta, err := deps.ColumnMetadataRepo.GetByTableColumn(tenantCtx, projectID, table, column)
			if err != nil {
				deps.Logger.Warn("Failed to get column metadata fallback",
					zap.String("project_id", projectID.String()),
					zap.String("table", table),
					zap.String("column", column),
					zap.Error(err))
			} else if columnMeta != nil {
				// Initialize metadata section if not already present
				if response.Metadata == nil {
					response.Metadata = &columnMetadataInfo{}
				}

				// Merge in column_metadata values if not already set from ontology
				if columnMeta.Description != nil && *columnMeta.Description != "" && response.Metadata.Description == "" {
					response.Metadata.Description = *columnMeta.Description
				}
				if columnMeta.Entity != nil && *columnMeta.Entity != "" && response.Metadata.Entity == "" {
					response.Metadata.Entity = *columnMeta.Entity
				}
				if columnMeta.Role != nil && *columnMeta.Role != "" && response.Metadata.Role == "" {
					response.Metadata.Role = *columnMeta.Role
				}
				if len(columnMeta.EnumValues) > 0 && len(response.Metadata.EnumValues) == 0 {
					response.Metadata.EnumValues = columnMeta.EnumValues
				}
			}
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// getColumnMetadataResponse is the response format for get_column_metadata tool.
type getColumnMetadataResponse struct {
	Table    string              `json:"table"`
	Column   string              `json:"column"`
	Schema   columnSchemaInfo    `json:"schema"`
	Metadata *columnMetadataInfo `json:"metadata,omitempty"`
}

// columnSchemaInfo contains schema-level information about a column.
type columnSchemaInfo struct {
	DataType     string `json:"data_type"`
	IsNullable   bool   `json:"is_nullable"`
	IsPrimaryKey bool   `json:"is_primary_key"`
}

// columnMetadataInfo contains ontology metadata for a column.
type columnMetadataInfo struct {
	Description  string   `json:"description,omitempty"`
	SemanticType string   `json:"semantic_type,omitempty"`
	EnumValues   []string `json:"enum_values,omitempty"`
	Entity       string   `json:"entity,omitempty"`
	Role         string   `json:"role,omitempty"`
}

// registerUpdateColumnTool adds the update_column tool for adding or updating column semantic information.
func registerUpdateColumnTool(s *server.MCPServer, deps *ColumnToolDeps) {
	tool := mcp.NewTool(
		"update_column",
		mcp.WithDescription(
			"Add or update semantic information about a column in the ontology. "+
				"The table and column name form the upsert key - if metadata exists for this column, it will be updated; otherwise, new metadata is created. "+
				"Optional parameters (description, enum_values, entity, role) are merged with existing data when provided. "+
				"Omitted parameters preserve existing values. "+
				"Example: update_column(table='users', column='status', description='User account status', enum_values=['ACTIVE - Normal active account', 'SUSPENDED - Temporarily disabled'], entity='User', role='attribute')",
		),
		mcp.WithString(
			"table",
			mcp.Required(),
			mcp.Description("Table name containing the column (e.g., 'users', 'billing_transactions')"),
		),
		mcp.WithString(
			"column",
			mcp.Required(),
			mcp.Description("Column name to update (e.g., 'status', 'transaction_state')"),
		),
		mcp.WithString(
			"description",
			mcp.Description("Optional - Business description of what this column represents"),
		),
		mcp.WithArray(
			"enum_values",
			mcp.Description("Optional - Array of enumeration values with descriptions (e.g., ['ACTIVE - Normal account', 'SUSPENDED - Temporary hold'])"),
		),
		mcp.WithString(
			"entity",
			mcp.Description("Optional - Entity this column belongs to (e.g., 'User', 'Account')"),
		),
		mcp.WithString(
			"role",
			mcp.Description("Optional - Semantic role: 'dimension', 'measure', 'identifier', or 'attribute'"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_column")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required parameters
		table, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}
		// Validate table is not empty after trimming whitespace
		table = trimString(table)
		if table == "" {
			return NewErrorResult("invalid_parameters", "parameter 'table' cannot be empty"), nil
		}

		column, err := req.RequireString("column")
		if err != nil {
			return nil, err
		}
		// Validate column is not empty after trimming whitespace
		column = trimString(column)
		if column == "" {
			return NewErrorResult("invalid_parameters", "parameter 'column' cannot be empty"), nil
		}

		// Get optional parameters
		description := getOptionalString(req, "description")
		entity := getOptionalString(req, "entity")
		role := getOptionalString(req, "role")

		// Validate role if provided
		if role != "" {
			validRoles := []string{"dimension", "measure", "identifier", "attribute"}
			isValidRole := false
			for _, validRole := range validRoles {
				if role == validRole {
					isValidRole = true
					break
				}
			}
			if !isValidRole {
				return NewErrorResultWithDetails(
					"invalid_parameters",
					fmt.Sprintf("parameter 'role' must be one of: dimension, measure, identifier, attribute. Got: %q", role),
					map[string]any{
						"parameter": "role",
						"expected":  validRoles,
						"actual":    role,
					},
				), nil
			}
		}

		// Extract and validate enum_values array
		var enumValues []string
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if enumArray, ok := args["enum_values"].([]any); ok {
				for i, ev := range enumArray {
					evStr, ok := ev.(string)
					if !ok {
						return NewErrorResultWithDetails(
							"invalid_parameters",
							fmt.Sprintf("parameter 'enum_values' must be an array of strings. Element at index %d is %T, not string", i, ev),
							map[string]any{
								"parameter":             "enum_values",
								"invalid_element_index": i,
								"invalid_element_type":  fmt.Sprintf("%T", ev),
							},
						), nil
					}
					enumValues = append(enumValues, evStr)
				}
			}
		}

		// Validate table and column exist in schema registry
		datasourceID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get datasource: %w", err)
		}

		// Validate table exists in schema registry
		schemaTable, err := deps.SchemaRepo.FindTableByName(tenantCtx, projectID, datasourceID, table)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup table: %w", err)
		}
		if schemaTable == nil {
			return NewErrorResult("TABLE_NOT_FOUND",
				fmt.Sprintf("table %q not found in schema registry. Run refresh_schema() after creating tables.", table)), nil
		}

		// Validate column exists in table
		schemaColumn, err := deps.SchemaRepo.GetColumnByName(tenantCtx, schemaTable.ID, column)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup column: %w", err)
		}
		if schemaColumn == nil {
			return NewErrorResult("COLUMN_NOT_FOUND",
				fmt.Sprintf("column %q not found in table %q", column, table)), nil
		}

		// Get or create active ontology (enables immediate use without extraction)
		ontology, err := ensureOntologyExists(tenantCtx, deps.OntologyRepo, projectID)
		if err != nil {
			return NewErrorResult("ontology_error", err.Error()), nil
		}

		// Get existing column details for this table
		existingColumns := ontology.GetColumnDetails(table)
		if existingColumns == nil {
			existingColumns = []models.ColumnDetail{}
		}

		// Find existing column or create new one
		var targetColumn *models.ColumnDetail
		columnIndex := -1
		for i := range existingColumns {
			if existingColumns[i].Name == column {
				targetColumn = &existingColumns[i]
				columnIndex = i
				break
			}
		}

		isNew := targetColumn == nil
		if isNew {
			// Create new column detail
			targetColumn = &models.ColumnDetail{
				Name: column,
			}
		}

		// Update fields if provided
		if description != "" {
			targetColumn.Description = description
		}

		if entity != "" {
			// Store entity as semantic type (this field is used for entity associations)
			targetColumn.SemanticType = entity
		}

		if role != "" {
			targetColumn.Role = role
		}

		// Process enum values if provided
		if enumValues != nil {
			targetColumn.EnumValues = parseEnumValues(enumValues)
		}

		// Update or append column
		if isNew {
			existingColumns = append(existingColumns, *targetColumn)
		} else {
			existingColumns[columnIndex] = *targetColumn
		}

		// Save updated column details back to ontology
		if err := deps.OntologyRepo.UpdateColumnDetails(tenantCtx, projectID, table, existingColumns); err != nil {
			return nil, fmt.Errorf("failed to update column details: %w", err)
		}

		// Also track provenance in column_metadata table if available
		if deps.ColumnMetadataRepo != nil {
			// Check if existing metadata exists and verify precedence
			existing, err := deps.ColumnMetadataRepo.GetByTableColumn(tenantCtx, projectID, table, column)
			if err != nil {
				return nil, fmt.Errorf("failed to check existing column metadata: %w", err)
			}

			// If metadata exists, check precedence before updating
			if existing != nil {
				if !canModify(existing.Source, existing.LastEditSource, models.ProvenanceMCP) {
					effectiveSource := existing.Source
					if existing.LastEditSource != nil && *existing.LastEditSource != "" {
						effectiveSource = *existing.LastEditSource
					}
					return NewErrorResult("precedence_blocked",
						fmt.Sprintf("Cannot modify column metadata: precedence blocked (existing: %s, modifier: %s). "+
							"Admin changes cannot be overridden by MCP. Use the UI to modify or delete this metadata.",
							effectiveSource, models.ProvenanceMCP)), nil
				}
			}

			lastEditSource := models.ProvenanceMCP
			colMeta := &models.ColumnMetadata{
				ProjectID:      projectID,
				TableName:      table,
				ColumnName:     column,
				Source:         models.ProvenanceMCP,
				LastEditSource: &lastEditSource,
			}
			if description != "" {
				colMeta.Description = &description
			}
			if entity != "" {
				colMeta.Entity = &entity
			}
			if role != "" {
				colMeta.Role = &role
			}
			if enumValues != nil {
				colMeta.EnumValues = enumValues
			}
			if err := deps.ColumnMetadataRepo.Upsert(tenantCtx, colMeta); err != nil {
				return nil, fmt.Errorf("failed to update column metadata: %w", err)
			}
		}

		// Build response
		response := updateColumnResponse{
			Table:       table,
			Column:      column,
			Description: targetColumn.Description,
			EnumValues:  formatEnumValues(targetColumn.EnumValues),
			Entity:      targetColumn.SemanticType,
			Role:        targetColumn.Role,
			Created:     isNew,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerDeleteColumnMetadataTool adds the delete_column_metadata tool for clearing custom column metadata.
func registerDeleteColumnMetadataTool(s *server.MCPServer, deps *ColumnToolDeps) {
	tool := mcp.NewTool(
		"delete_column_metadata",
		mcp.WithDescription(
			"Clear custom metadata for a column, reverting to schema-only information. "+
				"This removes the semantic enrichment added via update_column while preserving schema information. "+
				"Use this to remove incorrect or outdated column annotations. "+
				"Example: delete_column_metadata(table='users', column='status')",
		),
		mcp.WithString(
			"table",
			mcp.Required(),
			mcp.Description("Table name containing the column"),
		),
		mcp.WithString(
			"column",
			mcp.Required(),
			mcp.Description("Column name to clear metadata for"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "delete_column_metadata")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required parameters
		table, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}

		column, err := req.RequireString("column")
		if err != nil {
			return nil, err
		}

		// Get or create active ontology (enables immediate use without extraction)
		ontology, err := ensureOntologyExists(tenantCtx, deps.OntologyRepo, projectID)
		if err != nil {
			return NewErrorResult("ontology_error", err.Error()), nil
		}

		// Get existing column details for this table
		existingColumns := ontology.GetColumnDetails(table)
		if existingColumns == nil {
			// No columns for this table, nothing to delete
			result := deleteColumnMetadataResponse{
				Table:   table,
				Column:  column,
				Deleted: false,
			}
			jsonResult, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %w", err)
			}
			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Find and remove the column
		found := false
		newColumns := make([]models.ColumnDetail, 0, len(existingColumns))
		for i := range existingColumns {
			if existingColumns[i].Name != column {
				newColumns = append(newColumns, existingColumns[i])
			} else {
				found = true
			}
		}

		if !found {
			// Column not found in metadata, nothing to delete
			result := deleteColumnMetadataResponse{
				Table:   table,
				Column:  column,
				Deleted: false,
			}
			jsonResult, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %w", err)
			}
			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Save updated column details back to ontology
		if err := deps.OntologyRepo.UpdateColumnDetails(tenantCtx, projectID, table, newColumns); err != nil {
			return nil, fmt.Errorf("failed to update column details: %w", err)
		}

		// Build response
		result := deleteColumnMetadataResponse{
			Table:   table,
			Column:  column,
			Deleted: true,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// parseEnumValues converts string array to EnumValue structs.
// Supports format: "VALUE - Description" or just "VALUE"
func parseEnumValues(enumStrings []string) []models.EnumValue {
	result := make([]models.EnumValue, 0, len(enumStrings))
	for _, enumStr := range enumStrings {
		// Try to split on " - " to extract value and description
		ev := models.EnumValue{}
		// Simple parsing: if contains " - ", split it
		if len(enumStr) > 0 {
			// Find first occurrence of " - "
			sepIndex := -1
			for i := 0; i < len(enumStr)-2; i++ {
				if enumStr[i:i+3] == " - " {
					sepIndex = i
					break
				}
			}

			if sepIndex > 0 {
				ev.Value = enumStr[:sepIndex]
				ev.Description = enumStr[sepIndex+3:]
			} else {
				ev.Value = enumStr
			}
		}
		result = append(result, ev)
	}
	return result
}

// formatEnumValues converts EnumValue structs back to string array for response.
func formatEnumValues(enumValues []models.EnumValue) []string {
	if enumValues == nil {
		return nil
	}
	result := make([]string, 0, len(enumValues))
	for _, ev := range enumValues {
		if ev.Description != "" {
			result = append(result, fmt.Sprintf("%s - %s", ev.Value, ev.Description))
		} else {
			result = append(result, ev.Value)
		}
	}
	return result
}

// updateColumnResponse is the response format for update_column tool.
type updateColumnResponse struct {
	Table       string   `json:"table"`
	Column      string   `json:"column"`
	Description string   `json:"description,omitempty"`
	EnumValues  []string `json:"enum_values,omitempty"`
	Entity      string   `json:"entity,omitempty"`
	Role        string   `json:"role,omitempty"`
	Created     bool     `json:"created"` // true if column was newly added, false if updated
}

// deleteColumnMetadataResponse is the response format for delete_column_metadata tool.
type deleteColumnMetadataResponse struct {
	Table   string `json:"table"`
	Column  string `json:"column"`
	Deleted bool   `json:"deleted"` // true if metadata was deleted, false if not found
}

// canModify checks if a source can modify column metadata based on precedence.
// Precedence hierarchy: Admin (3) > MCP (2) > Inference (1)
// Returns true if the modification is allowed, false if blocked by higher precedence.
func canModify(elementCreatedBy string, elementUpdatedBy *string, modifierSource string) bool {
	modifierLevel := precedenceLevel(modifierSource)

	// Check against updated_by if present, otherwise check created_by
	var existingSource string
	if elementUpdatedBy != nil && *elementUpdatedBy != "" {
		existingSource = *elementUpdatedBy
	} else {
		existingSource = elementCreatedBy
	}

	existingLevel := precedenceLevel(existingSource)

	// Modifier can change if their level is >= existing level
	return modifierLevel >= existingLevel
}

// precedenceLevel returns the numeric precedence level for a source.
// Higher number = higher precedence.
func precedenceLevel(source string) int {
	switch source {
	case models.ProvenanceManual:
		return 3
	case models.ProvenanceMCP:
		return 2
	case models.ProvenanceInferred:
		return 1
	default:
		return 0 // Unknown source has lowest precedence
	}
}
