package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// EntityPromotionMethods defines the interface for entity promotion scoring.
type EntityPromotionMethods interface {
	ScoreAndPromoteEntities(ctx context.Context, projectID uuid.UUID) (promoted int, demoted int, err error)
}

// EntityPromotionNode evaluates entities and promotes/demotes based on semantic value.
// Entities that aggregate multiple tables, have multiple roles, or are hubs in the relationship
// graph are promoted. Simple 1:1 table-to-entity mappings are demoted.
type EntityPromotionNode struct {
	*BaseNode
	entityPromotionSvc EntityPromotionMethods
}

// NewEntityPromotionNode creates a new entity promotion node.
func NewEntityPromotionNode(
	dagRepo repositories.OntologyDAGRepository,
	entityPromotionSvc EntityPromotionMethods,
	logger *zap.Logger,
) *EntityPromotionNode {
	return &EntityPromotionNode{
		BaseNode:           NewBaseNode(models.DAGNodeEntityPromotion, dagRepo, logger),
		entityPromotionSvc: entityPromotionSvc,
	}
}

// Execute runs the entity promotion phase.
func (n *EntityPromotionNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting entity promotion",
		zap.String("project_id", dag.ProjectID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Evaluating entity promotion scores..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// If no service is configured, operate in no-op mode for backward compatibility
	if n.entityPromotionSvc == nil {
		n.Logger().Info("Entity promotion service not configured, skipping",
			zap.String("project_id", dag.ProjectID.String()))
		if err := n.ReportProgress(ctx, 100, 100, "Entity promotion skipped (no service configured)"); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
		return nil
	}

	// Call the underlying service method
	promoted, demoted, err := n.entityPromotionSvc.ScoreAndPromoteEntities(ctx, dag.ProjectID)
	if err != nil {
		return fmt.Errorf("score and promote entities: %w", err)
	}

	// Report completion
	msg := fmt.Sprintf("Entity promotion complete: %d promoted, %d demoted", promoted, demoted)
	if err := n.ReportProgress(ctx, 100, 100, msg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Entity promotion complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("promoted", promoted),
		zap.Int("demoted", demoted))

	return nil
}
