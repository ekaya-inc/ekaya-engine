package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/retry"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// OntologyDAGService orchestrates the ontology extraction workflow as a DAG.
// It manages the execution of nodes in sequence, handles failures, and provides
// status updates for UI visibility.
type OntologyDAGService interface {
	// Start initiates a new DAG execution or returns an existing active DAG.
	// projectOverview is optional user-provided context about the application domain.
	Start(ctx context.Context, projectID, datasourceID uuid.UUID, projectOverview string) (*models.OntologyDAG, error)

	// GetStatus returns the current DAG status with all node states.
	GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)

	// Cancel cancels a running DAG.
	Cancel(ctx context.Context, dagID uuid.UUID) error

	// Delete deletes all ontology data for a project including DAGs, ontologies, entities, and relationships.
	Delete(ctx context.Context, projectID uuid.UUID) error

	// Shutdown gracefully stops all DAGs owned by this server.
	Shutdown(ctx context.Context) error
}

type ontologyDAGService struct {
	dagRepo          repositories.OntologyDAGRepository
	ontologyRepo     repositories.OntologyRepository
	entityRepo       repositories.OntologyEntityRepository
	schemaRepo       repositories.SchemaRepository
	relationshipRepo repositories.EntityRelationshipRepository
	questionRepo     repositories.OntologyQuestionRepository
	chatRepo         repositories.OntologyChatRepository
	knowledgeRepo    repositories.KnowledgeRepository

	// Adapted service methods for dag package
	knowledgeSeedingMethods        dag.KnowledgeSeedingMethods
	columnFeatureExtractionMethods dag.ColumnFeatureExtractionMethods
	entityDiscoveryMethods         dag.EntityDiscoveryMethods
	entityEnrichmentMethods        dag.EntityEnrichmentMethods
	fkDiscoveryMethods             dag.FKDiscoveryMethods
	pkMatchDiscoveryMethods        dag.PKMatchDiscoveryMethods
	relationshipEnrichmentMethods  dag.RelationshipEnrichmentMethods
	entityPromotionMethods         dag.EntityPromotionMethods
	finalizationMethods            dag.OntologyFinalizationMethods
	columnEnrichmentMethods        dag.ColumnEnrichmentMethods
	glossaryDiscoveryMethods       dag.GlossaryDiscoveryMethods
	glossaryEnrichmentMethods      dag.GlossaryEnrichmentMethods

	getTenantCtx TenantContextFunc
	logger       *zap.Logger

	// Ownership tracking for graceful shutdown
	serverInstanceID uuid.UUID
	activeDAGs       sync.Map // dagID -> cancelFunc
	mu               sync.Mutex

	// Heartbeat management
	heartbeatCancel sync.Map // dagID -> cancelFunc
}

// NewOntologyDAGService creates a new OntologyDAGService.
func NewOntologyDAGService(
	dagRepo repositories.OntologyDAGRepository,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	schemaRepo repositories.SchemaRepository,
	relationshipRepo repositories.EntityRelationshipRepository,
	questionRepo repositories.OntologyQuestionRepository,
	chatRepo repositories.OntologyChatRepository,
	knowledgeRepo repositories.KnowledgeRepository,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) *ontologyDAGService {
	return &ontologyDAGService{
		dagRepo:          dagRepo,
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		schemaRepo:       schemaRepo,
		relationshipRepo: relationshipRepo,
		questionRepo:     questionRepo,
		chatRepo:         chatRepo,
		knowledgeRepo:    knowledgeRepo,
		getTenantCtx:     getTenantCtx,
		logger:           logger.Named("ontology-dag"),
		serverInstanceID: uuid.New(),
	}
}

var _ OntologyDAGService = (*ontologyDAGService)(nil)

// SetKnowledgeSeedingMethods sets the knowledge seeding methods interface.
// This is called after service construction to avoid circular dependencies.
func (s *ontologyDAGService) SetKnowledgeSeedingMethods(methods dag.KnowledgeSeedingMethods) {
	s.knowledgeSeedingMethods = methods
}

// SetColumnFeatureExtractionMethods sets the column feature extraction methods interface.
// This is called after service construction to avoid circular dependencies.
func (s *ontologyDAGService) SetColumnFeatureExtractionMethods(methods dag.ColumnFeatureExtractionMethods) {
	s.columnFeatureExtractionMethods = methods
}

