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
	EntitySummaries []EntitySummaryResponse `json:"entity_summaries"`
	ColumnDetails   []EntityColumnsResponse `json:"column_details"`
}

// EntitySummaryResponse represents a table-level summary.
type EntitySummaryResponse struct {
	TableName     string              `json:"table_name"`
	BusinessName  string              `json:"business_name"`
	Description   string              `json:"description"`
	Domain        string              `json:"domain"`
	Synonyms      []string            `json:"synonyms,omitempty"`
	KeyColumns    []KeyColumnResponse `json:"key_columns,omitempty"`
	ColumnCount   int                 `json:"column_count"`
	Relationships []string            `json:"relationships,omitempty"`
}

// KeyColumnResponse represents a key column in an entity summary.
type KeyColumnResponse struct {
	Name     string   `json:"name"`
	Synonyms []string `json:"synonyms,omitempty"`
}

// EntityColumnsResponse represents column details for a table.
type EntityColumnsResponse struct {
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
	ontologyRepo   repositories.OntologyRepository
	schemaRepo     repositories.SchemaRepository
	projectService services.ProjectService
	logger         *zap.Logger
}

// NewOntologyEnrichmentHandler creates a new ontology enrichment handler.
func NewOntologyEnrichmentHandler(
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	projectService services.ProjectService,
	logger *zap.Logger,
) *OntologyEnrichmentHandler {
	return &OntologyEnrichmentHandler{
		ontologyRepo:   ontologyRepo,
		schemaRepo:     schemaRepo,
		projectService: projectService,
		logger:         logger,
	}
}

// RegisterRoutes registers the ontology enrichment handler's routes.
func (h *OntologyEnrichmentHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Get enrichment data (entity summaries + column details)
	mux.HandleFunc("GET /api/projects/{pid}/ontology/enrichment",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetEnrichment)))
}

// GetEnrichment handles GET /api/projects/{pid}/ontology/enrichment
// Returns column enrichment data (features extracted from schema) for the Enrichment UI page.
// Works with or without a full ontology - column features come from engine_schema_columns.metadata.
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

	// Try to get ontology for entity summaries (optional)
	var entitySummaries []EntitySummaryResponse
	ontology, err := h.ontologyRepo.GetActive(r.Context(), projectID)
	if err != nil {
		h.logger.Warn("Failed to get active ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		// Continue without entity summaries
	}
	if ontology != nil {
		for _, summary := range ontology.EntitySummaries {
			if summary != nil {
				entitySummaries = append(entitySummaries, h.toEntitySummaryResponse(summary))
			}
		}
	}
	if entitySummaries == nil {
		entitySummaries = []EntitySummaryResponse{}
	}

	// Build column details response from schema columns
	columnDetails := h.buildColumnDetailsFromSchema(schemaColumnsByTable)

	response := ApiResponse{
		Success: true,
		Data: EnrichmentResponse{
			EntitySummaries: entitySummaries,
			ColumnDetails:   columnDetails,
		},
	}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// buildColumnDetailsFromSchema builds column detail responses directly from schema columns.
func (h *OntologyEnrichmentHandler) buildColumnDetailsFromSchema(schemaColumnsByTable map[string][]*models.SchemaColumn) []EntityColumnsResponse {
	result := make([]EntityColumnsResponse, 0, len(schemaColumnsByTable))

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

		result = append(result, EntityColumnsResponse{
			TableName: tableName,
			Columns:   columnResponses,
		})
	}

	return result
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *OntologyEnrichmentHandler) toEnrichmentResponse(ontology *models.TieredOntology, schemaColumnsByTable map[string][]*models.SchemaColumn) EnrichmentResponse {
	// Convert entity summaries (map to array)
	entitySummaries := make([]EntitySummaryResponse, 0, len(ontology.EntitySummaries))
	for _, summary := range ontology.EntitySummaries {
		if summary == nil {
			continue
		}
		entitySummaries = append(entitySummaries, h.toEntitySummaryResponse(summary))
	}

	// Build a lookup map for schema columns by table+column name
	schemaColLookup := make(map[string]*models.SchemaColumn)
	for tableName, cols := range schemaColumnsByTable {
		for _, col := range cols {
			key := tableName + "." + col.ColumnName
			schemaColLookup[key] = col
		}
	}

	// Convert column details (map to array)
	columnDetails := make([]EntityColumnsResponse, 0, len(ontology.ColumnDetails))
	for tableName, columns := range ontology.ColumnDetails {
		columnDetails = append(columnDetails, EntityColumnsResponse{
			TableName: tableName,
			Columns:   h.toColumnDetailResponses(tableName, columns, schemaColLookup),
		})
	}

	return EnrichmentResponse{
		EntitySummaries: entitySummaries,
		ColumnDetails:   columnDetails,
	}
}

func (h *OntologyEnrichmentHandler) toEntitySummaryResponse(summary *models.EntitySummary) EntitySummaryResponse {
	keyColumns := make([]KeyColumnResponse, 0, len(summary.KeyColumns))
	for _, kc := range summary.KeyColumns {
		keyColumns = append(keyColumns, KeyColumnResponse{
			Name:     kc.Name,
			Synonyms: kc.Synonyms,
		})
	}

	return EntitySummaryResponse{
		TableName:     summary.TableName,
		BusinessName:  summary.BusinessName,
		Description:   summary.Description,
		Domain:        summary.Domain,
		Synonyms:      summary.Synonyms,
		KeyColumns:    keyColumns,
		ColumnCount:   summary.ColumnCount,
		Relationships: summary.Relationships,
	}
}

func (h *OntologyEnrichmentHandler) toColumnDetailResponses(tableName string, columns []models.ColumnDetail, schemaColLookup map[string]*models.SchemaColumn) []ColumnDetailResponse {
	responses := make([]ColumnDetailResponse, 0, len(columns))
	for _, col := range columns {
		enumValues := make([]EnumValueResponse, 0, len(col.EnumValues))
		for _, ev := range col.EnumValues {
			// Use Label or Description as "meaning" for UI display
			meaning := ev.Label
			if meaning == "" {
				meaning = ev.Description
			}
			enumValues = append(enumValues, EnumValueResponse{
				Value:   ev.Value,
				Meaning: meaning,
			})
		}

		resp := ColumnDetailResponse{
			Name:         col.Name,
			Description:  col.Description,
			Synonyms:     col.Synonyms,
			SemanticType: col.SemanticType,
			Role:         col.Role,
			EnumValues:   enumValues,
			IsPrimaryKey: col.IsPrimaryKey,
			IsForeignKey: col.IsForeignKey,
			ForeignTable: col.ForeignTable,
		}

		// Look up schema column to get features
		key := tableName + "." + col.Name
		if schemaCol, ok := schemaColLookup[key]; ok {
			if features := schemaCol.GetColumnFeatures(); features != nil {
				resp.Features = h.toColumnFeaturesResponse(features)
			}
		}

		responses = append(responses, resp)
	}
	return responses
}

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
