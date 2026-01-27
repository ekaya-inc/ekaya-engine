package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ============================================================================
// Request/Response Types
// ============================================================================

// ChatInitResponse for GET /chat/initialize
type ChatInitResponse struct {
	OpeningMessage       string `json:"opening_message"`
	PendingQuestionCount int    `json:"pending_question_count"`
	HasExistingHistory   bool   `json:"has_existing_history"`
}

// SendMessageRequest for POST /chat/message
type SendMessageRequest struct {
	Message string `json:"message"`
}

// ChatMessageResponse for chat history endpoint.
type ChatMessageResponse struct {
	ID         string            `json:"id"`
	ProjectID  string            `json:"project_id"`
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []models.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	CreatedAt  string            `json:"created_at"`
}

// ChatHistoryResponse for GET /chat/history
type ChatHistoryResponse struct {
	Messages []ChatMessageResponse `json:"messages"`
	Total    int                   `json:"total"`
}

// KnowledgeFactResponse for knowledge endpoints.
type KnowledgeFactResponse struct {
	ID        string `json:"id"`
	FactType  string `json:"fact_type"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Context   string `json:"context,omitempty"`
	CreatedAt string `json:"created_at"`
}

// KnowledgeListResponse for GET /knowledge
type KnowledgeListResponse struct {
	Facts []KnowledgeFactResponse `json:"facts"`
	Total int                     `json:"total"`
}

// ============================================================================
// Handler
// ============================================================================

// OntologyChatHandler handles ontology chat HTTP requests with SSE support.
type OntologyChatHandler struct {
	chatService      services.OntologyChatService
	knowledgeService services.KnowledgeService
	logger           *zap.Logger
}

// NewOntologyChatHandler creates a new ontology chat handler.
func NewOntologyChatHandler(
	chatService services.OntologyChatService,
	knowledgeService services.KnowledgeService,
	logger *zap.Logger,
) *OntologyChatHandler {
	return &OntologyChatHandler{
		chatService:      chatService,
		knowledgeService: knowledgeService,
		logger:           logger,
	}
}

// RegisterRoutes registers the chat handler's routes on the given mux.
func (h *OntologyChatHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	chatBase := "/api/projects/{pid}/ontology/chat"

	// Read-only endpoints - no provenance needed
	mux.HandleFunc("GET "+chatBase+"/history",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetHistory)))

	// Write endpoints - require provenance for audit tracking
	mux.HandleFunc("POST "+chatBase+"/initialize",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.Initialize)))
	mux.HandleFunc("POST "+chatBase+"/message",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.SendMessage)))
	mux.HandleFunc("DELETE "+chatBase+"/history",
		authMiddleware.RequireAuthWithPathValidationAndProvenance("pid")(tenantMiddleware(h.ClearHistory)))

	// Knowledge endpoints - read-only
	knowledgeBase := "/api/projects/{pid}/ontology/knowledge"
	mux.HandleFunc("GET "+knowledgeBase,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetKnowledge)))
}

// Initialize handles POST /api/projects/{pid}/ontology/chat/initialize
func (h *OntologyChatHandler) Initialize(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	result, err := h.chatService.Initialize(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to initialize chat",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "init_failed", "Failed to initialize chat"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := ChatInitResponse{
		OpeningMessage:       result.OpeningMessage,
		PendingQuestionCount: result.PendingQuestionCount,
		HasExistingHistory:   result.HasExistingHistory,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// SendMessage handles POST /api/projects/{pid}/ontology/chat/message
// This endpoint uses Server-Sent Events (SSE) to stream the response.
func (h *OntologyChatHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req SendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Message == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_message", "Message is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.logger.Error("SSE not supported")
		if err := ErrorResponse(w, http.StatusInternalServerError, "sse_unsupported", "SSE not supported"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	eventChan := make(chan models.ChatEvent, 100)

	// Start streaming in background
	go func() {
		defer close(eventChan)
		if err := h.chatService.SendMessage(r.Context(), projectID, req.Message, eventChan); err != nil {
			h.logger.Error("Chat send message error",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			eventChan <- models.NewErrorEvent(err.Error())
		}
	}()

	// Stream events to client
	for event := range eventChan {
		data, err := json.Marshal(event)
		if err != nil {
			h.logger.Error("Failed to marshal event", zap.Error(err))
			continue
		}

		// Write SSE formatted data
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		// Stop on done or error
		if event.Type == models.ChatEventDone || event.Type == models.ChatEventError {
			break
		}
	}
}

// GetHistory handles GET /api/projects/{pid}/ontology/chat/history
func (h *OntologyChatHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	// Parse limit from query params
	limit := 50 // Default
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	messages, err := h.chatService.GetHistory(r.Context(), projectID, limit)
	if err != nil {
		h.logger.Error("Failed to get chat history",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get chat history"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := ChatHistoryResponse{
		Messages: make([]ChatMessageResponse, len(messages)),
		Total:    len(messages),
	}
	for i, m := range messages {
		data.Messages[i] = h.toChatMessageResponse(m)
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ClearHistory handles DELETE /api/projects/{pid}/ontology/chat/history
func (h *OntologyChatHandler) ClearHistory(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.chatService.ClearHistory(r.Context(), projectID); err != nil {
		h.logger.Error("Failed to clear chat history",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to clear chat history"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"message": "Chat history cleared"}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetKnowledge handles GET /api/projects/{pid}/ontology/knowledge
func (h *OntologyChatHandler) GetKnowledge(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	// Optional filter by type
	factType := r.URL.Query().Get("type")

	var facts []*models.KnowledgeFact
	var err error

	if factType != "" {
		facts, err = h.knowledgeService.GetByType(r.Context(), projectID, factType)
	} else {
		facts, err = h.knowledgeService.GetAll(r.Context(), projectID)
	}

	if err != nil {
		h.logger.Error("Failed to get knowledge facts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get knowledge facts"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := KnowledgeListResponse{
		Facts: make([]KnowledgeFactResponse, len(facts)),
		Total: len(facts),
	}
	for i, f := range facts {
		data.Facts[i] = h.toKnowledgeFactResponse(f)
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ============================================================================
// Helper Methods
// ============================================================================

func (h *OntologyChatHandler) toChatMessageResponse(m *models.ChatMessage) ChatMessageResponse {
	return ChatMessageResponse{
		ID:         m.ID.String(),
		ProjectID:  m.ProjectID.String(),
		Role:       string(m.Role),
		Content:    m.Content,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
		CreatedAt:  m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func (h *OntologyChatHandler) toKnowledgeFactResponse(f *models.KnowledgeFact) KnowledgeFactResponse {
	return KnowledgeFactResponse{
		ID:        f.ID.String(),
		FactType:  f.FactType,
		Key:       f.Key,
		Value:     f.Value,
		Context:   f.Context,
		CreatedAt: f.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
