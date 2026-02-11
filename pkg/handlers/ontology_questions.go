package handlers

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// QuestionResponse for question endpoints.
type QuestionResponse struct {
	ID              string   `json:"id"`
	ProjectID       string   `json:"project_id"`
	WorkflowID      *string  `json:"workflow_id,omitempty"`
	Text            string   `json:"text"`
	Priority        int      `json:"priority"`
	IsRequired      bool     `json:"is_required"`
	Category        string   `json:"category,omitempty"`
	Reasoning       string   `json:"reasoning,omitempty"`
	AffectedTables  []string `json:"affected_tables,omitempty"`
	AffectedColumns []string `json:"affected_columns,omitempty"`
	DetectedPattern string   `json:"detected_pattern,omitempty"`
	Status          string   `json:"status"`
	Answer          string   `json:"answer,omitempty"`
	AnsweredAt      *string  `json:"answered_at,omitempty"`
	CreatedAt       string   `json:"created_at"`
}

// QuestionCountsResponse for question counts.
type QuestionCountsResponse struct {
	Required int `json:"required"`
	Optional int `json:"optional"`
}

// ListQuestionsResponse for GET /questions endpoint.
type ListQuestionsResponse struct {
	Questions []QuestionResponse `json:"questions"`
	Total     int                `json:"total"`
}

// AnswerQuestionRequest for POST /questions/{id}/answer
type AnswerQuestionRequest struct {
	Answer string `json:"answer"`
}

// AnswerQuestionResponse for answer endpoint.
type AnswerQuestionResponse struct {
	QuestionID     string                  `json:"question_id"`
	NextQuestion   *QuestionResponse       `json:"next_question,omitempty"`
	AllComplete    bool                    `json:"all_complete"`
	ActionsSummary string                  `json:"actions_summary,omitempty"`
	Counts         *QuestionCountsResponse `json:"counts,omitempty"`
}

// ============================================================================
// Handler
// ============================================================================

// OntologyQuestionsHandler handles ontology question HTTP requests.
type OntologyQuestionsHandler struct {
	questionService services.OntologyQuestionService
	logger          *zap.Logger
}

// NewOntologyQuestionsHandler creates a new ontology questions handler.
func NewOntologyQuestionsHandler(questionService services.OntologyQuestionService, logger *zap.Logger) *OntologyQuestionsHandler {
	return &OntologyQuestionsHandler{
		questionService: questionService,
		logger:          logger,
	}
}

// RegisterRoutes registers the questions handler's routes on the given mux.
func (h *OntologyQuestionsHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/ontology/questions"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
	mux.HandleFunc("GET "+base+"/next",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetNext)))
	mux.HandleFunc("POST "+base+"/{qid}/answer",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Answer)))
	mux.HandleFunc("POST "+base+"/{qid}/skip",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Skip)))
	mux.HandleFunc("GET "+base+"/counts",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Counts)))
	mux.HandleFunc("DELETE "+base+"/{qid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Delete)))
}

// List handles GET /api/projects/{pid}/ontology/questions
func (h *OntologyQuestionsHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	questions, err := h.questionService.GetPendingQuestions(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list questions",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to list questions"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := ListQuestionsResponse{
		Questions: make([]QuestionResponse, len(questions)),
		Total:     len(questions),
	}
	for i, q := range questions {
		data.Questions[i] = h.toQuestionResponse(q)
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Counts handles GET /api/projects/{pid}/ontology/questions/counts
func (h *OntologyQuestionsHandler) Counts(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	counts, err := h.questionService.GetPendingCounts(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get question counts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get question counts"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := QuestionCountsResponse{Required: 0, Optional: 0}
	if counts != nil {
		data.Required = counts.Required
		data.Optional = counts.Optional
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetNext handles GET /api/projects/{pid}/ontology/questions/next
func (h *OntologyQuestionsHandler) GetNext(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	// Check query param for including skipped questions
	includeSkipped := r.URL.Query().Get("include_skipped") == "true"

	question, err := h.questionService.GetNextQuestion(r.Context(), projectID, includeSkipped)
	if err != nil {
		h.logger.Error("Failed to get next question",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get next question"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get question counts for UI display
	counts, err := h.questionService.GetPendingCounts(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get question counts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		// Non-fatal - continue without counts
		counts = nil
	}

	if question == nil {
		// No more questions
		data := map[string]any{
			"question":     nil,
			"all_complete": true,
			"counts":       QuestionCountsResponse{Required: 0, Optional: 0},
		}
		response := ApiResponse{Success: true, Data: data}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	data := map[string]any{
		"question":     h.toQuestionResponse(question),
		"all_complete": false,
	}
	if counts != nil {
		data["counts"] = QuestionCountsResponse{Required: counts.Required, Optional: counts.Optional}
	}
	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Answer handles POST /api/projects/{pid}/ontology/questions/{qid}/answer
func (h *OntologyQuestionsHandler) Answer(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	questionID, ok := ParseQuestionID(w, r, h.logger)
	if !ok {
		return
	}

	var req AnswerQuestionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Answer == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_answer", "Answer is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get user ID from auth context
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "Authentication required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	result, err := h.questionService.AnswerQuestion(r.Context(), questionID, req.Answer, claims.Subject)
	if err != nil {
		h.logger.Error("Failed to answer question",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "answer_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get updated question counts
	counts, err := h.questionService.GetPendingCounts(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get question counts after answer",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		// Non-fatal - continue without counts
		counts = nil
	}

	data := AnswerQuestionResponse{
		QuestionID:     result.QuestionID.String(),
		AllComplete:    result.AllComplete,
		ActionsSummary: result.ActionsSummary,
	}

	if counts != nil {
		data.Counts = &QuestionCountsResponse{Required: counts.Required, Optional: counts.Optional}
	}

	if result.NextQuestion != nil {
		next := h.toQuestionResponse(result.NextQuestion)
		data.NextQuestion = &next
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Skip handles POST /api/projects/{pid}/ontology/questions/{qid}/skip
func (h *OntologyQuestionsHandler) Skip(w http.ResponseWriter, r *http.Request) {
	_, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	questionID, ok := ParseQuestionID(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.questionService.SkipQuestion(r.Context(), questionID); err != nil {
		h.logger.Error("Failed to skip question",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "skip_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"message": "Question skipped"}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/ontology/questions/{qid}
func (h *OntologyQuestionsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	questionID, ok := ParseQuestionID(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.questionService.DeleteQuestion(r.Context(), questionID); err != nil {
		h.logger.Error("Failed to delete question",
			zap.String("question_id", questionID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"message": "Question deleted"}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *OntologyQuestionsHandler) toQuestionResponse(q *models.OntologyQuestion) QuestionResponse {
	resp := QuestionResponse{
		ID:              q.ID.String(),
		ProjectID:       q.ProjectID.String(),
		Text:            q.Text,
		Priority:        q.Priority,
		IsRequired:      q.IsRequired,
		Category:        q.Category,
		Reasoning:       q.Reasoning,
		DetectedPattern: q.DetectedPattern,
		Status:          string(q.Status),
		Answer:          q.Answer,
		CreatedAt:       q.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if q.WorkflowID != nil {
		wid := q.WorkflowID.String()
		resp.WorkflowID = &wid
	}

	if q.Affects != nil {
		resp.AffectedTables = q.Affects.Tables
		resp.AffectedColumns = q.Affects.Columns
	}

	if q.AnsweredAt != nil {
		answered := q.AnsweredAt.Format("2006-01-02T15:04:05Z07:00")
		resp.AnsweredAt = &answered
	}

	return resp
}