// SetEntityDiscoveryMethods sets the entity discovery methods interface.
// This is called after service construction to avoid circular dependencies.
func (s *ontologyDAGService) SetEntityDiscoveryMethods(methods dag.EntityDiscoveryMethods) {
	s.entityDiscoveryMethods = methods
}

// SetEntityEnrichmentMethods sets the entity enrichment methods interface.
// This is called after service construction to avoid circular dependencies.
func (s *ontologyDAGService) SetEntityEnrichmentMethods(methods dag.EntityEnrichmentMethods) {
	s.entityEnrichmentMethods = methods
}

// SetFKDiscoveryMethods sets the FK discovery methods interface.
func (s *ontologyDAGService) SetFKDiscoveryMethods(methods dag.FKDiscoveryMethods) {
	s.fkDiscoveryMethods = methods
}

// SetPKMatchDiscoveryMethods sets the pk_match discovery methods interface.
func (s *ontologyDAGService) SetPKMatchDiscoveryMethods(methods dag.PKMatchDiscoveryMethods) {
	s.pkMatchDiscoveryMethods = methods
}

// SetRelationshipEnrichmentMethods sets the relationship enrichment methods interface.
func (s *ontologyDAGService) SetRelationshipEnrichmentMethods(methods dag.RelationshipEnrichmentMethods) {
	s.relationshipEnrichmentMethods = methods
}

// SetEntityPromotionMethods sets the entity promotion methods interface.
func (s *ontologyDAGService) SetEntityPromotionMethods(methods dag.EntityPromotionMethods) {
	s.entityPromotionMethods = methods
}

// SetFinalizationMethods sets the ontology finalization methods interface.
func (s *ontologyDAGService) SetFinalizationMethods(methods dag.OntologyFinalizationMethods) {
	s.finalizationMethods = methods
}

// SetColumnEnrichmentMethods sets the column enrichment methods interface.
func (s *ontologyDAGService) SetColumnEnrichmentMethods(methods dag.ColumnEnrichmentMethods) {
	s.columnEnrichmentMethods = methods
}

// SetGlossaryDiscoveryMethods sets the glossary discovery methods interface.
func (s *ontologyDAGService) SetGlossaryDiscoveryMethods(methods dag.GlossaryDiscoveryMethods) {
	s.glossaryDiscoveryMethods = methods
}

// SetGlossaryEnrichmentMethods sets the glossary enrichment methods interface.
func (s *ontologyDAGService) SetGlossaryEnrichmentMethods(methods dag.GlossaryEnrichmentMethods) {
	s.glossaryEnrichmentMethods = methods
}

