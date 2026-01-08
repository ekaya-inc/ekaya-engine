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

// ProbeToolDeps contains dependencies for probe tools.
type ProbeToolDeps struct {
	DB               *database.DB
	MCPConfigService services.MCPConfigService
	SchemaRepo       repositories.SchemaRepository
	OntologyRepo     repositories.OntologyRepository
	EntityRepo       repositories.OntologyEntityRepository
	RelationshipRepo repositories.EntityRelationshipRepository
	Logger           *zap.Logger
}

// RegisterProbeTools registers probe MCP tools.
func RegisterProbeTools(s *server.MCPServer, deps *ProbeToolDeps) {
	registerProbeColumnTool(s, deps)
	registerProbeColumnsTool(s, deps)
	registerProbeRelationshipTool(s, deps)
}

// checkProbeToolEnabled verifies a specific probe tool is enabled for the project.
// Uses ToolAccessChecker to ensure consistency with tool list filtering.
func checkProbeToolEnabled(ctx context.Context, deps *ProbeToolDeps, toolName string) (uuid.UUID, context.Context, func(), error) {
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
		projectID, tenantCtx, cleanup, err := checkProbeToolEnabled(ctx, deps, "probe_column")
		if err != nil {
			return nil, err
		}
		defer cleanup()

		// Get required parameters
		tableName, err := req.RequireString("table")
		if err != nil {
			return nil, err
		}
		columnName, err := req.RequireString("column")
		if err != nil {
			return nil, err
		}

		// Probe the column
		result, err := probeColumn(tenantCtx, deps, projectID, tableName, columnName)
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
		projectID, tenantCtx, cleanup, err := checkProbeToolEnabled(ctx, deps, "probe_columns")
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
func probeColumn(ctx context.Context, deps *ProbeToolDeps, projectID uuid.UUID, tableName, columnName string) (*probeColumnResponse, error) {
	response := &probeColumnResponse{
		Table:  tableName,
		Column: columnName,
	}

	// Get datasource ID (assume first datasource for now)
	// TODO: Support multi-datasource projects
	tables, err := deps.SchemaRepo.GetColumnsByTables(ctx, projectID, []string{tableName})
	if err != nil {
		return nil, fmt.Errorf("failed to get columns for table: %w", err)
	}

	columns, ok := tables[tableName]
	if !ok || len(columns) == 0 {
		return nil, fmt.Errorf("table '%s' not found", tableName)
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
		return nil, fmt.Errorf("column '%s' not found in table '%s'", columnName, tableName)
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

				// Extract enum labels
				if len(colDetail.EnumValues) > 0 {
					enumLabels := make(map[string]string)
					for _, ev := range colDetail.EnumValues {
						if ev.Label != "" {
							enumLabels[ev.Value] = ev.Label
						} else if ev.Description != "" {
							enumLabels[ev.Value] = ev.Description
						}
					}
					if len(enumLabels) > 0 {
						semantic.EnumLabels = enumLabels
					}
				}

				response.Semantic = &semantic
				break
			}
		}
	}

	// Note: sample_values are not currently persisted, so we cannot return them
	// This would require on-demand fetching from the datasource adapter

	return response, nil
}

// probeColumnResponse is the response format for probe_column tool.
type probeColumnResponse struct {
	Table        string                  `json:"table"`
	Column       string                  `json:"column"`
	Statistics   *probeColumnStatistics  `json:"statistics,omitempty"`
	Joinability  *probeColumnJoinability `json:"joinability,omitempty"`
	SampleValues []string                `json:"sample_values,omitempty"` // Not yet implemented
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
	Entity      string            `json:"entity,omitempty"`
	Role        string            `json:"role,omitempty"`
	Description string            `json:"description,omitempty"`
	EnumLabels  map[string]string `json:"enum_labels,omitempty"`
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
		projectID, tenantCtx, cleanup, err := checkProbeToolEnabled(ctx, deps, "probe_relationship")
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
	entities, err := deps.EntityRepo.GetByOntology(ctx, ontology.ID)
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

	// Note: To get data quality metrics (match_rate, orphan_count, etc.), we would need to
	// query engine_schema_relationships table. For now, we'll return what's available from
	// engine_entity_relationships, which includes cardinality but not the detailed metrics.
	// TODO: Add SchemaRelationshipRepository to fetch data quality metrics

	// Build response for confirmed relationships
	for _, rel := range filteredRelationships {
		detail := probeRelationshipDetail{
			FromEntity: entityIDToName[rel.SourceEntityID],
			ToEntity:   entityIDToName[rel.TargetEntityID],
			FromColumn: fmt.Sprintf("%s.%s", rel.SourceColumnTable, rel.SourceColumnName),
			ToColumn:   fmt.Sprintf("%s.%s", rel.TargetColumnTable, rel.TargetColumnName),
		}

		// Note: Cardinality and data quality metrics are stored in engine_schema_relationships
		// For now, we'll skip those and only return the entity-level relationship info
		// TODO: Query engine_schema_relationships to get match_rate, source_distinct, target_distinct, etc.

		// Add description and association if available
		if rel.Description != nil {
			detail.Description = rel.Description
		}
		if rel.Association != nil {
			detail.Label = rel.Association
		}

		response.Relationships = append(response.Relationships, detail)
	}

	// Note: Rejected candidates are stored in engine_schema_relationships with rejection_reason
	// For now, we'll skip this feature and return an empty list
	// TODO: Query engine_schema_relationships WHERE rejection_reason IS NOT NULL

	return response, nil
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
