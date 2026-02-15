package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// LLMRelationshipDiscoveryResult contains the results of LLM-validated relationship discovery.
// This mirrors the result type from services package to avoid circular imports.
type LLMRelationshipDiscoveryResult struct {
	CandidatesEvaluated   int
	RelationshipsCreated  int
	RelationshipsRejected int
	PreservedDBFKs        int
	PreservedColumnFKs    int
	DurationMs            int64
}

// LLMRelationshipDiscoveryMethods defines the interface for LLM-validated relationship discovery.
// This is the new implementation that replaces the threshold-based PKMatchDiscovery.
type LLMRelationshipDiscoveryMethods interface {
	// DiscoverRelationships runs the full LLM-validated discovery pipeline:
	// 1. Preserve existing DB-declared FK relationships (skip LLM)
	// 2. Preserve ColumnFeatures FK relationships with high confidence
	// 3. Collect inference candidates for remaining potential relationships
	// 4. Validate candidates in parallel with worker pool
	// 5. Store validated relationships with LLM-provided cardinality and role
	DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*LLMRelationshipDiscoveryResult, error)
}

// RelationshipDiscoveryNode wraps LLM-validated relationship discovery.
// This replaces the old PKMatchDiscoveryNode that used threshold-based heuristics.
type RelationshipDiscoveryNode struct {
	*BaseNode
	discoverySvc LLMRelationshipDiscoveryMethods
}

// NewRelationshipDiscoveryNode creates a new relationship discovery node.
func NewRelationshipDiscoveryNode(
	dagRepo repositories.OntologyDAGRepository,
	discoverySvc LLMRelationshipDiscoveryMethods,
	logger *zap.Logger,
) *RelationshipDiscoveryNode {
	return &RelationshipDiscoveryNode{
		BaseNode:     NewBaseNode(models.DAGNodePKMatchDiscovery, dagRepo, logger),
		discoverySvc: discoverySvc,
	}
}

// Execute runs the LLM-validated relationship discovery phase.
//
// Note: This node does not require dag.OntologyID. Relationship discovery uses LLM
// validation to infer FK relationships from schema data and column features, storing
// results in engine metadata tables independently of the ontology.
func (n *RelationshipDiscoveryNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting LLM-validated relationship discovery",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 0, "Starting LLM-validated relationship discovery..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Create progress callback that wraps ReportProgress
	progressCallback := func(current, total int, message string) {
		if err := n.ReportProgress(ctx, current, total, message); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
	}

	// Call the underlying service method
	result, err := n.discoverySvc.DiscoverRelationships(ctx, dag.ProjectID, dag.DatasourceID, progressCallback)
	if err != nil {
		return fmt.Errorf("LLM relationship discovery failed: %w", err)
	}

	// Note: The service already reports "Discovery complete" - don't override it here
	// as additional messages can accidentally match phase patterns in the UI.

	n.Logger().Info("LLM relationship discovery complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("preserved_db_fks", result.PreservedDBFKs),
		zap.Int("preserved_column_fks", result.PreservedColumnFKs),
		zap.Int("candidates_evaluated", result.CandidatesEvaluated),
		zap.Int("relationships_created", result.RelationshipsCreated),
		zap.Int("relationships_rejected", result.RelationshipsRejected),
		zap.Int64("duration_ms", result.DurationMs))

	return nil
}
