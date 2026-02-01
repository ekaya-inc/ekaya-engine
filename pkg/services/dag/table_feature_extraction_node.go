package dag

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// TableFeatureExtractionMethods defines the methods needed for table feature extraction.
// This interface allows the node to call service methods without causing import cycles.
type TableFeatureExtractionMethods interface {
	// ExtractTableFeatures generates table-level descriptions based on column features.
	// Inputs per table: table name/schema, columns with ColumnFeatures, FK relationships, row count.
	// Outputs per table (stored in engine_table_metadata): description, usage_notes, is_ephemeral.
	// Returns the number of tables processed.
	ExtractTableFeatures(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (int, error)
}

// TableFeatureExtractionNode generates table-level descriptions based on column features.
// This node runs after ColumnFeatureExtraction to synthesize column-level analysis into
// meaningful table descriptions that help users understand what each table represents.
//
// The node makes ONE LLM call per table (not batched), enabling granular progress updates
// to the UI as each table completes.
type TableFeatureExtractionNode struct {
	*BaseNode
	methods TableFeatureExtractionMethods
}

// NewTableFeatureExtractionNode creates a new table feature extraction node.
func NewTableFeatureExtractionNode(
	dagRepo repositories.OntologyDAGRepository,
	methods TableFeatureExtractionMethods,
	logger *zap.Logger,
) *TableFeatureExtractionNode {
	return &TableFeatureExtractionNode{
		BaseNode: NewBaseNode(models.DAGNodeTableFeatureExtraction, dagRepo, logger),
		methods:  methods,
	}
}

// Execute runs the table feature extraction phase.
func (n *TableFeatureExtractionNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting table feature extraction",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 1, "Analyzing tables..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Check if methods are configured (nil means no-op mode)
	if n.methods == nil {
		n.Logger().Info("Table feature extraction skipped (not configured)",
			zap.String("project_id", dag.ProjectID.String()))
		if err := n.ReportProgress(ctx, 1, 1, "Table feature extraction skipped (not configured)"); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
		return nil
	}

	// Progress callback to report progress updates
	progressCallback := func(current, total int, message string) {
		if err := n.ReportProgress(ctx, current, total, message); err != nil {
			n.Logger().Warn("Failed to report progress", zap.Error(err))
		}
	}

	// Call the underlying service method to extract table features
	tablesProcessed, err := n.methods.ExtractTableFeatures(ctx, dag.ProjectID, dag.DatasourceID, progressCallback)
	if err != nil {
		return err
	}

	// Report completion
	if err := n.ReportProgress(ctx, 1, 1, "Table feature extraction complete"); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Table feature extraction complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("tables_processed", tablesProcessed))

	return nil
}
