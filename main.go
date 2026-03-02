package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/mssql"    // Register mssql adapter
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres" // Register postgres adapter
	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/central"
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
	"github.com/ekaya-inc/ekaya-engine/ui"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver for database/sql (migrations)
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
		logger, err = zap.NewDevelopment(zap.AddStacktrace(zap.DPanicLevel))
	} else {
		logger, err = zap.NewProduction(zap.AddStacktrace(zap.DPanicLevel))
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
		zap.String("engine_database", fmt.Sprintf("%s@%s:%d/%s", cfg.EngineDatabase.User, cfg.EngineDatabase.Host, cfg.EngineDatabase.Port, cfg.EngineDatabase.Database)),
	)

	// Validate required credentials key (fail fast)
	if cfg.ProjectCredentialsKey == "" {
		logger.Fatal("project_credentials_key is required in config.yaml. Generate with: openssl rand -base64 32")
	}
	credentialEncryptor, err := crypto.NewCredentialEncryptor(cfg.ProjectCredentialsKey)
	if err != nil {
		logger.Fatal("Failed to initialize credential encryptor", zap.Error(err))
	}

	// Initialize OAuth session store
	auth.InitSessionStore(cfg.OAuthSessionSecret)

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
	db, err := setupDatabase(ctx, &cfg.EngineDatabase, logger)
	if err != nil {
		logger.Fatal("Failed to setup database", zap.Error(err))
	}
	defer db.Close()

	// Create repositories
	projectRepo := repositories.NewProjectRepository()
	userRepo := repositories.NewUserRepository()
	datasourceRepo := repositories.NewDatasourceRepository()
	schemaRepo := repositories.NewSchemaRepository()
	queryRepo := repositories.NewQueryRepository()
	aiConfigRepo := repositories.NewAIConfigRepository(credentialEncryptor)

	// MCP config repository
	mcpConfigRepo := repositories.NewMCPConfigRepository()

	// Installed apps repository
	installedAppRepo := repositories.NewInstalledAppRepository()

	// Agent API key service (needed for MCP auth middleware)
	agentAPIKeyService := services.NewAgentAPIKeyService(mcpConfigRepo, credentialEncryptor, logger)

	// Ontology repositories
	ontologyChatRepo := repositories.NewOntologyChatRepository()
	knowledgeRepo := repositories.NewKnowledgeRepository()
	ontologyQuestionRepo := repositories.NewOntologyQuestionRepository()
	ontologyDAGRepo := repositories.NewOntologyDAGRepository()
	pendingChangeRepo := repositories.NewPendingChangeRepository()
	columnMetadataRepo := repositories.NewColumnMetadataRepository()
	tableMetadataRepo := repositories.NewTableMetadataRepository()

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

	// Create ekaya-central client for fetching project info
	centralClient := central.NewClient(logger)

	// Create services
	nonceStore := services.NewNonceStore()
	installedAppService := services.NewInstalledAppService(installedAppRepo, centralClient, nonceStore, cfg.BaseURL, logger)
	projectService := services.NewProjectService(db, projectRepo, userRepo, mcpConfigRepo, agentAPIKeyService, installedAppService, centralClient, nonceStore, cfg.BaseURL, logger)
	userService := services.NewUserService(userRepo, logger)
	datasourceService := services.NewDatasourceService(datasourceRepo, credentialEncryptor, adapterFactory, projectService, logger)
	schemaService := services.NewSchemaService(schemaRepo, columnMetadataRepo, datasourceService, adapterFactory, logger)
	schemaChangeDetectionService := services.NewSchemaChangeDetectionService(pendingChangeRepo, schemaRepo, logger)
	dataChangeDetectionService := services.NewDataChangeDetectionService(schemaRepo, columnMetadataRepo, pendingChangeRepo, datasourceService, projectService, adapterFactory, logger)
	queryService := services.NewQueryService(queryRepo, datasourceService, adapterFactory, securityAuditor, logger)
	queryHistoryRepo := repositories.NewQueryHistoryRepository()
	queryHistoryService := services.NewQueryHistoryService(queryHistoryRepo, logger)
	aiConfigService := services.NewAIConfigService(aiConfigRepo, &cfg.CommunityAI, &cfg.EmbeddedAI, logger)
	mcpConfigService := services.NewMCPConfigService(mcpConfigRepo, queryService, projectService, installedAppService, cfg.BaseURL, logger)

	// LLM factory for creating clients per project configuration
	llmFactory := llm.NewClientFactory(aiConfigService, logger)

	// Ontology services
	knowledgeService := services.NewKnowledgeService(knowledgeRepo, projectRepo, logger)
	ontologyBuilderService := services.NewOntologyBuilderService(llmFactory, logger)
	ontologyQuestionService := services.NewOntologyQuestionService(
		ontologyQuestionRepo, columnMetadataRepo, schemaRepo, knowledgeRepo,
		ontologyBuilderService, logger)
	getTenantCtx := services.NewTenantContextFunc(db)

	// Set up LLM conversation recording for debugging
	convRepo := repositories.NewConversationRepository()
	convRecorder := llm.NewAsyncConversationRecorder(convRepo, llm.TenantContextFunc(getTenantCtx), logger, 100)
	llmFactory.SetRecorder(convRecorder)

	ontologyChatService := services.NewOntologyChatService(
		ontologyChatRepo, knowledgeRepo,
		schemaRepo, ontologyDAGRepo, projectService,
		llmFactory, datasourceService, adapterFactory, logger)
	deterministicRelationshipService := services.NewDeterministicRelationshipService(
		datasourceService, projectService, adapterFactory, schemaRepo, columnMetadataRepo, logger)
	ontologyFinalizationService := services.NewOntologyFinalizationService(
		projectRepo, schemaRepo, columnMetadataRepo, convRepo, llmFactory, getTenantCtx, logger)
	ontologyContextService := services.NewOntologyContextService(
		schemaRepo, columnMetadataRepo, tableMetadataRepo, projectService, logger)

	// Create worker pool for parallel LLM calls
	workerPoolConfig := llm.DefaultWorkerPoolConfig()
	llmWorkerPool := llm.NewWorkerPool(workerPoolConfig, logger)

	// Create circuit breaker for LLM resilience
	circuitBreakerConfig := llm.DefaultCircuitBreakerConfig()
	llmCircuitBreaker := llm.NewCircuitBreaker(circuitBreakerConfig)

	columnEnrichmentService := services.NewColumnEnrichmentService(
		schemaRepo, columnMetadataRepo, convRepo, projectRepo, ontologyQuestionService,
		datasourceService, adapterFactory, llmFactory, llmWorkerPool, llmCircuitBreaker, getTenantCtx, logger)
	glossaryRepo := repositories.NewGlossaryRepository()
	glossaryService := services.NewGlossaryService(glossaryRepo, columnMetadataRepo, knowledgeRepo, schemaRepo, projectService, datasourceService, adapterFactory, llmFactory, getTenantCtx, logger, cfg.Env)

	// Ontology DAG service for orchestrated workflow execution
	ontologyDAGService := services.NewOntologyDAGService(
		ontologyDAGRepo, schemaRepo,
		ontologyQuestionRepo, ontologyChatRepo, knowledgeRepo,
		glossaryRepo, getTenantCtx, logger)

	// Wire DAG adapters using setter pattern (avoids import cycles)
	knowledgeSeedingService := services.NewKnowledgeSeedingService(knowledgeService, schemaService, llmFactory, logger)
	ontologyDAGService.SetKnowledgeSeedingMethods(knowledgeSeedingService)
	columnFeatureExtractionService := services.NewColumnFeatureExtractionServiceFull(
		schemaRepo, columnMetadataRepo, datasourceService, adapterFactory, llmFactory, llmWorkerPool, getTenantCtx,
		ontologyQuestionService, logger)
	ontologyDAGService.SetColumnFeatureExtractionMethods(columnFeatureExtractionService)
	ontologyDAGService.SetFKDiscoveryMethods(services.NewFKDiscoveryAdapter(deterministicRelationshipService))
	ontologyDAGService.SetPKMatchDiscoveryMethods(services.NewPKMatchDiscoveryAdapter(deterministicRelationshipService))
	// LLM-validated relationship discovery (replaces threshold-based PKMatch when configured)
	relationshipCandidateCollector := services.NewRelationshipCandidateCollector(
		schemaRepo, columnMetadataRepo, adapterFactory, datasourceService, logger)
	relationshipValidator := services.NewRelationshipValidator(
		llmFactory, llmWorkerPool, llmCircuitBreaker, convRepo, getTenantCtx, logger)
	llmRelationshipDiscoveryService := services.NewLLMRelationshipDiscoveryService(
		relationshipCandidateCollector, relationshipValidator, datasourceService, adapterFactory,
		schemaRepo, columnMetadataRepo, logger)
	ontologyDAGService.SetLLMRelationshipDiscoveryMethods(services.NewLLMRelationshipDiscoveryAdapter(llmRelationshipDiscoveryService))
	ontologyDAGService.SetFinalizationMethods(services.NewOntologyFinalizationAdapter(ontologyFinalizationService))
	ontologyDAGService.SetColumnEnrichmentMethods(services.NewColumnEnrichmentAdapter(columnEnrichmentService))
	ontologyDAGService.SetGlossaryDiscoveryMethods(services.NewGlossaryDiscoveryAdapter(glossaryService))
	ontologyDAGService.SetGlossaryEnrichmentMethods(services.NewGlossaryEnrichmentAdapter(glossaryService))
	tableFeatureExtractionSvc := services.NewTableFeatureExtractionService(
		schemaRepo, columnMetadataRepo, tableMetadataRepo, llmFactory, llmWorkerPool, getTenantCtx, logger)
	ontologyDAGService.SetTableFeatureExtractionMethods(tableFeatureExtractionSvc)

	// Incremental DAG service for targeted LLM enrichment after changes
	// Created first without ChangeReviewService due to circular dependency
	incrementalDAGService := services.NewIncrementalDAGService(&services.IncrementalDAGServiceDeps{
		ColumnMetadataRepo: columnMetadataRepo,
		SchemaRepo:         schemaRepo,
		ConversationRepo:   convRepo,
		AIConfigSvc:        aiConfigService,
		LLMFactory:         llmFactory,
		ChangeReviewSvc:    nil, // Will be set after ChangeReviewService is created
		GetTenantCtx:       getTenantCtx,
		Logger:             logger,
	})

	// Change review service for approving/rejecting pending ontology changes
	changeReviewService := services.NewChangeReviewService(&services.ChangeReviewServiceDeps{
		PendingChangeRepo:  pendingChangeRepo,
		ColumnMetadataRepo: columnMetadataRepo,
		IncrementalDAG:     incrementalDAGService,
		Logger:             logger,
	})

	// Wire up the circular dependency: IncrementalDAGService needs ChangeReviewService for precedence checks
	incrementalDAGService.SetChangeReviewService(changeReviewService)

	mux := http.NewServeMux()

	// Register health handler
	healthHandler := handlers.NewHealthHandler(cfg, connManager, logger)
	healthHandler.RegisterRoutes(mux)

	// Register auth handler (public - no auth required)
	authHandler := handlers.NewAuthHandler(oauthService, projectService, cfg, logger)
	authHandler.RegisterRoutes(mux)
	mux.HandleFunc("GET /api/auth/me", authMiddleware.RequireAuth(authHandler.GetMe))

	// Register config handler (public - no auth required)
	configHandler := handlers.NewConfigHandler(cfg, adapterFactory, logger)
	configHandler.RegisterRoutes(mux)

	// Register project config handler (authenticated - project-scoped config)
	projectConfigHandler := handlers.NewProjectConfigHandler(cfg, logger)
	projectConfigHandler.RegisterRoutes(mux, authMiddleware)

	// Register well-known endpoints (public - no auth required)
	wellKnownHandler := handlers.NewWellKnownHandler(cfg, projectService, logger)
	wellKnownHandler.RegisterRoutes(mux)

	// Register MCP server (authenticated - project-scoped)
	mcpToolDeps := &mcptools.MCPToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		DatasourceService:            datasourceService,
		SchemaService:                schemaService,
		ProjectService:               projectService,
		AdapterFactory:               adapterFactory,
		SchemaChangeDetectionService: schemaChangeDetectionService,
		DataChangeDetectionService:   dataChangeDetectionService,
		ChangeReviewService:          changeReviewService,
		PendingChangeRepo:            pendingChangeRepo,
		Auditor:                      securityAuditor, // For SIEM logging of modifying queries
		QueryHistoryService:          queryHistoryService,
	}
	mcpAuditLogger := mcp.NewAuditLogger(db, logger)
	mcpServer := mcp.NewServer("ekaya-engine", cfg.Version, logger,
		mcp.WithToolFilter(mcptools.NewToolFilter(mcpToolDeps)),
		mcp.WithHooks(mcpAuditLogger.Hooks()),
	)
	mcptools.RegisterHealthTool(mcpServer.MCP(), cfg.Version, &mcptools.HealthToolDeps{
		DB:                db,
		ProjectService:    projectService,
		DatasourceService: datasourceService,
		Logger:            logger,
	})
	mcptools.RegisterMCPTools(mcpServer.MCP(), mcpToolDeps)

	// Register approved queries tools (separate tool group from developer tools)
	queryToolDeps := &mcptools.QueryToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		ProjectService:      projectService,
		QueryService:        queryService,
		Auditor:             securityAuditor,
		QueryHistoryService: queryHistoryService,
	}
	mcptools.RegisterApprovedQueriesTools(mcpServer.MCP(), queryToolDeps)

	// Register dev query tools for administrators to manage query suggestions
	// (approve/reject, create/update/delete queries directly)
	devQueryToolDeps := &mcptools.DevQueryToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		ProjectService: projectService,
		QueryService:   queryService,
	}
	mcptools.RegisterDevQueryTools(mcpServer.MCP(), devQueryToolDeps)

	// Register schema tools for entity/role semantic context
	schemaToolDeps := &mcptools.SchemaToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		ProjectService: projectService,
		SchemaService:  schemaService,
	}
	mcptools.RegisterSchemaTools(mcpServer.MCP(), schemaToolDeps)

	// Register ontology tools for progressive semantic disclosure
	ontologyToolDeps := &mcptools.OntologyToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		ProjectService:         projectService,
		OntologyContextService: ontologyContextService,
		SchemaRepo:             schemaRepo,
	}
	mcptools.RegisterOntologyTools(mcpServer.MCP(), ontologyToolDeps)

	mcpHandler := handlers.NewMCPHandler(mcpServer, logger, cfg.MCP)
	tenantScopeProvider := database.NewTenantScopeProvider(db)
	mcpAuthMiddleware := mcpauth.NewMiddleware(authService, agentAPIKeyService, tenantScopeProvider, logger,
		mcpauth.WithAuditLogger(mcpAuditLogger),
	)
	mcpHandler.RegisterRoutes(mux, mcpAuthMiddleware)

	// Register MCP OAuth token endpoint (public - for MCP clients)
	// Pass projectService for looking up project-specific auth server URLs
	mcpOAuthHandler := handlers.NewMCPOAuthHandler(oauthService, projectService, logger)
	mcpOAuthHandler.RegisterRoutes(mux)

	// Create tenant middleware once for all handlers that need it
	tenantMiddleware := database.WithTenantContext(db, logger)

	// Register AI config handler (protected)
	connectionTester := llm.NewConnectionTester()
	aiConfigHandler := handlers.NewAIConfigHandler(aiConfigService, connectionTester, cfg, logger)
	aiConfigHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register projects handler (includes provisioning via POST /projects)
	projectsHandler := handlers.NewProjectsHandler(projectService, cfg, logger)
	projectsHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register users handler (protected)
	usersHandler := handlers.NewUsersHandler(userService, logger)
	usersHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register datasources handler (protected)
	datasourcesHandler := handlers.NewDatasourcesHandler(datasourceService, logger)
	datasourcesHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register schema handler (protected)
	schemaHandler := handlers.NewSchemaHandler(schemaService, schemaChangeDetectionService, logger)
	schemaHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register queries handler (protected)
	queriesHandler := handlers.NewQueriesHandler(queryService, logger)
	queriesHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register MCP config handler (protected)
	mcpConfigHandler := handlers.NewMCPConfigHandler(mcpConfigService, logger)
	mcpConfigHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register agent API key handler (protected)
	agentAPIKeyHandler := handlers.NewAgentAPIKeyHandler(agentAPIKeyService, logger)
	agentAPIKeyHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register ontology handlers (protected)
	ontologyQuestionsHandler := handlers.NewOntologyQuestionsHandler(ontologyQuestionService, logger)
	ontologyQuestionsHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	ontologyChatHandler := handlers.NewOntologyChatHandler(ontologyChatService, knowledgeService, logger)
	ontologyChatHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register ontology DAG handler (protected) - unified workflow execution
	ontologyDAGHandler := handlers.NewOntologyDAGHandler(ontologyDAGService, projectService, logger)
	ontologyDAGHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register ontology enrichment handler (protected) - read-only tiered ontology for UI
	ontologyEnrichmentHandler := handlers.NewOntologyEnrichmentHandler(schemaService, projectService, logger)
	ontologyEnrichmentHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register glossary handler (protected) - business glossary for MCP clients
	glossaryHandler := handlers.NewGlossaryHandler(glossaryService, ontologyQuestionService, logger)
	glossaryHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register knowledge handler (protected) - project knowledge facts
	knowledgeParsingService := services.NewKnowledgeParsingService(knowledgeService, llmFactory, logger)
	knowledgeHandler := handlers.NewKnowledgeHandler(knowledgeService, knowledgeParsingService, logger)
	knowledgeHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register installed apps handler (protected) - application installation tracking
	installedAppHandler := handlers.NewInstalledAppHandler(installedAppService, logger)
	installedAppHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register audit page handler (protected) - audit visibility UI
	auditPageRepo := repositories.NewAuditPageRepository()
	mcpAuditRepo := repositories.NewMCPAuditRepository()
	auditPageService := services.NewAuditPageService(auditPageRepo, mcpAuditRepo, logger)
	auditPageHandler := handlers.NewAuditPageHandler(auditPageService, logger)
	auditPageHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register alert handler (protected) - audit alerts management
	alertRepo := repositories.NewAlertRepository()
	alertService := services.NewAlertService(alertRepo, logger)
	alertHandler := handlers.NewAlertHandler(alertService, mcpConfigRepo, logger)
	alertHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Wire alert trigger detection into the MCP audit pipeline
	alertTriggerRepo := repositories.NewAlertTriggerRepository()
	alertTriggerService := services.NewAlertTriggerService(alertRepo, alertTriggerRepo, mcpConfigRepo, logger)
	mcpAuditLogger.SetAlertTrigger(alertTriggerService)

	// Create retention service and start scheduler for auto-pruning old audit/history data
	retentionService := services.NewRetentionService(db, queryHistoryRepo, mcpAuditRepo, mcpConfigRepo, logger)
	retentionCtx, retentionCancel := context.WithCancel(ctx)
	retentionService.RunScheduler(retentionCtx, 24*time.Hour)

	// Register retention config handler (protected) - audit retention configuration
	retentionHandler := handlers.NewRetentionHandler(mcpConfigRepo, logger)
	retentionHandler.RegisterRoutes(mux, authMiddleware, tenantMiddleware)

	// Register glossary MCP tools (uses glossaryService for get_glossary tool)
	glossaryToolDeps := &mcptools.GlossaryToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		GlossaryService: glossaryService,
	}
	mcptools.RegisterGlossaryTools(mcpServer.MCP(), glossaryToolDeps)

	// Register project knowledge MCP tools (for storing domain facts)
	knowledgeToolDeps := &mcptools.KnowledgeToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		KnowledgeRepository: knowledgeRepo,
	}
	mcptools.RegisterKnowledgeTools(mcpServer.MCP(), knowledgeToolDeps)

	// Register unified context tool (consolidates ontology, schema, and glossary)
	contextToolDeps := &mcptools.ContextToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		ProjectService:         projectService,
		OntologyContextService: ontologyContextService,
		SchemaService:          schemaService,
		GlossaryService:        glossaryService,
		SchemaRepo:             schemaRepo,
		ColumnMetadataRepo:     columnMetadataRepo,
		TableMetadataRepo:      tableMetadataRepo,
		KnowledgeRepo:          knowledgeRepo,
	}
	mcptools.RegisterContextTools(mcpServer.MCP(), contextToolDeps)

	// Register column metadata tools (for updating column semantic information)
	columnToolDeps := &mcptools.ColumnToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		SchemaRepo:         schemaRepo,
		ColumnMetadataRepo: columnMetadataRepo,
		ProjectService:     projectService,
	}
	mcptools.RegisterColumnTools(mcpServer.MCP(), columnToolDeps)
	mcptools.RegisterBatchTools(mcpServer.MCP(), columnToolDeps)

	// Register table metadata tools (for updating table-level semantic information)
	tableToolDeps := &mcptools.TableToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		SchemaRepo:        schemaRepo,
		TableMetadataRepo: tableMetadataRepo,
		ProjectService:    projectService,
	}
	mcptools.RegisterTableTools(mcpServer.MCP(), tableToolDeps)

	// Register column probe tools (for deep-diving into column statistics and semantics)
	probeToolDeps := &mcptools.ProbeToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		SchemaRepo:         schemaRepo,
		ColumnMetadataRepo: columnMetadataRepo,
		ProjectService:     projectService,
	}
	mcptools.RegisterProbeTools(mcpServer.MCP(), probeToolDeps)

	// Register search tools (for full-text search across schema and ontology)
	searchToolDeps := &mcptools.SearchToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		SchemaRepo: schemaRepo,
	}
	mcptools.RegisterSearchTools(mcpServer.MCP(), searchToolDeps)

	// Register ontology question tools (for listing and managing questions)
	questionToolDeps := &mcptools.QuestionToolDeps{
		BaseMCPToolDeps: mcptools.BaseMCPToolDeps{
			DB:                  db,
			MCPConfigService:    mcpConfigService,
			Logger:              logger,
			InstalledAppService: installedAppService,
		},
		QuestionRepo: ontologyQuestionRepo,
	}
	mcptools.RegisterQuestionTools(mcpServer.MCP(), questionToolDeps)

	// Serve static UI files with SPA routing.
	// In debug mode DistFS() returns os.DirFS("ui") (live disk reads),
	// in production it returns the compile-time embedded filesystem.
	uiFS, err := fs.Sub(ui.DistFS(), "dist")
	if err != nil {
		logger.Fatal("Failed to create UI filesystem", zap.Error(err))
	}
	fileServer := http.FileServer(http.FS(uiFS))

	// Handle SPA routing - serve index.html for non-API routes when file doesn't exist
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Don't serve index.html for API routes
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Check if the file exists in the UI filesystem
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if _, err := fs.Stat(uiFS, path); err == nil {
			// File exists, serve it
			fileServer.ServeHTTP(w, r)
			return
		}

		// File doesn't exist, serve index.html for SPA routing
		// Read fresh each time so debug mode picks up changes from disk
		indexHTML, err := fs.ReadFile(uiFS, "index.html")
		if err != nil {
			http.Error(w, "index.html not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})

	// Wrap mux with request logging middleware
	handler := middleware.RequestLogger(logger)(mux)

	// Create HTTP server
	server := &http.Server{
		Addr:    cfg.BindAddr + ":" + cfg.Port,
		Handler: handler,
	}

	// Configure TLS with minimum version 1.2 for security
	if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		server.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
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

		// 2. Stop retention scheduler
		retentionCancel()

		// 3. Shutdown DAG service (cancels DAGs, releases ownership)
		if err := ontologyDAGService.Shutdown(shutdownCtx); err != nil {
			logger.Error("DAG service shutdown error", zap.Error(err))
		}

		// 4. Close conversation recorder (drain pending writes)
		convRecorder.Close()

		close(shutdownComplete)
	}()

	// Start server
	if cfg.TLSCertPath != "" && cfg.TLSKeyPath != "" {
		logger.Info("Starting HTTPS server",
			zap.String("addr", cfg.BindAddr+":"+cfg.Port),
			zap.String("version", cfg.Version),
			zap.String("cert", cfg.TLSCertPath),
			zap.String("key", cfg.TLSKeyPath))
		err = server.ListenAndServeTLS(cfg.TLSCertPath, cfg.TLSKeyPath)
	} else {
		logger.Info("Starting HTTP server",
			zap.String("addr", cfg.BindAddr+":"+cfg.Port),
			zap.String("version", cfg.Version))
		err = server.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server failed", zap.Error(err))
	}

	// Wait for shutdown to complete
	<-shutdownComplete
	logger.Info("Server shutdown complete")
}

