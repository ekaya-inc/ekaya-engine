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
	ColumnMetadataRepo     repositories.ColumnMetadataRepository
	TableMetadataRepo      repositories.TableMetadataRepository
	Logger                 *zap.Logger
}

// GetDB implements ToolAccessDeps.
func (d *ContextToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *ContextToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }

// GetLogger implements ToolAccessDeps.
func (d *ContextToolDeps) GetLogger() *zap.Logger { return d.Logger }

// includeOptions specifies what additional data to include in the response.
type includeOptions struct {
	Statistics   bool
	SampleValues bool
}

// parseIncludeOptions parses the include parameter values.
func parseIncludeOptions(values []string) includeOptions {
	opts := includeOptions{}
	for _, v := range values {
		switch v {
		case "statistics":
			opts.Statistics = true
		case "sample_values":
			opts.SampleValues = true
		}
	}
	return opts
}

// RegisterContextTools registers the unified context tool.
func RegisterContextTools(s *server.MCPServer, deps *ContextToolDeps) {
	registerGetContextTool(s, deps)
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
		mcp.WithArray(
			"include",
			mcp.Description("Optional: additional data to include. Supported values: 'statistics' (distinct_count, row_count, null_rate, cardinality_ratio, is_joinable, joinability_reason), 'sample_values' (actual values for columns with ≤50 distinct values)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "get_context")
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
		includeOptions := parseIncludeOptions(getStringSlice(req, "include"))

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
			result, err = handleContextWithOntology(tenantCtx, deps, projectID, depth, tables, includeRelationships, ontologyStatus, glossary, includeOptions)
		} else {
			result, err = handleContextWithoutOntology(tenantCtx, deps, projectID, depth, tables, ontologyStatus, glossary, includeOptions)
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
	include includeOptions,
) (any, error) {
	// Build base response structure
	response := map[string]any{
		"has_ontology":    true,
		"ontology_status": ontologyStatus,
		"depth":           depth,
	}

	// Route to appropriate handler based on depth
	// Domain/Entities: Ontology-driven (conceptual business entities)
	// Tables/Columns: Schema-driven (physical structure with column features)
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

	case "tables", "columns":
		// Tables and columns always come from schema (engine_schema_columns)
		// not from ontology - column features are stored in schema metadata
		tablesResponse, err := buildTablesFromSchema(ctx, deps, projectID, depth, tables, include)
		if err != nil {
			return nil, err
		}
		response["tables"] = tablesResponse
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
	include includeOptions,
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
		// Use shared function for schema-based table/column retrieval
		tablesResponse, err := buildTablesFromSchema(ctx, deps, projectID, depth, tables, include)
		if err != nil {
			return nil, err
		}
		response["tables"] = tablesResponse
	}

	// Add glossary to response (always included regardless of depth)
	response["glossary"] = buildGlossaryResponse(glossary)

	return response, nil
}

// buildTablesFromSchema builds table/column details from schema (engine_schema_columns).
// This is the single source of truth for tables and columns, including column features.
func buildTablesFromSchema(
	ctx context.Context,
	deps *ContextToolDeps,
	projectID uuid.UUID,
	depth string,
	tableFilter []string,
	include includeOptions,
) ([]map[string]any, error) {
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

	// Filter tables if specified
	filteredTables := schema.Tables
	if len(tableFilter) > 0 {
		filteredTables = filterDatasourceTables(schema.Tables, tableFilter)
	}

	// Fetch table metadata for all tables in one batch
	var tableMetadataMap map[string]*models.TableMetadata
	if deps.TableMetadataRepo != nil {
		metaList, err := deps.TableMetadataRepo.List(ctx, projectID, dsID)
		if err != nil {
			deps.Logger.Warn("Failed to get table metadata",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without table metadata - not a fatal error
		} else if len(metaList) > 0 {
			tableMetadataMap = make(map[string]*models.TableMetadata, len(metaList))
			for _, meta := range metaList {
				tableMetadataMap[meta.TableName] = meta
			}
		}
	}

	// Build table details
	tableDetails := make([]map[string]any, 0, len(filteredTables))
	for _, table := range filteredTables {
		tableDetail := map[string]any{
			"schema_name": table.SchemaName,
			"table_name":  table.TableName,
			"row_count":   table.RowCount,
		}

		// Merge table metadata if available (omit null/empty fields)
		if tableMetadataMap != nil {
			if meta, ok := tableMetadataMap[table.TableName]; ok {
				if meta.Description != nil && *meta.Description != "" {
					tableDetail["description"] = *meta.Description
				}
				if meta.UsageNotes != nil && *meta.UsageNotes != "" {
					tableDetail["usage_notes"] = *meta.UsageNotes
				}
				if meta.IsEphemeral {
					tableDetail["is_ephemeral"] = true
				}
				if meta.PreferredAlternative != nil && *meta.PreferredAlternative != "" {
					tableDetail["preferred_alternative"] = *meta.PreferredAlternative
				}
			}
		}

		if depth == "columns" {
			// Include full column details with features
			columns, err := buildColumnDetails(ctx, deps, projectID, dsID, table, include)
			if err != nil {
				deps.Logger.Warn("Failed to build column details",
					zap.String("table", table.TableName),
					zap.Error(err))
				// Continue with partial data - don't fail the entire request
				columns = make([]map[string]any, 0, len(table.Columns))
				for _, col := range table.Columns {
					columns = append(columns, map[string]any{
						"column_name": col.ColumnName,
						"data_type":   col.DataType,
						"is_nullable": col.IsNullable,
					})
				}
			}
			tableDetail["columns"] = columns
		} else {
			// Just column count for 'tables' depth
			tableDetail["column_count"] = len(table.Columns)
		}

		tableDetails = append(tableDetails, tableDetail)
	}

	return tableDetails, nil
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

// buildColumnDetails builds column detail maps including features, statistics, and sample values.
// Column features are always included when available (from engine_schema_columns.metadata).
func buildColumnDetails(
	ctx context.Context,
	deps *ContextToolDeps,
	projectID uuid.UUID,
	_ uuid.UUID, // datasourceID - unused but kept for interface compatibility
	table *models.DatasourceTable,
	include includeOptions,
) ([]map[string]any, error) {
	columns := make([]map[string]any, 0, len(table.Columns))

	// Always fetch schema columns to get column features from metadata
	var schemaColumns map[string]*models.SchemaColumn
	cols, err := deps.SchemaRepo.GetColumnsByTables(ctx, projectID, []string{table.TableName}, false)
	if err != nil {
		deps.Logger.Warn("Failed to get schema columns",
			zap.String("table", table.TableName),
			zap.Error(err))
	} else if tableCols, ok := cols[table.TableName]; ok {
		schemaColumns = make(map[string]*models.SchemaColumn, len(tableCols))
		for _, col := range tableCols {
			schemaColumns[col.ColumnName] = col
		}
	}

	// Fetch column metadata for sensitive flag overrides
	var columnMetadata map[string]*models.ColumnMetadata
	if deps.ColumnMetadataRepo != nil {
		metaList, err := deps.ColumnMetadataRepo.GetByTable(ctx, projectID, table.TableName)
		if err != nil {
			deps.Logger.Warn("Failed to get column metadata",
				zap.String("table", table.TableName),
				zap.Error(err))
		} else if len(metaList) > 0 {
			columnMetadata = make(map[string]*models.ColumnMetadata, len(metaList))
			for _, meta := range metaList {
				columnMetadata[meta.ColumnName] = meta
			}
		}
	}

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

		// Add enriched column metadata from update_column (MCP enrichment)
		// These take precedence over datasource column values when present
		if columnMetadata != nil {
			if meta, ok := columnMetadata[col.ColumnName]; ok {
				// Description from update_column overrides datasource description
				if meta.Description != nil && *meta.Description != "" {
					colDetail["description"] = *meta.Description
				}
				// Entity association (e.g., 'User', 'Account')
				if meta.Entity != nil && *meta.Entity != "" {
					colDetail["entity"] = *meta.Entity
				}
				// Semantic role (e.g., 'dimension', 'measure', 'identifier', 'attribute')
				if meta.Role != nil && *meta.Role != "" {
					colDetail["role"] = *meta.Role
				}
				// Enum value labels if defined
				if len(meta.EnumValues) > 0 {
					colDetail["enum_values"] = meta.EnumValues
				}
			}
		}

		// Get corresponding schema column if available
		var schemaCol *models.SchemaColumn
		if schemaColumns != nil {
			schemaCol = schemaColumns[col.ColumnName]
		}

		// Add statistics if requested
		if include.Statistics && schemaCol != nil {
			addStatisticsToColumnDetail(colDetail, schemaCol, col)
		}

		// Add sample values if requested and available
		// Sample values are persisted during ontology extraction for low-cardinality columns (≤50 distinct values)
		// Sensitive data is automatically redacted to prevent exposure of API keys, secrets, etc.
		// Manual is_sensitive flag overrides automatic detection: true=always redact, false=never redact, nil=auto-detect
		if include.SampleValues && schemaCol != nil && len(schemaCol.SampleValues) > 0 {
			// Check for manual sensitive override from column metadata
			var isSensitiveOverride *bool
			if columnMetadata != nil {
				if meta, ok := columnMetadata[col.ColumnName]; ok && meta.IsSensitive != nil {
					isSensitiveOverride = meta.IsSensitive
				}
			}

			// Determine if column should be treated as sensitive
			isSensitive := false
			redactionReason := ""

			if isSensitiveOverride != nil {
				// Manual override takes precedence
				if *isSensitiveOverride {
					isSensitive = true
					redactionReason = "column marked as sensitive (manual override)"
				}
				// If explicitly marked as not sensitive (*isSensitiveOverride == false), skip auto-detection
			} else {
				// Use automatic detection
				if DefaultSensitiveDetector.IsSensitiveColumn(col.ColumnName) {
					isSensitive = true
					redactionReason = "column name matches sensitive pattern"
				}
			}

			if isSensitive {
				colDetail["sample_values_redacted"] = true
				colDetail["redaction_reason"] = redactionReason
			} else {
				// Check each sample value for sensitive content and redact if needed
				// (only if not explicitly marked as not sensitive)
				redactedValues := make([]string, 0, len(schemaCol.SampleValues))
				anyRedacted := false

				// Only do content-based detection if no manual override
				if isSensitiveOverride == nil {
					for _, val := range schemaCol.SampleValues {
						if DefaultSensitiveDetector.IsSensitiveContent(val) {
							redactedValues = append(redactedValues, DefaultSensitiveDetector.RedactContent(val))
							anyRedacted = true
						} else {
							redactedValues = append(redactedValues, val)
						}
					}
				} else {
					// Manual override to not sensitive - return values as-is
					redactedValues = schemaCol.SampleValues
				}

				colDetail["sample_values"] = redactedValues
				if anyRedacted {
					colDetail["sample_values_redacted"] = true
					colDetail["redaction_reason"] = "values contain sensitive patterns (api keys, secrets, etc.)"
				}
			}
		}

		// Add column features from metadata (from feature extraction pipeline)
		if schemaCol != nil {
			if features := schemaCol.GetColumnFeatures(); features != nil {
				featuresMap := map[string]any{}
				if features.Purpose != "" {
					featuresMap["purpose"] = features.Purpose
				}
				if features.SemanticType != "" {
					featuresMap["semantic_type"] = features.SemanticType
				}
				if features.Role != "" {
					featuresMap["role"] = features.Role
				}
				if features.Description != "" {
					featuresMap["description"] = features.Description
				}
				if features.Confidence > 0 {
					featuresMap["confidence"] = features.Confidence
				}
				if features.ClassificationPath != "" {
					featuresMap["classification_path"] = string(features.ClassificationPath)
				}

				// Add path-specific features
				if features.TimestampFeatures != nil {
					featuresMap["timestamp_features"] = map[string]any{
						"timestamp_purpose": features.TimestampFeatures.TimestampPurpose,
						"is_soft_delete":    features.TimestampFeatures.IsSoftDelete,
						"is_audit_field":    features.TimestampFeatures.IsAuditField,
					}
				}
				if features.BooleanFeatures != nil {
					featuresMap["boolean_features"] = map[string]any{
						"true_meaning":  features.BooleanFeatures.TrueMeaning,
						"false_meaning": features.BooleanFeatures.FalseMeaning,
						"boolean_type":  features.BooleanFeatures.BooleanType,
					}
				}
				if features.IdentifierFeatures != nil {
					idFeatures := map[string]any{
						"identifier_type": features.IdentifierFeatures.IdentifierType,
					}
					if features.IdentifierFeatures.ExternalService != "" {
						idFeatures["external_service"] = features.IdentifierFeatures.ExternalService
					}
					if features.IdentifierFeatures.FKTargetTable != "" {
						idFeatures["fk_target_table"] = features.IdentifierFeatures.FKTargetTable
						idFeatures["fk_target_column"] = features.IdentifierFeatures.FKTargetColumn
						idFeatures["fk_confidence"] = features.IdentifierFeatures.FKConfidence
					}
					if features.IdentifierFeatures.EntityReferenced != "" {
						idFeatures["entity_referenced"] = features.IdentifierFeatures.EntityReferenced
					}
					featuresMap["identifier_features"] = idFeatures
				}
				if features.MonetaryFeatures != nil {
					featuresMap["monetary_features"] = map[string]any{
						"is_monetary":            features.MonetaryFeatures.IsMonetary,
						"currency_unit":          features.MonetaryFeatures.CurrencyUnit,
						"paired_currency_column": features.MonetaryFeatures.PairedCurrencyColumn,
					}
				}

				if len(featuresMap) > 0 {
					colDetail["features"] = featuresMap
				}
			}
		}

		columns = append(columns, colDetail)
	}

	return columns, nil
}

// addStatisticsToColumnDetail adds statistics fields to a column detail map from SchemaColumn.
func addStatisticsToColumnDetail(colDetail map[string]any, schemaCol *models.SchemaColumn, datasourceCol *models.DatasourceColumn) {
	// Add distinct_count if available
	if schemaCol.DistinctCount != nil {
		colDetail["distinct_count"] = *schemaCol.DistinctCount
	}

	// Add row_count if available (denormalized from table)
	if schemaCol.RowCount != nil {
		colDetail["row_count"] = *schemaCol.RowCount

		// Calculate null_rate if we have the data
		if schemaCol.NullCount != nil {
			nullRate := float64(*schemaCol.NullCount) / float64(*schemaCol.RowCount)
			colDetail["null_rate"] = nullRate
		}

		// Calculate cardinality_ratio if we have distinct_count
		if schemaCol.DistinctCount != nil && *schemaCol.RowCount > 0 {
			cardinalityRatio := float64(*schemaCol.DistinctCount) / float64(*schemaCol.RowCount)
			colDetail["cardinality_ratio"] = cardinalityRatio
		}
	}

	// Add joinability information if available
	if schemaCol.IsJoinable != nil {
		colDetail["is_joinable"] = *schemaCol.IsJoinable
	}
	if schemaCol.JoinabilityReason != nil {
		colDetail["joinability_reason"] = *schemaCol.JoinabilityReason
	}
}
