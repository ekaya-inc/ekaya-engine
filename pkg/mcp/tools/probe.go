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
			return NewErrorResult("invalid_parameters", "invalid request arguments"), nil
		}

		columnsArg, ok := args["columns"].([]any)
		if !ok || len(columnsArg) == 0 {
			return NewErrorResult("invalid_parameters",
				"columns parameter is required and must be a non-empty array"), nil
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
				return NewErrorResult("invalid_parameters",
					"each column must be an object with 'table' and 'column' fields"), nil
			}

			table, ok := colMap["table"].(string)
			if !ok || table == "" {
				return NewErrorResult("invalid_parameters",
					"each column must have a non-empty 'table' field"), nil
			}

			column, ok := colMap["column"].(string)
			if !ok || column == "" {
				return NewErrorResult("invalid_parameters",
					"each column must have a non-empty 'column' field"), nil
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

		// Calculate null rate from NullCount or derive from NonNullCount
		// Adapters populate NonNullCount via COUNT(col) but not NullCount
		if column.RowCount != nil && *column.RowCount > 0 {
			var nullCount int64
			if column.NullCount != nil {
				nullCount = *column.NullCount
			} else if column.NonNullCount != nil {
				nullCount = *column.RowCount - *column.NonNullCount
			}
			if nullCount > 0 || column.NullCount != nil || column.NonNullCount != nil {
				nullRate := float64(nullCount) / float64(*column.RowCount)
				stats.NullRate = &nullRate
			}
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

	// TODO: Fetch column metadata from ColumnMetadataRepository using column.ID
	// The new schema uses SchemaColumnID (FK) instead of TableName/ColumnName.
	// Enum values, descriptions, and features are now in the Features JSONB.
	// See PLAN-column-schema-refactor.md for details.
	if deps.ColumnMetadataRepo != nil && column != nil {
		columnMeta, err := deps.ColumnMetadataRepo.GetBySchemaColumnID(ctx, column.ID)
		if err != nil {
			deps.Logger.Warn("Failed to get column metadata",
				zap.String("project_id", projectID.String()),
				zap.String("table", tableName),
				zap.String("column", columnName),
				zap.Error(err))
		} else if columnMeta != nil {
			// Initialize semantic section if not already present
			if response.Semantic == nil {
				response.Semantic = &probeColumnSemantic{}
			}

			// Merge description if present
			if columnMeta.Description != nil && *columnMeta.Description != "" && response.Semantic.Description == "" {
				response.Semantic.Description = *columnMeta.Description
			}

			// Merge role if present
			if columnMeta.Role != nil && *columnMeta.Role != "" && response.Semantic.Role == "" {
				response.Semantic.Role = *columnMeta.Role
			}

			// Extract features from column metadata
			response.Features = &probeColumnFeatures{
				Purpose:            ptrStrValue(columnMeta.Purpose),
				SemanticType:       ptrStrValue(columnMeta.SemanticType),
				Role:               ptrStrValue(columnMeta.Role),
				Description:        ptrStrValue(columnMeta.Description),
				ClassificationPath: ptrStrValue(columnMeta.ClassificationPath),
				Confidence:         ptrFloat64Value(columnMeta.Confidence),
			}

			// Add timestamp features if present
			if tsFeatures := columnMeta.GetTimestampFeatures(); tsFeatures != nil {
				response.Features.TimestampFeatures = &probeTimestampFeatures{
					TimestampPurpose: tsFeatures.TimestampPurpose,
					IsSoftDelete:     tsFeatures.IsSoftDelete,
					IsAuditField:     tsFeatures.IsAuditField,
				}
			}

			// Add boolean features if present
			if boolFeatures := columnMeta.GetBooleanFeatures(); boolFeatures != nil {
				response.Features.BooleanFeatures = &probeBooleanFeatures{
					TrueMeaning:  boolFeatures.TrueMeaning,
					FalseMeaning: boolFeatures.FalseMeaning,
					BooleanType:  boolFeatures.BooleanType,
				}
			}

			// Add identifier features if present
			if idFeatures := columnMeta.GetIdentifierFeatures(); idFeatures != nil {
				response.Features.IdentifierFeatures = &probeIdentifierFeatures{
					IdentifierType:  idFeatures.IdentifierType,
					ExternalService: idFeatures.ExternalService,
					FKTargetTable:   idFeatures.FKTargetTable,
					FKTargetColumn:  idFeatures.FKTargetColumn,
					FKConfidence:    idFeatures.FKConfidence,
				}
			}

			// Add monetary features if present
			if moneyFeatures := columnMeta.GetMonetaryFeatures(); moneyFeatures != nil {
				response.Features.MonetaryFeatures = &probeMonetaryFeatures{
					IsMonetary:           moneyFeatures.IsMonetary,
					CurrencyUnit:         moneyFeatures.CurrencyUnit,
					PairedCurrencyColumn: moneyFeatures.PairedCurrencyColumn,
				}
			}
		}
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
	Features     *probeColumnFeatures    `json:"features,omitempty"` // Column features from extraction pipeline
	Error        string                  `json:"error,omitempty"`    // For batch mode partial failures
}

// probeColumnFeatures contains extracted column features from the feature extraction pipeline.
type probeColumnFeatures struct {
	Purpose            string  `json:"purpose,omitempty"`             // "identifier", "timestamp", "flag", "measure", "enum", "text"
	SemanticType       string  `json:"semantic_type,omitempty"`       // More specific type like "soft_delete_timestamp", "monetary"
	Role               string  `json:"role,omitempty"`                // "primary_key", "foreign_key", "attribute", "measure"
	Description        string  `json:"description,omitempty"`         // LLM-generated business description
	ClassificationPath string  `json:"classification_path,omitempty"` // Analysis path taken: "timestamp", "boolean", "enum", etc.
	Confidence         float64 `json:"confidence,omitempty"`          // Classification confidence (0.0 - 1.0)

	// Path-specific features (populated based on classification)
	TimestampFeatures  *probeTimestampFeatures  `json:"timestamp_features,omitempty"`
	BooleanFeatures    *probeBooleanFeatures    `json:"boolean_features,omitempty"`
	IdentifierFeatures *probeIdentifierFeatures `json:"identifier_features,omitempty"`
	MonetaryFeatures   *probeMonetaryFeatures   `json:"monetary_features,omitempty"`
}

// probeTimestampFeatures contains timestamp-specific classification results.
type probeTimestampFeatures struct {
	TimestampPurpose string `json:"timestamp_purpose,omitempty"` // "audit_created", "audit_updated", "soft_delete", etc.
	IsSoftDelete     bool   `json:"is_soft_delete,omitempty"`
	IsAuditField     bool   `json:"is_audit_field,omitempty"`
}

// probeBooleanFeatures contains boolean-specific classification results.
type probeBooleanFeatures struct {
	TrueMeaning  string `json:"true_meaning,omitempty"`
	FalseMeaning string `json:"false_meaning,omitempty"`
	BooleanType  string `json:"boolean_type,omitempty"` // "feature_flag", "status_indicator", "permission"
}

// probeIdentifierFeatures contains identifier-specific classification results.
type probeIdentifierFeatures struct {
	IdentifierType   string  `json:"identifier_type,omitempty"`   // "internal_uuid", "foreign_key", "external_service_id"
	ExternalService  string  `json:"external_service,omitempty"`  // "stripe", "twilio", etc.
	FKTargetTable    string  `json:"fk_target_table,omitempty"`   // Resolved FK target table
	FKTargetColumn   string  `json:"fk_target_column,omitempty"`  // Resolved FK target column
	FKConfidence     float64 `json:"fk_confidence,omitempty"`     // FK resolution confidence
	EntityReferenced string  `json:"entity_referenced,omitempty"` // Entity this identifier refers to
}

// probeMonetaryFeatures contains monetary-specific classification results.
type probeMonetaryFeatures struct {
	IsMonetary           bool   `json:"is_monetary,omitempty"`
	CurrencyUnit         string `json:"currency_unit,omitempty"`          // "cents", "dollars", "USD"
	PairedCurrencyColumn string `json:"paired_currency_column,omitempty"` // Column containing currency code
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

// ptrStrValue safely dereferences a string pointer, returning empty string if nil.
func ptrStrValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ptrFloat64Value safely dereferences a float64 pointer, returning 0 if nil.
func ptrFloat64Value(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}
