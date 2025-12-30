package handlers

import (
	"net/http"
	"os"
	"runtime"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// PingResponse contains service status and version information.
type PingResponse struct {
	Status      string `json:"status"`
	Version     string `json:"version"`
	Service     string `json:"service"`
	GoVersion   string `json:"go_version"`
	Hostname    string `json:"hostname"`
	Environment string `json:"environment"`
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
// Returns detailed service information including version and environment.
func (h *HealthHandler) Ping(w http.ResponseWriter, r *http.Request) {
	hostname, err := os.Hostname()
	if err != nil {
		http.Error(w, "failed to get hostname", http.StatusInternalServerError)
		return
	}

	response := PingResponse{
		Status:      "ok",
		Version:     h.cfg.Version,
		Service:     "ekaya-engine",
		GoVersion:   runtime.Version(),
		Hostname:    hostname,
		Environment: h.cfg.Env,
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode ping response", zap.Error(err))
	}
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
