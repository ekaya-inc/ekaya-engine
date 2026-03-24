package handlers

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// OntologyImportHandler handles ontology bundle import requests.
type OntologyImportHandler struct {
	importService services.OntologyImportService
	logger        *zap.Logger
}

// NewOntologyImportHandler creates a new ontology import handler.
func NewOntologyImportHandler(importService services.OntologyImportService, logger *zap.Logger) *OntologyImportHandler {
	return &OntologyImportHandler{
		importService: importService,
		logger:        logger,
	}
}

// RegisterRoutes registers ontology import routes.
func (h *OntologyImportHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/datasources/{dsid}/ontology"

	mux.HandleFunc("POST "+base+"/import",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Import))))
}

// Import handles POST /api/projects/{pid}/datasources/{dsid}/ontology/import.
func (h *OntologyImportHandler) Import(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, models.OntologyImportMaxBytes)
	if err := r.ParseMultipartForm(models.OntologyImportMaxBytes); err != nil {
		if err := WriteJSON(w, http.StatusBadRequest, ApiResponse{
			Success: false,
			Error:   "file_too_large",
			Message: "Ontology bundle exceeds the 5 MB maximum size",
			Data: models.OntologyImportValidationReport{
				Problems: []models.OntologyImportProblem{{
					Code:    "file_too_large",
					Message: "Ontology bundle exceeds the 5 MB maximum size.",
				}},
			},
		}); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		if err := WriteJSON(w, http.StatusBadRequest, ApiResponse{
			Success: false,
			Error:   "invalid_file",
			Message: "No ontology bundle file was provided",
			Data: models.OntologyImportValidationReport{
				Problems: []models.OntologyImportProblem{{
					Code:    "invalid_file",
					Message: "Select a local .json ontology bundle to import.",
				}},
			},
		}); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".json") {
		if err := WriteJSON(w, http.StatusBadRequest, ApiResponse{
			Success: false,
			Error:   "invalid_file",
			Message: "Ontology bundle must be a .json file",
			Data: models.OntologyImportValidationReport{
				Problems: []models.OntologyImportProblem{{
					Code:    "invalid_file",
					Message: "Ontology bundle must use the .json file extension.",
				}},
			},
		}); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	payload, err := io.ReadAll(file)
	if err != nil {
		if err := WriteJSON(w, http.StatusBadRequest, ApiResponse{
			Success: false,
			Error:   "read_error",
			Message: "Failed to read the ontology bundle file",
		}); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	result, err := h.importService.ImportBundle(r.Context(), projectID, datasourceID, payload)
	if err != nil {
		var validationErr *services.OntologyImportValidationError
		if errors.As(err, &validationErr) {
			if err := WriteJSON(w, validationErr.StatusCode, ApiResponse{
				Success: false,
				Error:   validationErr.Code,
				Message: validationErr.Message,
				Data:    validationErr.Report,
			}); err != nil {
				h.logger.Error("Failed to write validation response", zap.Error(err))
			}
			return
		}

		h.logger.Error("Failed to import ontology bundle",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := WriteJSON(w, http.StatusInternalServerError, ApiResponse{
			Success: false,
			Error:   "ontology_import_failed",
			Message: "Failed to import ontology bundle",
		}); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data:    result,
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
