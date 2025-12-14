package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"runtime"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// Version is set at build time via ldflags
var Version = "dev"

func main() {
	// Load configuration
	cfg, err := config.Load(Version)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Log startup configuration
	log.Printf("Configuration loaded:")
	log.Printf("  Environment: %s", cfg.Env)
	log.Printf("  Base URL: %s", cfg.BaseURL)
	log.Printf("  Auth verification: %v", cfg.Auth.EnableVerification)
	log.Printf("  Database: %s@%s:%d/%s", cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)
	log.Printf("  Redis: %s:%d", cfg.Redis.Host, cfg.Redis.Port)

	mux := http.NewServeMux()

	// Health check endpoint for Cloud Run
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Ping endpoint with version info
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		hostname, _ := os.Hostname()
		response := map[string]interface{}{
			"status":      "ok",
			"version":     cfg.Version,
			"service":     "ekaya-engine",
			"go_version":  runtime.Version(),
			"hostname":    hostname,
			"environment": cfg.Env,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	})

	// Serve static UI files from ui/dist
	fs := http.FileServer(http.Dir("./ui/dist"))
	mux.Handle("/", fs)

	log.Printf("Starting ekaya-engine on port %s (version: %s)", cfg.Port, cfg.Version)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
