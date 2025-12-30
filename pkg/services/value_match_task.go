package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

const (
	// ValueMatchThreshold is the minimum sample value overlap rate to create a candidate.
	// Set to 0.30 (30%) per plan spec to be permissive and let LLM review decide.
	ValueMatchThreshold = 0.30

	// MinDistinctForFK is the minimum distinct count for a column to be considered for FK matching.
	// Columns with fewer distinct values are likely enums, not foreign keys.
	MinDistinctForFK = 3
)

// excludedDataTypes are data types that should not be considered for FK relationships.
var excludedDataTypes = map[string]bool{
	// Temporal types - not meaningful for FK relationships
	"timestamp":                   true,
	"timestamptz":                 true,
	"date":                        true,
	"time":                        true,
	"timetz":                      true,
	"interval":                    true,
	"datetime":                    true,
	"timestamp with time zone":    true,
	"timestamp without time zone": true,
	"time with time zone":         true,
	"time without time zone":      true,

	// Boolean - too few values
	"boolean": true,
	"bool":    true,

	// Binary/LOB types
	"bytea":  true,
	"blob":   true,
	"binary": true,

	// Structured data types
	"json":  true,
	"jsonb": true,
	"xml":   true,

	// Geometry types
	"point":    true,
	"line":     true,
	"polygon":  true,
	"geometry": true,
}

// ValueMatchTask performs pairwise sample value comparison across joinable columns.
// It filters columns to joinable candidates and creates relationship candidates
// where sample value match rate >= 0.30.
type ValueMatchTask struct {
	workqueue.BaseTask
	workflowStateRepo repositories.WorkflowStateRepository
	candidateRepo     repositories.RelationshipCandidateRepository
	schemaRepo        repositories.SchemaRepository
	getTenantCtx      TenantContextFunc
	projectID         uuid.UUID
	workflowID        uuid.UUID
	datasourceID      uuid.UUID
	logger            *zap.Logger
}

// NewValueMatchTask creates a new value matching task.
func NewValueMatchTask(
	workflowStateRepo repositories.WorkflowStateRepository,
	candidateRepo repositories.RelationshipCandidateRepository,
	schemaRepo repositories.SchemaRepository,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	logger *zap.Logger,
) *ValueMatchTask {
	return &ValueMatchTask{
		BaseTask:          workqueue.NewBaseTask("Match column values", false), // Non-LLM task
		workflowStateRepo: workflowStateRepo,
		candidateRepo:     candidateRepo,
		schemaRepo:        schemaRepo,
		getTenantCtx:      getTenantCtx,
		projectID:         projectID,
		workflowID:        workflowID,
		datasourceID:      datasourceID,
		logger:            logger,
	}
}

