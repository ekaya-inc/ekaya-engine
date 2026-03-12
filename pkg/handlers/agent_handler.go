package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/jsonutil"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/google/uuid"
)

type createAgentRequest struct {
	Name     string   `json:"name"`
	QueryIDs []string `json:"query_ids"`
}

type updateAgentRequest struct {
	QueryIDs []string `json:"query_ids"`
}

type agentResponse struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	QueryIDs     []string `json:"query_ids"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
	LastAccessAt *string  `json:"last_access_at"`
	MCPCallCount int64    `json:"mcp_call_count"`
}

type createAgentResponse struct {
	agentResponse
	APIKey string `json:"api_key"`
}

type listAgentsResponse struct {
	Agents []agentResponse `json:"agents"`
}

type getAgentKeyResponse struct {
	Key    string `json:"key"`
	Masked bool   `json:"masked"`
}

type rotateAgentKeyResponse struct {
	APIKey string `json:"api_key"`
}

// AgentHandler handles named AI agent management routes.
type AgentHandler struct {
	agentService services.AgentService
	logger       *zap.Logger
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(agentService services.AgentService, logger *zap.Logger) *AgentHandler {
	return &AgentHandler{
		agentService: agentService,
		logger:       logger,
	}
}

// RegisterRoutes registers the named agent routes.
func (h *AgentHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/agents"

	mux.HandleFunc("POST "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Create))))
	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.List))))
	mux.HandleFunc("GET "+base+"/{aid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Get))))
	mux.HandleFunc("PATCH "+base+"/{aid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Update))))
	mux.HandleFunc("GET "+base+"/{aid}/key",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.GetKey))))
	mux.HandleFunc("POST "+base+"/{aid}/rotate-key",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.RotateKey))))
	mux.HandleFunc("DELETE "+base+"/{aid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Delete))))
}

// Create handles POST /api/projects/{pid}/agents.
func (h *AgentHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req createAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	queryIDs, err := parseQueryIDs(req.QueryIDs)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_query_ids", err.Error())
		return
	}

	agent, apiKey, err := h.agentService.Create(r.Context(), projectID, req.Name, queryIDs)
	if err != nil {
		h.handleServiceError(w, err, "Failed to create agent")
		return
	}

	createdAgent, err := h.agentService.Get(r.Context(), projectID, agent.ID)
	if err != nil {
		h.handleServiceError(w, err, "Failed to get created agent")
		return
	}

	response := ApiResponse{
		Success: true,
		Data: createAgentResponse{
			agentResponse: buildAgentResponse(createdAgent),
			APIKey:        apiKey,
		},
	}

	if err := WriteJSON(w, http.StatusCreated, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// List handles GET /api/projects/{pid}/agents.
func (h *AgentHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	agents, err := h.agentService.List(r.Context(), projectID)
	if err != nil {
		h.handleServiceError(w, err, "Failed to list agents")
		return
	}

	items := make([]agentResponse, 0, len(agents))
	for _, agent := range agents {
		items = append(items, buildAgentResponse(agent))
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data:    listAgentsResponse{Agents: items},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Get handles GET /api/projects/{pid}/agents/{aid}.
func (h *AgentHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID, agentID, ok := h.parseProjectAndAgentID(w, r)
	if !ok {
		return
	}

	agent, err := h.agentService.Get(r.Context(), projectID, agentID)
	if err != nil {
		h.handleServiceError(w, err, "Failed to get agent")
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data:    buildAgentResponse(agent),
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Update handles PATCH /api/projects/{pid}/agents/{aid}.
func (h *AgentHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, agentID, ok := h.parseProjectAndAgentID(w, r)
	if !ok {
		return
	}

	var req updateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	queryIDs, err := parseQueryIDs(req.QueryIDs)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_query_ids", err.Error())
		return
	}

	if err := h.agentService.UpdateQueryAccess(r.Context(), projectID, agentID, queryIDs); err != nil {
		h.handleServiceError(w, err, "Failed to update agent")
		return
	}

	agent, err := h.agentService.Get(r.Context(), projectID, agentID)
	if err != nil {
		h.handleServiceError(w, err, "Failed to get updated agent")
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data:    buildAgentResponse(agent),
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetKey handles GET /api/projects/{pid}/agents/{aid}/key.
func (h *AgentHandler) GetKey(w http.ResponseWriter, r *http.Request) {
	projectID, agentID, ok := h.parseProjectAndAgentID(w, r)
	if !ok {
		return
	}

	reveal := r.URL.Query().Get("reveal") == "true"
	responseKey := "****"
	masked := true
	if reveal {
		key, err := h.agentService.GetKey(r.Context(), projectID, agentID)
		if err != nil {
			h.handleServiceError(w, err, "Failed to get agent key")
			return
		}
		responseKey = key
		masked = false
	} else {
		if err := h.agentService.EnsureExists(r.Context(), projectID, agentID); err != nil {
			h.handleServiceError(w, err, "Failed to get agent")
			return
		}
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: getAgentKeyResponse{
			Key:    responseKey,
			Masked: masked,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// RotateKey handles POST /api/projects/{pid}/agents/{aid}/rotate-key.
func (h *AgentHandler) RotateKey(w http.ResponseWriter, r *http.Request) {
	projectID, agentID, ok := h.parseProjectAndAgentID(w, r)
	if !ok {
		return
	}

	key, err := h.agentService.RotateKey(r.Context(), projectID, agentID)
	if err != nil {
		h.handleServiceError(w, err, "Failed to rotate agent key")
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: rotateAgentKeyResponse{
			APIKey: key,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/agents/{aid}.
func (h *AgentHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID, agentID, ok := h.parseProjectAndAgentID(w, r)
	if !ok {
		return
	}

	if err := h.agentService.Delete(r.Context(), projectID, agentID); err != nil {
		h.handleServiceError(w, err, "Failed to delete agent")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *AgentHandler) parseProjectAndAgentID(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}

	agentID, ok := ParseAgentID(w, r, h.logger)
	if !ok {
		return uuid.Nil, uuid.Nil, false
	}

	return projectID, agentID, true
}

func (h *AgentHandler) handleServiceError(w http.ResponseWriter, err error, defaultMessage string) {
	switch {
	case errors.Is(err, apperrors.ErrConflict):
		h.writeError(w, http.StatusConflict, "agent_conflict", "An agent with that name already exists")
	case errors.Is(err, apperrors.ErrNotFound):
		h.writeError(w, http.StatusNotFound, "not_found", "Agent not found")
	default:
		var validationErr *services.AgentValidationError
		if errors.As(err, &validationErr) {
			h.writeError(w, http.StatusBadRequest, "invalid_request", validationErr.Error())
			return
		}
		h.logger.Error(defaultMessage, zap.Error(err))
		h.writeError(w, http.StatusInternalServerError, "internal_error", defaultMessage)
	}
}

func (h *AgentHandler) writeError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	if err := ErrorResponse(w, statusCode, errorCode, message); err != nil {
		h.logger.Error("Failed to write error response", zap.Error(err))
	}
}

func parseQueryIDs(values []string) ([]uuid.UUID, error) {
	if len(values) == 0 {
		return nil, errors.New("query_ids must contain at least one query ID")
	}

	queryIDs := make([]uuid.UUID, 0, len(values))
	for _, raw := range values {
		queryID, err := uuid.Parse(raw)
		if err != nil {
			return nil, errors.New("query_ids must contain valid UUIDs")
		}
		queryIDs = append(queryIDs, queryID)
	}

	return queryIDs, nil
}

func buildAgentResponse(agent *services.AgentWithQueries) agentResponse {
	queryIDs := make([]string, 0, len(agent.QueryIDs))
	for _, queryID := range agent.QueryIDs {
		queryIDs = append(queryIDs, queryID.String())
	}

	var lastAccessed *string
	if agent.LastAccessAt != nil {
		lastAccessed = jsonutil.FormatUTCTimePtr(agent.LastAccessAt)
	}

	return agentResponse{
		ID:           agent.ID.String(),
		Name:         agent.Name,
		QueryIDs:     queryIDs,
		CreatedAt:    jsonutil.FormatUTCTime(agent.CreatedAt),
		UpdatedAt:    jsonutil.FormatUTCTime(agent.UpdatedAt),
		LastAccessAt: lastAccessed,
		MCPCallCount: agent.MCPCallCount,
	}
}
