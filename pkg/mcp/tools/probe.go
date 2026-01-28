// Package tools provides MCP tool implementations for ekaya-engine.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ProbeToolDeps contains dependencies for probe tools.
type ProbeToolDeps struct {
	DB                 *database.DB
	MCPConfigService   services.MCPConfigService
	SchemaRepo         repositories.SchemaRepository
	OntologyRepo       repositories.OntologyRepository
	EntityRepo         repositories.OntologyEntityRepository
	RelationshipRepo   repositories.EntityRelationshipRepository
	ColumnMetadataRepo repositories.ColumnMetadataRepository
	ProjectService     services.ProjectService
	Logger             *zap.Logger
}

// GetDB implements ToolAccessDeps.
func (d *ProbeToolDeps) GetDB() *database.DB { return d.DB }

// GetMCPConfigService implements ToolAccessDeps.
func (d *ProbeToolDeps) GetMCPConfigService() services.MCPConfigService { return d.MCPConfigService }

// GetLogger implements ToolAccessDeps.
func (d *ProbeToolDeps) GetLogger() *zap.Logger { return d.Logger }

// RegisterProbeTools registers probe MCP tools.
func RegisterProbeTools(s *server.MCPServer, deps *ProbeToolDeps) {
	registerProbeColumnTool(s, deps)
	registerProbeColumnsTool(s, deps)
	registerProbeRelationshipTool(s, deps)
}

