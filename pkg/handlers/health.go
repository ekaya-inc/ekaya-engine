package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"

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

// HealthHandler handles health check and ping endpoints.
type HealthHandler struct {
	cfg *config.Config
}

// NewHealthHandler creates a new HealthHandler with the given configuration.
func NewHealthHandler(cfg *config.Config) *HealthHandler {
	return &HealthHandler{cfg: cfg}
}

// RegisterRoutes registers the health handler's routes on the given mux.
func (h *HealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/ping", h.Ping)
}

// Health handles GET /health requests.
// Returns a simple "ok" status for Cloud Run health checks.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		return
	}
}
