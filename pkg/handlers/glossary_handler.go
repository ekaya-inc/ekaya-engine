package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// GlossaryListResponse for GET /glossary
type GlossaryListResponse struct {
	Terms []*models.BusinessGlossaryTerm `json:"terms"`
	Total int                            `json:"total"`
}

// CreateGlossaryTermRequest for POST /glossary
type CreateGlossaryTermRequest struct {
	Term        string   `json:"term"`
	Definition  string   `json:"definition"`
	DefiningSQL string   `json:"defining_sql"`
	BaseTable   string   `json:"base_table,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	Source      string   `json:"source,omitempty"`
}

// UpdateGlossaryTermRequest for PUT /glossary/{termId}
type UpdateGlossaryTermRequest struct {
	Term        string   `json:"term"`
	Definition  string   `json:"definition"`
	DefiningSQL string   `json:"defining_sql"`
	BaseTable   string   `json:"base_table,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
}

// TestSQLRequest for POST /glossary/test-sql
type TestSQLRequest struct {
	SQL string `json:"sql"`
}

// TestSQLResponse for POST /glossary/test-sql
type TestSQLResponse struct {
	Valid         bool                   `json:"valid"`
	Error         string                 `json:"error,omitempty"`
	OutputColumns []models.OutputColumn  `json:"output_columns,omitempty"`
	SampleRow     map[string]interface{} `json:"sample_row,omitempty"`
}

// ============================================================================
// Handler
// ============================================================================

// GlossaryHandler handles business glossary HTTP requests.
type GlossaryHandler struct {
	glossaryService services.GlossaryService
	logger          *zap.Logger
}

// NewGlossaryHandler creates a new glossary handler.
func NewGlossaryHandler(
	glossaryService services.GlossaryService,
	logger *zap.Logger,
) *GlossaryHandler {
	return &GlossaryHandler{
		glossaryService: glossaryService,
		logger:          logger,
	}
}

// RegisterRoutes registers the glossary handler's routes on the given mux.
func (h *GlossaryHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/glossary"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
	mux.HandleFunc("POST "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Create)))
	mux.HandleFunc("GET "+base+"/{tid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("PUT "+base+"/{tid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Update)))
	mux.HandleFunc("DELETE "+base+"/{tid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Delete)))
	mux.HandleFunc("POST "+base+"/suggest",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Suggest)))
	mux.HandleFunc("POST "+base+"/test-sql",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.TestSQL)))
}

// List handles GET /api/projects/{pid}/glossary
func (h *GlossaryHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	terms, err := h.glossaryService.GetTerms(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list glossary terms",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_glossary_terms_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := GlossaryListResponse{
		Terms: terms,
		Total: len(terms),
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Create handles POST /api/projects/{pid}/glossary
func (h *GlossaryHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req CreateGlossaryTermRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	term := &models.BusinessGlossaryTerm{
		Term:        req.Term,
		Definition:  req.Definition,
		DefiningSQL: req.DefiningSQL,
		BaseTable:   req.BaseTable,
		Aliases:     req.Aliases,
		Source:      req.Source,
	}

	if err := h.glossaryService.CreateTerm(r.Context(), projectID, term); err != nil {
		h.logger.Error("Failed to create glossary term",
			zap.String("project_id", projectID.String()),
			zap.String("term", req.Term),
			zap.Error(err))

		// Check for validation errors
		errMsg := err.Error()
		if errMsg == "term name is required" || errMsg == "term definition is required" || errMsg == "defining_sql is required" || strings.Contains(errMsg, "SQL validation failed") {
			if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", errMsg); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		if err := ErrorResponse(w, http.StatusInternalServerError, "create_glossary_term_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusCreated, ApiResponse{Success: true, Data: term}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Get handles GET /api/projects/{pid}/glossary/{tid}
func (h *GlossaryHandler) Get(w http.ResponseWriter, r *http.Request) {
	_, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	termID, ok := ParseTermID(w, r, h.logger)
	if !ok {
		return
	}

	term, err := h.glossaryService.GetTerm(r.Context(), termID)
	if err != nil {
		h.logger.Error("Failed to get glossary term",
			zap.String("term_id", termID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_glossary_term_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if term == nil {
		if err := ErrorResponse(w, http.StatusNotFound, "term_not_found", "Glossary term not found"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: term}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Update handles PUT /api/projects/{pid}/glossary/{tid}
func (h *GlossaryHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	termID, ok := ParseTermID(w, r, h.logger)
	if !ok {
		return
	}

	var req UpdateGlossaryTermRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	term := &models.BusinessGlossaryTerm{
		ID:          termID,
		ProjectID:   projectID,
		Term:        req.Term,
		Definition:  req.Definition,
		DefiningSQL: req.DefiningSQL,
		BaseTable:   req.BaseTable,
		Aliases:     req.Aliases,
		Source:      models.GlossarySourceManual, // Updates via UI are marked as manual
	}

	if err := h.glossaryService.UpdateTerm(r.Context(), term); err != nil {
		h.logger.Error("Failed to update glossary term",
			zap.String("term_id", termID.String()),
			zap.Error(err))

		// Check for validation errors
		errMsg := err.Error()
		if errMsg == "term name is required" || errMsg == "term definition is required" || errMsg == "defining_sql is required" || strings.Contains(errMsg, "SQL validation failed") {
			if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", errMsg); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		// Check for not found
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "term_not_found", "Glossary term not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		if err := ErrorResponse(w, http.StatusInternalServerError, "update_glossary_term_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Fetch the updated term to return it
	updatedTerm, err := h.glossaryService.GetTerm(r.Context(), termID)
	if err != nil {
		h.logger.Error("Failed to get updated glossary term",
			zap.String("term_id", termID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_glossary_term_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: updatedTerm}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/glossary/{tid}
func (h *GlossaryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	termID, ok := ParseTermID(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.glossaryService.DeleteTerm(r.Context(), termID); err != nil {
		h.logger.Error("Failed to delete glossary term",
			zap.String("term_id", termID.String()),
			zap.Error(err))

		// Check for not found
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "term_not_found", "Glossary term not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_glossary_term_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: map[string]string{"status": "deleted"}}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Suggest handles POST /api/projects/{pid}/glossary/suggest
func (h *GlossaryHandler) Suggest(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	suggestions, err := h.glossaryService.SuggestTerms(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to suggest glossary terms",
			zap.String("project_id", projectID.String()),
			zap.Error(err))

		// Check for no ontology error
		if err.Error() == "no active ontology found for project" {
			if err := ErrorResponse(w, http.StatusBadRequest, "no_ontology", "No active ontology found. Run ontology extraction first."); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		if err := ErrorResponse(w, http.StatusInternalServerError, "suggest_glossary_terms_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := GlossaryListResponse{
		Terms: suggestions,
		Total: len(suggestions),
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// TestSQL handles POST /api/projects/{pid}/glossary/test-sql
func (h *GlossaryHandler) TestSQL(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req TestSQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.SQL == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "validation_error", "SQL is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	testResult, err := h.glossaryService.TestSQL(r.Context(), projectID, req.SQL)
	if err != nil {
		h.logger.Error("Failed to test SQL",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "test_sql_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := TestSQLResponse{
		Valid:         testResult.Valid,
		Error:         testResult.Error,
		OutputColumns: testResult.OutputColumns,
		SampleRow:     testResult.SampleRow,
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
