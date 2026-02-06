package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Response Types
// ============================================================================

// EnrichmentResponse represents the enrichment data for UI display.
type EnrichmentResponse struct {
	ColumnDetails []TableColumnsResponse `json:"column_details"`
}

// TableColumnsResponse represents column details for a table.
type TableColumnsResponse struct {
	TableName string                 `json:"table_name"`
	Columns   []ColumnDetailResponse `json:"columns"`
}

// ColumnDetailResponse represents detailed column enrichment.
type ColumnDetailResponse struct {
	Name         string                  `json:"name"`
	Description  string                  `json:"description,omitempty"`
	Synonyms     []string                `json:"synonyms,omitempty"`
	SemanticType string                  `json:"semantic_type,omitempty"`
	Role         string                  `json:"role,omitempty"`
	EnumValues   []EnumValueResponse     `json:"enum_values,omitempty"`
	IsPrimaryKey bool                    `json:"is_primary_key"`
	IsForeignKey bool                    `json:"is_foreign_key"`
	ForeignTable string                  `json:"foreign_table,omitempty"`
	Features     *ColumnFeaturesResponse `json:"features,omitempty"` // From feature extraction pipeline
}

// ColumnFeaturesResponse contains extracted column features from the feature extraction pipeline.
type ColumnFeaturesResponse struct {
	Purpose            string  `json:"purpose,omitempty"`
	SemanticType       string  `json:"semantic_type,omitempty"`
	Role               string  `json:"role,omitempty"`
	Description        string  `json:"description,omitempty"`
	ClassificationPath string  `json:"classification_path,omitempty"`
	Confidence         float64 `json:"confidence,omitempty"`

	// Path-specific features
	TimestampFeatures  *TimestampFeaturesResponse  `json:"timestamp_features,omitempty"`
	BooleanFeatures    *BooleanFeaturesResponse    `json:"boolean_features,omitempty"`
	IdentifierFeatures *IdentifierFeaturesResponse `json:"identifier_features,omitempty"`
	MonetaryFeatures   *MonetaryFeaturesResponse   `json:"monetary_features,omitempty"`
}

// TimestampFeaturesResponse contains timestamp-specific classification results.
type TimestampFeaturesResponse struct {
	TimestampPurpose string `json:"timestamp_purpose,omitempty"`
	IsSoftDelete     bool   `json:"is_soft_delete,omitempty"`
	IsAuditField     bool   `json:"is_audit_field,omitempty"`
}

// BooleanFeaturesResponse contains boolean-specific classification results.
type BooleanFeaturesResponse struct {
	TrueMeaning  string `json:"true_meaning,omitempty"`
	FalseMeaning string `json:"false_meaning,omitempty"`
	BooleanType  string `json:"boolean_type,omitempty"`
}

// IdentifierFeaturesResponse contains identifier-specific classification results.
type IdentifierFeaturesResponse struct {
	IdentifierType   string  `json:"identifier_type,omitempty"`
	ExternalService  string  `json:"external_service,omitempty"`
	FKTargetTable    string  `json:"fk_target_table,omitempty"`
	FKTargetColumn   string  `json:"fk_target_column,omitempty"`
	FKConfidence     float64 `json:"fk_confidence,omitempty"`
	EntityReferenced string  `json:"entity_referenced,omitempty"`
}

// MonetaryFeaturesResponse contains monetary-specific classification results.
type MonetaryFeaturesResponse struct {
	IsMonetary           bool   `json:"is_monetary,omitempty"`
	CurrencyUnit         string `json:"currency_unit,omitempty"`
	PairedCurrencyColumn string `json:"paired_currency_column,omitempty"`
}

// EnumValueResponse represents an enum value.
type EnumValueResponse struct {
	Value   string `json:"value"`
	Meaning string `json:"meaning,omitempty"`
}

// ============================================================================
// Handler
// ============================================================================

// OntologyEnrichmentHandler handles ontology enrichment HTTP requests.
type OntologyEnrichmentHandler struct {
	schemaService  services.SchemaService
	projectService services.ProjectService
	logger         *zap.Logger
}

// NewOntologyEnrichmentHandler creates a new ontology enrichment handler.
func NewOntologyEnrichmentHandler(
	schemaService services.SchemaService,
	projectService services.ProjectService,
	logger *zap.Logger,
) *OntologyEnrichmentHandler {
	return &OntologyEnrichmentHandler{
		schemaService:  schemaService,
		projectService: projectService,
		logger:         logger,
	}
}

// RegisterRoutes registers the ontology enrichment handler's routes.
func (h *OntologyEnrichmentHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Get enrichment data (column details with features)
	mux.HandleFunc("GET /api/projects/{pid}/ontology/enrichment",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetEnrichment)))
}

