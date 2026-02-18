package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// DAGStatusResponse represents the DAG status for UI polling.
// Matches the response structure defined in PLAN-ontology-workflow-dag.md Section 6.
type DAGStatusResponse struct {
	DAGID       string            `json:"dag_id"`
	Status      string            `json:"status"`
	CurrentNode *string           `json:"current_node,omitempty"`
	Nodes       []DAGNodeResponse `json:"nodes"`
	StartedAt   *string           `json:"started_at,omitempty"`
	CompletedAt *string           `json:"completed_at,omitempty"`
}

// DAGNodeResponse represents a single node within the DAG.
type DAGNodeResponse struct {
	Name         string               `json:"name"`
	Status       string               `json:"status"`
	Progress     *DAGProgressResponse `json:"progress,omitempty"`
	ErrorMessage *string              `json:"error,omitempty"`
}

// DAGProgressResponse represents progress within a node.
type DAGProgressResponse struct {
	Current int    `json:"current"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
}

// StartExtractionRequest is the request body for starting ontology extraction.
type StartExtractionRequest struct {
	ProjectOverview string `json:"project_overview"`
}

// ============================================================================
// Handler
// ============================================================================

// OntologyDAGHandler handles ontology DAG workflow HTTP requests.
type OntologyDAGHandler struct {
	dagService     services.OntologyDAGService
	projectService services.ProjectService
	logger         *zap.Logger
}

// NewOntologyDAGHandler creates a new ontology DAG handler.
func NewOntologyDAGHandler(
	dagService services.OntologyDAGService,
	projectService services.ProjectService,
	logger *zap.Logger,
) *OntologyDAGHandler {
	return &OntologyDAGHandler{
		dagService:     dagService,
		projectService: projectService,
		logger:         logger,
	}
}

// RegisterRoutes registers the ontology DAG handler's routes on the given mux.
func (h *OntologyDAGHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/datasources/{dsid}/ontology"

	// Start/Refresh extraction - triggers DAG execution
	mux.HandleFunc("POST "+base+"/extract",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin, models.RoleData)(tenantMiddleware(h.StartExtraction))))

	// Get DAG status - for UI polling
	mux.HandleFunc("GET "+base+"/dag",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetStatus)))

	// Cancel running DAG
	mux.HandleFunc("POST "+base+"/dag/cancel",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin, models.RoleData)(tenantMiddleware(h.Cancel))))

	// Delete all ontology data for project
	mux.HandleFunc("DELETE "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin, models.RoleData)(tenantMiddleware(h.Delete))))
}

// StartExtraction handles POST /api/projects/{pid}/datasources/{dsid}/ontology/extract
// This initiates a new DAG execution or returns an existing active DAG.
func (h *OntologyDAGHandler) StartExtraction(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	// Parse request body for project overview
	var req StartExtractionRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			h.logger.Warn("Failed to parse request body, continuing without overview",
				zap.Error(err))
		}
	}

	dag, err := h.dagService.Start(r.Context(), projectID, datasourceID, req.ProjectOverview)
	if err != nil {
		h.logger.Error("Failed to start ontology DAG",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "start_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toDAGResponse(dag)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetStatus handles GET /api/projects/{pid}/datasources/{dsid}/ontology/dag
// Returns the current DAG status with all node states for UI polling.
func (h *OntologyDAGHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	dag, err := h.dagService.GetStatus(r.Context(), datasourceID)
	if err != nil {
		h.logger.Error("Failed to get DAG status",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_status_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if dag == nil {
		// No DAG exists yet - return empty response indicating ready to start
		response := ApiResponse{Success: true, Data: nil}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toDAGResponse(dag)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Cancel handles POST /api/projects/{pid}/datasources/{dsid}/ontology/dag/cancel
// Cancels a running DAG.
func (h *OntologyDAGHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	dag, err := h.dagService.GetStatus(r.Context(), datasourceID)
	if err != nil {
		h.logger.Error("Failed to get DAG for cancellation",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_dag_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if dag == nil {
		if err := ErrorResponse(w, http.StatusNotFound, "dag_not_found", "No DAG found for this datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if dag.Status.IsTerminal() {
		if err := ErrorResponse(w, http.StatusBadRequest, "dag_not_running", "DAG is not running"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.dagService.Cancel(r.Context(), dag.ID); err != nil {
		h.logger.Error("Failed to cancel DAG",
			zap.String("dag_id", dag.ID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "cancel_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"status": "cancelled"}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/datasources/{dsid}/ontology
// Deletes all ontology data for the project.
func (h *OntologyDAGHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.dagService.Delete(r.Context(), projectID); err != nil {
		h.logger.Error("Failed to delete ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"message": "Ontology deleted successfully"}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

// toDAGResponse converts a models.OntologyDAG to a DAGStatusResponse.
func (h *OntologyDAGHandler) toDAGResponse(dag *models.OntologyDAG) DAGStatusResponse {
	nodes := make([]DAGNodeResponse, len(dag.Nodes))
	for i, node := range dag.Nodes {
		nodes[i] = h.toNodeResponse(&node)
	}

	resp := DAGStatusResponse{
		DAGID:       dag.ID.String(),
		Status:      string(dag.Status),
		CurrentNode: dag.CurrentNode,
		Nodes:       nodes,
	}

	if dag.StartedAt != nil {
		startedAt := dag.StartedAt.Format(time.RFC3339)
		resp.StartedAt = &startedAt
	}

	if dag.CompletedAt != nil {
		completedAt := dag.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &completedAt
	}

	return resp
}

// toNodeResponse converts a models.DAGNode to a DAGNodeResponse.
func (h *OntologyDAGHandler) toNodeResponse(node *models.DAGNode) DAGNodeResponse {
	resp := DAGNodeResponse{
		Name:         node.NodeName,
		Status:       string(node.Status),
		ErrorMessage: node.ErrorMessage,
	}

	if node.Progress != nil {
		resp.Progress = &DAGProgressResponse{
			Current: node.Progress.Current,
			Total:   node.Progress.Total,
			Message: node.Progress.Message,
		}
	}

	return resp
}
