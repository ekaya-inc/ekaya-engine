package services

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// ColumnScanTask scans a single column to collect statistics and sample values.
// This is a shared task used by both relationship detection and ontology extraction.
// It's a non-LLM data task that populates workflow_state.state_data.gathered.
type ColumnScanTask struct {
	workqueue.BaseTask
	workflowStateRepo repositories.WorkflowStateRepository
	dsSvc             DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	getTenantCtx      TenantContextFunc
	projectID         uuid.UUID
	workflowID        uuid.UUID
	datasourceID      uuid.UUID
	tableName         string
	schemaName        string
	columnName        string
}

// NewColumnScanTask creates a new scan task for a single column.
func NewColumnScanTask(
	workflowStateRepo repositories.WorkflowStateRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	tableName string,
	schemaName string,
	columnName string,
) *ColumnScanTask {
	displayName := fmt.Sprintf("%s.%s", tableName, columnName)
	if schemaName != "" && schemaName != "public" {
		displayName = fmt.Sprintf("%s.%s.%s", schemaName, tableName, columnName)
	}
	return &ColumnScanTask{
		BaseTask:          workqueue.NewBaseTask(fmt.Sprintf("Scan %s", displayName), false), // Non-LLM task
		workflowStateRepo: workflowStateRepo,
		dsSvc:             dsSvc,
		adapterFactory:    adapterFactory,
		getTenantCtx:      getTenantCtx,
		projectID:         projectID,
		workflowID:        workflowID,
		datasourceID:      datasourceID,
		tableName:         tableName,
		schemaName:        schemaName,
		columnName:        columnName,
	}
}

// Execute implements workqueue.Task.
// Scans the column and updates its scan data in workflow state.
func (t *ColumnScanTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Get column workflow state
	colEntityKey := models.ColumnEntityKey(t.tableName, t.columnName)
	ws, err := t.workflowStateRepo.GetByEntity(tenantCtx, t.workflowID, models.WorkflowEntityTypeColumn, colEntityKey)
	if err != nil {
		return fmt.Errorf("get column workflow state: %w", err)
	}
	if ws == nil {
		return fmt.Errorf("column workflow state not found: %s", colEntityKey)
	}

	// Update status to scanning
	if err := t.workflowStateRepo.UpdateStatus(tenantCtx, ws.ID, models.WorkflowEntityStatusScanning, nil); err != nil {
		return fmt.Errorf("update column workflow state status to scanning: %w", err)
	}

	// Get datasource with decrypted config
	ds, err := t.dsSvc.Get(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer for background task.
	// Background tasks use empty userID since they run outside user session context.
	// Connection manager pools by (projectID, userID, datasourceID), so empty userID
	// means all background tasks for this project share one connection pool.
	discoverer, err := t.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, t.projectID, t.datasourceID, "")
	if err != nil {
		return fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Get column statistics
	stats, err := discoverer.AnalyzeColumnStats(ctx, t.schemaName, t.tableName, []string{t.columnName})
	if err != nil {
		return fmt.Errorf("analyze column stats: %w", err)
	}

	// Should have exactly one stat result
	if len(stats) != 1 {
		return fmt.Errorf("expected 1 column stat, got %d", len(stats))
	}
	colStats := stats[0]

	// Get sample values (up to 50)
	sampleValues, err := discoverer.GetDistinctValues(ctx, t.schemaName, t.tableName, t.columnName, 50)
	if err != nil {
		// Log but continue - some columns may not be scannable (e.g., binary types)
		sampleValues = nil
	}

	// Calculate null percentage
	nullPercent := 0.0
	if colStats.RowCount > 0 {
		nullPercent = float64(colStats.RowCount-colStats.NonNullCount) / float64(colStats.RowCount) * 100
	}

	// Determine if column is an enum candidate
	// Heuristic: distinct_count <= 50 AND distinct_count < row_count * 0.1
	isEnumCandidate := false
	if colStats.DistinctCount > 0 && colStats.DistinctCount <= 50 && colStats.RowCount > 0 {
		if float64(colStats.DistinctCount) < float64(colStats.RowCount)*0.1 {
			isEnumCandidate = true
		}
	}

	// Compute value fingerprint for change detection
	fingerprint := ""
	if len(sampleValues) > 0 {
		sorted := make([]string, len(sampleValues))
		copy(sorted, sampleValues)
		sort.Strings(sorted)
		hash := sha256.Sum256([]byte(fmt.Sprintf("%v", sorted)))
		fingerprint = fmt.Sprintf("%x", hash[:8]) // Use first 8 bytes
	}

	scannedAt := time.Now()

	// Update column workflow state with gathered data
	ws.Status = models.WorkflowEntityStatusScanned
	ws.StateData = &models.WorkflowStateData{
		Gathered: map[string]any{
			"row_count":         colStats.RowCount,
			"non_null_count":    colStats.NonNullCount,
			"distinct_count":    colStats.DistinctCount,
			"null_percent":      nullPercent,
			"sample_values":     sampleValues,
			"is_enum_candidate": isEnumCandidate,
			"value_fingerprint": fingerprint,
			"scanned_at":        scannedAt,
		},
	}
	if err := t.workflowStateRepo.Update(tenantCtx, ws); err != nil {
		return fmt.Errorf("update column workflow state: %w", err)
	}

	return nil
}
