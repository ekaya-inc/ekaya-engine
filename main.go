package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/handlers"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
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

	// Initialize zap logger
	var logger *zap.Logger
	if cfg.Env == "local" {
		logger, err = zap.NewDevelopment()
	} else {
		logger, err = zap.NewProduction()
	}
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	// Log startup configuration
	logger.Info("Configuration loaded",
		zap.String("env", cfg.Env),
		zap.String("base_url", cfg.BaseURL),
		zap.Bool("auth_verification", cfg.Auth.EnableVerification),
		zap.String("auth_server_url", cfg.AuthServerURL),
		zap.String("database", fmt.Sprintf("%s@%s:%d/%s", cfg.Database.User, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)),
		zap.String("redis", fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port)),
	)

	// Initialize OAuth session store
	auth.InitSessionStore()

	// Initialize JWKS client for JWT validation
	jwksClient, err := auth.NewJWKSClient(&auth.JWKSConfig{
		EnableVerification: cfg.Auth.EnableVerification,
		JWKSEndpoints:      cfg.Auth.JWKSEndpoints,
	})
	if err != nil {
		logger.Fatal("Failed to initialize JWKS client", zap.Error(err))
	}
	defer jwksClient.Close()

	// Create auth service and middleware
	authService := auth.NewAuthService(jwksClient, logger)
	authMiddleware := auth.NewMiddleware(authService, logger)

	// Create OAuth service
	oauthService := services.NewOAuthService(&services.OAuthConfig{
		BaseURL:       cfg.BaseURL,
		ClientID:      cfg.OAuth.ClientID,
		AuthServerURL: cfg.AuthServerURL,
		JWKSEndpoints: cfg.Auth.JWKSEndpoints,
	}, logger)

	// Connect to database
	ctx := context.Background()
	db, err := setupDatabase(ctx, &cfg.Database)
	if err != nil {
		logger.Fatal("Failed to setup database", zap.Error(err))
	}
	defer db.Close()

	// Connect to Redis (optional - returns nil if not configured)
	redisClient, err := database.NewRedisClient(&cfg.Redis)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	if redisClient != nil {
		defer func() { _ = redisClient.Close() }()
		logger.Info("Redis connected")
	} else {
		logger.Info("Redis not configured, caching disabled")
	}

	// Create repositories
	projectRepo := repositories.NewProjectRepository()
	userRepo := repositories.NewUserRepository()

	// Create services
	projectService := services.NewProjectService(db, projectRepo, userRepo, redisClient, cfg.BaseURL, logger)
	userService := services.NewUserService(userRepo, logger)

	mux := http.NewServeMux()

	// Register health handler
	healthHandler := handlers.NewHealthHandler(cfg)
	healthHandler.RegisterRoutes(mux)

	// Register auth handler (public - no auth required)
	authHandler := handlers.NewAuthHandler(oauthService, cfg, logger)
	authHandler.RegisterRoutes(mux)

	// Register config handler (public - no auth required)
	configHandler := handlers.NewConfigHandler(cfg, logger)
	configHandler.RegisterRoutes(mux)

	// Register well-known endpoints (public - no auth required)
	wellKnownHandler := handlers.NewWellKnownHandler(cfg, logger)
	wellKnownHandler.RegisterRoutes(mux)

	// Register AI config stub handler (must be before projects handler)
	// These stubs prevent /api/projects/ai-config from matching GET /api/projects/{pid}
	aiConfigStubHandler := handlers.NewAIConfigStubHandler(logger)
	aiConfigStubHandler.RegisterRoutes(mux, authMiddleware)

	// Create tenant middleware once for all handlers that need it
	tenantMiddleware := database.WithTenantContext(db, logger)

	// Register projects handler (includes provisioning via POST /projects)
	projectsHandler := handlers.NewProjectsHandler(projectService, logger)
	projectsHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register users handler (protected)
	usersHandler := handlers.NewUsersHandler(userService, logger)
	usersHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Serve static UI files from ui/dist with SPA routing
	uiDir := "./ui/dist"
	fileServer := http.FileServer(http.Dir(uiDir))

	// Handle SPA routing - serve index.html for non-API routes when file doesn't exist
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Don't serve index.html for API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Check if the file exists
		path := filepath.Join(uiDir, r.URL.Path)
		if _, err := os.Stat(path); err == nil {
			// File exists, serve it
			fileServer.ServeHTTP(w, r)
			return
		}

		// File doesn't exist, serve index.html for SPA routing
		indexPath := filepath.Join(uiDir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			http.ServeFile(w, r, indexPath)
		} else {
			http.NotFound(w, r)
		}
	})

	logger.Info("Starting ekaya-engine",
		zap.String("port", cfg.Port),
		zap.String("version", cfg.Version))
	if err := http.ListenAndServe(":"+cfg.Port, mux); err != nil {
		logger.Fatal("Server failed", zap.Error(err))
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
