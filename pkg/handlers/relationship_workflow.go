package handlers

import (
	"encoding/json"
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

// StartDetectionResponse for POST /relationships/detect
type StartDetectionResponse struct {
	WorkflowID string `json:"workflow_id"`
	Status     string `json:"status"`
}

// RelationshipWorkflowStatusResponse for GET /relationships/status
type RelationshipWorkflowStatusResponse struct {
	WorkflowID string                   `json:"workflow_id"`
	Phase      string                   `json:"phase"`
	State      string                   `json:"state"`
	Progress   *models.WorkflowProgress `json:"progress,omitempty"`
	TaskQueue  []models.WorkflowTask    `json:"task_queue,omitempty"`
	// Legacy candidate counts (deprecated, will be removed)
	ConfirmedCount   int `json:"confirmed_count"`
	NeedsReviewCount int `json:"needs_review_count"`
	RejectedCount    int `json:"rejected_count"`
	// New entity counts
	EntityCount     int  `json:"entity_count"`
	OccurrenceCount int  `json:"occurrence_count"`
	IslandCount     int  `json:"island_count"`
	CanSave         bool `json:"can_save"`
}

// CandidatesResponse for GET /relationships/candidates
type CandidatesResponse struct {
	Confirmed   []CandidateResponse `json:"confirmed"`
	NeedsReview []CandidateResponse `json:"needs_review"`
	Rejected    []CandidateResponse `json:"rejected"`
}

// CandidateResponse represents a single relationship candidate.
type CandidateResponse struct {
	ID              string  `json:"id"`
	SourceTable     string  `json:"source_table"`
	SourceColumn    string  `json:"source_column"`
	TargetTable     string  `json:"target_table"`
	TargetColumn    string  `json:"target_column"`
	Confidence      float64 `json:"confidence"`
	DetectionMethod string  `json:"detection_method"`
	LLMReasoning    *string `json:"llm_reasoning,omitempty"`
	Cardinality     *string `json:"cardinality,omitempty"`
	Status          string  `json:"status"`
	IsRequired      bool    `json:"is_required"`
}

// CandidateDecisionRequest for PUT /relationships/candidates/{cid}
type CandidateDecisionRequest struct {
	Decision string `json:"decision"` // "accepted" or "rejected"
}

// EntityOccurrenceResponse represents a single occurrence of an entity in a table/column.
type EntityOccurrenceResponse struct {
	ID         string  `json:"id"`
	SchemaName string  `json:"schema_name"`
	TableName  string  `json:"table_name"`
	ColumnName string  `json:"column_name"`
	Role       *string `json:"role,omitempty"`
	Confidence float64 `json:"confidence"`
}

// EntityResponse represents a discovered entity with its occurrences.
type EntityResponse struct {
	ID            string                     `json:"id"`
	Name          string                     `json:"name"`
	Description   string                     `json:"description"`
	PrimarySchema string                     `json:"primary_schema"`
	PrimaryTable  string                     `json:"primary_table"`
	PrimaryColumn string                     `json:"primary_column"`
	Occurrences   []EntityOccurrenceResponse `json:"occurrences"`
}

// EntitiesResponse for GET /relationships/entities
type EntitiesResponse struct {
	Entities []EntityResponse `json:"entities"`
}

// SaveRelationshipsResponse for POST /relationships/save
type SaveRelationshipsResponse struct {
	SavedCount int `json:"saved_count"`
}

// ============================================================================
// Handler
// ============================================================================

// RelationshipWorkflowHandler handles relationship workflow HTTP requests.
type RelationshipWorkflowHandler struct {
	workflowService services.RelationshipWorkflowService
	logger          *zap.Logger
}

// NewRelationshipWorkflowHandler creates a new relationship workflow handler.
func NewRelationshipWorkflowHandler(
	workflowService services.RelationshipWorkflowService,
	logger *zap.Logger,
) *RelationshipWorkflowHandler {
	return &RelationshipWorkflowHandler{
		workflowService: workflowService,
		logger:          logger,
	}
}

// RegisterRoutes registers the relationship workflow handler's routes on the given mux.
func (h *RelationshipWorkflowHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/datasources/{dsid}/relationships"

	mux.HandleFunc("POST "+base+"/detect",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.StartDetection)))
	mux.HandleFunc("GET "+base+"/status",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetStatus)))
	mux.HandleFunc("GET "+base+"/candidates",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetCandidates)))
	mux.HandleFunc("GET "+base+"/entities",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetEntities)))
	mux.HandleFunc("PUT "+base+"/candidates/{cid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.UpdateCandidate)))
	mux.HandleFunc("POST "+base+"/cancel",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Cancel)))
	mux.HandleFunc("POST "+base+"/save",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Save)))
}

