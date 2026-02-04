package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// RelationshipEnrichmentResult holds the result of a relationship enrichment operation.
type RelationshipEnrichmentResult struct {
	RelationshipsEnriched int
	RelationshipsFailed   int
	DurationMs            int64
}

// ProgressCallback is a function that reports progress updates.
type ProgressCallback func(current, total int, message string)

// RelationshipEnrichmentMethods defines the interface for relationship enrichment.
type RelationshipEnrichmentMethods interface {
	EnrichProject(ctx context.Context, projectID uuid.UUID, progressCallback ProgressCallback) (*RelationshipEnrichmentResult, error)
}

// RelationshipEnrichmentNode wraps LLM-based relationship enrichment.
// This generates business-meaningful descriptions for table relationships.
type RelationshipEnrichmentNode struct {
	*BaseNode
	relationshipEnrichmentSvc RelationshipEnrichmentMethods
}

// NewRelationshipEnrichmentNode creates a new relationship enrichment node.
func NewRelationshipEnrichmentNode(
	dagRepo repositories.OntologyDAGRepository,
	relationshipEnrichmentSvc RelationshipEnrichmentMethods,
	logger *zap.Logger,
) *RelationshipEnrichmentNode {
	return &RelationshipEnrichmentNode{
		BaseNode:                  NewBaseNode(models.DAGNodeRelationshipEnrichment, dagRepo, logger),
		relationshipEnrichmentSvc: relationshipEnrichmentSvc,
	}
}

// Execute runs the relationship enrichment phase.
func (n *RelationshipEnrichmentNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting relationship enrichment",
		zap.String("project_id", dag.ProjectID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Generating relationship descriptions..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Create a progress callback that wraps ReportProgress
	progressCallback := func(current, total int, message string) {
		if err := n.ReportProgress(ctx, current, total, message); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
	}

	// Call the underlying service method with progress callback
	result, err := n.relationshipEnrichmentSvc.EnrichProject(ctx, dag.ProjectID, progressCallback)
	if err != nil {
		return fmt.Errorf("enrich relationships: %w", err)
	}

	// Report completion
	msg := fmt.Sprintf("Enriched %d relationships", result.RelationshipsEnriched)
	if result.RelationshipsFailed > 0 {
		msg = fmt.Sprintf("Enriched %d relationships (%d failed)", result.RelationshipsEnriched, result.RelationshipsFailed)
	}
	if err := n.ReportProgress(ctx, 100, 100, msg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Relationship enrichment complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("enriched", result.RelationshipsEnriched),
		zap.Int("failed", result.RelationshipsFailed),
		zap.Int64("duration_ms", result.DurationMs))

	return nil
}
