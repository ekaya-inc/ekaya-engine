package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// GlossaryEnrichmentMethods defines the methods needed from GlossaryService for DAG execution.
// This interface allows the node to call the service without exposing internal implementation details.
type GlossaryEnrichmentMethods interface {
	// EnrichGlossaryTerms generates SQL definitions for discovered terms.
	// Processes terms in parallel via LLM calls and validates SQL against the database.
	// Only enriches terms with source="inferred" that lack defining_sql.
	EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error
}

// GlossaryEnrichmentNode wraps glossary term enrichment with SQL generation and validation.
// This operation generates SQL definitions for each discovered term and validates them.
type GlossaryEnrichmentNode struct {
	*BaseNode
	glossaryEnrichment GlossaryEnrichmentMethods
}

// NewGlossaryEnrichmentNode creates a new glossary enrichment node.
func NewGlossaryEnrichmentNode(
	dagRepo repositories.OntologyDAGRepository,
	glossaryEnrichment GlossaryEnrichmentMethods,
	logger *zap.Logger,
) *GlossaryEnrichmentNode {
	return &GlossaryEnrichmentNode{
		BaseNode:           NewBaseNode(models.DAGNodeGlossaryEnrichment, dagRepo, logger),
		glossaryEnrichment: glossaryEnrichment,
	}
}

// Execute runs the glossary enrichment phase.
func (n *GlossaryEnrichmentNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting glossary enrichment",
		zap.String("project_id", dag.ProjectID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 0, "Generating SQL definitions for glossary terms..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Ensure we have an ontology ID
	if dag.OntologyID == nil {
		return fmt.Errorf("ontology ID is required for glossary enrichment")
	}

	// Call the underlying service method
	if err := n.glossaryEnrichment.EnrichGlossaryTerms(ctx, dag.ProjectID, *dag.OntologyID); err != nil {
		// Log but don't fail - glossary terms can remain unenriched
		n.Logger().Warn("Failed to enrich glossary terms - terms will lack SQL definitions",
			zap.String("project_id", dag.ProjectID.String()),
			zap.String("degradation_type", "glossary_enrichment"),
			zap.Error(err))
	}

	// Report completion
	if err := n.ReportProgress(ctx, 1, 1, "Glossary enrichment complete"); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Glossary enrichment complete",
		zap.String("project_id", dag.ProjectID.String()))

	return nil
}
