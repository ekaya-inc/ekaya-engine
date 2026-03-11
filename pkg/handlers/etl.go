package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/etl"
)

// ETLHandler handles HTTP requests for ETL operations.
type ETLHandler struct {
	etlService *etl.Service
	logger     *zap.Logger
}

// NewETLHandler creates a new ETL handler.
func NewETLHandler(etlService *etl.Service, logger *zap.Logger) *ETLHandler {
	return &ETLHandler{
		etlService: etlService,
		logger:     logger,
	}
}

// RegisterRoutes registers ETL HTTP routes.
func (h *ETLHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/etl"

	// Load history across all ETL applets
	mux.HandleFunc("GET "+base+"/status",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListStatus)))

	// Load history for specific applet
	mux.HandleFunc("GET "+base+"/{appId}/status",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListAppStatus)))

	// Manual file upload
	mux.HandleFunc("POST "+base+"/{appId}/load",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Load)))

	// Preview file (infer schema + sample rows without loading)
	mux.HandleFunc("POST "+base+"/{appId}/preview",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Preview)))

	// Confirm and load after preview
	mux.HandleFunc("POST "+base+"/{appId}/confirm",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Confirm)))
}

// ListStatus returns load history across all ETL applets for a project.
func (h *ETLHandler) ListStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	history, err := h.etlService.GetLoadHistory(r.Context(), projectID, "", limit)
	if err != nil {
		h.logger.Error("failed to list ETL status",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "etl_status_failed", err.Error()); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: history}); err != nil {
		h.logger.Error("failed to write response", zap.Error(err))
	}
}

// ListAppStatus returns load history for a specific ETL applet.
func (h *ETLHandler) ListAppStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}
	appID := r.PathValue("appId")

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	history, err := h.etlService.GetLoadHistory(r.Context(), projectID, appID, limit)
	if err != nil {
		h.logger.Error("failed to list ETL app status",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "etl_status_failed", err.Error()); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: history}); err != nil {
		h.logger.Error("failed to write response", zap.Error(err))
	}
}

const maxUploadSize = 100 << 20 // 100MB

// Load handles manual file upload and loading.
func (h *ETLHandler) Load(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}
	appID := r.PathValue("appId")

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "file_too_large", "File exceeds maximum size of 100MB"); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_file", "No file provided"); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "read_error", "Failed to read uploaded file"); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	result, err := h.etlService.LoadFile(r.Context(), projectID, appID, header.Filename, data)
	if err != nil {
		h.logger.Error("ETL load failed",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.String("file", header.Filename),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "etl_load_failed", err.Error()); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: result}); err != nil {
		h.logger.Error("failed to write response", zap.Error(err))
	}
}

// Preview parses a file and returns inferred schema + sample rows without loading.
func (h *ETLHandler) Preview(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}
	appID := r.PathValue("appId")

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "file_too_large", "File exceeds maximum size of 100MB"); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_file", "No file provided"); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "read_error", "Failed to read uploaded file"); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	preview, err := h.etlService.Preview(r.Context(), projectID, appID, header.Filename, data)
	if err != nil {
		h.logger.Error("ETL preview failed",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.String("file", header.Filename),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "etl_preview_failed", err.Error()); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: preview}); err != nil {
		h.logger.Error("failed to write response", zap.Error(err))
	}
}

// ConfirmRequest holds the schema confirmation and optional overrides.
type ConfirmRequest struct {
	FileName  string `json:"file_name"`
	TableName string `json:"table_name,omitempty"`
}

// Confirm accepts a schema confirmation and loads the file.
func (h *ETLHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}
	appID := r.PathValue("appId")

	var req ConfirmRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("failed to write error response", zap.Error(err))
		}
		return
	}

	// For confirm, the file data should have been cached from preview.
	// For simplicity in MVP, we require re-upload via the load endpoint instead.
	if err := ErrorResponse(w, http.StatusBadRequest, "use_load_endpoint",
		"Use POST .../load to upload and load the file directly. Preview is for schema review only."); err != nil {
		h.logger.Error("failed to write error response", zap.Error(err))
	}
	_ = projectID
	_ = appID
}
