package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// GlossaryDiscoveryMethods defines the methods needed from GlossaryService for DAG execution.
// This interface allows the node to call the service without exposing internal implementation details.
type GlossaryDiscoveryMethods interface {
	// DiscoverGlossaryTerms identifies candidate business terms from ontology.
	// Saves discovered terms to database with source="discovered".
	// Returns count of terms discovered.
	DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error)
}

// GlossaryDiscoveryNode wraps glossary term discovery from ontology data.
// This operation analyzes the ontology to identify business glossary terms.
type GlossaryDiscoveryNode struct {
	*BaseNode
	glossaryDiscovery GlossaryDiscoveryMethods
}

// NewGlossaryDiscoveryNode creates a new glossary discovery node.
func NewGlossaryDiscoveryNode(
	dagRepo repositories.OntologyDAGRepository,
	glossaryDiscovery GlossaryDiscoveryMethods,
	logger *zap.Logger,
) *GlossaryDiscoveryNode {
	return &GlossaryDiscoveryNode{
		BaseNode:          NewBaseNode(models.DAGNodeGlossaryDiscovery, dagRepo, logger),
		glossaryDiscovery: glossaryDiscovery,
	}
}

// Execute runs the glossary discovery phase.
func (n *GlossaryDiscoveryNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting glossary discovery",
		zap.String("project_id", dag.ProjectID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Discovering business terms..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Ensure we have an ontology ID
	if dag.OntologyID == nil {
		return fmt.Errorf("ontology ID is required for glossary discovery")
	}

	// Call the underlying service method
	termCount, err := n.glossaryDiscovery.DiscoverGlossaryTerms(ctx, dag.ProjectID, *dag.OntologyID)
	if err != nil {
		return fmt.Errorf("discover glossary terms: %w", err)
	}

	// Report completion
	if err := n.ReportProgress(ctx, 100, 100, fmt.Sprintf("Discovered %d business terms", termCount)); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Glossary discovery complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("term_count", termCount))

	return nil
}