func setupDatabase(ctx context.Context, cfg *config.EngineDatabaseConfig, logger *zap.Logger) (*database.DB, error) {
	logger.Info("Connecting to database",
		zap.String("user", cfg.User),
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Database))

	// Build database URL with URL-encoded password
	databaseURL := fmt.Sprintf("postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User, url.QueryEscape(cfg.Password), cfg.Host, cfg.Port, cfg.Database, cfg.SSLMode)

	// Run database migrations first, using a separate connection with timeout.
	// This avoids the hang issue with stdlib.OpenDBFromPool + golang-migrate.
	logger.Info("Running database migrations")
	if err := runMigrations(databaseURL, logger); err != nil {
		return nil, err // Error already formatted with helpful guidance
	}
	logger.Info("Database migrations completed successfully")

	// Now establish the main connection pool
	db, err := database.NewConnection(ctx, &database.Config{
		URL:            databaseURL,
		MaxConnections: cfg.MaxConnections,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Check if connection has superuser privileges (bypasses RLS)
	var isSuperuser bool
	err = db.QueryRow(ctx, "SELECT rolsuper OR rolbypassrls FROM pg_roles WHERE rolname = current_user").Scan(&isSuperuser)
	if err == nil && isSuperuser {
		logger.Warn("Engine database connection has superuser or bypassrls privileges. " +
			"Row-Level Security (RLS) policies will be bypassed, which means tenant isolation " +
			"relies solely on application-level filtering. For production deployments, use a " +
			"non-superuser database role to ensure RLS enforcement.")
	}

	return db, nil
}

// migrationTimeout is the maximum time to wait for migrations to complete.
// This prevents indefinite hangs when the database user lacks schema permissions.
const migrationTimeout = 30 * time.Second

func runMigrations(databaseURL string, logger *zap.Logger) error {
	// Create a separate connection for migrations with statement_timeout set in the URL.
	// This is critical: using stdlib.OpenDBFromPool + golang-migrate can hang indefinitely
	// on permission errors. A direct sql.Open connection with timeout avoids this issue.
	timeoutMS := int(migrationTimeout.Milliseconds())

	// Add statement_timeout to the connection URL
	separator := "&"
	if !strings.Contains(databaseURL, "?") {
		separator = "?"
	}
	migrationURL := fmt.Sprintf("%s%sstatement_timeout=%d", databaseURL, separator, timeoutMS)

	db, err := sql.Open("pgx", migrationURL)
	if err != nil {
		return formatMigrationError(fmt.Errorf("failed to open migration connection: %w", err))
	}
	defer db.Close()

	// Verify connection before running migrations
	if err := db.Ping(); err != nil {
		return formatMigrationError(fmt.Errorf("failed to connect for migrations: %w", err))
	}

	err = database.RunMigrations(db, logger)
	if err != nil {
		return formatMigrationError(err)
	}
	return nil
}

// formatMigrationError wraps migration errors with helpful guidance for common issues.
func formatMigrationError(err error) error {
	errStr := err.Error()

	// Check for permission denied errors
	if strings.Contains(errStr, "permission denied") {
		return fmt.Errorf(`failed to run migrations: %w

This error typically occurs when the database user lacks CREATE privileges on the public schema.

To fix, run as a PostgreSQL superuser:
    \c <your_database>
    GRANT ALL ON SCHEMA public TO <your_user>;

Note: In PostgreSQL 15+, 'GRANT ALL ON DATABASE' does NOT grant schema privileges.
You must explicitly grant permissions on the public schema.`, err)
	}

	// Check for timeout errors (indicates possible permission issues causing hang)
	if strings.Contains(errStr, "statement timeout") || strings.Contains(errStr, "canceling statement") {
		return fmt.Errorf(`failed to run migrations (timed out after %v): %w

Migration timed out, which often indicates insufficient database permissions.
The database user may lack CREATE privileges on the public schema.

To fix, run as a PostgreSQL superuser:
    \c <your_database>
    GRANT ALL ON SCHEMA public TO <your_user>;

Note: In PostgreSQL 15+, 'GRANT ALL ON DATABASE' does NOT grant schema privileges.
You must explicitly grant permissions on the public schema.`, migrationTimeout, err)
	}

	return fmt.Errorf("failed to run migrations: %w", err)
}