// Execute implements workqueue.Task.
// Performs pairwise value matching across all joinable columns.
func (t *ValueMatchTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Get all column workflow states for this workflow
	allStates, err := t.workflowStateRepo.ListByWorkflow(tenantCtx, t.workflowID)
	if err != nil {
		return fmt.Errorf("list workflow states: %w", err)
	}

	// Load all columns and tables once to avoid N+N queries
	columns, err := t.schemaRepo.ListColumnsByDatasource(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}
	tables, err := t.schemaRepo.ListTablesByDatasource(tenantCtx, t.projectID, t.datasourceID)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	// Build lookup maps once
	tableIDByName := make(map[string]uuid.UUID)
	for _, table := range tables {
		tableIDByName[table.TableName] = table.ID
	}

	columnByTableAndName := make(map[string]*models.SchemaColumn)
	for _, col := range columns {
		// Find table name for this column
		for tableName, tableID := range tableIDByName {
			if col.SchemaTableID == tableID {
				key := fmt.Sprintf("%s.%s", tableName, col.ColumnName)
				columnByTableAndName[key] = col
				break
			}
		}
	}

	// Filter to scanned column entities and extract scan data
	scannedColumns := make([]*columnScanInfo, 0)
	for _, state := range allStates {
		if state.EntityType != models.WorkflowEntityTypeColumn {
			continue
		}
		if state.Status != models.WorkflowEntityStatusScanned {
			continue
		}

		// Extract column info from state data
		colInfo, err := t.extractColumnInfo(tenantCtx, state, columnByTableAndName)
		if err != nil {
			t.logger.Warn("Failed to extract column info from state",
				zap.String("entity_key", state.EntityKey),
				zap.Error(err))
			continue
		}

		scannedColumns = append(scannedColumns, colInfo)
	}

	t.logger.Info("Found scanned columns",
		zap.Int("total", len(scannedColumns)))

	// Filter to joinable columns
	joinableColumns := t.filterJoinable(scannedColumns)

	t.logger.Info("Filtered to joinable columns",
		zap.Int("joinable", len(joinableColumns)),
		zap.Int("excluded", len(scannedColumns)-len(joinableColumns)))

	// Perform pairwise matching
	// Follow FK convention: source = FK column (non-PK), target = PK column
	// This avoids creating bidirectional candidates
	candidatesCreated := 0
	for i := 0; i < len(joinableColumns); i++ {
		source := joinableColumns[i]

		// Source should not be a PK (we're looking for FK columns)
		// Exception: if no PKs exist, we still want to find relationships
		if source.isPK {
			continue
		}

		for j := 0; j < len(joinableColumns); j++ {
			// Skip if comparing column to itself
			if i == j {
				continue
			}

			target := joinableColumns[j]

			// Skip if same table
			if source.tableName == target.tableName {
				continue
			}

			// Target should preferably be a PK, but allow non-PK targets
			// (for cases where there's no explicit PK defined)

			// Compute match rate
			matchRate := t.computeMatchRate(source.sampleValues, target.sampleValues)

			// Create candidate if match rate meets threshold
			if matchRate >= ValueMatchThreshold {
				if err := t.createCandidate(tenantCtx, source, target, matchRate); err != nil {
					t.logger.Error("Failed to create candidate",
						zap.String("source", fmt.Sprintf("%s.%s", source.tableName, source.columnName)),
						zap.String("target", fmt.Sprintf("%s.%s", target.tableName, target.columnName)),
						zap.Error(err))
					continue
				}
				candidatesCreated++
			}
		}
	}

	t.logger.Info("Value matching completed",
		zap.Int("candidates_created", candidatesCreated))

	return nil
}

// columnScanInfo holds the extracted information from a scanned column.
type columnScanInfo struct {
	columnID      uuid.UUID
	tableName     string
	columnName    string
	dataType      string
	isPK          bool
	rowCount      int64
	distinctCount int64
	nullPercent   float64
	sampleValues  []string
}

