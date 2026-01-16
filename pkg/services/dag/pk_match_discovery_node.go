package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// PKMatchDiscoveryResult contains the results of pk_match relationship discovery.
type PKMatchDiscoveryResult struct {
	InferredRelationships int
}

// PKMatchDiscoveryMethods defines the interface for pk_match relationship discovery.
type PKMatchDiscoveryMethods interface {
	DiscoverPKMatchRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*PKMatchDiscoveryResult, error)
}

// PKMatchDiscoveryNode wraps pk_match relationship discovery via pairwise SQL joins.
type PKMatchDiscoveryNode struct {
	*BaseNode
	pkMatchDiscoverySvc PKMatchDiscoveryMethods
}

// NewPKMatchDiscoveryNode creates a new pk_match discovery node.
func NewPKMatchDiscoveryNode(
	dagRepo repositories.OntologyDAGRepository,
	pkMatchDiscoverySvc PKMatchDiscoveryMethods,
	logger *zap.Logger,
) *PKMatchDiscoveryNode {
	return &PKMatchDiscoveryNode{
		BaseNode:            NewBaseNode(models.DAGNodePKMatchDiscovery, dagRepo, logger),
		pkMatchDiscoverySvc: pkMatchDiscoverySvc,
	}
}

// Execute runs the pk_match discovery phase.
func (n *PKMatchDiscoveryNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting pk_match relationship discovery",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Discovering relationships via pairwise SQL join analysis..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Create progress callback that wraps ReportProgress
	progressCallback := func(current, total int, message string) {
		if err := n.ReportProgress(ctx, current, total, message); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
	}

	// Call the underlying service method
	result, err := n.pkMatchDiscoverySvc.DiscoverPKMatchRelationships(ctx, dag.ProjectID, dag.DatasourceID, progressCallback)
	if err != nil {
		return fmt.Errorf("discover pk_match relationships: %w", err)
	}

	// Report completion
	msg := fmt.Sprintf("Discovered %d relationships via join analysis", result.InferredRelationships)
	if err := n.ReportProgress(ctx, 100, 100, msg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("pk_match relationship discovery complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("inferred_relationships", result.InferredRelationships))

	return nil
}
