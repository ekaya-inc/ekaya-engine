package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// TestJoinTask runs actual SQL joins against the datasource to determine
// relationship cardinality, match rates, orphan rates, and target coverage.
type TestJoinTask struct {
	workqueue.BaseTask
	candidateRepo  repositories.RelationshipCandidateRepository
	schemaRepo     repositories.SchemaRepository
	dsSvc          DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	getTenantCtx   TenantContextFunc
	projectID      uuid.UUID
	workflowID     uuid.UUID
	datasourceID   uuid.UUID
	candidateID    uuid.UUID // Specific candidate to test
	logger         *zap.Logger
}

// NewTestJoinTask creates a new test join task for a specific candidate.
func NewTestJoinTask(
	candidateRepo repositories.RelationshipCandidateRepository,
	schemaRepo repositories.SchemaRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	candidateID uuid.UUID,
	logger *zap.Logger,
) *TestJoinTask {
	return &TestJoinTask{
		BaseTask:       workqueue.NewBaseTask(fmt.Sprintf("Test join for candidate %s", candidateID), false), // Non-LLM task
		candidateRepo:  candidateRepo,
		schemaRepo:     schemaRepo,
		dsSvc:          dsSvc,
		adapterFactory: adapterFactory,
		getTenantCtx:   getTenantCtx,
		projectID:      projectID,
		workflowID:     workflowID,
		datasourceID:   datasourceID,
		candidateID:    candidateID,
		logger:         logger,
	}
}

