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
// Request/Response Types
// ============================================================================

// StartEntityDiscoveryResponse for POST /entities/discover
type StartEntityDiscoveryResponse struct {
	WorkflowID string `json:"workflow_id"`
	Status     string `json:"status"`
}

// EntityDiscoveryStatusResponse for GET /entities/status
type EntityDiscoveryStatusResponse struct {
	WorkflowID      string                   `json:"workflow_id"`
	Phase           string                   `json:"phase"`
	State           string                   `json:"state"`
	Progress        *models.WorkflowProgress `json:"progress,omitempty"`
	TaskQueue       []models.WorkflowTask    `json:"task_queue,omitempty"`
	EntityCount     int                      `json:"entity_count"`
	OccurrenceCount int                      `json:"occurrence_count"`
}

// ============================================================================
// Handler
// ============================================================================

// EntityDiscoveryHandler handles entity discovery workflow HTTP requests.
type EntityDiscoveryHandler struct {
	discoveryService services.EntityDiscoveryService
	logger           *zap.Logger
}

// NewEntityDiscoveryHandler creates a new entity discovery handler.
func NewEntityDiscoveryHandler(
	discoveryService services.EntityDiscoveryService,
	logger *zap.Logger,
) *EntityDiscoveryHandler {
	return &EntityDiscoveryHandler{
		discoveryService: discoveryService,
		logger:           logger,
	}
}

// RegisterRoutes registers the entity discovery handler's routes on the given mux.
func (h *EntityDiscoveryHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/datasources/{dsid}/entities"

	mux.HandleFunc("POST "+base+"/discover",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.StartDiscovery)))
	mux.HandleFunc("GET "+base+"/status",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetStatus)))
	mux.HandleFunc("POST "+base+"/cancel",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Cancel)))
}

// StartDiscovery handles POST /api/projects/{pid}/datasources/{dsid}/entities/discover
func (h *EntityDiscoveryHandler) StartDiscovery(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	workflow, err := h.discoveryService.StartDiscovery(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to start entity discovery",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "start_discovery_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := StartEntityDiscoveryResponse{
		WorkflowID: workflow.ID.String(),
		Status:     string(workflow.State),
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetStatus handles GET /api/projects/{pid}/datasources/{dsid}/entities/status
func (h *EntityDiscoveryHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	workflow, counts, err := h.discoveryService.GetStatusWithCounts(r.Context(), datasourceID)
	if err != nil {
		h.logger.Error("Failed to get workflow status",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_status_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if workflow == nil {
		if err := ErrorResponse(w, http.StatusNotFound, "workflow_not_found", "No entity discovery workflow found for this datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	entityCount := 0
	occurrenceCount := 0
	if counts != nil {
		entityCount = counts.EntityCount
		occurrenceCount = counts.OccurrenceCount
	}

	response := EntityDiscoveryStatusResponse{
		WorkflowID:      workflow.ID.String(),
		Phase:           string(workflow.Phase),
		State:           string(workflow.State),
		Progress:        workflow.Progress,
		TaskQueue:       workflow.TaskQueue,
		EntityCount:     entityCount,
		OccurrenceCount: occurrenceCount,
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Cancel handles POST /api/projects/{pid}/datasources/{dsid}/entities/cancel
func (h *EntityDiscoveryHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	workflow, err := h.discoveryService.GetStatus(r.Context(), datasourceID)
	if err != nil {
		h.logger.Error("Failed to get workflow",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_workflow_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if workflow == nil {
		if err := ErrorResponse(w, http.StatusNotFound, "workflow_not_found", "No entity discovery workflow found for this datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.discoveryService.Cancel(r.Context(), workflow.ID); err != nil {
		h.logger.Error("Failed to cancel workflow",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "cancel_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, map[string]string{"status": "cancelled"}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

// parseProjectAndDatasourceIDs extracts and validates project and datasource IDs from the request path.
func (h *EntityDiscoveryHandler) parseProjectAndDatasourceIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectIDStr := r.PathValue("pid")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, uuid.Nil, false
	}

	datasourceIDStr := r.PathValue("dsid")
	datasourceID, err := uuid.Parse(datasourceIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_datasource_id", "Invalid datasource ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, uuid.Nil, false
	}

	return projectID, datasourceID, true
}
