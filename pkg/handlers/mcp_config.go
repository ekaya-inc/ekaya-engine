package handlers

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// UpdateMCPConfigRequest is the request body for PATCH /api/projects/{pid}/mcp/config.
// Uses optional pointers to distinguish "not sent" from "sent as false".
type UpdateMCPConfigRequest struct {
	// User Tools sub-option: allow MCP clients with user role to update ontology
	AllowOntologyMaintenance *bool `json:"allowOntologyMaintenance,omitempty"`
	// Developer Tools sub-options
	AddQueryTools          *bool `json:"addQueryTools,omitempty"`          // adds Query loadout
	AddOntologyMaintenance *bool `json:"addOntologyMaintenance,omitempty"` // adds Ontology Maintenance tools
}

// MCPConfigHandler handles MCP configuration HTTP requests.
type MCPConfigHandler struct {
	mcpConfigService services.MCPConfigService
	logger           *zap.Logger
}

// NewMCPConfigHandler creates a new MCP config handler.
func NewMCPConfigHandler(mcpConfigService services.MCPConfigService, logger *zap.Logger) *MCPConfigHandler {
	return &MCPConfigHandler{
		mcpConfigService: mcpConfigService,
		logger:           logger,
	}
}

// RegisterRoutes registers the MCP config handler's routes on the given mux.
func (h *MCPConfigHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/mcp/config"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("PATCH "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Update)))
}

// Get handles GET /api/projects/{pid}/mcp/config
func (h *MCPConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	config, err := h.mcpConfigService.Get(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get MCP config",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get MCP config"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: config}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Update handles PATCH /api/projects/{pid}/mcp/config
func (h *MCPConfigHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req UpdateMCPConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert flat request to service request format
	serviceReq := &services.UpdateMCPConfigRequest{
		AllowOntologyMaintenance: req.AllowOntologyMaintenance,
		AddQueryTools:            req.AddQueryTools,
		AddOntologyMaintenance:   req.AddOntologyMaintenance,
	}

	config, err := h.mcpConfigService.Update(r.Context(), projectID, serviceReq)
	if err != nil {
		h.logger.Error("Failed to update MCP config",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "update_failed", "Failed to update MCP config"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: config}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
