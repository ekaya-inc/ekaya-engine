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

// ContextToolDeps contains dependencies for the unified context tool.
type ContextToolDeps struct {
	DB                     *database.DB
	MCPConfigService       services.MCPConfigService
	ProjectService         services.ProjectService
	OntologyContextService services.OntologyContextService
	OntologyRepo           repositories.OntologyRepository
	SchemaService          services.SchemaService
	GlossaryService        services.GlossaryService
	SchemaRepo             repositories.SchemaRepository
	Logger                 *zap.Logger
}

const contextToolGroup = "approved_queries" // Context tool shares visibility with approved_queries

// RegisterContextTools registers the unified context tool.
func RegisterContextTools(s *server.MCPServer, deps *ContextToolDeps) {
	registerGetContextTool(s, deps)
}

// checkContextToolEnabled verifies the context tool is enabled for the project.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
func checkContextToolEnabled(ctx context.Context, deps *ContextToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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

// registerGetContextTool adds the get_context unified tool.
func registerGetContextTool(s *server.MCPServer, deps *ContextToolDeps) {
	tool := mcp.NewTool(
		"get_context",
		mcp.WithDescription(
			"Get unified database context with progressive depth levels that gracefully degrades when ontology is unavailable. "+
				"Consolidates ontology, schema, and glossary information into a single tool. "+
				"Depth levels: 'domain' (high-level context, ~500 tokens), 'entities' (entity summaries, ~2k tokens), "+
				"'tables' (table details with key columns, ~4k tokens), 'columns' (full column details, ~8k tokens). "+
				"Always returns useful information even without ontology extraction - schema data is always available.",
		),
		mcp.WithString(
			"depth",
			mcp.Required(),
			mcp.Description("Depth level: 'domain' (high-level), 'entities' (entity summaries), 'tables' (table details), or 'columns' (full column details)"),
		),
		mcp.WithArray(
			"tables",
			mcp.Description("Optional: filter to specific tables (for 'tables' or 'columns' depth)"),
		),
		mcp.WithBoolean(
			"include_relationships",
			mcp.Description("Include relationship graph (default: true)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := checkContextToolEnabled(ctx, deps, "get_context")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Parse required depth parameter
		depth, err := req.RequireString("depth")
		if err != nil {
			return nil, err
		}

		// Validate depth value
		validDepths := map[string]bool{
			"domain":   true,
			"entities": true,
			"tables":   true,
			"columns":  true,
		}
		if !validDepths[depth] {
			return nil, fmt.Errorf("invalid depth: must be one of 'domain', 'entities', 'tables', 'columns'")
		}

		// Parse optional parameters
		tables := getStringSlice(req, "tables")
		includeRelationships := getOptionalBoolWithDefault(req, "include_relationships", true)

		// Get the active ontology (may be nil)
		ontology, err := deps.OntologyRepo.GetActive(tenantCtx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to check for ontology: %w", err)
		}

		// Determine ontology status
		ontologyStatus := determineOntologyStatus(ontology)

		// Get glossary terms (always available, not dependent on ontology)
		glossary, err := deps.GlossaryService.GetTerms(tenantCtx, projectID)
		if err != nil {
			deps.Logger.Warn("Failed to get glossary terms",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without glossary - not a fatal error
		}

		// Route to appropriate handler based on ontology availability and depth
		var result any
		if ontology != nil {
			result, err = handleContextWithOntology(tenantCtx, deps, projectID, depth, tables, includeRelationships, ontologyStatus, glossary)
		} else {
			result, err = handleContextWithoutOntology(tenantCtx, deps, projectID, depth, tables, ontologyStatus, glossary)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to get context at depth '%s': %w", depth, err)
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// determineOntologyStatus determines the current state of the ontology.
func determineOntologyStatus(ontology *models.TieredOntology) string {
	if ontology == nil {
		return "none"
	}
	if ontology.IsActive {
		return "complete"
	}
	return "extracting"
}

// handleContextWithOntology returns enriched context when ontology is available.
func handleContextWithOntology(
	ctx context.Context,
	deps *ContextToolDeps,
	projectID uuid.UUID,
	depth string,
	tables []string,
	includeRelationships bool,
	ontologyStatus string,
	glossary []*models.BusinessGlossaryTerm,
) (any, error) {
	// Build base response structure
	response := map[string]any{
		"has_ontology":    true,
		"ontology_status": ontologyStatus,
		"depth":           depth,
	}

	// Route to appropriate handler based on depth
	switch depth {
	case "domain":
		domainCtx, err := deps.OntologyContextService.GetDomainContext(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get domain context: %w", err)
		}
		if !includeRelationships {
			domainCtx.Relationships = nil
		}
		response["domain"] = domainCtx.Domain
		response["entities"] = domainCtx.Entities
		response["relationships"] = domainCtx.Relationships

	case "entities":
		entitiesCtx, err := deps.OntologyContextService.GetEntitiesContext(ctx, projectID)
		if err != nil {
			return nil, fmt.Errorf("failed to get entities context: %w", err)
		}
		if !includeRelationships {
			entitiesCtx.Relationships = nil
		}
		response["entities"] = entitiesCtx.Entities
		response["relationships"] = entitiesCtx.Relationships

	case "tables":
		tablesCtx, err := deps.OntologyContextService.GetTablesContext(ctx, projectID, tables)
		if err != nil {
			return nil, fmt.Errorf("failed to get tables context: %w", err)
		}
		response["tables"] = tablesCtx.Tables

	case "columns":
		columnsCtx, err := deps.OntologyContextService.GetColumnsContext(ctx, projectID, tables)
		if err != nil {
			return nil, fmt.Errorf("failed to get columns context: %w", err)
		}
		response["tables"] = columnsCtx.Tables
	}

	// Add glossary to response (always included regardless of depth)
	response["glossary"] = buildGlossaryResponse(glossary)

	return response, nil
}

// handleContextWithoutOntology returns schema-only context when ontology is not available.
func handleContextWithoutOntology(
	ctx context.Context,
	deps *ContextToolDeps,
	projectID uuid.UUID,
	depth string,
	tables []string,
	ontologyStatus string,
	glossary []*models.BusinessGlossaryTerm,
) (any, error) {
	// Get default datasource
	dsID, err := deps.ProjectService.GetDefaultDatasourceID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get default datasource: %w", err)
	}

	// Get schema information
	schema, err := deps.SchemaService.GetDatasourceSchema(ctx, projectID, dsID)
	if err != nil {
		return nil, fmt.Errorf("failed to get schema: %w", err)
	}

	// Build base response
	response := map[string]any{
		"has_ontology":    false,
		"ontology_status": ontologyStatus,
		"depth":           depth,
	}

	// Build schema-only response based on depth
	switch depth {
	case "domain":
		// High-level summary without ontology
		tableCount := len(schema.Tables)
		columnCount := 0
		for _, table := range schema.Tables {
			columnCount += len(table.Columns)
		}

		response["domain"] = map[string]any{
			"description":     "Database schema information. Ontology not yet extracted for business context.",
			"primary_domains": []string{},
			"table_count":     tableCount,
			"column_count":    columnCount,
		}

		// List tables with row counts
		tableList := make([]map[string]any, 0, len(schema.Tables))
		for _, table := range schema.Tables {
			tableList = append(tableList, map[string]any{
				"table":     fmt.Sprintf("%s.%s", table.SchemaName, table.TableName),
				"row_count": table.RowCount,
			})
		}
		response["entities"] = tableList

	case "entities":
		// Table list with basic information
		tableList := make([]map[string]any, 0, len(schema.Tables))
		for _, table := range schema.Tables {
			tableList = append(tableList, map[string]any{
				"name":         fmt.Sprintf("%s.%s", table.SchemaName, table.TableName),
				"row_count":    table.RowCount,
				"column_count": len(table.Columns),
			})
		}
		response["entities"] = tableList

	case "tables", "columns":
		// Filter tables if specified
		filteredTables := schema.Tables
		if len(tables) > 0 {
			filteredTables = filterDatasourceTables(schema.Tables, tables)
		}

		// Build table details
		tableDetails := make([]map[string]any, 0, len(filteredTables))
		for _, table := range filteredTables {
			tableDetail := map[string]any{
				"schema_name": table.SchemaName,
				"table_name":  table.TableName,
				"row_count":   table.RowCount,
			}

			if depth == "columns" {
				// Include full column details
				columns := make([]map[string]any, 0, len(table.Columns))
				for _, col := range table.Columns {
					colDetail := map[string]any{
						"column_name": col.ColumnName,
						"data_type":   col.DataType,
						"is_nullable": col.IsNullable,
					}
					if col.BusinessName != "" {
						colDetail["business_name"] = col.BusinessName
					}
					if col.Description != "" {
						colDetail["description"] = col.Description
					}
					columns = append(columns, colDetail)
				}
				tableDetail["columns"] = columns
			} else {
				// Just column count for 'tables' depth
				tableDetail["column_count"] = len(table.Columns)
			}

			tableDetails = append(tableDetails, tableDetail)
		}
		response["tables"] = tableDetails
	}

	// Add glossary to response (always included regardless of depth)
	response["glossary"] = buildGlossaryResponse(glossary)

	return response, nil
}

// buildGlossaryResponse converts glossary terms to response format.
func buildGlossaryResponse(terms []*models.BusinessGlossaryTerm) []map[string]any {
	if len(terms) == 0 {
		return []map[string]any{}
	}

	result := make([]map[string]any, 0, len(terms))
	for _, term := range terms {
		termData := map[string]any{
			"term":       term.Term,
			"definition": term.Definition,
		}
		if len(term.Aliases) > 0 {
			termData["aliases"] = term.Aliases
		}
		if term.DefiningSQL != "" {
			termData["sql_pattern"] = term.DefiningSQL
		}
		result = append(result, termData)
	}
	return result
}

// filterDatasourceTables filters datasource tables by name.
func filterDatasourceTables(tables []*models.DatasourceTable, tableNames []string) []*models.DatasourceTable {
	if len(tableNames) == 0 {
		return tables
	}

	// Build a set of requested table names
	requested := make(map[string]bool)
	for _, name := range tableNames {
		requested[name] = true
	}

	filtered := make([]*models.DatasourceTable, 0)
	for _, table := range tables {
		fullName := fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)
		if requested[table.TableName] || requested[fullName] {
			filtered = append(filtered, table)
		}
	}

	return filtered
}