// GetEnrichment handles GET /api/projects/{pid}/ontology/enrichment
// Returns column enrichment data for the Enrichment UI page.
// Joins schema columns with column metadata using schema_column_id.
func (h *OntologyEnrichmentHandler) GetEnrichment(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}
	ctx := r.Context()

	// Get default datasource
	dsID, err := h.projectService.GetDefaultDatasourceID(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to get default datasource", zap.Error(err))
		_ = ErrorResponse(w, http.StatusInternalServerError, "get_default_datasource_failed", err.Error())
		return
	}
	if dsID == (uuid.UUID{}) {
		// No datasource configured - return empty
		response := ApiResponse{
			Success: true,
			Data: EnrichmentResponse{
				ColumnDetails: []TableColumnsResponse{},
			},
		}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	// Get all tables for the datasource
	tables, err := h.schemaService.ListTablesByDatasource(ctx, projectID, dsID, false)
	if err != nil {
		h.logger.Error("Failed to list tables", zap.Error(err))
		_ = ErrorResponse(w, http.StatusInternalServerError, "list_tables_failed", err.Error())
		return
	}

	// Get all column metadata for the project
	allMetadata, err := h.schemaService.GetColumnMetadataByProject(ctx, projectID)
	if err != nil {
		h.logger.Error("Failed to get column metadata", zap.Error(err))
		_ = ErrorResponse(w, http.StatusInternalServerError, "get_column_metadata_failed", err.Error())
		return
	}

	// Build map of schema_column_id -> metadata for quick lookup
	metadataByColumnID := make(map[uuid.UUID]*models.ColumnMetadata)
	for _, meta := range allMetadata {
		metadataByColumnID[meta.SchemaColumnID] = meta
	}

	// Build response for each table
	tableResponses := make([]TableColumnsResponse, 0, len(tables))
	for _, table := range tables {
		// Get columns for this table
		columns, err := h.schemaService.ListColumnsByTable(ctx, projectID, table.ID, false)
		if err != nil {
			h.logger.Warn("Failed to list columns for table",
				zap.String("table_name", table.TableName),
				zap.Error(err))
			continue
		}

		// Build column details
		columnDetails := make([]ColumnDetailResponse, 0, len(columns))
		for _, col := range columns {
			detail := ColumnDetailResponse{
				Name:         col.ColumnName,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Look up metadata for this column
			if meta, ok := metadataByColumnID[col.ID]; ok {
				if meta.Description != nil {
					detail.Description = *meta.Description
				}
				if meta.SemanticType != nil {
					detail.SemanticType = *meta.SemanticType
				}
				if meta.Role != nil {
					detail.Role = *meta.Role
				}

				// Check for FK from identifier features
				if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil {
					if idFeatures.FKTargetTable != "" {
						detail.IsForeignKey = true
						detail.ForeignTable = idFeatures.FKTargetTable
					}
				}

				// Convert full features to response format
				detail.Features = h.toColumnFeaturesResponseFromMetadata(meta)
			}

			columnDetails = append(columnDetails, detail)
		}

		if len(columnDetails) > 0 {
			tableResponses = append(tableResponses, TableColumnsResponse{
				TableName: table.TableName,
				Columns:   columnDetails,
			})
		}
	}

	response := ApiResponse{
		Success: true,
		Data: EnrichmentResponse{
			ColumnDetails: tableResponses,
		},
	}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

// toColumnFeaturesResponseFromMetadata converts ColumnMetadata to the API response format.
func (h *OntologyEnrichmentHandler) toColumnFeaturesResponseFromMetadata(meta *models.ColumnMetadata) *ColumnFeaturesResponse {
	if meta == nil {
		return nil
	}

	resp := &ColumnFeaturesResponse{}

	if meta.Purpose != nil {
		resp.Purpose = *meta.Purpose
	}
	if meta.SemanticType != nil {
		resp.SemanticType = *meta.SemanticType
	}
	if meta.Role != nil {
		resp.Role = *meta.Role
	}
	if meta.Description != nil {
		resp.Description = *meta.Description
	}
	if meta.ClassificationPath != nil {
		resp.ClassificationPath = *meta.ClassificationPath
	}
	if meta.Confidence != nil {
		resp.Confidence = *meta.Confidence
	}

	if meta.Features.TimestampFeatures != nil {
		resp.TimestampFeatures = &TimestampFeaturesResponse{
			TimestampPurpose: meta.Features.TimestampFeatures.TimestampPurpose,
			IsSoftDelete:     meta.Features.TimestampFeatures.IsSoftDelete,
			IsAuditField:     meta.Features.TimestampFeatures.IsAuditField,
		}
	}

	if meta.Features.BooleanFeatures != nil {
		resp.BooleanFeatures = &BooleanFeaturesResponse{
			TrueMeaning:  meta.Features.BooleanFeatures.TrueMeaning,
			FalseMeaning: meta.Features.BooleanFeatures.FalseMeaning,
			BooleanType:  meta.Features.BooleanFeatures.BooleanType,
		}
	}

	if meta.Features.IdentifierFeatures != nil {
		resp.IdentifierFeatures = &IdentifierFeaturesResponse{
			IdentifierType:   meta.Features.IdentifierFeatures.IdentifierType,
			ExternalService:  meta.Features.IdentifierFeatures.ExternalService,
			FKTargetTable:    meta.Features.IdentifierFeatures.FKTargetTable,
			FKTargetColumn:   meta.Features.IdentifierFeatures.FKTargetColumn,
			FKConfidence:     meta.Features.IdentifierFeatures.FKConfidence,
			EntityReferenced: meta.Features.IdentifierFeatures.EntityReferenced,
		}
	}

	if meta.Features.MonetaryFeatures != nil {
		resp.MonetaryFeatures = &MonetaryFeaturesResponse{
			IsMonetary:           meta.Features.MonetaryFeatures.IsMonetary,
			CurrencyUnit:         meta.Features.MonetaryFeatures.CurrencyUnit,
			PairedCurrencyColumn: meta.Features.MonetaryFeatures.PairedCurrencyColumn,
		}
	}

	return resp
}
