package dag

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ColumnFeatureExtractionMethods defines the methods needed for column feature extraction.
// This interface allows the node to call service methods without causing import cycles.
type ColumnFeatureExtractionMethods interface {
	// ExtractColumnFeatures extracts deterministic features from all columns in the datasource.
	// Features include: data types, sample values, distributions, cardinality, null rates,
	// UUID patterns, currency codes, external ID patterns, timestamp scales, etc.
	// Returns the number of columns processed.
	ExtractColumnFeatures(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (int, error)
}

// ColumnFeatureExtractionNode extracts deterministic features from columns.
// This node runs early in the DAG (after KnowledgeSeeding) to provide feature data
// that can inform FK Discovery, Column Enrichment, and other downstream LLM-based nodes.
//
// Features extracted:
// - Data types and nullable status
// - Sample values
// - Value distributions (distinct count, null rate, cardinality ratio)
// - Pattern detection (UUID, ISO 4217 currency, Stripe/Twilio IDs, timestamp scales)
// - Statistical metrics for numeric columns
//
// These features are stored and made available to downstream nodes via the schema repository.
type ColumnFeatureExtractionNode struct {
	*BaseNode
	methods ColumnFeatureExtractionMethods
}

// NewColumnFeatureExtractionNode creates a new column feature extraction node.
func NewColumnFeatureExtractionNode(
	dagRepo repositories.OntologyDAGRepository,
	methods ColumnFeatureExtractionMethods,
	logger *zap.Logger,
) *ColumnFeatureExtractionNode {
	return &ColumnFeatureExtractionNode{
		BaseNode: NewBaseNode(models.DAGNodeColumnFeatureExtraction, dagRepo, logger),
		methods:  methods,
	}
}

// Execute runs the column feature extraction phase.
func (n *ColumnFeatureExtractionNode) Execute(ctx context.Context, dag *models.OntologyDAG) error {
	n.Logger().Info("Starting column feature extraction",
		zap.String("project_id", dag.ProjectID.String()),
		zap.String("datasource_id", dag.DatasourceID.String()))

	// Report initial progress
	if err := n.ReportProgress(ctx, 0, 1, "Extracting column features..."); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	// Check if methods are configured (nil means no-op mode)
	if n.methods == nil {
		n.Logger().Info("Column feature extraction skipped (not configured)",
			zap.String("project_id", dag.ProjectID.String()))
		if err := n.ReportProgress(ctx, 1, 1, "Column feature extraction skipped (not configured)"); err != nil {
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

	// Call the underlying service method to extract column features
	columnsProcessed, err := n.methods.ExtractColumnFeatures(ctx, dag.ProjectID, dag.DatasourceID, progressCallback)
	if err != nil {
		return err
	}

	// Report completion
	if err := n.ReportProgress(ctx, 1, 1, "Column feature extraction complete"); err != nil {
		n.Logger().Warn("Failed to report progress", zap.Error(err))
	}

	n.Logger().Info("Column feature extraction complete",
		zap.String("project_id", dag.ProjectID.String()),
		zap.Int("columns_processed", columnsProcessed))

	return nil
}
