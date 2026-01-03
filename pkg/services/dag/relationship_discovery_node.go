package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// RelationshipDiscoveryResult contains the results of deterministic relationship discovery.
type RelationshipDiscoveryResult struct {
	FKRelationships       int
	InferredRelationships int
	TotalRelationships    int
}

// DeterministicRelationshipMethods defines the interface for relationship discovery.
type DeterministicRelationshipMethods interface {
	DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*RelationshipDiscoveryResult, error)
}

// RelationshipDiscoveryNode wraps deterministic relationship discovery from FK constraints.
// This discovers FK relationships and PK-match inferences.
type RelationshipDiscoveryNode struct {
	*BaseNode
	deterministicRelSvc DeterministicRelationshipMethods
}

// NewRelationshipDiscoveryNode creates a new relationship discovery node.
func NewRelationshipDiscoveryNode(
	dagRepo repositories.OntologyDAGRepository,
	deterministicRelSvc DeterministicRelationshipMethods,
	logger *zap.Logger,
) *RelationshipDiscoveryNode {
	return &RelationshipDiscoveryNode{
		BaseNode:            NewBaseNode(models.DAGNodeRelationshipDiscovery, dagRepo, logger),
		deterministicRelSvc: deterministicRelSvc,
	}
}

// Execute runs the relationship discovery phase.
func (n *RelationshipDiscoveryNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting relationship discovery",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Discovering FK relationships..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Call the underlying service method
	result, err := n.deterministicRelSvc.DiscoverRelationships(ctx, dag.ProjectID, dag.DatasourceID)
	if err != nil {
		return fmt.Errorf("discover relationships: %w", err)
	}

	// Report completion
	msg := fmt.Sprintf("Discovered %d relationships (%d FK, %d inferred)",
		result.TotalRelationships, result.FKRelationships, result.InferredRelationships)
	if err := n.ReportProgress(ctx, 100, 100, msg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Relationship discovery complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("fk_relationships", result.FKRelationships),
		zap.Int("inferred_relationships", result.InferredRelationships),
		zap.Int("total_relationships", result.TotalRelationships))

	return nil
}