// Start initiates a new DAG execution or returns an existing active DAG.
// projectOverview is optional user-provided context about the application domain.
// If provided, the overview is stored as project knowledge with source='manual'.
// Knowledge facts have project-lifecycle scope and persist across re-extractions.
func (s *ontologyDAGService) Start(ctx context.Context, projectID, datasourceID uuid.UUID, projectOverview string) (*models.OntologyDAG, error) {
	s.logger.Info("Starting ontology DAG",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()),
		zap.Bool("has_overview", projectOverview != ""))

	// Extract user ID from JWT claims for provenance tracking.
	// The user who triggered extraction will be recorded as created_by for all inference-created objects.
	userID, err := auth.RequireUserUUIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user authentication required to start extraction: %w", err)
	}

	// Store project overview as knowledge if provided.
	// Uses manual provenance since this is user-provided context.
	// Knowledge facts have project-lifecycle scope and persist across re-extractions.
	if projectOverview != "" {
		manualCtx := models.WithManualProvenance(ctx, userID)
		overviewFact := &models.KnowledgeFact{
			ProjectID: projectID,
			FactType:  "project_overview",
			Value:     projectOverview,
		}
		if err := s.knowledgeRepo.Create(manualCtx, overviewFact); err != nil {
			s.logger.Warn("Failed to store project overview, continuing with extraction",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			// Non-fatal - continue with extraction
		} else {
			s.logger.Info("Stored project overview",
				zap.String("project_id", projectID.String()),
				zap.Int("overview_length", len(projectOverview)))
		}
	}

	// Check for existing active DAG
	existing, err := s.dagRepo.GetActiveByDatasource(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("check existing DAG: %w", err)
	}
	if existing != nil {
		s.logger.Info("Returning existing active DAG",
			zap.String("dag_id", existing.ID.String()),
			zap.String("status", string(existing.Status)))
		return existing, nil
	}

	// Get or create ontology
	ontology, err := s.getOrCreateOntology(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get or create ontology: %w", err)
	}

	// Mark inference-created entities and relationships as stale for re-evaluation
	// Manual and MCP entities/relationships are preserved unchanged
	// The stale flag will be cleared when items are re-discovered or re-enriched
	if err := s.entityRepo.MarkInferenceEntitiesStale(ctx, ontology.ID); err != nil {
		return nil, fmt.Errorf("mark inference entities stale: %w", err)
	}
	if err := s.relationshipRepo.MarkInferenceRelationshipsStale(ctx, ontology.ID); err != nil {
		return nil, fmt.Errorf("mark inference relationships stale: %w", err)
	}

	// Create new DAG
	now := time.Now()
	dagRecord := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		OntologyID:   &ontology.ID,
		Status:       models.DAGStatusPending,
	}

	if err := s.dagRepo.Create(ctx, dagRecord); err != nil {
		return nil, fmt.Errorf("create DAG: %w", err)
	}

	// Create nodes
	nodes := s.createNodes(dagRecord.ID)
	if err := s.dagRepo.CreateNodes(ctx, nodes); err != nil {
		return nil, fmt.Errorf("create nodes: %w", err)
	}

	// Claim ownership
	claimed, err := s.dagRepo.ClaimOwnership(ctx, dagRecord.ID, s.serverInstanceID)
	if err != nil {
		return nil, fmt.Errorf("claim ownership: %w", err)
	}
	if !claimed {
		return nil, fmt.Errorf("failed to claim ownership of DAG")
	}

	// Transition to running
	currentNode := string(models.DAGNodeKnowledgeSeeding)
	dagRecord.Status = models.DAGStatusRunning
	dagRecord.CurrentNode = &currentNode
	dagRecord.StartedAt = &now

	if err := s.dagRepo.UpdateStatus(ctx, dagRecord.ID, models.DAGStatusRunning, &currentNode); err != nil {
		return nil, fmt.Errorf("update DAG status: %w", err)
	}

	// Run DAG execution in background
	// Pass userID for provenance tracking - all inference-created objects will record this user as created_by
	// Note: heartbeat is started inside executeDAG after defer is established
	go s.executeDAG(projectID, dagRecord.ID, userID)

	// Return DAG with nodes
	dagRecord.Nodes = nodes
	return dagRecord, nil
}

// GetStatus returns the current DAG status with all node states.
func (s *ontologyDAGService) GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	dagRecord, err := s.dagRepo.GetLatestByDatasource(ctx, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get latest DAG: %w", err)
	}
	if dagRecord == nil {
		return nil, nil
	}

	// Fetch nodes
	nodes, err := s.dagRepo.GetNodesByDAG(ctx, dagRecord.ID)
	if err != nil {
		return nil, fmt.Errorf("get nodes: %w", err)
	}
	dagRecord.Nodes = nodes

	return dagRecord, nil
}

// Cancel cancels a running DAG.
func (s *ontologyDAGService) Cancel(ctx context.Context, dagID uuid.UUID) error {
	s.logger.Info("Cancelling DAG", zap.String("dag_id", dagID.String()))

	// Cancel execution if we own it
	if cancel, ok := s.activeDAGs.Load(dagID); ok {
		cancel.(context.CancelFunc)()
	}

	// Get all nodes for this DAG
	nodes, err := s.dagRepo.GetNodesByDAG(ctx, dagID)
	if err != nil {
		s.logger.Error("Failed to get nodes for cancellation", zap.String("dag_id", dagID.String()), zap.Error(err))
		return fmt.Errorf("get nodes: %w", err)
	}

	// Mark all non-completed nodes as skipped
	for _, node := range nodes {
		if node.Status != models.DAGNodeStatusCompleted && node.Status != models.DAGNodeStatusFailed {
			if err := s.dagRepo.UpdateNodeStatus(ctx, node.ID, models.DAGNodeStatusSkipped, nil); err != nil {
				s.logger.Error("Failed to mark node as skipped",
					zap.String("node_id", node.ID.String()),
					zap.String("node_name", node.NodeName),
					zap.Error(err))
				// Continue with other nodes even if one fails
			} else {
				s.logger.Debug("Marked node as skipped",
					zap.String("node_id", node.ID.String()),
					zap.String("node_name", node.NodeName))
			}
		}
	}

	// Update DAG status
	return s.dagRepo.UpdateStatus(ctx, dagID, models.DAGStatusCancelled, nil)
}

