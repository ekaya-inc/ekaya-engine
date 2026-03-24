package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// OntologyExportHandler handles ontology bundle export requests.
type OntologyExportHandler struct {
	exportService services.OntologyExportService
	logger        *zap.Logger
}

// NewOntologyExportHandler creates a new ontology export handler.
func NewOntologyExportHandler(exportService services.OntologyExportService, logger *zap.Logger) *OntologyExportHandler {
	return &OntologyExportHandler{
		exportService: exportService,
		logger:        logger,
	}
}

// RegisterRoutes registers ontology export routes.
func (h *OntologyExportHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/datasources/{dsid}/ontology"

	mux.HandleFunc("GET "+base+"/export",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin, models.RoleData)(tenantMiddleware(h.Export))))
}

// Export handles GET /api/projects/{pid}/datasources/{dsid}/ontology/export.
func (h *OntologyExportHandler) Export(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	bundle, err := h.exportService.BuildBundle(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to build ontology export bundle",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "export_failed", "Failed to export ontology bundle"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	payload, err := h.exportService.MarshalBundle(bundle)
	if err != nil {
		h.logger.Error("Failed to marshal ontology export bundle",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "export_failed", "Failed to serialize ontology bundle"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	filename := h.exportService.SuggestedFilename(bundle)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(payload); err != nil {
		h.logger.Error("Failed to write ontology export bundle", zap.Error(err))
	}
}
