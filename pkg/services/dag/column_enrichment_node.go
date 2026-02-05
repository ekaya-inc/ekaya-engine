package dag

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ColumnEnrichmentResult holds the result of a column enrichment operation.
type ColumnEnrichmentResult struct {
	TablesEnriched []string
	TablesFailed   map[string]string
	DurationMs     int64
}

// ColumnEnrichmentMethods defines the interface for column enrichment.
type ColumnEnrichmentMethods interface {
	EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback ProgressCallback) (*ColumnEnrichmentResult, error)
}

// ColumnEnrichmentNode wraps LLM-based column enrichment.
// This generates column descriptions, semantic types, roles, and enum value mappings.
type ColumnEnrichmentNode struct {
	*BaseNode
	columnEnrichmentSvc ColumnEnrichmentMethods
}

// NewColumnEnrichmentNode creates a new column enrichment node.
func NewColumnEnrichmentNode(
	dagRepo repositories.OntologyDAGRepository,
	columnEnrichmentSvc ColumnEnrichmentMethods,
	logger *zap.Logger,
) *ColumnEnrichmentNode {
	return &ColumnEnrichmentNode{
		BaseNode:            NewBaseNode(models.DAGNodeColumnEnrichment, dagRepo, logger),
		columnEnrichmentSvc: columnEnrichmentSvc,
	}
}

// Execute runs the column enrichment phase.
//
// Note: This node does not require dag.OntologyID. Column enrichment generates LLM-based
// descriptions and semantic types for columns, storing results in engine_ontology_column_metadata
// which is linked via schema_column_id, not ontology_id.
func (n *ColumnEnrichmentNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting column enrichment",
		zap.String("project_id", dag.ProjectID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 100, "Enriching column metadata..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Create a progress callback that wraps ReportProgress
	progressCallback := func(current, total int, message string) {
		if err := n.ReportProgress(ctx, current, total, message); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
	}

	// Call the underlying service method (nil means enrich all tables)
	result, err := n.columnEnrichmentSvc.EnrichProject(ctx, dag.ProjectID, nil, progressCallback)
	if err != nil {
		return fmt.Errorf("enrich columns: %w", err)
	}

	// Report completion
	msg := fmt.Sprintf("Enriched %d tables", len(result.TablesEnriched))
	if len(result.TablesFailed) > 0 {
		msg = fmt.Sprintf("Enriched %d tables (%d failed)", len(result.TablesEnriched), len(result.TablesFailed))
	}
	if err := n.ReportProgress(ctx, 100, 100, msg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Column enrichment complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("tables_enriched", len(result.TablesEnriched)),
		zap.Int("tables_failed", len(result.TablesFailed)),
		zap.Int64("duration_ms", result.DurationMs))

	return nil
}