// registerProbeColumnTool adds the probe_column tool for deep-diving into specific columns.
func registerProbeColumnTool(s *server.MCPServer, deps *ProbeToolDeps) {
	tool := mcp.NewTool(
		"probe_column",
		mcp.WithDescription(
			"Deep-dive into a specific column to retrieve statistics (distinct_count, row_count, null_rate, "+
				"cardinality_ratio), joinability classification (is_joinable, reason), and semantic information "+
				"(entity, role, enum_labels). Use this when you need detailed information about a column "+
				"without writing SQL queries. Example: probe_column(table='users', column='status') returns "+
				"statistics, sample values, and semantic meaning.",
		),
		mcp.WithString(
			"table",
			mcp.Required(),
			mcp.Description("Name of the table containing the column"),
		),
		mcp.WithString(
			"column",
			mcp.Required(),
			mcp.Description("Name of the column to probe"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "probe_column")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required parameters
		tableName, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}
		// Validate table is not empty after trimming whitespace
		tableName = trimString(tableName)
		if tableName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'table' cannot be empty"), nil
		}

		columnName, err := req.RequireString("column")
		if err != nil {
			return nil, err
		}
		// Validate column is not empty after trimming whitespace
		columnName = trimString(columnName)
		if columnName == "" {
			return NewErrorResult("invalid_parameters", "parameter 'column' cannot be empty"), nil
		}

		// Probe the column
		result, err := probeColumn(tenantCtx, deps, projectID, tableName, columnName)
		if err != nil {
			return nil, err
		}

		// Check if probe returned an error result (table/column not found)
		if result.Error != "" {
			// Extract error code from error message (format: "CODE: message")
			errorCode := "query_error" // default
			errorMessage := result.Error
			if idx := strings.Index(result.Error, ": "); idx > 0 {
				errorCode = result.Error[:idx]
				errorMessage = result.Error[idx+2:]
			}
			return NewErrorResult(errorCode, errorMessage), nil
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// registerProbeColumnsTool adds the probe_columns tool for batch probing multiple columns.
func registerProbeColumnsTool(s *server.MCPServer, deps *ProbeToolDeps) {
	tool := mcp.NewTool(
		"probe_columns",
		mcp.WithDescription(
			"Batch variant of probe_column for deep-diving into multiple columns at once. "+
				"Returns statistics, joinability classification, and semantic information for each column. "+
				"Example: probe_columns(columns=[{table='users', column='status'}, {table='users', column='role'}]) "+
				"returns detailed information for both columns.",
		),
		mcp.WithArray(
			"columns",
			mcp.Required(),
			mcp.Description("Array of {table, column} objects to probe"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "probe_columns")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get columns array from arguments
		args, ok := req.Params.Arguments.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid request arguments")
		}

		columnsArg, ok := args["columns"].([]any)
		if !ok || len(columnsArg) == 0 {
			return nil, fmt.Errorf("columns parameter is required and must be a non-empty array")
		}

		// Parse column requests
		type columnRequest struct {
			Table  string `json:"table"`
			Column string `json:"column"`
		}

		var columnRequests []columnRequest
		for _, col := range columnsArg {
			colMap, ok := col.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("each column must be an object with 'table' and 'column' fields")
			}

			table, ok := colMap["table"].(string)
			if !ok || table == "" {
				return nil, fmt.Errorf("each column must have a non-empty 'table' field")
			}

			column, ok := colMap["column"].(string)
			if !ok || column == "" {
				return nil, fmt.Errorf("each column must have a non-empty 'column' field")
			}

			columnRequests = append(columnRequests, columnRequest{
				Table:  table,
				Column: column,
			})
		}

		// Probe each column
		results := make(map[string]*probeColumnResponse)
		for _, req := range columnRequests {
			result, err := probeColumn(tenantCtx, deps, projectID, req.Table, req.Column)
			if err != nil {
				// Store error in result instead of failing the entire batch
				results[fmt.Sprintf("%s.%s", req.Table, req.Column)] = &probeColumnResponse{
					Table:  req.Table,
					Column: req.Column,
					Error:  err.Error(),
				}
			} else {
				results[fmt.Sprintf("%s.%s", req.Table, req.Column)] = result
			}
		}

		response := probeColumnsResponse{
			Results: results,
		}

		jsonResult, err := json.Marshal(response)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// probeColumn retrieves detailed information about a specific column.
// Returns error results for table not found and column not found.
// Returns Go errors for database connection failures.
func probeColumn(ctx context.Context, deps *ProbeToolDeps, projectID uuid.UUID, tableName, columnName string) (*probeColumnResponse, error) {
	response := &probeColumnResponse{
		Table:  tableName,
		Column: columnName,
	}

	// Get datasource ID (assume first datasource for now)
	// TODO: Support multi-datasource projects
	tables, err := deps.SchemaRepo.GetColumnsByTables(ctx, projectID, []string{tableName}, false)
	if err != nil {
		// Database connection failures remain as Go errors
		return nil, fmt.Errorf("failed to get columns for table: %w", err)
	}

	columns, ok := tables[tableName]
	if !ok || len(columns) == 0 {
		// Table not found - return error result for Claude to see
		response.Error = fmt.Sprintf("TABLE_NOT_FOUND: table %q not found in schema registry", tableName)
		return response, nil
	}

	// Find the specific column
	var column *models.SchemaColumn
	for _, col := range columns {
		if col.ColumnName == columnName {
			column = col
			break
		}
	}

	if column == nil {
		// Column not found - return error result for Claude to see
		response.Error = fmt.Sprintf("COLUMN_NOT_FOUND: column %q not found in table %q", columnName, tableName)
		return response, nil
	}

	// Build statistics section
	if column.DistinctCount != nil && column.RowCount != nil {
		stats := probeColumnStatistics{
			DistinctCount: *column.DistinctCount,
			RowCount:      *column.RowCount,
		}

		if column.NonNullCount != nil {
			stats.NonNullCount = *column.NonNullCount
		}

		if column.NullCount != nil && column.RowCount != nil && *column.RowCount > 0 {
			nullRate := float64(*column.NullCount) / float64(*column.RowCount)
			stats.NullRate = &nullRate
		}

		if column.DistinctCount != nil && column.RowCount != nil && *column.RowCount > 0 {
			cardinalityRatio := float64(*column.DistinctCount) / float64(*column.RowCount)
			stats.CardinalityRatio = &cardinalityRatio
		}

		if column.MinLength != nil {
			stats.MinLength = column.MinLength
		}

		if column.MaxLength != nil {
			stats.MaxLength = column.MaxLength
		}

		response.Statistics = &stats
	}

	// Build joinability section
	if column.IsJoinable != nil {
		joinability := probeColumnJoinability{
			IsJoinable: *column.IsJoinable,
		}

		if column.JoinabilityReason != nil {
			joinability.Reason = *column.JoinabilityReason
		}

		response.Joinability = &joinability
	}

	// Get semantic information from ontology if available
	ontology, err := deps.OntologyRepo.GetActive(ctx, projectID)
	if err != nil {
		deps.Logger.Warn("Failed to get active ontology for semantic enrichment",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	}

	if ontology != nil {
		// Get column details from ontology
		columnDetails := ontology.GetColumnDetails(tableName)
		for _, colDetail := range columnDetails {
			if colDetail.Name == columnName {
				semantic := probeColumnSemantic{
					Role:        colDetail.Role,
					Description: colDetail.Description,
				}

				// Get entity from entity summaries
				entitySummary := ontology.GetEntitySummary(tableName)
				if entitySummary != nil {
					semantic.Entity = entitySummary.BusinessName
				}

				// Extract enum labels and distribution data
				if len(colDetail.EnumValues) > 0 {
					enumLabels := make(map[string]string)
					var enumDist []probeEnumValueDetail

					for _, ev := range colDetail.EnumValues {
						if ev.Label != "" {
							enumLabels[ev.Value] = ev.Label
						} else if ev.Description != "" {
							enumLabels[ev.Value] = ev.Description
						}

						// Add distribution data if available
						if ev.Count != nil || ev.IsLikelyInitialState != nil || ev.IsLikelyTerminalState != nil || ev.IsLikelyErrorState != nil {
							detail := probeEnumValueDetail{
								Value:                 ev.Value,
								Label:                 ev.Label,
								Count:                 ev.Count,
								Percentage:            ev.Percentage,
								IsLikelyInitialState:  ev.IsLikelyInitialState,
								IsLikelyTerminalState: ev.IsLikelyTerminalState,
								IsLikelyErrorState:    ev.IsLikelyErrorState,
							}
							enumDist = append(enumDist, detail)
						}
					}

					if len(enumLabels) > 0 {
						semantic.EnumLabels = enumLabels
					}
					if len(enumDist) > 0 {
						semantic.EnumDistribution = enumDist
					}
				}

				response.Semantic = &semantic
				break
			}
		}
	}

	// Fallback: check engine_column_metadata for approved changes not yet in ontology
	// This handles the case where approve_change writes to column_metadata but not ontology
	if deps.ColumnMetadataRepo != nil {
		columnMeta, err := deps.ColumnMetadataRepo.GetByTableColumn(ctx, projectID, tableName, columnName)
		if err != nil {
			deps.Logger.Warn("Failed to get column metadata fallback",
				zap.String("project_id", projectID.String()),
				zap.String("table", tableName),
				zap.String("column", columnName),
				zap.Error(err))
		} else if columnMeta != nil {
			// Initialize semantic section if not already present
			if response.Semantic == nil {
				response.Semantic = &probeColumnSemantic{}
			}

			// Merge enum values if present in column_metadata but not in ontology response
			if len(columnMeta.EnumValues) > 0 && len(response.Semantic.EnumLabels) == 0 {
				enumLabels := make(map[string]string)
				for _, ev := range columnMeta.EnumValues {
					enumLabels[ev] = ev // Value as its own label (no enrichment from approve_change)
				}
				response.Semantic.EnumLabels = enumLabels
			}

			// Merge description if present in column_metadata but missing from ontology
			if columnMeta.Description != nil && *columnMeta.Description != "" && response.Semantic.Description == "" {
				response.Semantic.Description = *columnMeta.Description
			}

			// Merge entity if present in column_metadata but missing from ontology
			if columnMeta.Entity != nil && *columnMeta.Entity != "" && response.Semantic.Entity == "" {
				response.Semantic.Entity = *columnMeta.Entity
			}

			// Merge role if present in column_metadata but missing from ontology
			if columnMeta.Role != nil && *columnMeta.Role != "" && response.Semantic.Role == "" {
				response.Semantic.Role = *columnMeta.Role
			}
		}
	}

	// Add sample values from persisted data (low-cardinality columns ≤50 distinct values)
	if len(column.SampleValues) > 0 {
		response.SampleValues = column.SampleValues
	}

	return response, nil
}

// probeColumnResponse is the response format for probe_column tool.
type probeColumnResponse struct {
	Table        string                  `json:"table"`
	Column       string                  `json:"column"`
	Statistics   *probeColumnStatistics  `json:"statistics,omitempty"`
	Joinability  *probeColumnJoinability `json:"joinability,omitempty"`
	SampleValues []string                `json:"sample_values,omitempty"` // Distinct values for low-cardinality columns (≤50 values)
	Semantic     *probeColumnSemantic    `json:"semantic,omitempty"`
	Error        string                  `json:"error,omitempty"` // For batch mode partial failures
}

// probeColumnStatistics contains statistical information about a column.
type probeColumnStatistics struct {
	DistinctCount    int64    `json:"distinct_count"`
	RowCount         int64    `json:"row_count"`
	NonNullCount     int64    `json:"non_null_count,omitempty"`
	NullRate         *float64 `json:"null_rate,omitempty"`
	CardinalityRatio *float64 `json:"cardinality_ratio,omitempty"`
	MinLength        *int64   `json:"min_length,omitempty"`
	MaxLength        *int64   `json:"max_length,omitempty"`
}

// probeColumnJoinability contains joinability classification information.
type probeColumnJoinability struct {
	IsJoinable bool   `json:"is_joinable"`
	Reason     string `json:"reason,omitempty"`
}

// probeColumnSemantic contains semantic information from the ontology.
type probeColumnSemantic struct {
	Entity           string                 `json:"entity,omitempty"`
	Role             string                 `json:"role,omitempty"`
	Description      string                 `json:"description,omitempty"`
	EnumLabels       map[string]string      `json:"enum_labels,omitempty"`
	EnumDistribution []probeEnumValueDetail `json:"enum_distribution,omitempty"` // Distribution with state semantics
}

// probeEnumValueDetail provides detailed enum value information including distribution and state semantics.
type probeEnumValueDetail struct {
	Value                 string   `json:"value"`
	Label                 string   `json:"label,omitempty"`
	Count                 *int64   `json:"count,omitempty"`
	Percentage            *float64 `json:"percentage,omitempty"`
	IsLikelyInitialState  *bool    `json:"is_likely_initial_state,omitempty"`
	IsLikelyTerminalState *bool    `json:"is_likely_terminal_state,omitempty"`
	IsLikelyErrorState    *bool    `json:"is_likely_error_state,omitempty"`
}

// probeColumnsResponse is the response format for probe_columns batch tool.
type probeColumnsResponse struct {
	Results map[string]*probeColumnResponse `json:"results"` // key is "table.column"
}

// registerProbeRelationshipTool adds the probe_relationship tool for deep-diving into relationships.
func registerProbeRelationshipTool(s *server.MCPServer, deps *ProbeToolDeps) {
	tool := mcp.NewTool(
		"probe_relationship",
		mcp.WithDescription(
			"Deep-dive into relationships between entities with pre-computed metrics. "+
				"Returns cardinality, data quality metrics (match_rate, orphan_count, source_distinct, target_distinct), "+
				"and rejected candidates with rejection reasons. Supports filtering by from_entity and to_entity parameters. "+
				"Example: probe_relationship(from_entity='Account', to_entity='User') returns relationship details "+
				"including cardinality and data quality metrics.",
		),
		mcp.WithString(
			"from_entity",
			mcp.Description("Filter by source entity name (optional)"),
		),
		mcp.WithString(
			"to_entity",
			mcp.Description("Filter by target entity name (optional)"),
		),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		projectID, tenantCtx, cleanup, err := AcquireToolAccess(ctx, deps, "probe_relationship")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get optional parameters
		var fromEntity, toEntity *string
		args, ok := req.Params.Arguments.(map[string]any)
		if ok {
			if fe, ok := args["from_entity"].(string); ok && fe != "" {
				fromEntity = &fe
			}
			if te, ok := args["to_entity"].(string); ok && te != "" {
				toEntity = &te
			}
		}

		// Probe relationships
		result, err := probeRelationships(tenantCtx, deps, projectID, fromEntity, toEntity)
		if err != nil {
			return nil, err
		}

		jsonResult, err := json.Marshal(result)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal result: %w", err)
		}

		return mcp.NewToolResultText(string(jsonResult)), nil
	})
}

// probeRelationships retrieves detailed information about relationships between entities.
func probeRelationships(ctx context.Context, deps *ProbeToolDeps, projectID uuid.UUID, fromEntity, toEntity *string) (*probeRelationshipResponse, error) {
	response := &probeRelationshipResponse{
		Relationships:      []probeRelationshipDetail{},
		RejectedCandidates: []probeRelationshipCandidate{},
	}

	// Get active ontology
	ontology, err := deps.OntologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active ontology: %w", err)
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found for project")
	}

	// Get all entity relationships
	entityRelationships, err := deps.RelationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entity relationships: %w", err)
	}

	// Get all entities to build entity ID -> name map
	// Use GetByProject (not GetByOntology) to match the pattern used by get_ontology,
	// ensuring entities created via MCP tools are found regardless of which ontology ID they have
	entities, err := deps.EntityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get entities: %w", err)
	}

	entityIDToName := make(map[uuid.UUID]string)
	entityNameToID := make(map[string]uuid.UUID)
	entityIDToTable := make(map[uuid.UUID]string)
	for _, entity := range entities {
		entityIDToName[entity.ID] = entity.Name
		entityNameToID[entity.Name] = entity.ID
		entityIDToTable[entity.ID] = entity.PrimaryTable
	}

	// Get default datasource ID
	datasourceID, err := deps.ProjectService.GetDefaultDatasourceID(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get default datasource ID: %w", err)
	}
	if datasourceID == uuid.Nil {
		return nil, fmt.Errorf("no default datasource configured for project")
	}

	// Get schema relationships with discovery metrics
	// We need to query engine_schema_relationships to get cardinality and data quality metrics
	schemaRelationshipsMap, rejectedCandidates, err := getSchemaRelationshipsWithMetrics(ctx, deps, projectID, datasourceID)
	if err != nil {
		// Log warning but continue without metrics (graceful degradation)
		deps.Logger.Warn("Failed to fetch schema relationships with metrics",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	}

	// Build a map of (table_name, column_name) -> column_id for lookups
	columnKeyToID, err := buildColumnKeyToIDMap(ctx, deps, projectID, datasourceID)
	if err != nil {
		// Log warning but continue without metrics (graceful degradation)
		deps.Logger.Warn("Failed to build column key to ID map",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	}

	// Filter by from_entity and to_entity if provided
	var filteredRelationships []*models.EntityRelationship
	for _, rel := range entityRelationships {
		// Apply filters
		if fromEntity != nil {
			fromName := entityIDToName[rel.SourceEntityID]
			if fromName != *fromEntity {
				continue
			}
		}
		if toEntity != nil {
			toName := entityIDToName[rel.TargetEntityID]
			if toName != *toEntity {
				continue
			}
		}
		filteredRelationships = append(filteredRelationships, rel)
	}

	// Build response for confirmed relationships
	for _, rel := range filteredRelationships {
		detail := probeRelationshipDetail{
			FromEntity: entityIDToName[rel.SourceEntityID],
			ToEntity:   entityIDToName[rel.TargetEntityID],
			FromColumn: fmt.Sprintf("%s.%s", rel.SourceColumnTable, rel.SourceColumnName),
			ToColumn:   fmt.Sprintf("%s.%s", rel.TargetColumnTable, rel.TargetColumnName),
		}

		// Add description and association if available
		if rel.Description != nil {
			detail.Description = rel.Description
		}
		if rel.Association != nil {
			detail.Label = rel.Association
		}

		// Look up corresponding schema relationship for cardinality and data quality metrics
		// Build key using column IDs from the columnKeyToID map
		sourceKey := columnKey{tableName: rel.SourceColumnTable, columnName: rel.SourceColumnName}
		targetKey := columnKey{tableName: rel.TargetColumnTable, columnName: rel.TargetColumnName}
		sourceColID, sourceOK := columnKeyToID[sourceKey]
		targetColID, targetOK := columnKeyToID[targetKey]

		if sourceOK && targetOK {
			schemaRelKey := schemaRelationshipKey{
				sourceColumnID: sourceColID,
				targetColumnID: targetColID,
			}
			if schemaRel, ok := schemaRelationshipsMap[schemaRelKey]; ok {
				// Add cardinality from schema relationship
				if schemaRel.Cardinality != "" {
					detail.Cardinality = schemaRel.Cardinality
				}

				// Add data quality metrics if available
				if schemaRel.MatchRate != nil && schemaRel.SourceDistinct != nil && schemaRel.TargetDistinct != nil {
					dataQuality := &probeRelationshipDataQuality{
						MatchRate:      *schemaRel.MatchRate,
						SourceDistinct: *schemaRel.SourceDistinct,
						TargetDistinct: *schemaRel.TargetDistinct,
					}

					// Add matched_count if available
					if schemaRel.MatchedCount != nil {
						dataQuality.MatchedCount = *schemaRel.MatchedCount
					}

					// Calculate orphan_count if we have source_distinct and matched_count
					if schemaRel.SourceDistinct != nil && schemaRel.MatchedCount != nil {
						orphanCount := *schemaRel.SourceDistinct - *schemaRel.MatchedCount
						dataQuality.OrphanCount = &orphanCount
					}

					detail.DataQuality = dataQuality
				}
			}
		}

		response.Relationships = append(response.Relationships, detail)
	}

	// Add rejected candidates (filter by entity if specified)
	if fromEntity != nil || toEntity != nil {
		// If entity filters are specified, filter rejected candidates
		for _, candidate := range rejectedCandidates {
			// Check if the candidate involves the filtered entities
			// We need to check table names against entity primary tables
			includeCandidate := true

			if fromEntity != nil {
				entityID := entityNameToID[*fromEntity]
				fromTable := entityIDToTable[entityID]
				if candidate.FromColumn[:len(fromTable)] != fromTable {
					includeCandidate = false
				}
			}

			if toEntity != nil && includeCandidate {
				entityID := entityNameToID[*toEntity]
				toTable := entityIDToTable[entityID]
				if candidate.ToColumn[:len(toTable)] != toTable {
					includeCandidate = false
				}
			}

			if includeCandidate {
				response.RejectedCandidates = append(response.RejectedCandidates, candidate)
			}
		}
	} else {
		// No filters, return all rejected candidates
		response.RejectedCandidates = rejectedCandidates
	}

	return response, nil
}

// schemaRelationshipKey is used to match entity relationships to schema relationships.
type schemaRelationshipKey struct {
	sourceColumnID uuid.UUID
	targetColumnID uuid.UUID
}

// getSchemaRelationshipsWithMetrics queries engine_schema_relationships to get cardinality
// and data quality metrics. Returns a map keyed by (source_column_id, target_column_id)
// for fast lookup, plus a list of rejected candidates.
func getSchemaRelationshipsWithMetrics(ctx context.Context, deps *ProbeToolDeps, projectID, datasourceID uuid.UUID) (map[schemaRelationshipKey]*models.SchemaRelationship, []probeRelationshipCandidate, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, nil, fmt.Errorf("no tenant scope in context")
	}

	// Query all schema relationships (both confirmed and rejected) with discovery metrics
	query := `
		SELECT r.id, r.project_id, r.source_table_id, r.source_column_id,
		       r.target_table_id, r.target_column_id, r.relationship_type,
		       r.cardinality, r.confidence, r.inference_method, r.is_validated,
		       r.validation_results, r.is_approved, r.created_at, r.updated_at,
		       r.match_rate, r.source_distinct, r.target_distinct, r.matched_count, r.rejection_reason,
		       sc.schema_table_id as source_table_id_fk, sc.column_name as source_column_name,
		       st.table_name as source_table_name,
		       tc.schema_table_id as target_table_id_fk, tc.column_name as target_column_name,
		       tt.table_name as target_table_name
		FROM engine_schema_relationships r
		JOIN engine_schema_columns sc ON r.source_column_id = sc.id
		JOIN engine_schema_columns tc ON r.target_column_id = tc.id
		JOIN engine_schema_tables st ON sc.schema_table_id = st.id
		JOIN engine_schema_tables tt ON tc.schema_table_id = tt.id
		WHERE r.project_id = $1 AND st.datasource_id = $2
		  AND r.deleted_at IS NULL
		ORDER BY r.created_at`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query schema relationships: %w", err)
	}
	defer rows.Close()

	confirmedMap := make(map[schemaRelationshipKey]*models.SchemaRelationship)
	var rejectedCandidates []probeRelationshipCandidate

	for rows.Next() {
		var rel models.SchemaRelationship
		var validationResultsJSON []byte
		var sourceTableName, sourceColumnName, targetTableName, targetColumnName string

		err := rows.Scan(
			&rel.ID, &rel.ProjectID, &rel.SourceTableID, &rel.SourceColumnID,
			&rel.TargetTableID, &rel.TargetColumnID, &rel.RelationshipType,
			&rel.Cardinality, &rel.Confidence, &rel.InferenceMethod, &rel.IsValidated,
			&validationResultsJSON, &rel.IsApproved, &rel.CreatedAt, &rel.UpdatedAt,
			&rel.MatchRate, &rel.SourceDistinct, &rel.TargetDistinct, &rel.MatchedCount, &rel.RejectionReason,
			&rel.SourceTableID, &sourceColumnName, &sourceTableName,
			&rel.TargetTableID, &targetColumnName, &targetTableName,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan schema relationship: %w", err)
		}

		// Unmarshal validation results if present
		if validationResultsJSON != nil {
			var vr models.ValidationResults
			if err := json.Unmarshal(validationResultsJSON, &vr); err == nil {
				rel.ValidationResults = &vr
			}
		}

		// If rejected, add to rejected candidates list
		if rel.RejectionReason != nil && *rel.RejectionReason != "" {
			candidate := probeRelationshipCandidate{
				FromColumn:      fmt.Sprintf("%s.%s", sourceTableName, sourceColumnName),
				ToColumn:        fmt.Sprintf("%s.%s", targetTableName, targetColumnName),
				RejectionReason: *rel.RejectionReason,
				MatchRate:       rel.MatchRate,
			}
			rejectedCandidates = append(rejectedCandidates, candidate)
		} else {
			// If confirmed, add to map for lookup
			key := schemaRelationshipKey{
				sourceColumnID: rel.SourceColumnID,
				targetColumnID: rel.TargetColumnID,
			}
			confirmedMap[key] = &rel
		}
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating schema relationships: %w", err)
	}

	return confirmedMap, rejectedCandidates, nil
}

