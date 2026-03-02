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
func (n *ColumnEnrichmentNode) Execute(ctx context.Context, dag *models.OntologyDAG, changeSet *models.ChangeSet) error {
	n.Logger().Info("Starting column enrichment",
		zap.String("project_id", dag.ProjectID.String()))

	// During incremental extraction, skip if no columns changed
	if changeSet != nil && !changeSet.HasChangedColumns() && !changeSet.HasChangedTables() {
		n.Logger().Info("Skipping column enrichment (no changed columns or tables)",
			zap.String("project_id", dag.ProjectID.String()))
		if err := n.ReportProgress(ctx, 1, 1, "Skipped (no changed columns)"); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
		return nil
	}

	// Report initial progress (total=0 hides the progress bar until the service reports real counts)
	if err := n.ReportProgress(ctx, 0, 0, "Enriching column metadata..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Create a progress callback that wraps ReportProgress
	progressCallback := func(current, total int, message string) {
		if err := n.ReportProgress(ctx, current, total, message); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
	}

	// During incremental extraction, only enrich affected tables
	var tableNames []string
	if changeSet != nil {
		tableNames = changeSet.AffectedTableNames()
		n.Logger().Info("Incremental column enrichment — processing affected tables only",
			zap.String("project_id", dag.ProjectID.String()),
			zap.Strings("tables", tableNames))
	}

	// Call the underlying service method (nil means enrich all tables)
	result, err := n.columnEnrichmentSvc.EnrichProject(ctx, dag.ProjectID, tableNames, progressCallback)
	if err != nil {
		return fmt.Errorf("enrich columns: %w", err)
	}

	// Report completion
	msg := fmt.Sprintf("Enriched %d tables", len(result.TablesEnriched))
	if len(result.TablesFailed) > 0 {
		msg = fmt.Sprintf("Enriched %d tables (%d failed)", len(result.TablesEnriched), len(result.TablesFailed))
	}
	total := len(result.TablesEnriched) + len(result.TablesFailed)
	if err := n.ReportProgress(ctx, total, total, msg); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Column enrichment complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("tables_enriched", len(result.TablesEnriched)),
		zap.Int("tables_failed", len(result.TablesFailed)),
		zap.Int64("duration_ms", result.DurationMs))

	return nil
}
