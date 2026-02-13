package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
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

	// Callback endpoint is unauthenticated — browser redirects don't carry Authorization headers.
	// Security relies on the single-use nonce (same pattern as OAuth callbacks).
	mux.HandleFunc("GET "+base+"/{appId}/callback", tenantMiddleware(h.Callback))
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

// Callback handles GET /api/projects/{pid}/apps/{appId}/callback
// This endpoint is unauthenticated — browser redirects from central don't carry
// Authorization headers. Security relies on the single-use nonce (state param).
// Returns HTTP 302 redirects to UI pages, not JSON.
func (h *InstalledAppHandler) Callback(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	appID := r.PathValue("appId")
	action := r.URL.Query().Get("action")
	status := r.URL.Query().Get("status")
	nonce := r.URL.Query().Get("state")

	if appID == "" || action == "" || status == "" || nonce == "" {
		http.Error(w, "Missing required callback parameters", http.StatusBadRequest)
		return
	}

	// Validate action is one of the known values
	if action != "install" && action != "activate" && action != "uninstall" {
		http.Error(w, "Invalid action parameter", http.StatusBadRequest)
		return
	}

	// For install callbacks, we don't have the userID from auth (unauthenticated endpoint).
	// Use empty string — the nonce validates the request originated from a legitimate flow.
	userID := ""

	err := h.installedAppService.CompleteCallback(r.Context(), projectID, appID, action, status, nonce, userID)
	if err != nil {
		h.logger.Error("Failed to complete callback",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.String("action", action),
			zap.String("status", status),
			zap.Error(err))
		http.Error(w, "Callback processing failed", http.StatusBadRequest)
		return
	}

	// Redirect to the appropriate UI page
	redirectTarget := h.callbackRedirectTarget(projectID, appID, action, status)
	http.Redirect(w, r, redirectTarget, http.StatusFound)
}

// callbackRedirectTarget determines which UI page to redirect to after a callback.
func (h *InstalledAppHandler) callbackRedirectTarget(projectID uuid.UUID, appID, action, status string) string {
	pid := projectID.String()

	// If cancelled, always go back to the app page
	if status == "cancelled" {
		return "/projects/" + pid + "/apps/" + appID
	}

	switch action {
	case "uninstall":
		return "/projects/" + pid
	case "install":
		return "/projects/" + pid + "/apps/" + appID
	case "activate":
		return "/projects/" + pid + "/apps/" + appID
	default:
		return "/projects/" + pid
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