// Execute implements workqueue.Task.
// Performs SQL join analysis for the candidate and updates metrics.
func (t *TestJoinTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Get candidate
	candidate, err := t.candidateRepo.GetByID(tenantCtx, t.candidateID)
	if err != nil {
		return fmt.Errorf("get candidate: %w", err)
	}

	// Get source and target column info
	sourceCol, err := t.schemaRepo.GetColumnByID(tenantCtx, t.projectID, candidate.SourceColumnID)
	if err != nil {
		return fmt.Errorf("get source column: %w", err)
	}

	targetCol, err := t.schemaRepo.GetColumnByID(tenantCtx, t.projectID, candidate.TargetColumnID)
	if err != nil {
		return fmt.Errorf("get target column: %w", err)
	}

	// Get table info
	sourceTable, err := t.schemaRepo.GetTableByID(tenantCtx, t.projectID, sourceCol.SchemaTableID)
	if err != nil {
		return fmt.Errorf("get source table: %w", err)
	}

	targetTable, err := t.schemaRepo.GetTableByID(tenantCtx, t.projectID, targetCol.SchemaTableID)
	if err != nil {
		return fmt.Errorf("get target table: %w", err)
	}

	// Get datasource with decrypted config
	ds, err := t.dsSvc.Get(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer to run join analysis.
	// Background tasks use empty userID since they run outside user session context.
	// Connection manager pools by (projectID, userID, datasourceID), so empty userID
	// means all background tasks for this project share one connection pool.
	discoverer, err := t.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, t.projectID, t.datasourceID, "")
	if err != nil {
		return fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	t.logger.Info("Running join analysis",
		zap.String("source", fmt.Sprintf("%s.%s.%s", sourceTable.SchemaName, sourceTable.TableName, sourceCol.ColumnName)),
		zap.String("target", fmt.Sprintf("%s.%s.%s", targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)))

	// Run join analysis via adapter
	joinResult, err := discoverer.AnalyzeJoin(ctx,
		sourceTable.SchemaName, sourceTable.TableName, sourceCol.ColumnName,
		targetTable.SchemaName, targetTable.TableName, targetCol.ColumnName)
	if err != nil {
		// Log error but don't fail the task - mark candidate with error state
		t.logger.Error("Join analysis failed",
			zap.String("candidate_id", t.candidateID.String()),
			zap.Error(err))
		// Could set an error field on candidate here if we add one
		return fmt.Errorf("join analysis failed: %w", err)
	}

	// Calculate metrics from join result
	metrics, err := t.calculateMetrics(tenantCtx, joinResult, sourceTable, targetTable)
	if err != nil {
		return fmt.Errorf("calculate metrics: %w", err)
	}

	// Update candidate with metrics
	candidate.Cardinality = &metrics.Cardinality
	candidate.JoinMatchRate = &metrics.JoinMatchRate
	candidate.OrphanRate = &metrics.OrphanRate
	candidate.TargetCoverage = &metrics.TargetCoverage
	candidate.SourceRowCount = &metrics.SourceRowCount
	candidate.TargetRowCount = &metrics.TargetRowCount
	candidate.MatchedRows = &metrics.MatchedRows
	candidate.OrphanRows = &metrics.OrphanRows

	if err := t.candidateRepo.Update(tenantCtx, candidate); err != nil {
		return fmt.Errorf("update candidate: %w", err)
	}

	t.logger.Info("Join analysis completed",
		zap.String("candidate_id", t.candidateID.String()),
		zap.String("cardinality", metrics.Cardinality),
		zap.Float64("match_rate", metrics.JoinMatchRate),
		zap.Float64("orphan_rate", metrics.OrphanRate),
		zap.Float64("target_coverage", metrics.TargetCoverage))

	return nil
}

// JoinMetrics holds the calculated metrics from join analysis.
type JoinMetrics struct {
	Cardinality    string
	JoinMatchRate  float64
	OrphanRate     float64
	TargetCoverage float64
	SourceRowCount int64
	TargetRowCount int64
	MatchedRows    int64
	OrphanRows     int64
}

// calculateMetrics computes cardinality and percentages from join analysis result.
func (t *TestJoinTask) calculateMetrics(
	ctx context.Context,
	joinResult *datasource.JoinAnalysis,
	sourceTable, targetTable *models.SchemaTable,
) (*JoinMetrics, error) {
	// Get actual row counts from tables (more reliable than join counts)
	// RowCount is *int64, need to dereference or use default
	sourceRowCount := int64(1) // Default to avoid division by zero
	if sourceTable.RowCount != nil && *sourceTable.RowCount > 0 {
		sourceRowCount = *sourceTable.RowCount
	}
	targetRowCount := int64(1) // Default to avoid division by zero
	if targetTable.RowCount != nil && *targetTable.RowCount > 0 {
		targetRowCount = *targetTable.RowCount
	}

	// Calculate matched rows (distinct source values that have matches)
	matchedRows := sourceRowCount - joinResult.OrphanCount

	metrics := &JoinMetrics{
		SourceRowCount: sourceRowCount,
		TargetRowCount: targetRowCount,
		MatchedRows:    matchedRows,
		OrphanRows:     joinResult.OrphanCount,
		JoinMatchRate:  float64(matchedRows) / float64(sourceRowCount),
		OrphanRate:     float64(joinResult.OrphanCount) / float64(sourceRowCount),
		TargetCoverage: float64(joinResult.TargetMatched) / float64(targetRowCount),
	}

	// Determine cardinality from join patterns
	metrics.Cardinality = t.determineCardinality(joinResult, sourceRowCount, targetRowCount)

	return metrics, nil
}

// determineCardinality infers the relationship cardinality from join counts.
//
// Cardinality interpretation:
//   - 1:1 - Each source row matches exactly one target row, and vice versa
//   - N:1 - Many source rows point to one target row (typical FK)
//   - 1:N - One source row can have many target rows
//   - N:M - Many-to-many (junction table or data quality issue)
//
// The SQL analysis provides:
//   - JoinCount: total rows from inner join
//   - SourceMatched: COUNT(DISTINCT source_column) in join results
//   - TargetMatched: COUNT(DISTINCT target_column) in join results
//
// Key insight: SourceMatched/TargetMatched count distinct COLUMN VALUES, not rows.
// For a typical FK relationship (source.fk → target.pk):
//   - SourceMatched = distinct FK values that matched
//   - TargetMatched = distinct PK values that were referenced
//   - JoinCount = source rows that matched (no inflation from unique target)
//
// Detection strategy:
//   - N:M: Both JoinCount/SourceMatched > 1 AND JoinCount/TargetMatched > 1
//   - N:1: Only JoinCount/TargetMatched > 1 (many sources share each target)
//   - 1:N: Only JoinCount/SourceMatched > 1 (each source has many targets)
//   - 1:1: Both ratios ≈ 1
func (t *TestJoinTask) determineCardinality(
	joinResult *datasource.JoinAnalysis,
	sourceRowCount, targetRowCount int64,
) string {
	// Edge cases
	if joinResult.JoinCount == 0 || joinResult.SourceMatched == 0 {
		return "N:1" // No matches, assume typical FK pattern
	}

	// Use a small tolerance for "approximately equal" comparisons
	const tolerance = 0.05

	// Calculate ratios
	// avgTargetsPerSource: if > 1, each distinct source value maps to multiple targets
	avgTargetsPerSource := float64(joinResult.JoinCount) / float64(joinResult.SourceMatched)
	// avgSourcesPerTarget: if > 1, each distinct target value is referenced by multiple sources
	avgSourcesPerTarget := float64(joinResult.JoinCount) / float64(joinResult.TargetMatched)

	sourceHasMultiple := avgTargetsPerSource > (1.0 + tolerance)
	targetHasMultiple := avgSourcesPerTarget > (1.0 + tolerance)

	switch {
	case sourceHasMultiple && targetHasMultiple:
		// Both sides have row inflation
		// For FK→PK relationships, source can have duplicate FK values (normal for N:1)
		// Distinguish N:1 from N:M by checking if target looks like the "one" side
		// Heuristic: if target_matched <= source_matched, target is likely the "one" side
		if joinResult.TargetMatched <= joinResult.SourceMatched {
			return "N:1" // Typical FK pattern with duplicate FK values
		}
		return "N:M" // Junction table: more distinct targets than sources
	case !sourceHasMultiple && targetHasMultiple:
		// Many sources share each target → N:1 (typical FK pattern)
		// Example: many orders reference the same user
		return "N:1"
	case sourceHasMultiple && !targetHasMultiple:
		// Each source maps to multiple targets → 1:N
		// Example: one user has many phone numbers
		return "1:N"
	default:
		// Both ratios ≈ 1 → 1:1
		return "1:1"
	}
}

// FormatJoinTaskDescription creates a display name for a test join task.
func FormatJoinTaskDescription(sourceTable, sourceColumn, targetTable, targetColumn string) string {
	// Shorten for display: "users.id → orders.user_id"
	source := fmt.Sprintf("%s.%s", shortTableName(sourceTable), sourceColumn)
	target := fmt.Sprintf("%s.%s", shortTableName(targetTable), targetColumn)
	return fmt.Sprintf("Test join: %s → %s", source, target)
}

// shortTableName extracts the table name without schema prefix for display.
func shortTableName(fullName string) string {
	parts := strings.Split(fullName, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return fullName
}
