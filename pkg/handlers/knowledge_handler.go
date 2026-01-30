package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// ProjectKnowledgeListResponse for GET /project-knowledge
type ProjectKnowledgeListResponse struct {
	Facts []*models.KnowledgeFact `json:"facts"`
	Total int                     `json:"total"`
}

// CreateKnowledgeRequest for POST /project-knowledge
type CreateKnowledgeRequest struct {
	FactType string `json:"fact_type"`
	Value    string `json:"value"`
	Context  string `json:"context,omitempty"`
}

// UpdateKnowledgeRequest for PUT /project-knowledge/{id}
type UpdateKnowledgeRequest struct {
	FactType string `json:"fact_type"`
	Value    string `json:"value"`
	Context  string `json:"context,omitempty"`
}

// ProjectOverviewResponse for GET /project-knowledge/overview
type ProjectOverviewResponse struct {
	Overview *string `json:"overview"`
}

// ParseKnowledgeRequest for POST /project-knowledge/parse
type ParseKnowledgeRequest struct {
	Text string `json:"text"`
}

// ParseKnowledgeResponse for POST /project-knowledge/parse
type ParseKnowledgeResponse struct {
	Facts []*models.KnowledgeFact `json:"facts"`
}

// ============================================================================
// Handler
// ============================================================================

// KnowledgeHandler handles project knowledge HTTP requests.
type KnowledgeHandler struct {
	knowledgeService        services.KnowledgeService
	knowledgeParsingService services.KnowledgeParsingService
	logger                  *zap.Logger
}

// NewKnowledgeHandler creates a new knowledge handler.
func NewKnowledgeHandler(
	knowledgeService services.KnowledgeService,
	knowledgeParsingService services.KnowledgeParsingService,
	logger *zap.Logger,
) *KnowledgeHandler {
	return &KnowledgeHandler{
		knowledgeService:        knowledgeService,
		knowledgeParsingService: knowledgeParsingService,
		logger:                  logger,
	}
}

// RegisterRoutes registers the knowledge handler's routes on the given mux.
func (h *KnowledgeHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/project-knowledge"

	// Read-only endpoints - no provenance needed
	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
	mux.HandleFunc("GET "+base+"/overview",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetOverview)))

	// Write endpoints - require provenance for audit tracking
	mux.HandleFunc("POST "+base,
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.Create)))
	mux.HandleFunc("POST "+base+"/parse",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.Parse)))
	mux.HandleFunc("PUT "+base+"/{kid}",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.Update)))
	mux.HandleFunc("DELETE "+base+"/{kid}",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.Delete)))
	mux.HandleFunc("DELETE "+base,
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.DeleteAll)))
}

// List handles GET /api/projects/{pid}/project-knowledge
func (h *KnowledgeHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	facts, err := h.knowledgeService.GetAll(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list knowledge facts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_knowledge_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ProjectKnowledgeListResponse{
		Facts: facts,
		Total: len(facts),
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Create handles POST /api/projects/{pid}/project-knowledge
func (h *KnowledgeHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req CreateKnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate required fields
	if req.FactType == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", "fact_type is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}
	if req.Value == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", "value is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	fact, err := h.knowledgeService.Store(r.Context(), projectID, req.FactType, req.Value, req.Context)
	if err != nil {
		h.logger.Error("Failed to create knowledge fact",
			zap.String("project_id", projectID.String()),
			zap.String("fact_type", req.FactType),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "create_knowledge_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusCreated, ApiResponse{Success: true, Data: fact}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Update handles PUT /api/projects/{pid}/project-knowledge/{kid}
func (h *KnowledgeHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	knowledgeID, ok := ParseKnowledgeID(w, r, h.logger)
	if !ok {
		return
	}

	var req UpdateKnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate required fields
	if req.FactType == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", "fact_type is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}
	if req.Value == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", "value is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	fact, err := h.knowledgeService.Update(r.Context(), projectID, knowledgeID, req.FactType, req.Value, req.Context)
	if err != nil {
		h.logger.Error("Failed to update knowledge fact",
			zap.String("project_id", projectID.String()),
			zap.String("knowledge_id", knowledgeID.String()),
			zap.Error(err))

		// Check for not found
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "fact_not_found", "Knowledge fact not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		if err := ErrorResponse(w, http.StatusInternalServerError, "update_knowledge_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: fact}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/project-knowledge/{kid}
func (h *KnowledgeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	knowledgeID, ok := ParseKnowledgeID(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.knowledgeService.Delete(r.Context(), knowledgeID); err != nil {
		h.logger.Error("Failed to delete knowledge fact",
			zap.String("knowledge_id", knowledgeID.String()),
			zap.Error(err))

		// Check for not found
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "fact_not_found", "Knowledge fact not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_knowledge_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: map[string]string{"status": "deleted"}}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// DeleteAll handles DELETE /api/projects/{pid}/project-knowledge
func (h *KnowledgeHandler) DeleteAll(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.knowledgeService.DeleteAll(r.Context(), projectID); err != nil {
		h.logger.Error("Failed to delete all knowledge facts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_all_knowledge_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: map[string]string{"status": "deleted"}}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetOverview handles GET /api/projects/{pid}/project-knowledge/overview
func (h *KnowledgeHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	facts, err := h.knowledgeService.GetByType(r.Context(), projectID, "project_overview")
	if err != nil {
		h.logger.Error("Failed to get project overview",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_overview_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var overview *string
	if len(facts) > 0 {
		overview = &facts[0].Value
	}

	response := ProjectOverviewResponse{
		Overview: overview,
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Parse handles POST /api/projects/{pid}/project-knowledge/parse
// It takes a free-form text input and uses LLM to extract structured knowledge facts.
func (h *KnowledgeHandler) Parse(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req ParseKnowledgeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate required fields
	if req.Text == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", "text is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	facts, err := h.knowledgeParsingService.ParseAndStore(r.Context(), projectID, req.Text)
	if err != nil {
		h.logger.Error("Failed to parse knowledge fact",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "parse_knowledge_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ParseKnowledgeResponse{
		Facts: facts,
	}

	if err := WriteJSON(w, http.StatusCreated, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
