package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workflow"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// EntityDiscoveryService provides operations for standalone entity discovery workflows.
// This is separate from relationship detection and can be run independently.
type EntityDiscoveryService interface {
	// StartDiscovery initiates a new entity discovery workflow.
	StartDiscovery(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetStatus returns the current entity discovery workflow status for a datasource.
	GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetStatusWithCounts returns workflow status with entity counts.
	GetStatusWithCounts(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *EntityDiscoveryCounts, error)

	// Cancel cancels a running entity discovery workflow.
	Cancel(ctx context.Context, workflowID uuid.UUID) error

	// Shutdown gracefully stops all active workflows owned by this server.
	Shutdown(ctx context.Context) error
}

// EntityDiscoveryCounts holds counts of discovered entities.
type EntityDiscoveryCounts struct {
	EntityCount     int
	OccurrenceCount int
}

type entityDiscoveryService struct {
	workflowRepo   repositories.OntologyWorkflowRepository
	entityRepo     repositories.OntologyEntityRepository
	schemaRepo     repositories.SchemaRepository
	ontologyRepo   repositories.OntologyRepository
	dsSvc          DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	llmFactory     llm.LLMClientFactory
	getTenantCtx   TenantContextFunc
	logger         *zap.Logger
	infra          *workflow.WorkflowInfra
}

// NewEntityDiscoveryService creates a new entity discovery service.
func NewEntityDiscoveryService(
	workflowRepo repositories.OntologyWorkflowRepository,
	entityRepo repositories.OntologyEntityRepository,
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) EntityDiscoveryService {
	namedLogger := logger.Named("entity-discovery")
	infra := workflow.NewWorkflowInfra(workflowRepo, getTenantCtx, namedLogger)

	return &entityDiscoveryService{
		workflowRepo:   workflowRepo,
		entityRepo:     entityRepo,
		schemaRepo:     schemaRepo,
		ontologyRepo:   ontologyRepo,
		dsSvc:          dsSvc,
		adapterFactory: adapterFactory,
		llmFactory:     llmFactory,
		getTenantCtx:   getTenantCtx,
		logger:         namedLogger,
		infra:          infra,
	}
}

var _ EntityDiscoveryService = (*entityDiscoveryService)(nil)

func (s *entityDiscoveryService) StartDiscovery(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	// Step 1: Check for existing workflow for this datasource in entities phase
	existing, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseEntities)
	if err != nil {
		s.logger.Error("Failed to check existing workflow",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}

	// If there's an active workflow, don't start a new one
	if existing != nil && !existing.State.IsTerminal() {
		return nil, fmt.Errorf("entity discovery already in progress for this datasource")
	}

	// Step 2: Get or create ontology for this project
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to check for existing ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("check existing ontology: %w", err)
	}

	if ontology == nil {
		// No active ontology exists - create one
		nextVersion, err := s.ontologyRepo.GetNextVersion(ctx, projectID)
		if err != nil {
			s.logger.Error("Failed to get next ontology version",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("get next version: %w", err)
		}

		ontology = &models.TieredOntology{
			ID:              uuid.New(),
			ProjectID:       projectID,
			Version:         nextVersion,
			IsActive:        true,
			EntitySummaries: make(map[string]*models.EntitySummary),
			ColumnDetails:   make(map[string][]models.ColumnDetail),
			Metadata:        make(map[string]any),
		}

		if err := s.ontologyRepo.Create(ctx, ontology); err != nil {
			s.logger.Error("Failed to create ontology",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("failed to create ontology: %w", err)
		}

		s.logger.Info("Created new ontology for entity discovery",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Int("version", nextVersion))
	} else {
		s.logger.Info("Reusing existing ontology for entity discovery",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Int("version", ontology.Version))

		// Delete existing entities for this ontology (fresh discovery)
		if err := s.entityRepo.DeleteByOntology(ctx, ontology.ID); err != nil {
			s.logger.Error("Failed to delete existing entities",
				zap.String("ontology_id", ontology.ID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("delete existing entities: %w", err)
		}
	}

	// Step 3: Create workflow for entities phase
	now := time.Now()
	wf := &models.OntologyWorkflow{
		ID:           uuid.New(),
		ProjectID:    projectID,
		OntologyID:   ontology.ID,
		State:        models.WorkflowStatePending,
		Phase:        models.WorkflowPhaseEntities,
		DatasourceID: &datasourceID,
		Progress: &models.WorkflowProgress{
			CurrentPhase: "initializing",
			Current:      0,
			Total:        100,
			Message:      "Starting entity discovery...",
		},
		TaskQueue: []models.WorkflowTask{},
		Config: &models.WorkflowConfig{
			DatasourceID: datasourceID,
		},
		StartedAt: &now,
	}

	if err := s.workflowRepo.Create(ctx, wf); err != nil {
		s.logger.Error("Failed to create workflow",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}

	// Step 4: Claim ownership and transition to running
	claimed, err := s.workflowRepo.ClaimOwnership(ctx, wf.ID, s.infra.ServerInstanceID())
	if err != nil {
		s.logger.Error("Failed to claim workflow ownership",
			zap.String("workflow_id", wf.ID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("claim ownership: %w", err)
	}
	if !claimed {
		s.logger.Error("Workflow already owned by another server",
			zap.String("workflow_id", wf.ID.String()))
		return nil, fmt.Errorf("workflow already owned by another server")
	}

	// Transition to running
	wf.State = models.WorkflowStateRunning
	if err := s.workflowRepo.UpdateState(ctx, wf.ID, models.WorkflowStateRunning, ""); err != nil {
		s.logger.Error("Failed to start workflow",
			zap.String("workflow_id", wf.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Start heartbeat goroutine to maintain ownership
	s.infra.StartHeartbeat(wf.ID, projectID)

	s.logger.Info("Entity discovery started",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", wf.ID.String()),
		zap.String("datasource_id", datasourceID.String()),
		zap.String("server_instance_id", s.infra.ServerInstanceID().String()))

	// Step 5: Create work queue and enqueue tasks
	queue := workqueue.New(s.logger, workqueue.WithStrategy(workqueue.NewParallelLLMStrategy()))
	s.infra.StoreQueue(wf.ID, queue)

	// Start single writer goroutine for task queue updates
	s.infra.StartTaskQueueWriter(wf.ID)

	// Set up callback to sync task queue to database for UI visibility
	workflowID := wf.ID // Capture for closure
	queue.SetOnUpdate(func(snapshots []workqueue.TaskSnapshot) {
		// Convert snapshots to models.WorkflowTask with status mapping
		tasks := make([]models.WorkflowTask, len(snapshots))
		for i, snap := range snapshots {
			status := string(snap.Status)
			switch snap.Status {
			case workqueue.TaskStatusPending:
				status = models.TaskStatusQueued
			case workqueue.TaskStatusRunning:
				status = models.TaskStatusProcessing
			case workqueue.TaskStatusCompleted:
				status = models.TaskStatusComplete
			case workqueue.TaskStatusFailed, workqueue.TaskStatusCancelled:
				status = models.TaskStatusFailed
			case workqueue.TaskStatusPaused:
				status = models.TaskStatusPaused
			}

			tasks[i] = models.WorkflowTask{
				ID:          snap.ID,
				Name:        snap.Name,
				Status:      status,
				RequiresLLM: snap.RequiresLLM,
				Error:       snap.Error,
				RetryCount:  snap.RetryCount,
			}
		}
		// Send to single writer goroutine (non-blocking due to buffered channel)
		s.infra.SendTaskQueueUpdate(workflow.TaskQueueUpdate{
			ProjectID:  projectID,
			WorkflowID: workflowID,
			Tasks:      tasks,
		})
	})

	// Run workflow in background - HTTP request returns immediately
	go s.runWorkflow(projectID, wf.ID, ontology.ID, datasourceID, queue)

	return wf, nil
}

// runWorkflow orchestrates the entity discovery phases.
// Runs in a background goroutine - acquires its own DB connection.
func (s *entityDiscoveryService) runWorkflow(projectID, workflowID, ontologyID, datasourceID uuid.UUID, queue *workqueue.Queue) {
	// Clean up when done
	defer s.infra.DeleteQueue(workflowID)
	defer s.infra.StopTaskQueueWriter(workflowID)
	defer s.infra.StopHeartbeat(workflowID)
	defer func() {
		// Release ownership so other servers can take over if needed
		ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
		if err != nil {
			s.logger.Error("Failed to acquire DB connection for ownership release",
				zap.String("workflow_id", workflowID.String()),
				zap.Error(err))
			return
		}
		defer cleanup()
		if releaseErr := s.workflowRepo.ReleaseOwnership(ctx, workflowID); releaseErr != nil {
			s.logger.Error("Failed to release workflow ownership",
				zap.String("workflow_id", workflowID.String()),
				zap.Error(releaseErr))
		}
	}()

	// Entity discovery: DDL-based algorithm
	// Uses is_primary_key and is_unique flags from engine_schema_columns
	// instead of running expensive COUNT(DISTINCT) queries

	ctx := context.Background()

	// Identify entities from DDL metadata (PK and unique constraints)
	s.logger.Info("Identifying entities from DDL metadata",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Analyzing schema constraints...", 20); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	entityCount, tables, columns, err := s.identifyEntitiesFromDDL(ctx, projectID, ontologyID, datasourceID)
	if err != nil {
		s.logger.Error("Failed to identify entities from DDL", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("entity identification: %v", err))
		return
	}

	// Enrich entities with LLM-generated names and descriptions
	if err := s.updateProgress(ctx, projectID, workflowID, "Generating entity names and descriptions...", 60); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	s.logger.Info("Starting LLM enrichment for entities",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("entity_count", entityCount))

	if err := s.enrichEntitiesWithLLM(ctx, projectID, ontologyID, datasourceID, tables, columns); err != nil {
		// Log but don't fail - entities will have table names as fallback
		s.logger.Error("Failed to enrich entities with LLM - entities will use table names as fallback",
			zap.Error(err),
			zap.String("workflow_id", workflowID.String()))
	} else {
		s.logger.Info("LLM enrichment completed successfully",
			zap.String("workflow_id", workflowID.String()))
	}

	// Mark workflow as complete
	s.logger.Info("Entity discovery complete",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("entities_discovered", entityCount))

	if err := s.updateProgress(ctx, projectID, workflowID, "Complete", 100); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	s.finalizeWorkflow(projectID, workflowID)
}

// entityCandidate represents a column that may represent an entity.
type entityCandidate struct {
	schemaName string
	tableName  string
	columnName string
	confidence float64 // 1.0 for PK, 0.9 for unique+not null
	reason     string  // "primary_key" or "unique_not_null"
}

// identifyEntitiesFromDDL finds entities using DDL metadata (is_primary_key, is_unique)
// from engine_schema_columns instead of running expensive COUNT(DISTINCT) queries.
// Returns the count and the tables/columns for LLM enrichment.
func (s *entityDiscoveryService) identifyEntitiesFromDDL(
	ctx context.Context,
	projectID, ontologyID, datasourceID uuid.UUID,
) (int, []*models.SchemaTable, []*models.SchemaColumn, error) {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get all tables for this datasource
	tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("list tables: %w", err)
	}

	// Build table lookup by ID
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	// Get all columns for this datasource
	columns, err := s.schemaRepo.ListColumnsByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("list columns: %w", err)
	}

	// Find entity candidates from DDL metadata
	// Priority: primary key (100% confidence) > unique+not null (90% confidence)
	var candidates []entityCandidate

	for _, col := range columns {
		table, ok := tableByID[col.SchemaTableID]
		if !ok {
			continue
		}

		// Primary key: 100% confidence
		if col.IsPrimaryKey {
			candidates = append(candidates, entityCandidate{
				schemaName: table.SchemaName,
				tableName:  table.TableName,
				columnName: col.ColumnName,
				confidence: 1.0,
				reason:     "primary_key",
			})
			s.logger.Info("Found primary key column",
				zap.String("column", fmt.Sprintf("%s.%s.%s", table.SchemaName, table.TableName, col.ColumnName)))
			continue
		}

		// Unique + not nullable: 90% confidence
		if col.IsUnique && !col.IsNullable {
			candidates = append(candidates, entityCandidate{
				schemaName: table.SchemaName,
				tableName:  table.TableName,
				columnName: col.ColumnName,
				confidence: 0.9,
				reason:     "unique_not_null",
			})
			s.logger.Info("Found unique non-nullable column",
				zap.String("column", fmt.Sprintf("%s.%s.%s", table.SchemaName, table.TableName, col.ColumnName)),
				zap.Float64("confidence", 0.9))
		}
	}

	// Group candidates by table to select the best one per table
	// (prefer PK over unique+not null)
	bestByTable := make(map[string]entityCandidate)
	for _, c := range candidates {
		tableKey := fmt.Sprintf("%s.%s", c.schemaName, c.tableName)
		if existing, ok := bestByTable[tableKey]; !ok || c.confidence > existing.confidence {
			bestByTable[tableKey] = c
		}
	}

	// Create entity records (one per table, using the best candidate)
	// Use table name as temporary name - LLM will enrich with proper names later
	entityCount := 0
	for _, c := range bestByTable {
		entity := &models.OntologyEntity{
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          c.tableName, // Temporary - will be enriched by LLM
			Description:   "",          // Will be filled by LLM
			PrimarySchema: c.schemaName,
			PrimaryTable:  c.tableName,
			PrimaryColumn: c.columnName,
		}

		if err := s.entityRepo.Create(tenantCtx, entity); err != nil {
			s.logger.Error("Failed to create entity",
				zap.String("table_name", c.tableName),
				zap.Error(err))
			return 0, nil, nil, fmt.Errorf("create entity for table %s: %w", c.tableName, err)
		}

		// Create primary occurrence with confidence
		occurrence := &models.OntologyEntityOccurrence{
			EntityID:   entity.ID,
			SchemaName: c.schemaName,
			TableName:  c.tableName,
			ColumnName: c.columnName,
			Confidence: c.confidence,
		}
		if err := s.entityRepo.CreateOccurrence(tenantCtx, occurrence); err != nil {
			s.logger.Error("Failed to create primary occurrence",
				zap.String("table_name", c.tableName),
				zap.Error(err))
			// Don't fail the whole process for occurrence creation
		}

		s.logger.Info("Entity created (pending LLM enrichment)",
			zap.String("entity_id", entity.ID.String()),
			zap.String("table_name", c.tableName),
			zap.String("primary_location", fmt.Sprintf("%s.%s.%s", c.schemaName, c.tableName, c.columnName)),
			zap.Float64("confidence", c.confidence),
			zap.String("reason", c.reason))

		entityCount++
	}

	return entityCount, tables, columns, nil
}

// entityEnrichment holds LLM-generated entity name and description.
type entityEnrichment struct {
	TableName   string `json:"table_name"`
	EntityName  string `json:"entity_name"`
	Description string `json:"description"`
}

// enrichEntitiesWithLLM uses an LLM to generate clean entity names and descriptions
// based on the full schema context.
func (s *entityDiscoveryService) enrichEntitiesWithLLM(
	ctx context.Context,
	projectID, ontologyID, datasourceID uuid.UUID,
	tables []*models.SchemaTable,
	columns []*models.SchemaColumn,
) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get entities we just created
	entities, err := s.entityRepo.GetByOntology(tenantCtx, ontologyID)
	if err != nil {
		return fmt.Errorf("get entities: %w", err)
	}

	if len(entities) == 0 {
		return nil
	}

	// Build table -> columns map for context
	tableColumns := make(map[string][]string)
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}
	for _, col := range columns {
		if t, ok := tableByID[col.SchemaTableID]; ok {
			key := fmt.Sprintf("%s.%s", t.SchemaName, t.TableName)
			tableColumns[key] = append(tableColumns[key], col.ColumnName)
		}
	}

	// Build the prompt
	prompt := s.buildEntityEnrichmentPrompt(entities, tableColumns)
	systemMsg := s.entityEnrichmentSystemMessage()

	// Get LLM client - must use tenant context for config lookup
	llmClient, err := s.llmFactory.CreateForProject(tenantCtx, projectID)
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM
	result, err := llmClient.GenerateResponse(tenantCtx, prompt, systemMsg, 0.3, false)
	if err != nil {
		return fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	enrichments, err := s.parseEntityEnrichmentResponse(result.Content)
	if err != nil {
		s.logger.Warn("Failed to parse entity enrichment response, keeping original names",
			zap.Error(err))
		return nil // Don't fail the workflow for enrichment parsing errors
	}

	// Update entities with enriched names and descriptions
	enrichmentByTable := make(map[string]entityEnrichment)
	for _, e := range enrichments {
		enrichmentByTable[e.TableName] = e
	}

	for _, entity := range entities {
		if enrichment, ok := enrichmentByTable[entity.PrimaryTable]; ok {
			entity.Name = enrichment.EntityName
			entity.Description = enrichment.Description
			if err := s.entityRepo.Update(tenantCtx, entity); err != nil {
				s.logger.Error("Failed to update entity with enrichment",
					zap.String("entity_id", entity.ID.String()),
					zap.Error(err))
				// Continue with other entities
			}
		}
	}

	s.logger.Info("Enriched entities with LLM-generated names and descriptions",
		zap.Int("entity_count", len(entities)),
		zap.Int("enrichments_applied", len(enrichments)))

	return nil
}

func (s *entityDiscoveryService) entityEnrichmentSystemMessage() string {
	return `You are a data modeling expert. Your task is to convert database table names into clean, human-readable entity names and provide brief descriptions of what each entity represents.

Consider the full schema context to understand the domain and make informed guesses about each entity's purpose.`
}

func (s *entityDiscoveryService) buildEntityEnrichmentPrompt(
	entities []*models.OntologyEntity,
	tableColumns map[string][]string,
) string {
	var sb strings.Builder

	sb.WriteString("# Schema Context\n\n")
	sb.WriteString("Below are all the tables in this database with their columns. Use this context to understand what domain/industry this database serves.\n\n")

	// List all tables with columns for context
	for tableKey, cols := range tableColumns {
		sb.WriteString(fmt.Sprintf("**%s**: %s\n", tableKey, strings.Join(cols, ", ")))
	}

	sb.WriteString("\n# Task\n\n")
	sb.WriteString("For each table below, provide:\n")
	sb.WriteString("1. **Entity Name**: A clean, singular, Title Case name (e.g., \"users\" → \"User\", \"billing_activities\" → \"Billing Activity\")\n")
	sb.WriteString("2. **Description**: A brief (1-2 sentence) description of what this entity represents in the domain\n\n")

	sb.WriteString("## Examples\n\n")
	sb.WriteString("- `accounts` → **Account** - \"A user account that can access the platform and manage resources.\"\n")
	sb.WriteString("- `billing_activities` → **Billing Activity** - \"A record of billing-related actions such as charges, refunds, or adjustments.\"\n")
	sb.WriteString("- `new_host_bonus_statuses` → **New Host Bonus Status** - \"Tracks the status of promotional bonuses awarded to newly registered hosts.\"\n")
	sb.WriteString("- `order_items` → **Order Item** - \"A line item within a customer order, representing a specific product and quantity.\"\n\n")

	sb.WriteString("## Tables to Process\n\n")
	for _, entity := range entities {
		tableKey := fmt.Sprintf("%s.%s", entity.PrimarySchema, entity.PrimaryTable)
		cols := tableColumns[tableKey]
		sb.WriteString(fmt.Sprintf("- `%s` (columns: %s)\n", entity.PrimaryTable, strings.Join(cols, ", ")))
	}

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("Respond with a JSON array:\n")
	sb.WriteString("```json\n")
	sb.WriteString("[\n")
	sb.WriteString("  {\"table_name\": \"accounts\", \"entity_name\": \"Account\", \"description\": \"A user account...\"},\n")
	sb.WriteString("  ...\n")
	sb.WriteString("]\n")
	sb.WriteString("```\n")

	return sb.String()
}

func (s *entityDiscoveryService) parseEntityEnrichmentResponse(content string) ([]entityEnrichment, error) {
	// Use the generic ParseJSONResponse helper
	enrichments, err := llm.ParseJSONResponse[[]entityEnrichment](content)
	if err != nil {
		return nil, fmt.Errorf("parse entity enrichment response: %w", err)
	}
	return enrichments, nil
}

// analyzeGraphConnectivity builds a graph from foreign key relationships.
func (s *entityDiscoveryService) analyzeGraphConnectivity(
	ctx context.Context,
	projectID, workflowID, datasourceID uuid.UUID,
) ([]ConnectedComponent, []string, error) {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get datasource to create adapter
	ds, err := s.dsSvc.Get(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer adapter
	adapter, err := s.adapterFactory.NewSchemaDiscoverer(tenantCtx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Check if datasource supports foreign keys
	if !adapter.SupportsForeignKeys() {
		s.logger.Info("Datasource does not support foreign keys, skipping graph analysis",
			zap.String("workflow_id", workflowID.String()),
			zap.String("datasource_type", string(ds.DatasourceType)))
		return []ConnectedComponent{}, []string{}, nil
	}

	// Discover foreign keys
	fks, err := adapter.DiscoverForeignKeys(tenantCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("discover foreign keys: %w", err)
	}

	// Build graph from foreign keys
	graph := NewTableGraph()
	for _, fk := range fks {
		graph.AddForeignKey(fk)
	}

	// Get all tables and add them to the graph (to identify islands)
	tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("list tables: %w", err)
	}

	for _, t := range tables {
		graph.AddTable(t.SchemaName, t.TableName)
	}

	// Find connected components and islands
	components, islands := graph.FindConnectedComponents(s.logger)

	// Log results
	s.logger.Info("Graph connectivity analysis complete",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("connected_components", len(components)),
		zap.Int("island_tables", len(islands)))

	return components, islands, nil
}

// updateProgress updates the workflow progress.
func (s *entityDiscoveryService) updateProgress(ctx context.Context, projectID, workflowID uuid.UUID, message string, percentage int) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	progress := &models.WorkflowProgress{
		CurrentPhase: "entity_discovery",
		Current:      percentage,
		Total:        100,
		Message:      message,
	}

	return s.workflowRepo.UpdateProgress(tenantCtx, workflowID, progress)
}

// finalizeWorkflow marks the workflow as completed.
func (s *entityDiscoveryService) finalizeWorkflow(projectID, workflowID uuid.UUID) {
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to acquire DB connection for finalization",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return
	}
	defer cleanup()

	if err := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateCompleted, ""); err != nil {
		s.logger.Error("Failed to mark workflow complete",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return
	}

	s.logger.Info("Workflow completed successfully",
		zap.String("workflow_id", workflowID.String()))
}

// markWorkflowFailed marks the workflow as failed with an error message.
func (s *entityDiscoveryService) markWorkflowFailed(projectID, workflowID uuid.UUID, errMsg string) {
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to acquire DB connection for failure marking",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return
	}
	defer cleanup()

	if err := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateFailed, errMsg); err != nil {
		s.logger.Error("Failed to mark workflow as failed",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
	}

	s.logger.Error("Workflow failed",
		zap.String("workflow_id", workflowID.String()),
		zap.String("error", errMsg))
}

// GetStatus returns the current entity discovery workflow status for a datasource.
func (s *entityDiscoveryService) GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	return s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseEntities)
}

// GetStatusWithCounts returns workflow status with entity counts.
func (s *entityDiscoveryService) GetStatusWithCounts(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *EntityDiscoveryCounts, error) {
	workflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseEntities)
	if err != nil {
		return nil, nil, fmt.Errorf("get workflow: %w", err)
	}

	if workflow == nil {
		return nil, nil, nil
	}

	// Get entity counts
	counts := &EntityDiscoveryCounts{}
	entities, err := s.entityRepo.GetByOntology(ctx, workflow.OntologyID)
	if err != nil {
		s.logger.Error("Failed to get entities for counts",
			zap.String("ontology_id", workflow.OntologyID.String()),
			zap.Error(err))
		// Return workflow without counts rather than failing
		return workflow, counts, nil
	}

	counts.EntityCount = len(entities)
	// Count occurrences for each entity
	for _, e := range entities {
		occurrences, err := s.entityRepo.GetOccurrencesByEntity(ctx, e.ID)
		if err != nil {
			s.logger.Error("Failed to get occurrences for entity",
				zap.String("entity_id", e.ID.String()),
				zap.Error(err))
			continue
		}
		counts.OccurrenceCount += len(occurrences)
	}

	return workflow, counts, nil
}

// Cancel cancels a running entity discovery workflow.
func (s *entityDiscoveryService) Cancel(ctx context.Context, workflowID uuid.UUID) error {
	// Get the workflow to find its project ID
	wf, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("get workflow: %w", err)
	}

	if wf == nil {
		return fmt.Errorf("workflow not found")
	}

	// Cancel the queue if we own it
	if queue, ok := s.infra.LoadQueue(workflowID); ok {
		queue.Cancel()
	}

	// Update state to failed (cancelled is treated as failed)
	return s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateFailed, "cancelled by user")
}

// Shutdown gracefully stops all active workflows owned by this server.
func (s *entityDiscoveryService) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down entity discovery service",
		zap.String("server_instance_id", s.infra.ServerInstanceID().String()))

	return s.infra.Shutdown(ctx, func(workflowID, projectID uuid.UUID, queue *workqueue.Queue) {
		s.logger.Info("Cancelling workflow for shutdown",
			zap.String("workflow_id", workflowID.String()))

		// Cancel the queue (signals tasks to stop)
		queue.Cancel()

		// Release ownership if we have a project ID
		if projectID != uuid.Nil {
			tenantCtx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
			if err == nil {
				if releaseErr := s.workflowRepo.ReleaseOwnership(tenantCtx, workflowID); releaseErr != nil {
					s.logger.Error("Failed to release ownership during shutdown",
						zap.String("workflow_id", workflowID.String()),
						zap.Error(releaseErr))
				}
				cleanup()
			}
		}
	})
}
