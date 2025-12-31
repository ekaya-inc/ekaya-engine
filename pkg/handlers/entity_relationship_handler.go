package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// EntityRelationshipResponse represents a relationship between two entities.
type EntityRelationshipResponse struct {
	ID                 string  `json:"id"`
	SourceEntityID     string  `json:"source_entity_id"`
	TargetEntityID     string  `json:"target_entity_id"`
	SourceColumnSchema string  `json:"source_column_schema"`
	SourceColumnTable  string  `json:"source_column_table"`
	SourceColumnName   string  `json:"source_column_name"`
	TargetColumnSchema string  `json:"target_column_schema"`
	TargetColumnTable  string  `json:"target_column_table"`
	TargetColumnName   string  `json:"target_column_name"`
	DetectionMethod    string  `json:"detection_method"`
	Confidence         float64 `json:"confidence"`
	Status             string  `json:"status"`
}

// EntityRelationshipListResponse for GET /relationships
type EntityRelationshipListResponse struct {
	Relationships []EntityRelationshipResponse `json:"relationships"`
	Total         int                          `json:"total"`
}

// DiscoverEntityRelationshipsResponse for POST /datasources/{dsid}/relationships/discover
type DiscoverEntityRelationshipsResponse struct {
	FKRelationships       int `json:"fk_relationships"`
	InferredRelationships int `json:"inferred_relationships"`
	TotalRelationships    int `json:"total_relationships"`
}

// ============================================================================
// Handler
// ============================================================================

// EntityRelationshipHandler handles entity relationship HTTP requests.
type EntityRelationshipHandler struct {
	relationshipService services.DeterministicRelationshipService
	relationshipRepo    repositories.EntityRelationshipRepository
	logger              *zap.Logger
}

// NewEntityRelationshipHandler creates a new entity relationship handler.
func NewEntityRelationshipHandler(
	relationshipService services.DeterministicRelationshipService,
	relationshipRepo repositories.EntityRelationshipRepository,
	logger *zap.Logger,
) *EntityRelationshipHandler {
	return &EntityRelationshipHandler{
		relationshipService: relationshipService,
		relationshipRepo:    relationshipRepo,
		logger:              logger,
	}
}

// RegisterRoutes registers the entity relationship handler's routes on the given mux.
func (h *EntityRelationshipHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Discovery endpoint - per datasource
	mux.HandleFunc("POST /api/projects/{pid}/datasources/{dsid}/relationships/discover",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Discover)))

	// List endpoint - per project
	mux.HandleFunc("GET /api/projects/{pid}/relationships",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
}

// Discover handles POST /api/projects/{pid}/datasources/{dsid}/relationships/discover
func (h *EntityRelationshipHandler) Discover(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	datasourceID, ok := h.parseDatasourceID(w, r)
	if !ok {
		return
	}

	result, err := h.relationshipService.DiscoverRelationships(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to discover relationships",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "discover_relationships_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := DiscoverEntityRelationshipsResponse{
		FKRelationships:       result.FKRelationships,
		InferredRelationships: result.InferredRelationships,
		TotalRelationships:    result.TotalRelationships,
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// List handles GET /api/projects/{pid}/relationships
func (h *EntityRelationshipHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	relationships, err := h.relationshipRepo.GetByProject(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list relationships",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_relationships_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert to response format
	relResponses := make([]EntityRelationshipResponse, 0, len(relationships))
	for _, rel := range relationships {
		relResponses = append(relResponses, EntityRelationshipResponse{
			ID:                 rel.ID.String(),
			SourceEntityID:     rel.SourceEntityID.String(),
			TargetEntityID:     rel.TargetEntityID.String(),
			SourceColumnSchema: rel.SourceColumnSchema,
			SourceColumnTable:  rel.SourceColumnTable,
			SourceColumnName:   rel.SourceColumnName,
			TargetColumnSchema: rel.TargetColumnSchema,
			TargetColumnTable:  rel.TargetColumnTable,
			TargetColumnName:   rel.TargetColumnName,
			DetectionMethod:    rel.DetectionMethod,
			Confidence:         rel.Confidence,
			Status:             rel.Status,
		})
	}

	response := EntityRelationshipListResponse{
		Relationships: relResponses,
		Total:         len(relResponses),
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *EntityRelationshipHandler) parseProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	projectIDStr := r.PathValue("pid")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return projectID, true
}

func (h *EntityRelationshipHandler) parseDatasourceID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	datasourceIDStr := r.PathValue("dsid")
	datasourceID, err := uuid.Parse(datasourceIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_datasource_id", "Invalid datasource ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return datasourceID, true
}
