package dag

import (
	"context"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// KnowledgeSeedingNode is a no-op node that exists for DAG structure compatibility.
// Knowledge is now inferred from schema analysis and refined via MCP tools,
// not loaded from files.
type KnowledgeSeedingNode struct {
	*BaseNode
}

// NewKnowledgeSeedingNode creates a new knowledge seeding node.
func NewKnowledgeSeedingNode(
	dagRepo repositories.OntologyDAGRepository,
	logger *zap.Logger,
) *KnowledgeSeedingNode {
	return &KnowledgeSeedingNode{
		BaseNode: NewBaseNode(models.DAGNodeKnowledgeSeeding, dagRepo, logger),
	}
}

// Execute is a no-op - knowledge seeding from files is no longer supported.
// Knowledge is now inferred from schema analysis and refined via MCP tools.
func (n *KnowledgeSeedingNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Knowledge seeding skipped (inference-based)",
		zap.String("project_id", dag.ProjectID.String()))

	if err := n.ReportProgress(ctx, 100, 100, "Knowledge seeding complete (inference-based)"); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	return nil
}
