package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

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
	Start(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyDAG, error)

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
	entityDiscoveryMethods        dag.EntityDiscoveryMethods
	entityEnrichmentMethods       dag.EntityEnrichmentMethods
	fkDiscoveryMethods            dag.FKDiscoveryMethods
	pkMatchDiscoveryMethods       dag.PKMatchDiscoveryMethods
	relationshipEnrichmentMethods dag.RelationshipEnrichmentMethods
	finalizationMethods           dag.OntologyFinalizationMethods
	columnEnrichmentMethods       dag.ColumnEnrichmentMethods

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

// SetFinalizationMethods sets the ontology finalization methods interface.
func (s *ontologyDAGService) SetFinalizationMethods(methods dag.OntologyFinalizationMethods) {
	s.finalizationMethods = methods
}

// SetColumnEnrichmentMethods sets the column enrichment methods interface.
func (s *ontologyDAGService) SetColumnEnrichmentMethods(methods dag.ColumnEnrichmentMethods) {
	s.columnEnrichmentMethods = methods
}

// Start initiates a new DAG execution or returns an existing active DAG.
func (s *ontologyDAGService) Start(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	s.logger.Info("Starting ontology DAG",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()))

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

	// Delete existing entities for fresh discovery
	if err := s.entityRepo.DeleteByOntology(ctx, ontology.ID); err != nil {
		return nil, fmt.Errorf("delete existing entities: %w", err)
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
	currentNode := string(models.DAGNodeEntityDiscovery)
	dagRecord.Status = models.DAGStatusRunning
	dagRecord.CurrentNode = &currentNode
	dagRecord.StartedAt = &now

	if err := s.dagRepo.UpdateStatus(ctx, dagRecord.ID, models.DAGStatusRunning, &currentNode); err != nil {
		return nil, fmt.Errorf("update DAG status: %w", err)
	}

	// Run DAG execution in background
	// Note: heartbeat is started inside executeDAG after defer is established
	go s.executeDAG(projectID, dagRecord.ID)

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
	// 8. Project knowledge
	// 9. Ontologies

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

	// Delete project knowledge
	if _, err := tx.Exec(ctx, "DELETE FROM engine_project_knowledge WHERE project_id = $1", projectID); err != nil {
		s.logger.Error("Failed to delete project knowledge", zap.String("project_id", projectID.String()), zap.Error(err))
		return fmt.Errorf("delete project knowledge: %w", err)
	}
	s.logger.Debug("Deleted project knowledge", zap.String("project_id", projectID.String()))

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
func (s *ontologyDAGService) executeDAG(projectID, dagID uuid.UUID) {
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
		zap.String("project_id", projectID.String()))

	// Get tenant context
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get tenant context", zap.Error(err))
		s.markDAGFailed(projectID, dagID, "failed to get tenant context")
		return
	}
	defer cleanup()

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

	case models.DAGNodeFKDiscovery, models.DAGNodeRelationshipDiscovery:
		// DAGNodeRelationshipDiscovery is deprecated but supported for backward compatibility
		// Target removal: v1.0.0 or 2025-06-01, whichever comes first.
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
