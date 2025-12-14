package main

import (
	"log"
	"net/http"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/handlers"
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

	// Register handlers
	healthHandler := handlers.NewHealthHandler(cfg)
	healthHandler.RegisterRoutes(mux)

	// Serve static UI files from ui/dist
	fs := http.FileServer(http.Dir("./ui/dist"))
	mux.Handle("/", fs)

	log.Printf("Starting ekaya-engine on port %s (version: %s)", cfg.Port, cfg.Version)
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
