package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// StartExtractionRequest for POST /ontology/extract
// Note: datasource_id is no longer required - fetched from project configuration
type StartExtractionRequest struct {
	IncludeAllTables   bool     `json:"include_all_tables,omitempty"`
	SelectedTableIDs   []string `json:"selected_table_ids,omitempty"`
	SkipDataProfiling  bool     `json:"skip_data_profiling,omitempty"`
	ProjectDescription string   `json:"project_description,omitempty"`
}

// WorkflowStatusResponse for GET /ontology/status
// Field names match frontend WorkflowStatusResponse type in ui/src/types/ontology.ts
type WorkflowStatusResponse struct {
	WorkflowID      string                `json:"workflow_id"`
	ProjectID       string                `json:"project_id,omitempty"`
	CurrentPhase    string                `json:"current_phase"`
	CompletedPhases []string              `json:"completed_phases"`
	ConfidenceScore float64               `json:"confidence_score"`
	IterationCount  int                   `json:"iteration_count"`
	IsComplete      bool                  `json:"is_complete"`
	StatusLabel     string                `json:"status_label"`
	StatusType      string                `json:"status_type"`
	CanStartNew     bool                  `json:"can_start_new"`
	HasResult       bool                  `json:"has_result"`
	TaskQueue       []models.WorkflowTask `json:"task_queue,omitempty"`
	TotalTasks      int                   `json:"total_tasks,omitempty"`
	CompletedTasks  int                   `json:"completed_tasks,omitempty"`
	LastError       string                `json:"last_error,omitempty"`
	CreatedAt       *string               `json:"created_at,omitempty"`
	UpdatedAt       *string               `json:"updated_at,omitempty"`
	CompletedAt     *string               `json:"completed_at,omitempty"`
	// New fields for UX improvements
	OntologyReady   bool   `json:"ontology_ready,omitempty"`
	TotalEntities   int    `json:"total_entities,omitempty"`
	CurrentEntity   int    `json:"current_entity,omitempty"`
	ProgressMessage string `json:"progress_message,omitempty"`
}

// OntologyResponse for GET /ontology/result
type OntologyResponse struct {
	ID              string                           `json:"id"`
	ProjectID       string                           `json:"project_id"`
	Version         int                              `json:"version"`
	IsActive        bool                             `json:"is_active"`
	DomainSummary   *models.DomainSummary            `json:"domain_summary,omitempty"`
	EntitySummaries map[string]*models.EntitySummary `json:"entity_summaries,omitempty"`
	ColumnDetails   map[string][]models.ColumnDetail `json:"column_details,omitempty"`
	CreatedAt       string                           `json:"created_at"`
}

// ============================================================================
// Handler
// ============================================================================

// OntologyHandler handles ontology workflow HTTP requests.
type OntologyHandler struct {
	workflowService services.OntologyWorkflowService
	projectService  services.ProjectService
	logger          *zap.Logger
}

// NewOntologyHandler creates a new ontology handler.
func NewOntologyHandler(
	workflowService services.OntologyWorkflowService,
	projectService services.ProjectService,
	logger *zap.Logger,
) *OntologyHandler {
	return &OntologyHandler{
		workflowService: workflowService,
		projectService:  projectService,
		logger:          logger,
	}
}

// RegisterRoutes registers the ontology handler's routes on the given mux.
func (h *OntologyHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/ontology"

	// Workflow management
	mux.HandleFunc("POST "+base+"/extract",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.StartExtraction)))
	mux.HandleFunc("GET "+base+"/workflow",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetWorkflowStatus)))
	mux.HandleFunc("GET "+base+"/workflow/{wfid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetWorkflowByID)))
	mux.HandleFunc("POST "+base+"/cancel",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Cancel)))
	mux.HandleFunc("DELETE "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.DeleteOntology)))

	// Ontology results
	mux.HandleFunc("GET "+base+"/result",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetResult)))
}

