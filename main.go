package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres" // Register postgres adapter
	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/handlers"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp"
	mcpauth "github.com/ekaya-inc/ekaya-engine/pkg/mcp/auth"
	mcptools "github.com/ekaya-inc/ekaya-engine/pkg/mcp/tools"
	"github.com/ekaya-inc/ekaya-engine/pkg/middleware"
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

	// Validate required credentials key (fail fast)
	if cfg.ProjectCredentialsKey == "" {
		logger.Fatal("PROJECT_CREDENTIALS_KEY environment variable is required. Generate with: openssl rand -base64 32")
	}
	credentialEncryptor, err := crypto.NewCredentialEncryptor(cfg.ProjectCredentialsKey)
	if err != nil {
		logger.Fatal("Failed to initialize credential encryptor", zap.Error(err))
	}

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
	db, err := setupDatabase(ctx, &cfg.Database, logger)
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
	datasourceRepo := repositories.NewDatasourceRepository()
	schemaRepo := repositories.NewSchemaRepository()
	queryRepo := repositories.NewQueryRepository()
	aiConfigRepo := repositories.NewAIConfigRepository(credentialEncryptor)

	// MCP config repository
	mcpConfigRepo := repositories.NewMCPConfigRepository()

	// Ontology repositories
	ontologyRepo := repositories.NewOntologyRepository()
	ontologyWorkflowRepo := repositories.NewOntologyWorkflowRepository()
	ontologyChatRepo := repositories.NewOntologyChatRepository()
	knowledgeRepo := repositories.NewKnowledgeRepository()
	workflowStateRepo := repositories.NewWorkflowStateRepository()
	ontologyQuestionRepo := repositories.NewOntologyQuestionRepository()
	relationshipCandidateRepo := repositories.NewRelationshipCandidateRepository()
	ontologyEntityRepo := repositories.NewOntologyEntityRepository()
	entityRelationshipRepo := repositories.NewEntityRelationshipRepository()

	// Create connection manager with config-driven settings
	connManagerCfg := datasource.ConnectionManagerConfig{
		TTLMinutes:            cfg.Datasource.ConnectionTTLMinutes,
		MaxConnectionsPerUser: cfg.Datasource.MaxConnectionsPerUser,
		PoolMaxConns:          cfg.Datasource.PoolMaxConns,
		PoolMinConns:          cfg.Datasource.PoolMinConns,
	}
	connManager := datasource.NewConnectionManager(connManagerCfg, logger)
	defer connManager.Close()

	// Create adapter factory for datasource connections
	adapterFactory := datasource.NewDatasourceAdapterFactory(connManager)

	// Create security auditor for SIEM logging
	securityAuditor := audit.NewSecurityAuditor(logger)

	// Create services
	projectService := services.NewProjectService(db, projectRepo, userRepo, redisClient, cfg.BaseURL, logger)
	userService := services.NewUserService(userRepo, logger)
	datasourceService := services.NewDatasourceService(datasourceRepo, credentialEncryptor, adapterFactory, projectService, logger)
	schemaService := services.NewSchemaService(schemaRepo, ontologyEntityRepo, datasourceService, adapterFactory, logger)
	discoveryService := services.NewRelationshipDiscoveryService(schemaRepo, datasourceService, adapterFactory, logger)
	queryService := services.NewQueryService(queryRepo, datasourceService, adapterFactory, securityAuditor, logger)
	aiConfigService := services.NewAIConfigService(aiConfigRepo, &cfg.CommunityAI, &cfg.EmbeddedAI, logger)
	mcpConfigService := services.NewMCPConfigService(mcpConfigRepo, queryService, projectService, cfg.BaseURL, logger)

	// LLM factory for creating clients per project configuration
	llmFactory := llm.NewClientFactory(aiConfigService, logger)

	// Ontology services
	knowledgeService := services.NewKnowledgeService(knowledgeRepo, logger)
	ontologyBuilderService := services.NewOntologyBuilderService(
		ontologyRepo, schemaRepo, ontologyWorkflowRepo,
		knowledgeRepo, workflowStateRepo, ontologyEntityRepo, entityRelationshipRepo,
		llmFactory, logger)
	ontologyQuestionService := services.NewOntologyQuestionService(
		ontologyQuestionRepo, ontologyRepo, knowledgeRepo,
		ontologyBuilderService, logger)
	getTenantCtx := services.NewTenantContextFunc(db)

	// Set up LLM conversation recording for debugging
	convRepo := repositories.NewConversationRepository()
	convRecorder := llm.NewAsyncConversationRecorder(convRepo, llm.TenantContextFunc(getTenantCtx), logger, 100)
	llmFactory.SetRecorder(convRecorder)

	ontologyWorkflowService := services.NewOntologyWorkflowService(
		ontologyWorkflowRepo, ontologyRepo, schemaRepo, workflowStateRepo, ontologyQuestionRepo,
		convRepo, datasourceService, adapterFactory, ontologyBuilderService, getTenantCtx, logger)
	ontologyChatService := services.NewOntologyChatService(
		ontologyChatRepo, ontologyRepo, knowledgeRepo,
		schemaRepo, ontologyWorkflowRepo, workflowStateRepo,
		ontologyEntityRepo, entityRelationshipRepo,
		llmFactory, datasourceService, adapterFactory, logger)
	deterministicRelationshipService := services.NewDeterministicRelationshipService(
		datasourceService, adapterFactory, ontologyRepo, ontologyEntityRepo, entityRelationshipRepo, schemaRepo)
	relationshipWorkflowService := services.NewRelationshipWorkflowService(
		ontologyWorkflowRepo, relationshipCandidateRepo, schemaRepo, workflowStateRepo, ontologyRepo, ontologyEntityRepo,
		datasourceService, adapterFactory, llmFactory, discoveryService, deterministicRelationshipService, getTenantCtx, logger)
	entityService := services.NewEntityService(ontologyEntityRepo, ontologyRepo, logger)
	entityDiscoveryService := services.NewEntityDiscoveryService(
		ontologyWorkflowRepo, ontologyEntityRepo, schemaRepo, ontologyRepo,
		datasourceService, adapterFactory, llmFactory, getTenantCtx, logger)
	ontologyContextService := services.NewOntologyContextService(
		ontologyRepo, ontologyEntityRepo, entityRelationshipRepo, schemaRepo, projectService, logger)

	mux := http.NewServeMux()

	// Register health handler
	healthHandler := handlers.NewHealthHandler(cfg, connManager, logger)
	healthHandler.RegisterRoutes(mux)

	// Register auth handler (public - no auth required)
	authHandler := handlers.NewAuthHandler(oauthService, cfg, logger)
	authHandler.RegisterRoutes(mux)

	// Register config handler (public - no auth required)
	configHandler := handlers.NewConfigHandler(cfg, adapterFactory, logger)
	configHandler.RegisterRoutes(mux)

	// Register project config handler (authenticated - project-scoped config)
	projectConfigHandler := handlers.NewProjectConfigHandler(cfg, logger)
	projectConfigHandler.RegisterRoutes(mux, authMiddleware)

	// Register well-known endpoints (public - no auth required)
	wellKnownHandler := handlers.NewWellKnownHandler(cfg, logger)
	wellKnownHandler.RegisterRoutes(mux)

	// Register MCP server (authenticated - project-scoped)
	developerToolDeps := &mcptools.DeveloperToolDeps{
		DB:                db,
		MCPConfigService:  mcpConfigService,
		DatasourceService: datasourceService,
		SchemaService:     schemaService,
		ProjectService:    projectService,
		AdapterFactory:    adapterFactory,
		Logger:            logger,
	}
	mcpServer := mcp.NewServer("ekaya-engine", cfg.Version, logger,
		mcp.WithToolFilter(mcptools.NewToolFilter(developerToolDeps)),
	)
	mcptools.RegisterHealthTool(mcpServer.MCP(), cfg.Version, &mcptools.HealthToolDeps{
		DB:                db,
		ProjectService:    projectService,
		DatasourceService: datasourceService,
		Logger:            logger,
	})
	mcptools.RegisterDeveloperTools(mcpServer.MCP(), developerToolDeps)

	// Register approved queries tools (separate tool group from developer tools)
	queryToolDeps := &mcptools.QueryToolDeps{
		DB:               db,
		MCPConfigService: mcpConfigService,
		ProjectService:   projectService,
		QueryService:     queryService,
		Logger:           logger,
	}
	mcptools.RegisterApprovedQueriesTools(mcpServer.MCP(), queryToolDeps)

	// Register schema tools for entity/role semantic context
	schemaToolDeps := &mcptools.SchemaToolDeps{
		DB:             db,
		ProjectService: projectService,
		SchemaService:  schemaService,
		Logger:         logger,
	}
	mcptools.RegisterSchemaTools(mcpServer.MCP(), schemaToolDeps)

	// Register ontology tools for progressive semantic disclosure
	ontologyToolDeps := &mcptools.OntologyToolDeps{
		DB:                     db,
		MCPConfigService:       mcpConfigService,
		ProjectService:         projectService,
		OntologyContextService: ontologyContextService,
		OntologyRepo:           ontologyRepo,
		EntityRepo:             ontologyEntityRepo,
		SchemaRepo:             schemaRepo,
		Logger:                 logger,
	}
	mcptools.RegisterOntologyTools(mcpServer.MCP(), ontologyToolDeps)

	mcpHandler := handlers.NewMCPHandler(mcpServer, logger)
	mcpAuthMiddleware := mcpauth.NewMiddleware(authService, logger)
	mcpHandler.RegisterRoutes(mux, mcpAuthMiddleware)

	// Register MCP OAuth token endpoint (public - for MCP clients)
	mcpOAuthHandler := handlers.NewMCPOAuthHandler(oauthService, logger)
	mcpOAuthHandler.RegisterRoutes(mux)

	// Create tenant middleware once for all handlers that need it
	tenantMiddleware := database.WithTenantContext(db, logger)

	// Register AI config handler (protected)
	connectionTester := llm.NewConnectionTester()
	aiConfigHandler := handlers.NewAIConfigHandler(aiConfigService, connectionTester, cfg, logger)
	aiConfigHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register projects handler (includes provisioning via POST /projects)
	projectsHandler := handlers.NewProjectsHandler(projectService, logger)
	projectsHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register users handler (protected)
	usersHandler := handlers.NewUsersHandler(userService, logger)
	usersHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register datasources handler (protected)
	datasourcesHandler := handlers.NewDatasourcesHandler(datasourceService, logger)
	datasourcesHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register schema handler (protected, includes discovery)
	schemaHandler := handlers.NewSchemaHandlerWithDiscovery(schemaService, discoveryService, logger)
	schemaHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register queries handler (protected)
	queriesHandler := handlers.NewQueriesHandler(queryService, logger)
	queriesHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register MCP config handler (protected)
	mcpConfigHandler := handlers.NewMCPConfigHandler(mcpConfigService, logger)
	mcpConfigHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register ontology handlers (protected)
	ontologyHandler := handlers.NewOntologyHandler(ontologyWorkflowService, projectService, logger)
	ontologyHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	ontologyQuestionsHandler := handlers.NewOntologyQuestionsHandler(ontologyQuestionService, logger)
	ontologyQuestionsHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	ontologyChatHandler := handlers.NewOntologyChatHandler(ontologyChatService, knowledgeService, logger)
	ontologyChatHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register relationship workflow handler (protected)
	relationshipWorkflowHandler := handlers.NewRelationshipWorkflowHandler(
		relationshipWorkflowService, logger)
	relationshipWorkflowHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register entity handler (protected)
	entityHandler := handlers.NewEntityHandler(entityService, logger)
	entityHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register entity discovery handler (protected)
	entityDiscoveryHandler := handlers.NewEntityDiscoveryHandler(entityDiscoveryService, logger)
	entityDiscoveryHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register entity relationship handler (protected)
	entityRelationshipHandler := handlers.NewEntityRelationshipHandler(
		deterministicRelationshipService, logger)
	entityRelationshipHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

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

	// Wrap mux with request logging middleware
	handler := middleware.RequestLogger(logger)(mux)

	// Create HTTP server
	server := &http.Server{
		Addr:    cfg.BindAddr + ":" + cfg.Port,
		Handler: handler,
	}

	// Channel to signal shutdown complete
	shutdownComplete := make(chan struct{})

	// Handle shutdown signals
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan

		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

		// Create shutdown context with 30 second timeout
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// 1. Stop accepting new HTTP requests
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("HTTP server shutdown error", zap.Error(err))
		}

		// 2. Shutdown workflow service (cancels tasks, releases ownership)
		if err := ontologyWorkflowService.Shutdown(shutdownCtx); err != nil {
			logger.Error("Workflow service shutdown error", zap.Error(err))
		}

		// 3. Close conversation recorder (drain pending writes)
		convRecorder.Close()

		close(shutdownComplete)
	}()

	// Start server
	logger.Info("Starting ekaya-engine",
		zap.String("addr", cfg.BindAddr+":"+cfg.Port),
		zap.String("version", cfg.Version))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server failed", zap.Error(err))
	}

	// Wait for shutdown to complete
	<-shutdownComplete
	logger.Info("Server shutdown complete")
}

func setupDatabase(ctx context.Context, cfg *config.DatabaseConfig, logger *zap.Logger) (*database.DB, error) {
	logger.Info("Connecting to database",
		zap.String("user", cfg.User),
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database))

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
	logger.Info("Running database migrations")
	if err := runMigrations(stdDB, logger); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	logger.Info("Database migrations completed successfully")

	return db, nil
}

func runMigrations(stdDB *sql.DB, logger *zap.Logger) error {
	return database.RunMigrations(stdDB, "./migrations", logger)
}
