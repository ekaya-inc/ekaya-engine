package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// EntityEnrichmentMethods defines the methods needed from EntityDiscoveryService for LLM enrichment.
type EntityEnrichmentMethods interface {
	// EnrichEntitiesWithLLM enriches entities with LLM-generated names and descriptions.
	// The progressCallback is called to report enrichment progress (current entity, total entities, message).
	EnrichEntitiesWithLLM(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID, progressCallback ProgressCallback) error
}

// EntityEnrichmentNode wraps LLM-based entity enrichment.
// This generates human-readable names, descriptions, domains, and key columns for entities.
type EntityEnrichmentNode struct {
	*BaseNode
	entityEnrichment EntityEnrichmentMethods
}

// NewEntityEnrichmentNode creates a new entity enrichment node.
func NewEntityEnrichmentNode(
	dagRepo repositories.OntologyDAGRepository,
	entityEnrichment EntityEnrichmentMethods,
	logger *zap.Logger,
) *EntityEnrichmentNode {
	return &EntityEnrichmentNode{
		BaseNode:         NewBaseNode(models.DAGNodeEntityEnrichment, dagRepo, logger),
		entityEnrichment: entityEnrichment,
	}
}

// Execute runs the entity enrichment phase.
func (n *EntityEnrichmentNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting entity enrichment",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Generating entity names and descriptions..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Ensure we have an ontology ID
	if dag.OntologyID == nil {
		return fmt.Errorf("ontology ID is required for entity enrichment")
	}

	// Create progress callback that wraps ReportProgress
	progressCallback := func(current, total int, message string) {
		if err := n.ReportProgress(ctx, current, total, message); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
	}

	// Call the underlying service method
	if err := n.entityEnrichment.EnrichEntitiesWithLLM(ctx, dag.ProjectID, *dag.OntologyID, dag.DatasourceID, progressCallback); err != nil {
		return fmt.Errorf("entity enrichment failed: %w", err)
	}

	// Report completion
	if err := n.ReportProgress(ctx, 100, 100, "Entity enrichment complete"); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Entity enrichment complete",
		zap.String("project_id", dag.ProjectID.String()))

	return nil
}
