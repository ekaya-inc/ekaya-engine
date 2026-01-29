package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
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
	Name         string              `json:"name"`
	Description  string              `json:"description,omitempty"`
	Synonyms     []string            `json:"synonyms,omitempty"`
	SemanticType string              `json:"semantic_type,omitempty"`
	Role         string              `json:"role,omitempty"`
	EnumValues   []EnumValueResponse `json:"enum_values,omitempty"`
	IsPrimaryKey bool                `json:"is_primary_key"`
	IsForeignKey bool                `json:"is_foreign_key"`
	ForeignTable string              `json:"foreign_table,omitempty"`
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
	ontologyRepo repositories.OntologyRepository
	logger       *zap.Logger
}

// NewOntologyEnrichmentHandler creates a new ontology enrichment handler.
func NewOntologyEnrichmentHandler(
	ontologyRepo repositories.OntologyRepository,
	logger *zap.Logger,
) *OntologyEnrichmentHandler {
	return &OntologyEnrichmentHandler{
		ontologyRepo: ontologyRepo,
		logger:       logger,
	}
}

// RegisterRoutes registers the ontology enrichment handler's routes.
func (h *OntologyEnrichmentHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Get enrichment data (entity summaries + column details)
	mux.HandleFunc("GET /api/projects/{pid}/ontology/enrichment",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetEnrichment)))
}

// GetEnrichment handles GET /api/projects/{pid}/ontology/enrichment
// Returns the tiered ontology data formatted for the Enrichment UI page.
func (h *OntologyEnrichmentHandler) GetEnrichment(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	ontology, err := h.ontologyRepo.GetActive(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get active ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_ontology_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if ontology == nil {
		// No ontology exists yet - return empty response
		response := ApiResponse{
			Success: true,
			Data: EnrichmentResponse{
				EntitySummaries: []EntitySummaryResponse{},
				ColumnDetails:   []EntityColumnsResponse{},
			},
		}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	// Convert to response format
	enrichment := h.toEnrichmentResponse(ontology)

	response := ApiResponse{Success: true, Data: enrichment}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *OntologyEnrichmentHandler) toEnrichmentResponse(ontology *models.TieredOntology) EnrichmentResponse {
	// Convert entity summaries (map to array)
	entitySummaries := make([]EntitySummaryResponse, 0, len(ontology.EntitySummaries))
	for _, summary := range ontology.EntitySummaries {
		if summary == nil {
			continue
		}
		entitySummaries = append(entitySummaries, h.toEntitySummaryResponse(summary))
	}

	// Convert column details (map to array)
	columnDetails := make([]EntityColumnsResponse, 0, len(ontology.ColumnDetails))
	for tableName, columns := range ontology.ColumnDetails {
		columnDetails = append(columnDetails, EntityColumnsResponse{
			TableName: tableName,
			Columns:   h.toColumnDetailResponses(columns),
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

func (h *OntologyEnrichmentHandler) toColumnDetailResponses(columns []models.ColumnDetail) []ColumnDetailResponse {
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

		responses = append(responses, ColumnDetailResponse{
			Name:         col.Name,
			Description:  col.Description,
			Synonyms:     col.Synonyms,
			SemanticType: col.SemanticType,
			Role:         col.Role,
			EnumValues:   enumValues,
			IsPrimaryKey: col.IsPrimaryKey,
			IsForeignKey: col.IsForeignKey,
			ForeignTable: col.ForeignTable,
		})
	}
	return responses
}
