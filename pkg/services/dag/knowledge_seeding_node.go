package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// KnowledgeSeedingMethods defines the methods needed for seeding project knowledge.
// This interface allows the node to call the service without exposing internal implementation details.
type KnowledgeSeedingMethods interface {
	// SeedKnowledgeFromFile loads knowledge facts from the project's configured seed file path
	// and upserts them into the knowledge repository.
	// Returns the count of facts seeded.
	// If no seed path is configured, returns (0, nil) without error.
	SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error)
}

// KnowledgeSeedingNode wraps knowledge seeding from a configured file path.
// This node runs BEFORE entity discovery to ensure domain knowledge is available during extraction.
type KnowledgeSeedingNode struct {
	*BaseNode
	knowledgeSeeding KnowledgeSeedingMethods
}

// NewKnowledgeSeedingNode creates a new knowledge seeding node.
func NewKnowledgeSeedingNode(
	dagRepo repositories.OntologyDAGRepository,
	knowledgeSeeding KnowledgeSeedingMethods,
	logger *zap.Logger,
) *KnowledgeSeedingNode {
	return &KnowledgeSeedingNode{
		BaseNode:         NewBaseNode(models.DAGNodeKnowledgeSeeding, dagRepo, logger),
		knowledgeSeeding: knowledgeSeeding,
	}
}

// Execute runs the knowledge seeding phase.
func (n *KnowledgeSeedingNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting knowledge seeding",
		zap.String("project_id", dag.ProjectID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Loading project knowledge..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Call the underlying service method
	factCount, err := n.knowledgeSeeding.SeedKnowledgeFromFile(ctx, dag.ProjectID)
	if err != nil {
		return fmt.Errorf("seed knowledge from file: %w", err)
	}

	// Report completion
	message := "No knowledge seed file configured"
	if factCount > 0 {
		message = fmt.Sprintf("Seeded %d knowledge facts", factCount)
	}
	if err := n.ReportProgress(ctx, 100, 100, message); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Knowledge seeding complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("fact_count", factCount))

	return nil
}