// extractColumnInfo extracts column information from workflow state.
func (t *ValueMatchTask) extractColumnInfo(ctx context.Context, state *models.WorkflowEntityState, columnByTableAndName map[string]*models.SchemaColumn) (*columnScanInfo, error) {
	// Parse entity key: "table.column"
	parts := strings.SplitN(state.EntityKey, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid column entity key: %s", state.EntityKey)
	}
	tableName := parts[0]
	columnName := parts[1]

	// Extract scan data from state_data.gathered
	if state.StateData == nil || state.StateData.Gathered == nil {
		return nil, fmt.Errorf("no state data for column %s", state.EntityKey)
	}

	gathered := state.StateData.Gathered

	// Get row count
	rowCount, ok := gathered["row_count"].(int64)
	if !ok {
		// Try float64 (JSON unmarshaling may produce float64)
		if f, ok := gathered["row_count"].(float64); ok {
			rowCount = int64(f)
		} else {
			return nil, fmt.Errorf("row_count not found or invalid type")
		}
	}

	// Get distinct count
	distinctCount, ok := gathered["distinct_count"].(int64)
	if !ok {
		if f, ok := gathered["distinct_count"].(float64); ok {
			distinctCount = int64(f)
		} else {
			return nil, fmt.Errorf("distinct_count not found or invalid type")
		}
	}

	// Get null percent
	nullPercent, ok := gathered["null_percent"].(float64)
	if !ok {
		return nil, fmt.Errorf("null_percent not found or invalid type")
	}

	// Get sample values
	sampleValuesRaw, ok := gathered["sample_values"]
	if !ok {
		return nil, fmt.Errorf("sample_values not found")
	}

	// Convert sample values to []string
	sampleValues := make([]string, 0)
	if sampleValuesRaw != nil {
		switch v := sampleValuesRaw.(type) {
		case []interface{}:
			for _, item := range v {
				if str, ok := item.(string); ok {
					sampleValues = append(sampleValues, str)
				}
			}
		case []string:
			sampleValues = v
		default:
			return nil, fmt.Errorf("sample_values has unexpected type: %T", sampleValuesRaw)
		}
	}

	// Get column metadata from lookup map
	key := fmt.Sprintf("%s.%s", tableName, columnName)
	column, ok := columnByTableAndName[key]
	if !ok {
		return nil, fmt.Errorf("column not found: %s", key)
	}

	return &columnScanInfo{
		columnID:      column.ID,
		tableName:     tableName,
		columnName:    columnName,
		dataType:      column.DataType,
		isPK:          column.IsPrimaryKey,
		rowCount:      rowCount,
		distinctCount: distinctCount,
		nullPercent:   nullPercent,
		sampleValues:  sampleValues,
	}, nil
}

// filterJoinable filters columns to those suitable for FK relationships.
func (t *ValueMatchTask) filterJoinable(columns []*columnScanInfo) []*columnScanInfo {
	joinable := make([]*columnScanInfo, 0)

	for _, col := range columns {
		// Exclude by data type
		if t.isExcludedType(col.dataType) {
			continue
		}

		// Exclude low cardinality non-PK columns
		// If it's a PK, include even if low cardinality (it's a valid FK target)
		if col.distinctCount < MinDistinctForFK && !col.isPK {
			continue
		}

		// Column passes all filters
		joinable = append(joinable, col)
	}

	return joinable
}

// isExcludedType checks if a data type should be excluded from FK matching.
func (t *ValueMatchTask) isExcludedType(dataType string) bool {
	// Normalize type (lowercase, strip length/precision)
	normalized := strings.ToLower(dataType)
	if idx := strings.Index(normalized, "("); idx > 0 {
		normalized = normalized[:idx]
	}
	normalized = strings.TrimSpace(normalized)

	return excludedDataTypes[normalized]
}

// computeMatchRate calculates the proportion of source values that exist in target values.
func (t *ValueMatchTask) computeMatchRate(source, target []string) float64 {
	if len(source) == 0 {
		return 0.0
	}

	// Build target set for O(1) lookup
	targetSet := make(map[string]struct{}, len(target))
	for _, val := range target {
		targetSet[val] = struct{}{}
	}

	// Count matches
	matches := 0
	for _, val := range source {
		if _, exists := targetSet[val]; exists {
			matches++
		}
	}

	return float64(matches) / float64(len(source))
}

// createCandidate creates a new relationship candidate.
func (t *ValueMatchTask) createCandidate(ctx context.Context, source, target *columnScanInfo, matchRate float64) error {
	candidate := &models.RelationshipCandidate{
		WorkflowID:      t.workflowID,
		DatasourceID:    t.datasourceID,
		SourceColumnID:  source.columnID,
		TargetColumnID:  target.columnID,
		DetectionMethod: models.DetectionMethodValueMatch,
		Confidence:      matchRate, // Initial confidence is just the match rate
		Status:          models.RelCandidateStatusPending,
		IsRequired:      false, // Will be set by LLM analysis task
	}

	// Store match rate
	candidate.ValueMatchRate = &matchRate

	return t.candidateRepo.Create(ctx, candidate)
}
