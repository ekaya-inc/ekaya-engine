package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
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
	schemaRepo     repositories.SchemaRepository
	projectService services.ProjectService
	logger         *zap.Logger
}

// NewOntologyEnrichmentHandler creates a new ontology enrichment handler.
func NewOntologyEnrichmentHandler(
	schemaRepo repositories.SchemaRepository,
	projectService services.ProjectService,
	logger *zap.Logger,
) *OntologyEnrichmentHandler {
	return &OntologyEnrichmentHandler{
		schemaRepo:     schemaRepo,
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
// Returns column enrichment data (features extracted from schema) for the Enrichment UI page.
// Column features come from engine_schema_columns.metadata.
func (h *OntologyEnrichmentHandler) GetEnrichment(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	// Get default datasource to fetch schema columns
	datasourceID, err := h.projectService.GetDefaultDatasourceID(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get default datasource",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_datasource_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get all schema columns with features
	schemaColumnsByTable, err := h.schemaRepo.GetColumnsWithFeaturesByDatasource(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to get schema columns",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_columns_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Build column details response from schema columns
	columnDetails := h.buildColumnDetailsFromSchema(schemaColumnsByTable)

	response := ApiResponse{
		Success: true,
		Data: EnrichmentResponse{
			ColumnDetails: columnDetails,
		},
	}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// buildColumnDetailsFromSchema builds column detail responses directly from schema columns.
func (h *OntologyEnrichmentHandler) buildColumnDetailsFromSchema(schemaColumnsByTable map[string][]*models.SchemaColumn) []TableColumnsResponse {
	result := make([]TableColumnsResponse, 0, len(schemaColumnsByTable))

	for tableName, columns := range schemaColumnsByTable {
		columnResponses := make([]ColumnDetailResponse, 0, len(columns))

		for _, col := range columns {
			resp := ColumnDetailResponse{
				Name:         col.ColumnName,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Get features from column metadata
			if features := col.GetColumnFeatures(); features != nil {
				resp.Description = features.Description
				resp.SemanticType = features.SemanticType
				resp.Role = features.Role
				resp.Features = h.toColumnFeaturesResponse(features)

				// Set IsForeignKey based on features
				if features.Role == "foreign_key" {
					resp.IsForeignKey = true
					if features.IdentifierFeatures != nil && features.IdentifierFeatures.FKTargetTable != "" {
						resp.ForeignTable = features.IdentifierFeatures.FKTargetTable
					}
				}
			}

			columnResponses = append(columnResponses, resp)
		}

		result = append(result, TableColumnsResponse{
			TableName: tableName,
			Columns:   columnResponses,
		})
	}

	return result
}

// ============================================================================
// Helper Methods
// ============================================================================

// toColumnFeaturesResponse converts ColumnFeatures to the API response format.
func (h *OntologyEnrichmentHandler) toColumnFeaturesResponse(features *models.ColumnFeatures) *ColumnFeaturesResponse {
	if features == nil {
		return nil
	}

	resp := &ColumnFeaturesResponse{
		Purpose:            features.Purpose,
		SemanticType:       features.SemanticType,
		Role:               features.Role,
		Description:        features.Description,
		ClassificationPath: string(features.ClassificationPath),
		Confidence:         features.Confidence,
	}

	if features.TimestampFeatures != nil {
		resp.TimestampFeatures = &TimestampFeaturesResponse{
			TimestampPurpose: features.TimestampFeatures.TimestampPurpose,
			IsSoftDelete:     features.TimestampFeatures.IsSoftDelete,
			IsAuditField:     features.TimestampFeatures.IsAuditField,
		}
	}

	if features.BooleanFeatures != nil {
		resp.BooleanFeatures = &BooleanFeaturesResponse{
			TrueMeaning:  features.BooleanFeatures.TrueMeaning,
			FalseMeaning: features.BooleanFeatures.FalseMeaning,
			BooleanType:  features.BooleanFeatures.BooleanType,
		}
	}

	if features.IdentifierFeatures != nil {
		resp.IdentifierFeatures = &IdentifierFeaturesResponse{
			IdentifierType:   features.IdentifierFeatures.IdentifierType,
			ExternalService:  features.IdentifierFeatures.ExternalService,
			FKTargetTable:    features.IdentifierFeatures.FKTargetTable,
			FKTargetColumn:   features.IdentifierFeatures.FKTargetColumn,
			FKConfidence:     features.IdentifierFeatures.FKConfidence,
			EntityReferenced: features.IdentifierFeatures.EntityReferenced,
		}
	}

	if features.MonetaryFeatures != nil {
		resp.MonetaryFeatures = &MonetaryFeaturesResponse{
			IsMonetary:           features.MonetaryFeatures.IsMonetary,
			CurrencyUnit:         features.MonetaryFeatures.CurrencyUnit,
			PairedCurrencyColumn: features.MonetaryFeatures.PairedCurrencyColumn,
		}
	}

	return resp
}
