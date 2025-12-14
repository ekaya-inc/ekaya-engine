package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/handlers"
	"github.com/jackc/pgx/v5/stdlib"
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

	// Connect to database
	ctx := context.Background()
	db, err := setupDatabase(ctx, &cfg.Database)
	if err != nil {
		log.Fatalf("Failed to setup database: %v", err)
	}
	defer db.Close()

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

func setupDatabase(ctx context.Context, cfg *config.DatabaseConfig) (*database.DB, error) {
	log.Printf("Connecting to database %s@%s:%d/%s...", cfg.User, cfg.Host, cfg.Port, cfg.Database)

	// Build database URL with URL-encoded password
	databaseURL := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, url.QueryEscape(cfg.Password), cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode)

	db, err := database.NewConnection(ctx, &database.Config{
		URL:            databaseURL,
		MaxConnections: cfg.MaxConnections,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Convert pgxpool to *sql.DB for migrations
	stdDB := stdlib.OpenDBFromPool(db.Pool)

	// Run database migrations automatically
	log.Printf("Running database migrations...")
	if err := runMigrations(stdDB); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	log.Printf("Database migrations completed successfully")

	return db, nil
}

func runMigrations(stdDB *sql.DB) error {
	return database.RunMigrations(stdDB, "./migrations")
}
