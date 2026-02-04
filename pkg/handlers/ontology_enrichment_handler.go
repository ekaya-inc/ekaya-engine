package handlers

import (
	"net/http"

	"github.com/google/uuid"
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
	ontologyRepo       repositories.OntologyRepository
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	projectService     services.ProjectService
	logger             *zap.Logger
}

// NewOntologyEnrichmentHandler creates a new ontology enrichment handler.
func NewOntologyEnrichmentHandler(
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	projectService services.ProjectService,
	logger *zap.Logger,
) *OntologyEnrichmentHandler {
	return &OntologyEnrichmentHandler{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		projectService:     projectService,
		logger:             logger,
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
// Works with or without a full ontology - column features come from engine_ontology_column_metadata.
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

	// Get all tables for this datasource
	tables, err := h.schemaRepo.ListTablesByDatasource(r.Context(), projectID, datasourceID, false)
	if err != nil {
		h.logger.Error("Failed to list tables",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_tables_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Extract table names for batch column fetch
	tableNames := make([]string, len(tables))
	for i, t := range tables {
		tableNames[i] = t.TableName
	}

	// Get all columns for these tables
	schemaColumnsByTable, err := h.schemaRepo.GetColumnsByTables(r.Context(), projectID, tableNames, false)
	if err != nil {
		h.logger.Error("Failed to get schema columns",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_columns_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get column metadata for all columns (features are now in engine_ontology_column_metadata)
	var allColumnIDs []uuid.UUID
	for _, columns := range schemaColumnsByTable {
		for _, col := range columns {
			allColumnIDs = append(allColumnIDs, col.ID)
		}
	}
	columnMetadataMap := make(map[uuid.UUID]*models.ColumnMetadata)
	if len(allColumnIDs) > 0 && h.columnMetadataRepo != nil {
		metadataByID, err := h.columnMetadataRepo.GetBySchemaColumnIDs(r.Context(), allColumnIDs)
		if err != nil {
			h.logger.Warn("Failed to get column metadata",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Continue without metadata - graceful degradation
		} else {
			// Build lookup map by schema column ID
			for _, meta := range metadataByID {
				columnMetadataMap[meta.SchemaColumnID] = meta
			}
		}
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
	columnDetails := h.buildColumnDetailsFromSchema(schemaColumnsByTable, columnMetadataMap)

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
// Column features are now stored in engine_ontology_column_metadata.features JSONB.
func (h *OntologyEnrichmentHandler) buildColumnDetailsFromSchema(schemaColumnsByTable map[string][]*models.SchemaColumn, columnMetadataMap map[uuid.UUID]*models.ColumnMetadata) []EntityColumnsResponse {
	result := make([]EntityColumnsResponse, 0, len(schemaColumnsByTable))

	for tableName, columns := range schemaColumnsByTable {
		columnResponses := make([]ColumnDetailResponse, 0, len(columns))

		for _, col := range columns {
			resp := ColumnDetailResponse{
				Name:         col.ColumnName,
				IsPrimaryKey: col.IsPrimaryKey,
			}

			// Get features from column metadata (now in separate table)
			if meta, ok := columnMetadataMap[col.ID]; ok && meta != nil {
				if meta.Description != nil {
					resp.Description = *meta.Description
				}
				if meta.Role != nil {
					resp.Role = *meta.Role
				}
				resp.Features = h.toColumnMetadataFeaturesResponse(meta)

				// Set IsForeignKey based on identifier features
				if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil {
					if idFeatures.IdentifierType == "foreign_key" {
						resp.IsForeignKey = true
						if idFeatures.FKTargetTable != "" {
							resp.ForeignTable = idFeatures.FKTargetTable
						}
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

// toColumnMetadataFeaturesResponse converts ColumnMetadata to the API response format.
// Column features are now stored in engine_ontology_column_metadata.features JSONB.
func (h *OntologyEnrichmentHandler) toColumnMetadataFeaturesResponse(meta *models.ColumnMetadata) *ColumnFeaturesResponse {
	if meta == nil {
		return nil
	}

	resp := &ColumnFeaturesResponse{}

	// Set basic fields from ColumnMetadata
	if meta.Purpose != nil {
		resp.Purpose = *meta.Purpose
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

	// Add timestamp features if present
	if tsFeatures := meta.GetTimestampFeatures(); tsFeatures != nil {
		resp.TimestampFeatures = &TimestampFeaturesResponse{
			TimestampPurpose: tsFeatures.TimestampPurpose,
			IsSoftDelete:     tsFeatures.IsSoftDelete,
			IsAuditField:     tsFeatures.IsAuditField,
		}
	}

	// Add boolean features if present
	if boolFeatures := meta.GetBooleanFeatures(); boolFeatures != nil {
		resp.BooleanFeatures = &BooleanFeaturesResponse{
			TrueMeaning:  boolFeatures.TrueMeaning,
			FalseMeaning: boolFeatures.FalseMeaning,
			BooleanType:  boolFeatures.BooleanType,
		}
	}

	// Add identifier features if present
	if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil {
		resp.IdentifierFeatures = &IdentifierFeaturesResponse{
			IdentifierType:   idFeatures.IdentifierType,
			ExternalService:  idFeatures.ExternalService,
			FKTargetTable:    idFeatures.FKTargetTable,
			FKTargetColumn:   idFeatures.FKTargetColumn,
			FKConfidence:     idFeatures.FKConfidence,
			EntityReferenced: idFeatures.EntityReferenced,
		}
	}

	// Add monetary features if present
	if monFeatures := meta.GetMonetaryFeatures(); monFeatures != nil {
		resp.MonetaryFeatures = &MonetaryFeaturesResponse{
			IsMonetary:           monFeatures.IsMonetary,
			CurrencyUnit:         monFeatures.CurrencyUnit,
			PairedCurrencyColumn: monFeatures.PairedCurrencyColumn,
		}
	}

	// Check if we actually have any non-empty fields
	if resp.Purpose == "" && resp.Role == "" && resp.Description == "" && resp.ClassificationPath == "" &&
		resp.TimestampFeatures == nil && resp.BooleanFeatures == nil &&
		resp.IdentifierFeatures == nil && resp.MonetaryFeatures == nil {
		return nil
	}

	return resp
}
