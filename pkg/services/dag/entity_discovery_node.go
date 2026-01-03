package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// EntityDiscoveryMethods defines the methods needed from EntityDiscoveryService for DAG execution.
// This interface allows the node to call the service without exposing internal implementation details.
type EntityDiscoveryMethods interface {
	// IdentifyEntitiesFromDDL discovers entities from DDL metadata (PKs, unique constraints).
	// Returns count of entities discovered, tables, and columns for enrichment phase.
	IdentifyEntitiesFromDDL(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID) (int, []*models.SchemaTable, []*models.SchemaColumn, error)
}

// EntityDiscoveryNode wraps entity identification from DDL metadata.
// This is a deterministic operation that discovers entities from primary keys and unique constraints.
type EntityDiscoveryNode struct {
	*BaseNode
	entityDiscovery EntityDiscoveryMethods
	ontologyRepo    repositories.OntologyRepository
	entityRepo      repositories.OntologyEntityRepository
}

// NewEntityDiscoveryNode creates a new entity discovery node.
func NewEntityDiscoveryNode(
	dagRepo repositories.OntologyDAGRepository,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	entityDiscovery EntityDiscoveryMethods,
	logger *zap.Logger,
) *EntityDiscoveryNode {
	return &EntityDiscoveryNode{
		BaseNode:        NewBaseNode(models.DAGNodeEntityDiscovery, dagRepo, logger),
		entityDiscovery: entityDiscovery,
		ontologyRepo:    ontologyRepo,
		entityRepo:      entityRepo,
	}
}

// Execute runs the entity discovery phase.
func (n *EntityDiscoveryNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting entity discovery",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Analyzing schema constraints..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Ensure we have an ontology ID
	if dag.OntologyID == nil {
		return fmt.Errorf("ontology ID is required for entity discovery")
	}

	// Call the underlying service method
	entityCount, _, _, err := n.entityDiscovery.IdentifyEntitiesFromDDL(ctx, dag.ProjectID, *dag.OntologyID, dag.DatasourceID)
	if err != nil {
		return fmt.Errorf("identify entities from DDL: %w", err)
	}

	// Report completion
	if err := n.ReportProgress(ctx, 100, 100, fmt.Sprintf("Discovered %d entities", entityCount)); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Entity discovery complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("entity_count", entityCount))

	return nil
}