// columnKey is used to identify columns by (table_name, column_name).
type columnKey struct {
	tableName  string
	columnName string
}

// buildColumnKeyToIDMap builds a map from (table_name, column_name) to column_id.
// This is needed to match entity relationships (which only have table/column names)
// to schema relationships (which use column IDs).
func buildColumnKeyToIDMap(ctx context.Context, deps *ProbeToolDeps, projectID, datasourceID uuid.UUID) (map[columnKey]uuid.UUID, error) {
	// Get all tables for this datasource to build table_id -> table_name map
	tables, err := deps.SchemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	tableIDToName := make(map[uuid.UUID]string)
	for _, table := range tables {
		tableIDToName[table.ID] = table.TableName
	}

	// Get all columns for this datasource
	columns, err := deps.SchemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns: %w", err)
	}

	// Build map
	keyToID := make(map[columnKey]uuid.UUID)
	for _, col := range columns {
		tableName, ok := tableIDToName[col.SchemaTableID]
		if !ok {
			// Skip columns whose table we don't know about (shouldn't happen)
			continue
		}
		key := columnKey{tableName: tableName, columnName: col.ColumnName}
		keyToID[key] = col.ID
	}

	return keyToID, nil
}

// probeRelationshipResponse is the response format for probe_relationship tool.
type probeRelationshipResponse struct {
	Relationships      []probeRelationshipDetail    `json:"relationships"`
	RejectedCandidates []probeRelationshipCandidate `json:"rejected_candidates,omitempty"`
}

