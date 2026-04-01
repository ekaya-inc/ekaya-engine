package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// PingResponse contains service status and version information.
type PingResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Service string `json:"service"`
}

// HealthResponse contains comprehensive health check information including
// connection manager statistics for monitoring connection pool health.
type HealthResponse struct {
	Status      string                      `json:"status"`
	Connections *datasource.ConnectionStats `json:"connections,omitempty"`
}

// HealthHandler handles health check and ping endpoints.
type HealthHandler struct {
	cfg         *config.Config
	connManager *datasource.ConnectionManager
	logger      *zap.Logger
}

// NewHealthHandler creates a new HealthHandler with the given configuration.
// connManager is optional - if nil, connection stats will not be included.
func NewHealthHandler(cfg *config.Config, connManager *datasource.ConnectionManager, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		cfg:         cfg,
		connManager: connManager,
		logger:      logger,
	}
}

// RegisterRoutes registers the health handler's routes on the given mux.
func (h *HealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/ping", h.Ping)
	mux.HandleFunc("/metrics", h.Metrics)
}

// Health handles GET /health requests.
// Returns comprehensive health status including connection manager statistics.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status: "ok",
	}

	// Include connection manager stats if available
	if h.connManager != nil {
		stats := h.connManager.GetStats()
		response.Connections = &stats
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode health response", zap.Error(err))
	}
}

// Ping handles GET /ping requests.
// Returns minimal service information for browser-based probe checks.
func (h *HealthHandler) Ping(w http.ResponseWriter, r *http.Request) {
	setPingCORSHeaders(w)

	switch r.Method {
	case http.MethodGet, http.MethodHead:
		response := PingResponse{
			Status:  "ok",
			Version: h.cfg.Version,
			Service: "ekaya-engine",
		}

		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to encode ping response", zap.Error(err))
		}
	case http.MethodOptions:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func setPingCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

// Metrics handles GET /metrics requests.
// Returns detailed connection manager metrics for monitoring and alerting.
func (h *HealthHandler) Metrics(w http.ResponseWriter, r *http.Request) {
	if h.connManager == nil {
		http.Error(w, "connection manager not available", http.StatusServiceUnavailable)
		return
	}

	stats := h.connManager.GetStats()
	if err := WriteJSON(w, http.StatusOK, stats); err != nil {
		h.logger.Error("Failed to encode metrics response", zap.Error(err))
	}
}