// StartExtraction handles POST /api/projects/{pid}/ontology/extract
func (h *OntologyHandler) StartExtraction(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	var req StartExtractionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Fetch datasource ID from project configuration
	dsID, err := h.projectService.GetDefaultDatasourceID(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get project datasource",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get project configuration"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if dsID == uuid.Nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "no_datasource_configured",
			"No datasource configured for this project. Please configure a datasource in project settings first."); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Build workflow config
	config := &models.WorkflowConfig{
		DatasourceID:       dsID,
		IncludeAllTables:   req.IncludeAllTables,
		SkipDataProfiling:  req.SkipDataProfiling,
		ProjectDescription: req.ProjectDescription,
	}

	// Parse selected table IDs
	if len(req.SelectedTableIDs) > 0 {
		config.SelectedTableIDs = make([]uuid.UUID, 0, len(req.SelectedTableIDs))
		for _, idStr := range req.SelectedTableIDs {
			tableID, err := uuid.Parse(idStr)
			if err != nil {
				if err := ErrorResponse(w, http.StatusBadRequest, "invalid_table_id", "Invalid table ID format: "+idStr); err != nil {
					h.logger.Error("Failed to write error response", zap.Error(err))
				}
				return
			}
			config.SelectedTableIDs = append(config.SelectedTableIDs, tableID)
		}
	}

	workflow, err := h.workflowService.StartExtraction(r.Context(), projectID, config)
	if err != nil {
		h.logger.Error("Failed to start extraction",
			zap.String("project_id", projectID.String()),
			zap.Error(err))

		// Map service errors to appropriate HTTP status codes
		statusCode := http.StatusInternalServerError
		errorCode := "extraction_failed"
		errorMsg := err.Error()

		if strings.Contains(err.Error(), "relationships phase must complete") {
			statusCode = http.StatusBadRequest
			errorCode = "relationships_not_complete"
			errorMsg = "Relationship detection must be completed before starting ontology extraction. Please run relationship detection first."
		}

		if err := ErrorResponse(w, statusCode, errorCode, errorMsg); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toWorkflowResponse(workflow)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetWorkflowStatus handles GET /api/projects/{pid}/ontology/workflow
func (h *OntologyHandler) GetWorkflowStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	workflow, err := h.workflowService.GetStatus(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get workflow status",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get workflow status"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if workflow == nil {
		// No workflow - check if ontology data exists
		ontology, _ := h.workflowService.GetOntology(r.Context(), projectID)
		hasOntologyData := ontology != nil && len(ontology.EntitySummaries) > 0

		currentEntity := 0
		if ontology != nil {
			currentEntity = len(ontology.EntitySummaries)
		}

		// Get total entity count from schema (1 global + tables + columns)
		totalEntities, err := h.workflowService.GetOntologyEntityCount(r.Context(), projectID)
		if err != nil {
			h.logger.Warn("Failed to get schema entity count", zap.Error(err))
			totalEntities = currentEntity // Fallback to current count
		}

		response := ApiResponse{Success: true, Data: WorkflowStatusResponse{
			WorkflowID:      "",
			CurrentPhase:    "",
			CompletedPhases: []string{},
			StatusType:      "info",
			StatusLabel:     "Ready",
			CanStartNew:     true,
			HasResult:       hasOntologyData,
			OntologyReady:   hasOntologyData,
			TotalEntities:   totalEntities,
			CurrentEntity:   currentEntity,
		}}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	// Get ontology to calculate entity progress from entity states
	ontology, _ := h.workflowService.GetOntology(r.Context(), projectID)

	response := ApiResponse{Success: true, Data: h.toWorkflowResponseWithOntology(workflow, ontology)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetWorkflowByID handles GET /api/projects/{pid}/ontology/workflow/{wfid}
func (h *OntologyHandler) GetWorkflowByID(w http.ResponseWriter, r *http.Request) {
	_, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	wfidStr := r.PathValue("wfid")
	workflowID, err := uuid.Parse(wfidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_workflow_id", "Invalid workflow ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	workflow, err := h.workflowService.GetByID(r.Context(), workflowID)
	if err != nil {
		h.logger.Error("Failed to get workflow",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get workflow"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if workflow == nil {
		if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Workflow not found"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toWorkflowResponse(workflow)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Cancel handles POST /api/projects/{pid}/ontology/cancel
func (h *OntologyHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	workflow, err := h.workflowService.GetStatus(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get workflow",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get workflow"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if workflow == nil {
		if err := ErrorResponse(w, http.StatusNotFound, "not_found", "No active workflow"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.workflowService.Cancel(r.Context(), workflow.ID); err != nil {
		h.logger.Error("Failed to cancel workflow",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "cancel_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"message": "Workflow cancelled"}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// DeleteOntology handles DELETE /api/projects/{pid}/ontology
func (h *OntologyHandler) DeleteOntology(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	if err := h.workflowService.DeleteOntology(r.Context(), projectID); err != nil {
		h.logger.Error("Failed to delete ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"message": "Ontology deleted"}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetResult handles GET /api/projects/{pid}/ontology/result
func (h *OntologyHandler) GetResult(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.parseProjectID(w, r)
	if !ok {
		return
	}

	ontology, err := h.workflowService.GetOntology(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get ontology"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if ontology == nil {
		response := ApiResponse{Success: true, Data: nil}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toOntologyResponse(ontology)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *OntologyHandler) parseProjectID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return projectID, true
}

func (h *OntologyHandler) toWorkflowResponse(w *models.OntologyWorkflow) WorkflowStatusResponse {
	// Derive current phase and progress fields with nil check
	currentPhase := ""
	progressMessage := ""
	totalEntities := 0
	currentEntity := 0
	ontologyReady := false
	if w.Progress != nil {
		currentPhase = w.Progress.CurrentPhase
		progressMessage = w.Progress.Message
		totalEntities = w.Progress.Total
		currentEntity = w.Progress.Current
		ontologyReady = w.Progress.OntologyReady
	}

	// Count tasks
	totalTasks := len(w.TaskQueue)
	completedTasks := 0
	for _, t := range w.TaskQueue {
		if t.Status == models.TaskStatusComplete {
			completedTasks++
		}
	}

	// Derive status fields from state
	isComplete := w.State == models.WorkflowStateCompleted || w.State == models.WorkflowStateFailed
	statusType, statusLabel := deriveStatusFromState(w.State)
	canStartNew := w.State.IsTerminal()
	hasResult := w.State == models.WorkflowStateCompleted

	resp := WorkflowStatusResponse{
		WorkflowID:      w.ID.String(),
		ProjectID:       w.ProjectID.String(),
		CurrentPhase:    currentPhase,
		CompletedPhases: []string{},
		ConfidenceScore: 0,
		IterationCount:  0,
		IsComplete:      isComplete,
		StatusLabel:     statusLabel,
		StatusType:      statusType,
		CanStartNew:     canStartNew,
		HasResult:       hasResult,
		TaskQueue:       w.TaskQueue,
		TotalTasks:      totalTasks,
		CompletedTasks:  completedTasks,
		LastError:       w.ErrorMessage,
		// New fields for UX improvements
		OntologyReady:   ontologyReady,
		TotalEntities:   totalEntities,
		CurrentEntity:   currentEntity,
		ProgressMessage: progressMessage,
	}

	// Format timestamps
	if !w.CreatedAt.IsZero() {
		created := w.CreatedAt.Format(time.RFC3339)
		resp.CreatedAt = &created
	}
	if !w.UpdatedAt.IsZero() {
		updated := w.UpdatedAt.Format(time.RFC3339)
		resp.UpdatedAt = &updated
	}
	if w.CompletedAt != nil {
		completed := w.CompletedAt.Format(time.RFC3339)
		resp.CompletedAt = &completed
	}

	return resp
}

// toWorkflowResponseWithOntology is like toWorkflowResponse but uses workflow progress directly.
// Entity counts (global + tables + columns) are now tracked by the orchestrator, not derived from ontology.
func (h *OntologyHandler) toWorkflowResponseWithOntology(w *models.OntologyWorkflow, ontology *models.TieredOntology) WorkflowStatusResponse {
	return h.toWorkflowResponse(w)
}

// deriveStatusFromState converts workflow state to UI-friendly status type and label.
func deriveStatusFromState(state models.WorkflowState) (statusType, statusLabel string) {
	switch state {
	case models.WorkflowStatePending:
		return "processing", "Initializing"
	case models.WorkflowStateRunning:
		return "processing", "Building Ontology"
	case models.WorkflowStatePaused:
		return "warning", "Paused"
	case models.WorkflowStateAwaitingInput:
		return "info", "Awaiting Input"
	case models.WorkflowStateCompleted:
		return "success", "Completed"
	case models.WorkflowStateFailed:
		return "error", "Failed"
	default:
		return "info", "Unknown"
	}
}

func (h *OntologyHandler) toOntologyResponse(o *models.TieredOntology) OntologyResponse {
	return OntologyResponse{
		ID:              o.ID.String(),
		ProjectID:       o.ProjectID.String(),
		Version:         o.Version,
		IsActive:        o.IsActive,
		DomainSummary:   o.DomainSummary,
		EntitySummaries: o.EntitySummaries,
		ColumnDetails:   o.ColumnDetails,
		CreatedAt:       o.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
