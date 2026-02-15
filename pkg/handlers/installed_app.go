package handlers

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// InstalledAppHandler handles installed apps HTTP requests.
type InstalledAppHandler struct {
	installedAppService services.InstalledAppService
	logger              *zap.Logger
}

// NewInstalledAppHandler creates a new installed app handler.
func NewInstalledAppHandler(installedAppService services.InstalledAppService, logger *zap.Logger) *InstalledAppHandler {
	return &InstalledAppHandler{
		installedAppService: installedAppService,
		logger:              logger,
	}
}

// RegisterRoutes registers the installed app handler's routes on the given mux.
func (h *InstalledAppHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/apps"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
	mux.HandleFunc("GET "+base+"/{appId}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("POST "+base+"/{appId}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Install)))
	mux.HandleFunc("POST "+base+"/{appId}/activate",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Activate)))
	mux.HandleFunc("DELETE "+base+"/{appId}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Uninstall)))
	mux.HandleFunc("PATCH "+base+"/{appId}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.UpdateSettings)))

	// Callback is authenticated — central redirects to a UI route, the SPA re-establishes
	// auth context, then calls this endpoint. Security: JWT auth + single-use nonce.
	mux.HandleFunc("POST "+base+"/{appId}/callback",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Callback)))
}

// List handles GET /api/projects/{pid}/apps
func (h *InstalledAppHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	apps, err := h.installedAppService.ListInstalled(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list installed apps",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to list installed apps"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]any{"apps": apps}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Get handles GET /api/projects/{pid}/apps/{appId}
func (h *InstalledAppHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	appID := r.PathValue("appId")
	if appID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_app_id", "App ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	app, err := h.installedAppService.GetApp(r.Context(), projectID, appID)
	if err != nil {
		h.logger.Error("Failed to get app",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusNotFound, "not_found", "App not installed"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: app}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Install handles POST /api/projects/{pid}/apps/{appId}
func (h *InstalledAppHandler) Install(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	appID := r.PathValue("appId")
	if appID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_app_id", "App ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get user ID from auth context for tracking who installed the app
	userID := auth.GetUserIDFromContext(r.Context())

	result, err := h.installedAppService.Install(r.Context(), projectID, appID, userID)
	if err != nil {
		h.logger.Error("Failed to install app",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.Error(err))

		// Determine appropriate error response
		statusCode := http.StatusInternalServerError
		errorCode := "install_failed"
		message := "Failed to install app"

		switch err.Error() {
		case "unknown app: " + appID:
			statusCode = http.StatusBadRequest
			errorCode = "unknown_app"
			message = "Unknown application"
		case "app already installed":
			statusCode = http.StatusConflict
			errorCode = "already_installed"
			message = "App is already installed"
		case "mcp-server is always installed":
			statusCode = http.StatusBadRequest
			errorCode = "cannot_install"
			message = "MCP Server is always available"
		}

		if err := ErrorResponse(w, statusCode, errorCode, message); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// If central requires a redirect, return the redirect URL
	if result.RedirectUrl != "" {
		response := ApiResponse{Success: true, Data: map[string]string{"redirectUrl": result.RedirectUrl}}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: result.App}
	if err := WriteJSON(w, http.StatusCreated, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Activate handles POST /api/projects/{pid}/apps/{appId}/activate
func (h *InstalledAppHandler) Activate(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	appID := r.PathValue("appId")
	if appID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_app_id", "App ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	result, err := h.installedAppService.Activate(r.Context(), projectID, appID)
	if err != nil {
		h.logger.Error("Failed to activate app",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.Error(err))

		statusCode := http.StatusInternalServerError
		errorCode := "activate_failed"
		message := "Failed to activate app"

		if err.Error() == "mcp-server does not require activation" {
			statusCode = http.StatusBadRequest
			errorCode = "cannot_activate"
			message = "MCP Server does not require activation"
		}

		if err := ErrorResponse(w, statusCode, errorCode, message); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// If central requires a redirect, return the redirect URL
	if result.RedirectUrl != "" {
		response := ApiResponse{Success: true, Data: map[string]string{"redirectUrl": result.RedirectUrl}}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"status": result.Status}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Uninstall handles DELETE /api/projects/{pid}/apps/{appId}
func (h *InstalledAppHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	appID := r.PathValue("appId")
	if appID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_app_id", "App ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	result, err := h.installedAppService.Uninstall(r.Context(), projectID, appID)
	if err != nil {
		h.logger.Error("Failed to uninstall app",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.Error(err))

		// Determine appropriate error response
		statusCode := http.StatusInternalServerError
		errorCode := "uninstall_failed"
		message := "Failed to uninstall app"

		switch err.Error() {
		case "mcp-server cannot be uninstalled":
			statusCode = http.StatusBadRequest
			errorCode = "cannot_uninstall"
			message = "MCP Server cannot be uninstalled"
		case "failed to uninstall app: app not installed":
			statusCode = http.StatusNotFound
			errorCode = "not_installed"
			message = "App is not installed"
		}

		if err := ErrorResponse(w, statusCode, errorCode, message); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// If central requires a redirect, return the redirect URL
	if result.RedirectUrl != "" {
		response := ApiResponse{Success: true, Data: map[string]string{"redirectUrl": result.RedirectUrl}}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{"status": result.Status}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// CallbackRequest is the request body for POST /api/projects/{pid}/apps/{appId}/callback.
type CallbackRequest struct {
	Action string `json:"action"` // install, activate, uninstall
	Status string `json:"status"` // success, cancelled
	State  string `json:"state"`  // single-use nonce
}

// Callback handles POST /api/projects/{pid}/apps/{appId}/callback
// This endpoint is authenticated — central redirects to a UI route, the SPA
// re-establishes auth context, then calls this endpoint with the callback params.
// Security: JWT auth + tenant middleware + single-use nonce.
func (h *InstalledAppHandler) Callback(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	appID := r.PathValue("appId")
	if appID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_app_id", "App ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req CallbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Action == "" || req.Status == "" || req.State == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_params", "action, status, and state are required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Action != "install" && req.Action != "activate" && req.Action != "uninstall" {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_action", "Invalid action parameter"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	userID := auth.GetUserIDFromContext(r.Context())

	err := h.installedAppService.CompleteCallback(r.Context(), projectID, appID, req.Action, req.Status, req.State, userID)
	if err != nil {
		h.logger.Error("Failed to complete callback",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.String("action", req.Action),
			zap.String("status", req.Status),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "callback_failed", "Callback processing failed"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{
		"action": req.Action,
		"status": req.Status,
	}}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// UpdateSettingsRequest is the request body for PATCH /api/projects/{pid}/apps/{appId}.
type UpdateSettingsRequest struct {
	Settings map[string]any `json:"settings"`
}

// UpdateSettings handles PATCH /api/projects/{pid}/apps/{appId}
func (h *InstalledAppHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	appID := r.PathValue("appId")
	if appID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_app_id", "App ID is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Settings == nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_settings", "settings field is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	err := h.installedAppService.UpdateSettings(r.Context(), projectID, appID, req.Settings)
	if err != nil {
		h.logger.Error("Failed to update app settings",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.Error(err))

		// Determine appropriate error response
		statusCode := http.StatusInternalServerError
		errorCode := "update_failed"
		message := "Failed to update app settings"

		switch err.Error() {
		case "mcp-server settings are managed via the MCP configuration API":
			statusCode = http.StatusBadRequest
			errorCode = "invalid_app"
			message = "MCP Server settings are managed via the MCP configuration API"
		case "failed to update settings: app not installed":
			statusCode = http.StatusNotFound
			errorCode = "not_installed"
			message = "App is not installed"
		}

		if err := ErrorResponse(w, statusCode, errorCode, message); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get the updated app to return in response
	app, err := h.installedAppService.GetApp(r.Context(), projectID, appID)
	if err != nil {
		// Settings were updated but failed to fetch - still return success
		response := ApiResponse{Success: true, Data: map[string]string{"status": "updated"}}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: app}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
