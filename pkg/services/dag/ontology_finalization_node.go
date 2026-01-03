package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// OntologyFinalizationMethods defines the interface for ontology finalization.
type OntologyFinalizationMethods interface {
	Finalize(ctx context.Context, projectID uuid.UUID) error
}

// OntologyFinalizationNode wraps ontology finalization.
// This generates domain summary, detects conventions, and aggregates primary domains.
type OntologyFinalizationNode struct {
	*BaseNode
	finalizationSvc OntologyFinalizationMethods
}

// NewOntologyFinalizationNode creates a new ontology finalization node.
func NewOntologyFinalizationNode(
	dagRepo repositories.OntologyDAGRepository,
	finalizationSvc OntologyFinalizationMethods,
	logger *zap.Logger,
) *OntologyFinalizationNode {
	return &OntologyFinalizationNode{
		BaseNode:        NewBaseNode(models.DAGNodeOntologyFinalization, dagRepo, logger),
		finalizationSvc: finalizationSvc,
	}
}

// Execute runs the ontology finalization phase.
func (n *OntologyFinalizationNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting ontology finalization",
		zap.String("project_id", dag.ProjectID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Generating domain summary..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Call the underlying service method
	if err := n.finalizationSvc.Finalize(ctx, dag.ProjectID); err != nil {
		return fmt.Errorf("finalize ontology: %w", err)
	}

	// Report completion
	if err := n.ReportProgress(ctx, 100, 100, "Ontology finalization complete"); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Ontology finalization complete",
		zap.String("project_id", dag.ProjectID.String()))

	return nil
}