// Delete deletes all ontology data for a project including DAGs, ontologies, entities, and relationships.
// This is a destructive operation that removes all extracted knowledge.
// All deletions are performed in a single transaction to prevent partial state.
func (s *ontologyDAGService) Delete(ctx context.Context, projectID uuid.UUID) error {
	s.logger.Info("Deleting ontology data", zap.String("project_id", projectID.String()))

	// Check if there's a running DAG - cannot delete while extraction is in progress
	activeDAG, err := s.dagRepo.GetActiveByProject(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to check for active DAG", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("check active DAG: %w", err)
	}
	if activeDAG != nil && activeDAG.Status == models.DAGStatusRunning {
		s.logger.Warn("Cannot delete ontology while extraction is running",
			zap.String("project_id", projectID.String()),
			zap.String("dag_id", activeDAG.ID.String()))
		return fmt.Errorf("cannot delete ontology while extraction is running")
	}

	// Get the tenant scope for transaction
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Start a transaction to ensure atomic deletion
	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on defer is best-effort

	// Delete in order respecting foreign key constraints:
	// 1. DAG nodes (via CASCADE from DAGs)
	// 2. DAGs
	// 3. Entity aliases (via CASCADE from entities)
	// 4. Entity relationships
	// 5. Ontology entities
	// 6. Ontology questions
	// 7. Chat messages
	// 8. Inferred glossary terms (preserving manual/client terms)
	// 9. Ontologies
	// NOTE: Project knowledge is NOT deleted - it has project-lifecycle scope

	// Delete DAGs (cascade deletes dag_nodes)
	if _, err := tx.Exec(ctx, "DELETE FROM engine_ontology_dag WHERE project_id = $1", projectID); err != nil {
		s.logger.Error("Failed to delete DAGs", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete DAGs: %w", err)
	}
	s.logger.Debug("Deleted DAGs", zap.String("project_id", projectID.String()))

	// Delete entity relationships (must be before entities due to FK)
	if _, err := tx.Exec(ctx, `
		DELETE FROM engine_entity_relationships
		WHERE ontology_id IN (SELECT id FROM engine_ontologies WHERE project_id = $1)
	`, projectID); err != nil {
		s.logger.Error("Failed to delete relationships", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete relationships: %w", err)
	}
	s.logger.Debug("Deleted relationships", zap.String("project_id", projectID.String()))

	// Delete ontology entities (cascade deletes entity_aliases)
	if _, err := tx.Exec(ctx, `
		DELETE FROM engine_ontology_entities
		WHERE ontology_id IN (SELECT id FROM engine_ontologies WHERE project_id = $1)
	`, projectID); err != nil {
		s.logger.Error("Failed to delete entities", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete entities: %w", err)
	}
	s.logger.Debug("Deleted entities", zap.String("project_id", projectID.String()))

	// Delete ontology questions
	if _, err := tx.Exec(ctx, "DELETE FROM engine_ontology_questions WHERE project_id = $1", projectID); err != nil {
		s.logger.Error("Failed to delete questions", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete questions: %w", err)
	}
	s.logger.Debug("Deleted questions", zap.String("project_id", projectID.String()))

	// Delete chat messages
	if _, err := tx.Exec(ctx, "DELETE FROM engine_ontology_chat_messages WHERE project_id = $1", projectID); err != nil {
		s.logger.Error("Failed to delete chat messages", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete chat messages: %w", err)
	}
	s.logger.Debug("Deleted chat messages", zap.String("project_id", projectID.String()))

	// NOTE: engine_project_knowledge is NOT deleted here.
	// Knowledge facts have project-lifecycle scope and persist across ontology re-extractions.
	// They are only deleted when the project itself is deleted (via CASCADE on project_id FK).

	// Delete inferred glossary terms (preserve manual and client terms)
	// First delete aliases for inferred terms
	if _, err := tx.Exec(ctx, `
		DELETE FROM engine_glossary_aliases
		WHERE glossary_id IN (
			SELECT id FROM engine_business_glossary
			WHERE project_id = $1 AND source = 'inferred'
		)
	`, projectID); err != nil {
		s.logger.Error("Failed to delete inferred glossary aliases", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete inferred glossary aliases: %w", err)
	}
	// Then delete the inferred terms
	if _, err := tx.Exec(ctx, "DELETE FROM engine_business_glossary WHERE project_id = $1 AND source = 'inferred'", projectID); err != nil {
		s.logger.Error("Failed to delete inferred glossary terms", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete inferred glossary terms: %w", err)
	}
	s.logger.Debug("Deleted inferred glossary terms", zap.String("project_id", projectID.String()))

	// Delete ontologies (must be last due to FK references)
	if _, err := tx.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", projectID); err != nil {
		s.logger.Error("Failed to delete ontologies", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete ontologies: %w", err)
	}
	s.logger.Debug("Deleted ontologies", zap.String("project_id", projectID.String()))

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		s.logger.Error("Failed to commit transaction", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("commit transaction: %w", err)
	}

	s.logger.Info("Successfully deleted all ontology data", zap.String("project_id", projectID.String()))
	return nil
}

// Shutdown gracefully stops all DAGs owned by this server.
func (s *ontologyDAGService) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down ontology DAG service",
		zap.String("server_instance_id", s.serverInstanceID.String()))

	// Cancel all active DAGs
	s.activeDAGs.Range(func(key, value any) bool {
		dagID := key.(uuid.UUID)
		cancel := value.(context.CancelFunc)

		s.logger.Info("Cancelling DAG for shutdown", zap.String("dag_id", dagID.String()))
		cancel()

		// Stop heartbeat
		if hbCancel, ok := s.heartbeatCancel.Load(dagID); ok {
			hbCancel.(context.CancelFunc)()
		}

		return true
	})

	return nil
}

// getOrCreateOntology gets the active ontology or creates a new one.
func (s *ontologyDAGService) getOrCreateOntology(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get active ontology: %w", err)
	}

	if ontology != nil {
		return ontology, nil
	}

	// Create new ontology
	nextVersion, err := s.ontologyRepo.GetNextVersion(ctx, projectID)
	if err != nil {
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
		return nil, fmt.Errorf("create ontology: %w", err)
	}

	s.logger.Info("Created new ontology",
		zap.String("project_id", projectID.String()),
		zap.String("ontology_id", ontology.ID.String()),
		zap.Int("version", nextVersion))

	return ontology, nil
}

// createNodes creates all DAG nodes in pending state.
func (s *ontologyDAGService) createNodes(dagID uuid.UUID) []models.DAGNode {
	allNodes := models.AllDAGNodes()
	nodes := make([]models.DAGNode, len(allNodes))

	for i, nodeName := range allNodes {
		nodes[i] = models.DAGNode{
			ID:        uuid.New(),
			DAGID:     dagID,
			NodeName:  string(nodeName),
			NodeOrder: models.DAGNodeOrder[nodeName],
			Status:    models.DAGNodeStatusPending,
		}
	}

	return nodes
}

// executeDAG runs the DAG execution in a background goroutine.
// userID is the user who triggered the extraction - used for provenance tracking on all created objects.
func (s *ontologyDAGService) executeDAG(projectID, dagID, userID uuid.UUID) {
	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	s.activeDAGs.Store(dagID, cancel)

	// Set up defer FIRST to ensure cleanup happens even if panic occurs
	defer func() {
		// Recover from panics and mark DAG as failed
		if r := recover(); r != nil {
			s.logger.Error("DAG execution panicked",
				zap.String("dag_id", dagID.String()),
				zap.String("project_id", projectID.String()),
				zap.Any("panic", r),
				zap.Stack("stack"))
			// Mark DAG as failed with panic message
			s.markDAGFailed(projectID, dagID, fmt.Sprintf("panic during execution: %v", r))
		}

		s.activeDAGs.Delete(dagID)
		s.stopHeartbeat(dagID)
		s.releaseOwnership(projectID, dagID)
	}()

	// Start heartbeat after defer is established so stopHeartbeat is guaranteed to run
	s.startHeartbeat(dagID, projectID)

	s.logger.Info("Starting DAG execution",
		zap.String("dag_id", dagID.String()),
		zap.String("project_id", projectID.String()),
		zap.String("triggered_by_user", userID.String()))

	// Get tenant context with inference provenance.
	// All ontology objects created during DAG execution will record:
	// - source = "inferred" (auto-detected by LLM)
	// - created_by = userID (the user who triggered extraction)
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get tenant context", zap.Error(err))
		s.markDAGFailed(projectID, dagID, "failed to get tenant context")
		return
	}
	defer cleanup()

	// Wrap the tenant context with inference provenance.
	// This ensures all repository operations during DAG execution have proper provenance.
	tenantCtx = models.WithInferredProvenance(tenantCtx, userID)

	// Get DAG with nodes
	dagRecord, err := s.dagRepo.GetByIDWithNodes(tenantCtx, dagID)
	if err != nil {
		s.logger.Error("Failed to get DAG record", zap.Error(err))
		s.markDAGFailed(projectID, dagID, "failed to get DAG record")
		return
	}

	// Execute each node in sequence
	for _, node := range dagRecord.Nodes {
		// Check for cancellation
		select {
		case <-ctx.Done():
			s.logger.Info("DAG execution cancelled", zap.String("dag_id", dagID.String()))
			return
		default:
		}

		// Skip completed nodes (for resume support)
		if node.Status == models.DAGNodeStatusCompleted {
			continue
		}

		// Execute node
		if err := s.executeNode(tenantCtx, dagRecord, &node); err != nil {
			s.logger.Error("Node execution failed",
				zap.String("dag_id", dagID.String()),
				zap.String("node_name", node.NodeName),
				zap.Error(err))

			// Let markDAGFailed handle marking both the node and DAG as failed
			s.markDAGFailed(projectID, dagID, err.Error())
			return
		}
	}

	// All nodes completed successfully
	s.markDAGCompleted(projectID, dagID)
}

// executeNode runs a single node with retry logic.
func (s *ontologyDAGService) executeNode(ctx context.Context, dagRecord *models.OntologyDAG, node *models.DAGNode) error {
	s.logger.Info("Executing node",
		zap.String("dag_id", dagRecord.ID.String()),
		zap.String("node_name", node.NodeName))

	// Update DAG current node
	nodeName := node.NodeName
	if err := s.dagRepo.UpdateStatus(ctx, dagRecord.ID, models.DAGStatusRunning, &nodeName); err != nil {
		return fmt.Errorf("update current node: %w", err)
	}

	// Mark node as running
	if err := s.dagRepo.UpdateNodeStatus(ctx, node.ID, models.DAGNodeStatusRunning, nil); err != nil {
		return fmt.Errorf("mark node running: %w", err)
	}

	// Get the appropriate executor
	executor, err := s.getNodeExecutor(models.DAGNodeName(node.NodeName), node.ID)
	if err != nil {
		return fmt.Errorf("get node executor: %w", err)
	}

	// Execute with retry
	retryCfg := retry.DefaultConfig()
	err = retry.DoIfRetryable(ctx, retryCfg, func() error {
		return executor.Execute(ctx, dagRecord)
	})

	if err != nil {
		// Check if it's retryable for logging
		if llm.IsRetryable(err) {
			s.logger.Warn("Node failed after retries",
				zap.String("node_name", node.NodeName),
				zap.Error(err))
		}
		return err
	}

	// Mark node as completed
	if err := s.dagRepo.UpdateNodeStatus(ctx, node.ID, models.DAGNodeStatusCompleted, nil); err != nil {
		return fmt.Errorf("mark node completed: %w", err)
	}

	s.logger.Info("Node completed",
		zap.String("dag_id", dagRecord.ID.String()),
		zap.String("node_name", node.NodeName))

	return nil
}

// getNodeExecutor returns the appropriate executor for a node.
func (s *ontologyDAGService) getNodeExecutor(nodeName models.DAGNodeName, nodeID uuid.UUID) (dag.NodeExecutor, error) {
	switch nodeName {
	case models.DAGNodeKnowledgeSeeding:
		// Knowledge seeding extracts domain facts from project overview.
		// If knowledgeSeedingMethods is nil, the node operates in no-op mode for backward compatibility.
		node := dag.NewKnowledgeSeedingNode(s.dagRepo, s.knowledgeSeedingMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeColumnFeatureExtraction:
		// Column feature extraction extracts deterministic features from columns.
		// If columnFeatureExtractionMethods is nil, the node operates in no-op mode.
		node := dag.NewColumnFeatureExtractionNode(s.dagRepo, s.columnFeatureExtractionMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeEntityDiscovery:
		if s.entityDiscoveryMethods == nil {
			return nil, fmt.Errorf("entity discovery methods not set")
		}
		node := dag.NewEntityDiscoveryNode(s.dagRepo, s.ontologyRepo, s.entityRepo, s.entityDiscoveryMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeEntityEnrichment:
		if s.entityEnrichmentMethods == nil {
			return nil, fmt.Errorf("entity enrichment methods not set")
		}
		node := dag.NewEntityEnrichmentNode(s.dagRepo, s.entityEnrichmentMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeFKDiscovery:
		if s.fkDiscoveryMethods == nil {
			return nil, fmt.Errorf("FK discovery methods not set")
		}
		node := dag.NewFKDiscoveryNode(s.dagRepo, s.fkDiscoveryMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodePKMatchDiscovery:
		if s.pkMatchDiscoveryMethods == nil {
			return nil, fmt.Errorf("pk_match discovery methods not set")
		}
		node := dag.NewPKMatchDiscoveryNode(s.dagRepo, s.pkMatchDiscoveryMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeRelationshipEnrichment:
		if s.relationshipEnrichmentMethods == nil {
			return nil, fmt.Errorf("relationship enrichment methods not set")
		}
		node := dag.NewRelationshipEnrichmentNode(s.dagRepo, s.relationshipEnrichmentMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeEntityPromotion:
		// Entity promotion is optional - operates in no-op mode if not configured.
		// This allows backward compatibility during the incremental rollout.
		node := dag.NewEntityPromotionNode(s.dagRepo, s.entityPromotionMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeOntologyFinalization:
		if s.finalizationMethods == nil {
			return nil, fmt.Errorf("finalization methods not set")
		}
		node := dag.NewOntologyFinalizationNode(s.dagRepo, s.finalizationMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeColumnEnrichment:
		if s.columnEnrichmentMethods == nil {
			return nil, fmt.Errorf("column enrichment methods not set")
		}
		node := dag.NewColumnEnrichmentNode(s.dagRepo, s.columnEnrichmentMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeGlossaryDiscovery:
		if s.glossaryDiscoveryMethods == nil {
			return nil, fmt.Errorf("glossary discovery methods not set")
		}
		node := dag.NewGlossaryDiscoveryNode(s.dagRepo, s.glossaryDiscoveryMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	case models.DAGNodeGlossaryEnrichment:
		if s.glossaryEnrichmentMethods == nil {
			return nil, fmt.Errorf("glossary enrichment methods not set")
		}
		node := dag.NewGlossaryEnrichmentNode(s.dagRepo, s.glossaryEnrichmentMethods, s.logger)
		node.SetCurrentNodeID(nodeID)
		return node, nil

	default:
		return nil, fmt.Errorf("unknown node: %s", nodeName)
	}
}

// startHeartbeat starts a goroutine that periodically updates the DAG heartbeat.
func (s *ontologyDAGService) startHeartbeat(dagID, projectID uuid.UUID) {
	ctx, cancel := context.WithCancel(context.Background())
	s.heartbeatCancel.Store(dagID, cancel)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				tenantCtx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
				if err != nil {
					s.logger.Warn("Failed to get tenant context for heartbeat", zap.Error(err))
					continue
				}

				if err := s.dagRepo.UpdateHeartbeat(tenantCtx, dagID, s.serverInstanceID); err != nil {
					s.logger.Warn("Failed to update heartbeat", zap.Error(err))
				}
				cleanup()
			}
		}
	}()
}

// stopHeartbeat stops the heartbeat goroutine for a DAG.
func (s *ontologyDAGService) stopHeartbeat(dagID uuid.UUID) {
	if cancel, ok := s.heartbeatCancel.Load(dagID); ok {
		cancel.(context.CancelFunc)()
		s.heartbeatCancel.Delete(dagID)
	}
}

// releaseOwnership releases ownership of a DAG.
func (s *ontologyDAGService) releaseOwnership(projectID, dagID uuid.UUID) {
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to get tenant context for ownership release", zap.Error(err))
		return
	}
	defer cleanup()

	if err := s.dagRepo.ReleaseOwnership(ctx, dagID); err != nil {
		s.logger.Error("Failed to release ownership", zap.Error(err))
	}
}

// markDAGFailed marks the DAG as failed and stores the error message on the appropriate node.
// If a current node is set, the error is stored there; otherwise it's stored on the first node.
func (s *ontologyDAGService) markDAGFailed(projectID, dagID uuid.UUID, errMsg string) {
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to get tenant context for marking DAG failed", zap.Error(err))
		return
	}
	defer cleanup()

	// Get the DAG to find the current node or determine which node to mark failed
	dagRecord, err := s.dagRepo.GetByIDWithNodes(ctx, dagID)
	if err != nil {
		s.logger.Error("Failed to get DAG for error marking", zap.Error(err))
		// Still mark the DAG as failed even if we can't get the nodes
		if updateErr := s.dagRepo.UpdateStatus(ctx, dagID, models.DAGStatusFailed, nil); updateErr != nil {
			s.logger.Error("Failed to mark DAG as failed", zap.Error(updateErr))
		}
		return
	}

	// Find the node to mark as failed:
	// 1. If current_node is set, use that node
	// 2. Otherwise, use the first pending/running node
	// 3. If no pending/running nodes, use the first node (EntityDiscovery)
	var targetNode *models.DAGNode
	if dagRecord.CurrentNode != nil {
		// Find the current node
		for i := range dagRecord.Nodes {
			if dagRecord.Nodes[i].NodeName == *dagRecord.CurrentNode {
				targetNode = &dagRecord.Nodes[i]
				break
			}
		}
	}

	// If no current node, find the first pending or running node
	if targetNode == nil {
		for i := range dagRecord.Nodes {
			if dagRecord.Nodes[i].Status == models.DAGNodeStatusPending ||
				dagRecord.Nodes[i].Status == models.DAGNodeStatusRunning {
				targetNode = &dagRecord.Nodes[i]
				break
			}
		}
	}

	// If still no target node (shouldn't happen), use the first node
	if targetNode == nil && len(dagRecord.Nodes) > 0 {
		targetNode = &dagRecord.Nodes[0]
	}

	// Mark the target node as failed with the error message
	if targetNode != nil {
		if err := s.dagRepo.UpdateNodeStatus(ctx, targetNode.ID, models.DAGNodeStatusFailed, &errMsg); err != nil {
			s.logger.Error("Failed to update node status with error",
				zap.String("node_id", targetNode.ID.String()),
				zap.Error(err))
		}
	}

	// Mark the DAG as failed
	if err := s.dagRepo.UpdateStatus(ctx, dagID, models.DAGStatusFailed, nil); err != nil {
		s.logger.Error("Failed to mark DAG as failed", zap.Error(err))
	}

	s.logger.Error("DAG failed",
		zap.String("dag_id", dagID.String()),
		zap.String("error", errMsg))
}

// markDAGCompleted marks the DAG as completed.
func (s *ontologyDAGService) markDAGCompleted(projectID, dagID uuid.UUID) {
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to get tenant context for marking DAG completed", zap.Error(err))
		return
	}
	defer cleanup()

	if err := s.dagRepo.UpdateStatus(ctx, dagID, models.DAGStatusCompleted, nil); err != nil {
		s.logger.Error("Failed to mark DAG as completed", zap.Error(err))
	}

	s.logger.Info("DAG completed successfully", zap.String("dag_id", dagID.String()))
}