// probeRelationshipDetail contains detailed information about a confirmed relationship.
type probeRelationshipDetail struct {
	FromEntity  string                        `json:"from_entity"`
	ToEntity    string                        `json:"to_entity"`
	FromColumn  string                        `json:"from_column"`
	ToColumn    string                        `json:"to_column"`
	Cardinality string                        `json:"cardinality,omitempty"`
	DataQuality *probeRelationshipDataQuality `json:"data_quality,omitempty"`
	Description *string                       `json:"description,omitempty"`
	Label       *string                       `json:"label,omitempty"`
}

// probeRelationshipDataQuality contains data quality metrics for a relationship.
type probeRelationshipDataQuality struct {
	MatchRate      float64 `json:"match_rate"`
	OrphanCount    *int64  `json:"orphan_count,omitempty"`
	SourceDistinct int64   `json:"source_distinct"`
	TargetDistinct int64   `json:"target_distinct"`
	MatchedCount   int64   `json:"matched_count,omitempty"`
}

// probeRelationshipCandidate represents a rejected relationship candidate.
type probeRelationshipCandidate struct {
	FromColumn      string   `json:"from_column"`
	ToColumn        string   `json:"to_column"`
	RejectionReason string   `json:"rejection_reason"`
	MatchRate       *float64 `json:"match_rate,omitempty"`
}
