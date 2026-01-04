package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// FKDiscoveryResult contains the results of FK relationship discovery.
type FKDiscoveryResult struct {
	FKRelationships int
}

// FKDiscoveryMethods defines the interface for FK relationship discovery.
type FKDiscoveryMethods interface {
	DiscoverFKRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*FKDiscoveryResult, error)
}

// FKDiscoveryNode wraps FK relationship discovery from database constraints.
type FKDiscoveryNode struct {
	*BaseNode
	fkDiscoverySvc FKDiscoveryMethods
}

// NewFKDiscoveryNode creates a new FK discovery node.
func NewFKDiscoveryNode(
	dagRepo repositories.OntologyDAGRepository,
	fkDiscoverySvc FKDiscoveryMethods,
	logger *zap.Logger,
) *FKDiscoveryNode {
	return &FKDiscoveryNode{
		BaseNode:       NewBaseNode(models.DAGNodeFKDiscovery, dagRepo, logger),
		fkDiscoverySvc: fkDiscoverySvc,
	}
}

// Execute runs the FK discovery phase.
func (n *FKDiscoveryNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting FK relationship discovery",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Discovering FK relationships from database constraints..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Call the underlying service method
	result, err := n.fkDiscoverySvc.DiscoverFKRelationships(ctx, dag.ProjectID, dag.DatasourceID)
	if err != nil {
		return fmt.Errorf("discover FK relationships: %w", err)
	}

	// Report completion
	msg := fmt.Sprintf("Discovered %d FK relationships from database constraints", result.FKRelationships)
	if err := n.ReportProgress(ctx, 100, 100, msg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("FK relationship discovery complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("fk_relationships", result.FKRelationships))

	return nil
}