// StartDetection handles POST /api/projects/{pid}/datasources/{dsid}/relationships/detect
func (h *RelationshipWorkflowHandler) StartDetection(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	workflow, err := h.workflowService.StartDetection(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to start relationship detection",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "start_detection_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := StartDetectionResponse{
		WorkflowID: workflow.ID.String(),
		Status:     string(workflow.State),
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetStatus handles GET /api/projects/{pid}/datasources/{dsid}/relationships/status
func (h *RelationshipWorkflowHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	workflow, counts, err := h.workflowService.GetStatusWithCounts(r.Context(), datasourceID)
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
		if err := ErrorResponse(w, http.StatusNotFound, "workflow_not_found", "No relationship workflow found for this datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := RelationshipWorkflowStatusResponse{
		WorkflowID:       workflow.ID.String(),
		Phase:            string(workflow.Phase),
		State:            string(workflow.State),
		Progress:         workflow.Progress,
		TaskQueue:        workflow.TaskQueue,
		ConfirmedCount:   counts.Confirmed,
		NeedsReviewCount: counts.NeedsReview,
		RejectedCount:    counts.Rejected,
		EntityCount:      counts.EntityCount,
		OccurrenceCount:  counts.OccurrenceCount,
		IslandCount:      counts.IslandCount,
		CanSave:          counts.CanSave,
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetCandidates handles GET /api/projects/{pid}/datasources/{dsid}/relationships/candidates
func (h *RelationshipWorkflowHandler) GetCandidates(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	grouped, err := h.workflowService.GetCandidatesGrouped(r.Context(), datasourceID)
	if err != nil {
		h.logger.Error("Failed to get candidates",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_candidates_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert to response format
	confirmed := make([]CandidateResponse, 0, len(grouped.Confirmed))
	for _, c := range grouped.Confirmed {
		confirmed = append(confirmed, h.toCandidateResponse(c))
	}

	needsReview := make([]CandidateResponse, 0, len(grouped.NeedsReview))
	for _, c := range grouped.NeedsReview {
		needsReview = append(needsReview, h.toCandidateResponse(c))
	}

	rejected := make([]CandidateResponse, 0, len(grouped.Rejected))
	for _, c := range grouped.Rejected {
		rejected = append(rejected, h.toCandidateResponse(c))
	}

	response := CandidatesResponse{
		Confirmed:   confirmed,
		NeedsReview: needsReview,
		Rejected:    rejected,
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetEntities handles GET /api/projects/{pid}/datasources/{dsid}/relationships/entities
func (h *RelationshipWorkflowHandler) GetEntities(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	entitiesWithOccurrences, err := h.workflowService.GetEntitiesWithOccurrences(r.Context(), datasourceID)
	if err != nil {
		h.logger.Error("Failed to get entities",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_entities_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert to response format
	entities := make([]EntityResponse, 0, len(entitiesWithOccurrences))
	for _, ew := range entitiesWithOccurrences {
		occurrences := make([]EntityOccurrenceResponse, 0, len(ew.Occurrences))
		for _, occ := range ew.Occurrences {
			occurrences = append(occurrences, EntityOccurrenceResponse{
				ID:         occ.ID.String(),
				SchemaName: occ.SchemaName,
				TableName:  occ.TableName,
				ColumnName: occ.ColumnName,
				Role:       occ.Role,
				Confidence: occ.Confidence,
			})
		}

		entities = append(entities, EntityResponse{
			ID:            ew.Entity.ID.String(),
			Name:          ew.Entity.Name,
			Description:   ew.Entity.Description,
			PrimarySchema: ew.Entity.PrimarySchema,
			PrimaryTable:  ew.Entity.PrimaryTable,
			PrimaryColumn: ew.Entity.PrimaryColumn,
			Occurrences:   occurrences,
		})
	}

	response := EntitiesResponse{
		Entities: entities,
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// UpdateCandidate handles PUT /api/projects/{pid}/datasources/{dsid}/relationships/candidates/{cid}
func (h *RelationshipWorkflowHandler) UpdateCandidate(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	candidateIDStr := r.PathValue("cid")
	candidateID, err := uuid.Parse(candidateIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_candidate_id", "Invalid candidate ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req CandidateDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate decision
	if req.Decision != "accepted" && req.Decision != "rejected" {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_decision", "Decision must be 'accepted' or 'rejected'"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Update candidate via service (includes datasource ownership verification)
	candidate, err := h.workflowService.UpdateCandidateDecision(r.Context(), datasourceID, candidateID, req.Decision)
	if err != nil {
		h.logger.Error("Failed to update candidate",
			zap.String("candidate_id", candidateID.String()),
			zap.Error(err))
		// Check if it's a "not found" error (which includes security failures)
		if err.Error() == "candidate not found" {
			if err := ErrorResponse(w, http.StatusNotFound, "candidate_not_found", "Candidate not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if err := ErrorResponse(w, http.StatusInternalServerError, "update_candidate_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := h.toCandidateResponse(candidate)
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Cancel handles POST /api/projects/{pid}/datasources/{dsid}/relationships/cancel
func (h *RelationshipWorkflowHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	workflow, err := h.workflowService.GetStatus(r.Context(), datasourceID)
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
		if err := ErrorResponse(w, http.StatusNotFound, "workflow_not_found", "No relationship workflow found for this datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.workflowService.Cancel(r.Context(), workflow.ID); err != nil {
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

// Save handles POST /api/projects/{pid}/datasources/{dsid}/relationships/save
func (h *RelationshipWorkflowHandler) Save(w http.ResponseWriter, r *http.Request) {
	_, datasourceID, ok := h.parseProjectAndDatasourceIDs(w, r)
	if !ok {
		return
	}

	workflow, err := h.workflowService.GetStatus(r.Context(), datasourceID)
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
		if err := ErrorResponse(w, http.StatusNotFound, "workflow_not_found", "No relationship workflow found for this datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Check workflow state
	if workflow.State != models.WorkflowStateCompleted {
		if err := ErrorResponse(w, http.StatusBadRequest, "workflow_not_complete", "Workflow must be complete before saving"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	savedCount, err := h.workflowService.SaveRelationships(r.Context(), workflow.ID)
	if err != nil {
		h.logger.Error("Failed to save relationships",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "save_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := SaveRelationshipsResponse{
		SavedCount: savedCount,
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

// parseProjectAndDatasourceIDs extracts and validates project and datasource IDs from the request path.
func (h *RelationshipWorkflowHandler) parseProjectAndDatasourceIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
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

// toCandidateResponse converts a model to a response type.
func (h *RelationshipWorkflowHandler) toCandidateResponse(c *models.RelationshipCandidate) CandidateResponse {
	return CandidateResponse{
		ID:              c.ID.String(),
		SourceTable:     c.SourceTable,
		SourceColumn:    c.SourceColumn,
		TargetTable:     c.TargetTable,
		TargetColumn:    c.TargetColumn,
		Confidence:      c.Confidence,
		DetectionMethod: string(c.DetectionMethod),
		LLMReasoning:    c.LLMReasoning,
		Cardinality:     c.Cardinality,
		Status:          string(c.Status),
		IsRequired:      c.IsRequired,
	}
}
