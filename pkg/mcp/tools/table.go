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

// TableToolDeps contains dependencies for table metadata tools.
type TableToolDeps struct {
	DB                *database.DB
	MCPConfigService  services.MCPConfigService
	SchemaRepo        repositories.SchemaRepository
	TableMetadataRepo repositories.TableMetadataRepository
	ProjectService    services.ProjectService
	Logger            *zap.Logger
}

// GetDB implements ToolAccessDeps.
func (d *TableToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *TableToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }

// GetLogger implements ToolAccessDeps.
func (d *TableToolDeps) GetLogger() *zap.Logger { return d.Logger }

// RegisterTableTools registers table metadata MCP tools.
func RegisterTableTools(s *server.MCPServer, deps *TableToolDeps) {
	registerUpdateTableTool(s, deps)
	registerDeleteTableMetadataTool(s, deps)
}

// registerUpdateTableTool adds the update_table tool for adding or updating table metadata.
func registerUpdateTableTool(s *server.MCPServer, deps *TableToolDeps) {
	tool := mcp.NewTool(
		"update_table",
		mcp.WithDescription(
			"Add or update metadata about a table. "+
				"Use this to document table purpose, mark tables as ephemeral/transient, or indicate preferred alternatives. "+
				"The table name is the upsert key - if metadata exists for this table, it will be updated; otherwise, new metadata is created. "+
				"Optional parameters (description, usage_notes, is_ephemeral, preferred_alternative) are merged with existing data when provided. "+
				"Omitted parameters preserve existing values. "+
				"Example: update_table(table='sessions', description='Transient session tracking', usage_notes='Do not use for analytics', is_ephemeral=true, preferred_alternative='billing_engagements')",
		),
		mcp.WithString(
			"table",
			mcp.Required(),
			mcp.Description("Table name to update (e.g., 'sessions', 'billing_transactions')"),
		),
		mcp.WithString(
			"description",
			mcp.Description("Optional - What this table represents and contains"),
		),
		mcp.WithString(
			"usage_notes",
			mcp.Description("Optional - When to use or not use this table for queries"),
		),
		mcp.WithBoolean(
			"is_ephemeral",
			mcp.Description("Optional - Mark as transient/temporary table not suitable for analytics"),
		),
		mcp.WithString(
			"preferred_alternative",
			mcp.Description("Optional - Table to use instead if this one is ephemeral or deprecated"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "update_table")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required table parameter
		table, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}
		table = trimString(table)
		if table == "" {
			return NewErrorResult("invalid_parameters", "parameter 'table' cannot be empty"), nil
		}

		// Get optional parameters
		description := getOptionalString(req, "description")
		usageNotes := getOptionalString(req, "usage_notes")
		preferredAlternative := getOptionalString(req, "preferred_alternative")

		// Extract optional is_ephemeral flag
		var isEphemeral *bool
		if args, ok := req.Params.Arguments.(map[string]any); ok {
			if val, exists := args["is_ephemeral"]; exists {
				if boolVal, ok := val.(bool); ok {
					isEphemeral = &boolVal
				}
			}
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
				fmt.Sprintf("table %q not found in schema registry. Run refresh_schema() after creating tables.", table)), nil
		}

		// Validate preferred_alternative exists if provided
		if preferredAlternative != "" {
			altTable, err := deps.SchemaRepo.FindTableByName(tenantCtx, projectID, datasourceID, preferredAlternative)
			if err != nil {
				return nil, fmt.Errorf("failed to lookup preferred_alternative table: %w", err)
			}
			if altTable == nil {
				return NewErrorResult("PREFERRED_ALTERNATIVE_NOT_FOUND",
					fmt.Sprintf("preferred_alternative table %q not found in schema registry", preferredAlternative)), nil
			}
		}

		// Check existing metadata for precedence
		existing, err := deps.TableMetadataRepo.Get(tenantCtx, projectID, datasourceID, table)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing table metadata: %w", err)
		}

		// If metadata exists, check precedence before updating
		if existing != nil {
			if !canModify(existing.Source, existing.LastEditSource, models.ProvenanceMCP) {
				effectiveSource := existing.Source
				if existing.LastEditSource != nil && *existing.LastEditSource != "" {
					effectiveSource = *existing.LastEditSource
				}
				return NewErrorResult("precedence_blocked",
					fmt.Sprintf("Cannot modify table metadata: precedence blocked (existing: %s, modifier: %s). "+
						"Admin changes cannot be overridden by MCP. Use the UI to modify or delete this metadata.",
						effectiveSource, models.ProvenanceMCP)), nil
			}
		}

		// Build metadata for upsert
		meta := &models.TableMetadata{
			ProjectID:    projectID,
			DatasourceID: datasourceID,
			TableName:    table,
			Source:       models.ProvenanceMCP,
		}

		// Set optional fields if provided
		if description != "" {
			meta.Description = &description
		}
		if usageNotes != "" {
			meta.UsageNotes = &usageNotes
		}
		if isEphemeral != nil {
			meta.IsEphemeral = *isEphemeral
		}
		if preferredAlternative != "" {
			meta.PreferredAlternative = &preferredAlternative
		}

		// Upsert metadata
		if err := deps.TableMetadataRepo.Upsert(tenantCtx, meta); err != nil {
			return nil, fmt.Errorf("failed to update table metadata: %w", err)
		}

		// Build response
		isNew := existing == nil
		response := updateTableResponse{
			Table:                table,
			Description:          ptrToString(meta.Description),
			UsageNotes:           ptrToString(meta.UsageNotes),
			IsEphemeral:          meta.IsEphemeral,
			PreferredAlternative: ptrToString(meta.PreferredAlternative),
			Created:              isNew,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// updateTableResponse is the response format for update_table tool.
type updateTableResponse struct {
	Table                string `json:"table"`
	Description          string `json:"description,omitempty"`
	UsageNotes           string `json:"usage_notes,omitempty"`
	IsEphemeral          bool   `json:"is_ephemeral"`
	PreferredAlternative string `json:"preferred_alternative,omitempty"`
	Created              bool   `json:"created"` // true if table metadata was newly added, false if updated
}

// ptrToString safely converts a string pointer to string, returning empty string for nil.
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// registerDeleteTableMetadataTool adds the delete_table_metadata tool for clearing custom table metadata.
func registerDeleteTableMetadataTool(s *server.MCPServer, deps *TableToolDeps) {
	tool := mcp.NewTool(
		"delete_table_metadata",
		mcp.WithDescription(
			"Clear custom metadata for a table, removing semantic enrichment. "+
				"Use this to remove incorrect or outdated table annotations. "+
				"Example: delete_table_metadata(table='sessions')",
		),
		mcp.WithString(
			"table",
			mcp.Required(),
			mcp.Description("Table name to clear metadata for"),
		),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "delete_table_metadata")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required table parameter
		table, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}
		table = trimString(table)
		if table == "" {
			return NewErrorResult("invalid_parameters", "parameter 'table' cannot be empty"), nil
		}

		// Get datasource ID
		datasourceID, err := deps.ProjectService.GetDefaultDatasourceID(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get datasource: %w", err)
		}

		// Check if metadata exists before deleting
		existing, err := deps.TableMetadataRepo.Get(tenantCtx, projectID, datasourceID, table)
		if err != nil {
			return nil, fmt.Errorf("failed to check existing table metadata: %w", err)
		}

		if existing == nil {
			// No metadata found for this table
			result := deleteTableMetadataResponse{
				Table:   table,
				Deleted: false,
			}
			jsonResult, err := json.Marshal(result)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal result: %w", err)
			}
			return mcp.NewToolResultText(string(jsonResult)), nil
		}

		// Delete the metadata
		if err := deps.TableMetadataRepo.Delete(tenantCtx, projectID, datasourceID, table); err != nil {
			return nil, fmt.Errorf("failed to delete table metadata: %w", err)
		}

		// Build response
		result := deleteTableMetadataResponse{
			Table:   table,
			Deleted: true,
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// deleteTableMetadataResponse is the response format for delete_table_metadata tool.
type deleteTableMetadataResponse struct {
	Table   string `json:"table"`
	Deleted bool   `json:"deleted"` // true if metadata was deleted, false if not found
}
