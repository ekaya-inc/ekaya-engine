package services

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// ColumnFeatureExtractionService extracts deterministic features from database columns.
// This runs early in the ontology extraction DAG to provide feature data that informs
// downstream LLM-based nodes (Entity Discovery, FK Discovery, Column Enrichment).
//
// Features extracted:
// - Data types and nullable status (from schema)
// - Sample values (already collected during schema extraction)
// - Value distributions (distinct count, null rate, cardinality ratio)
// - Pattern detection (UUID, ISO 4217 currency, Stripe/Twilio IDs, timestamp scales)
//
// Unlike the current approach (which hardcodes English column name patterns and overrides
// LLM output), this service extracts features that are then passed TO the LLM prompts,
// allowing the LLM to make semantic decisions with full context.
type ColumnFeatureExtractionService interface {
	// ExtractColumnFeatures extracts deterministic features from all columns in the datasource.
	// Returns the number of columns processed.
	ExtractColumnFeatures(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) (int, error)
}

type columnFeatureExtractionService struct {
	schemaRepo        repositories.SchemaRepository
	datasourceService DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	llmFactory        llm.LLMClientFactory
	workerPool        *llm.WorkerPool
	getTenantCtx      TenantContextFunc
	logger            *zap.Logger

	// Cached classifiers (created lazily)
	classifiersMu sync.RWMutex
	classifiers   map[models.ClassificationPath]ColumnClassifier
}

// NewColumnFeatureExtractionService creates a new column feature extraction service.
func NewColumnFeatureExtractionService(
	schemaRepo repositories.SchemaRepository,
	logger *zap.Logger,
) ColumnFeatureExtractionService {
	return &columnFeatureExtractionService{
		schemaRepo:  schemaRepo,
		logger:      logger.Named("column-feature-extraction"),
		classifiers: make(map[models.ClassificationPath]ColumnClassifier),
	}
}

// NewColumnFeatureExtractionServiceWithLLM creates a column feature extraction service with LLM support.
// Use this constructor for full Phase 2+ functionality.
func NewColumnFeatureExtractionServiceWithLLM(
	schemaRepo repositories.SchemaRepository,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) ColumnFeatureExtractionService {
	return &columnFeatureExtractionService{
		schemaRepo:   schemaRepo,
		llmFactory:   llmFactory,
		workerPool:   workerPool,
		getTenantCtx: getTenantCtx,
		logger:       logger.Named("column-feature-extraction"),
		classifiers:  make(map[models.ClassificationPath]ColumnClassifier),
	}
}

// NewColumnFeatureExtractionServiceFull creates a column feature extraction service with all dependencies.
// Use this constructor for full Phase 2-4 functionality including FK resolution with data overlap queries.
func NewColumnFeatureExtractionServiceFull(
	schemaRepo repositories.SchemaRepository,
	datasourceService DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	workerPool *llm.WorkerPool,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) ColumnFeatureExtractionService {
	return &columnFeatureExtractionService{
		schemaRepo:        schemaRepo,
		datasourceService: datasourceService,
		adapterFactory:    adapterFactory,
		llmFactory:        llmFactory,
		workerPool:        workerPool,
		getTenantCtx:      getTenantCtx,
		logger:            logger.Named("column-feature-extraction"),
		classifiers:       make(map[models.ClassificationPath]ColumnClassifier),
	}
}

// Phase1Result contains everything needed for subsequent phases.
type Phase1Result struct {
	Profiles     []*models.ColumnDataProfile
	TotalColumns int

	// Queue for Phase 2: all columns for classification
	Phase2Queue []uuid.UUID
}

// samplePatterns defines regex patterns for detecting specific data formats in sample values.
// Patterns are matched against column DATA (not column names) to make data-driven classification decisions.
var samplePatterns = map[string]*regexp.Regexp{
	models.PatternUUID:        regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
	models.PatternStripeID:    regexp.MustCompile(`^(pi_|pm_|ch_|cus_|sub_|inv_|price_|prod_|txn_|re_|pout_|seti_|cs_)[a-zA-Z0-9]+$`),
	models.PatternAWSSES:      regexp.MustCompile(`^[0-9a-f-]+@email\.amazonses\.com$`),
	models.PatternTwilioSID:   regexp.MustCompile(`^(AC|SM|MM|PN|SK)[a-f0-9]{32}$`),
	models.PatternISO4217:     regexp.MustCompile(`^[A-Z]{3}$`),
	models.PatternUnixSeconds: regexp.MustCompile(`^[0-9]{10}$`),
	models.PatternUnixMillis:  regexp.MustCompile(`^[0-9]{13}$`),
	models.PatternUnixMicros:  regexp.MustCompile(`^[0-9]{16}$`),
	models.PatternUnixNanos:   regexp.MustCompile(`^[0-9]{19}$`),
	models.PatternEmail:       regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`),
	models.PatternURL:         regexp.MustCompile(`^https?://`),
}

// ExtractColumnFeatures extracts deterministic features from all columns in the datasource.
// When LLM support is configured (via NewColumnFeatureExtractionServiceWithLLM), this also
// runs Phase 2 for LLM-based column classification.
func (s *columnFeatureExtractionService) ExtractColumnFeatures(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) (int, error) {
	s.logger.Info("Starting column feature extraction",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()),
		zap.Bool("llm_enabled", s.llmFactory != nil))

	// Run Phase 1: Data Collection (deterministic, no LLM)
	phase1Result, err := s.runPhase1DataCollection(ctx, projectID, datasourceID, progressCallback)
	if err != nil {
		return 0, fmt.Errorf("phase 1 data collection failed: %w", err)
	}

	s.logger.Info("Column feature extraction Phase 1 complete",
		zap.Int("columns_processed", phase1Result.TotalColumns))

	// Phase 2: Column Classification (requires LLM support)
	if s.llmFactory != nil && s.workerPool != nil && len(phase1Result.Profiles) > 0 {
		s.logger.Info("Starting Phase 2: Column Classification",
			zap.Int("columns_to_classify", len(phase1Result.Profiles)))

		phase2Result, err := s.runPhase2ColumnClassification(ctx, projectID, phase1Result.Profiles, progressCallback)
		if err != nil {
			return 0, fmt.Errorf("phase 2 column classification failed: %w", err)
		}

		s.logger.Info("Column feature extraction Phase 2 complete",
			zap.Int("columns_classified", len(phase2Result.Features)),
			zap.Int("enum_candidates", len(phase2Result.Phase3EnumQueue)),
			zap.Int("fk_candidates", len(phase2Result.Phase4FKQueue)),
			zap.Int("cross_column_tables", len(phase2Result.Phase5CrossColumnQueue)))

		// Phase 3: Enum Value Analysis (parallel LLM, 1 request/enum column)
		if err := s.runPhase3EnumAnalysis(ctx, projectID, phase2Result.Phase3EnumQueue, phase1Result.Profiles, phase2Result.Features, progressCallback); err != nil {
			return 0, fmt.Errorf("phase 3 enum analysis failed: %w", err)
		}

		// Phase 4: FK Resolution (parallel LLM, with data overlap queries)
		if err := s.runPhase4FKResolution(ctx, projectID, datasourceID, phase2Result.Phase4FKQueue, phase1Result.Profiles, phase2Result.Features, progressCallback); err != nil {
			return 0, fmt.Errorf("phase 4 FK resolution failed: %w", err)
		}

		// Phase 5: Cross-Column Analysis (parallel LLM, 1 request/table)
		if err := s.runPhase5CrossColumnAnalysis(ctx, projectID, phase2Result.Phase5CrossColumnQueue, phase1Result.Profiles, phase2Result.Features, progressCallback); err != nil {
			return 0, fmt.Errorf("phase 5 cross-column analysis failed: %w", err)
		}

		// TODO: Phase 6 (store results) will be implemented in subsequent tasks
	}

	return phase1Result.TotalColumns, nil
}

// runPhase1DataCollection gathers column data and determines classification paths.
// Phase 1 is deterministic with NO LLM calls. It collects metadata, runs regex patterns
// against sample values, and routes columns to classification paths based on TYPE + DATA.
func (s *columnFeatureExtractionService) runPhase1DataCollection(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) (*Phase1Result, error) {
	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, 1, "Collecting column metadata...")
	}

	// Get all tables to build a tableID -> tableName lookup
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	tableNameByID := make(map[uuid.UUID]string, len(tables))
	tableRowCountByID := make(map[uuid.UUID]int64, len(tables))
	for _, t := range tables {
		tableNameByID[t.ID] = t.TableName
		if t.RowCount != nil {
			tableRowCountByID[t.ID] = *t.RowCount
		}
	}

	// Get all columns for the datasource
	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns: %w", err)
	}

	totalColumns := len(columns)
	s.logger.Info("Found columns to process",
		zap.Int("total_columns", totalColumns),
		zap.Int("total_tables", len(tables)))

	if totalColumns == 0 {
		if progressCallback != nil {
			progressCallback(1, 1, "No columns to process")
		}
		return &Phase1Result{
			Profiles:     make([]*models.ColumnDataProfile, 0),
			TotalColumns: 0,
			Phase2Queue:  make([]uuid.UUID, 0),
		}, nil
	}

	// Build profiles for each column
	profiles := make([]*models.ColumnDataProfile, 0, totalColumns)
	phase2Queue := make([]uuid.UUID, 0, totalColumns)

	for i, col := range columns {
		// Report progress
		if progressCallback != nil {
			tableName := tableNameByID[col.SchemaTableID]
			progressCallback(i+1, totalColumns, fmt.Sprintf("%s.%s", tableName, col.ColumnName))
		}

		// Build the column profile
		profile := s.buildColumnProfile(col, tableNameByID, tableRowCountByID)

		// Detect patterns in sample values
		profile.DetectedPatterns = s.detectPatternsInSamples(col.SampleValues)

		// Route to classification path based on TYPE + DATA (not names)
		profile.ClassificationPath = s.routeToClassificationPath(profile)

		profiles = append(profiles, profile)
		phase2Queue = append(phase2Queue, col.ID)
	}

	if progressCallback != nil {
		progressCallback(totalColumns, totalColumns, fmt.Sprintf("Found %d columns in %d tables", totalColumns, len(tables)))
	}

	return &Phase1Result{
		Profiles:     profiles,
		TotalColumns: totalColumns,
		Phase2Queue:  phase2Queue,
	}, nil
}

// buildColumnProfile converts a SchemaColumn to a ColumnDataProfile.
func (s *columnFeatureExtractionService) buildColumnProfile(
	col *models.SchemaColumn,
	tableNameByID map[uuid.UUID]string,
	tableRowCountByID map[uuid.UUID]int64,
) *models.ColumnDataProfile {
	profile := &models.ColumnDataProfile{
		ColumnID:     col.ID,
		ColumnName:   col.ColumnName,
		TableID:      col.SchemaTableID,
		TableName:    tableNameByID[col.SchemaTableID],
		DataType:     col.DataType,
		IsPrimaryKey: col.IsPrimaryKey,
		IsUnique:     col.IsUnique,
		IsNullable:   col.IsNullable,
		SampleValues: col.SampleValues,
	}

	// Get row count from table
	rowCount := tableRowCountByID[col.SchemaTableID]
	if col.RowCount != nil {
		rowCount = *col.RowCount
	}
	profile.RowCount = rowCount

	// Set distinct count
	if col.DistinctCount != nil {
		profile.DistinctCount = *col.DistinctCount
	}

	// Set null count and compute null rate
	if col.NullCount != nil {
		profile.NullCount = *col.NullCount
	}
	if rowCount > 0 {
		profile.NullRate = float64(profile.NullCount) / float64(rowCount)
	}

	// Compute cardinality (distinct_count / row_count)
	if rowCount > 0 && profile.DistinctCount > 0 {
		profile.Cardinality = float64(profile.DistinctCount) / float64(rowCount)
	}

	// Set text column length stats
	if col.MinLength != nil {
		profile.MinLength = col.MinLength
	}
	if col.MaxLength != nil {
		profile.MaxLength = col.MaxLength
	}

	return profile
}

// detectPatternsInSamples runs regex patterns against sample values to detect data formats.
// This is the core data-driven approach: patterns are matched against actual DATA, not column names.
func (s *columnFeatureExtractionService) detectPatternsInSamples(sampleValues []string) []models.DetectedPattern {
	if len(sampleValues) == 0 {
		return nil
	}

	var patterns []models.DetectedPattern

	for patternName, regex := range samplePatterns {
		matchCount := 0
		matchedValues := make([]string, 0)

		for _, val := range sampleValues {
			if regex.MatchString(val) {
				matchCount++
				// Keep up to 5 examples
				if len(matchedValues) < 5 {
					matchedValues = append(matchedValues, val)
				}
			}
		}

		if matchCount > 0 {
			matchRate := float64(matchCount) / float64(len(sampleValues))
			patterns = append(patterns, models.DetectedPattern{
				PatternName:   patternName,
				MatchRate:     matchRate,
				MatchedValues: matchedValues,
			})
		}
	}

	return patterns
}

// routeToClassificationPath determines the classification path based on TYPE + DATA characteristics.
// This is the core routing logic that replaces static column name pattern matching.
// Columns are routed to paths based on their data type and sample value patterns.
func (s *columnFeatureExtractionService) routeToClassificationPath(profile *models.ColumnDataProfile) models.ClassificationPath {
	// Route based on data type hierarchy
	switch {
	case isTimestampType(profile.DataType):
		return models.ClassificationPathTimestamp

	case isBooleanType(profile.DataType):
		return models.ClassificationPathBoolean

	case isIntegerType(profile.DataType):
		return s.routeIntegerColumn(profile)

	case isUUIDTypeForClassification(profile.DataType):
		return models.ClassificationPathUUID

	case isTextType(profile.DataType):
		return s.routeTextColumn(profile)

	case isJSONTypeForClassification(profile.DataType):
		return models.ClassificationPathJSON

	default:
		return models.ClassificationPathUnknown
	}
}

// routeIntegerColumn routes integer columns based on their data characteristics.
// Sub-routing: boolean values → Boolean path, unix timestamps → Timestamp path, low cardinality → Enum path.
func (s *columnFeatureExtractionService) routeIntegerColumn(profile *models.ColumnDataProfile) models.ClassificationPath {
	// Check if values are boolean-like (0, 1 only)
	if profile.HasOnlyBooleanValues() {
		return models.ClassificationPathBoolean
	}

	// Check if values look like unix timestamps
	if hasUnixTimestampPattern(profile) {
		return models.ClassificationPathTimestamp
	}

	// Check for low cardinality enum pattern
	// Cardinality < 1% and at most 50 distinct values suggests an enum
	if profile.Cardinality < 0.01 && profile.DistinctCount <= 50 && profile.DistinctCount > 0 {
		return models.ClassificationPathEnum
	}

	return models.ClassificationPathNumeric
}

// routeTextColumn routes text columns based on their data characteristics.
// Sub-routing: UUID patterns → UUID path, external ID patterns → ExtID path, low cardinality → Enum path.
func (s *columnFeatureExtractionService) routeTextColumn(profile *models.ColumnDataProfile) models.ClassificationPath {
	// Check if values match UUID pattern (>95% match rate)
	if profile.MatchesPattern(models.PatternUUID) {
		return models.ClassificationPathUUID
	}

	// Check for external service ID patterns
	if matchesExternalIDPattern(profile) {
		return models.ClassificationPathExternalID
	}

	// Check for low cardinality enum pattern
	// Cardinality < 1% and at most 50 distinct values suggests an enum
	if profile.Cardinality < 0.01 && profile.DistinctCount <= 50 && profile.DistinctCount > 0 {
		return models.ClassificationPathEnum
	}

	return models.ClassificationPathText
}

// hasUnixTimestampPattern checks if integer sample values match unix timestamp patterns
// and validates that they convert to reasonable dates (1970-2100).
func hasUnixTimestampPattern(profile *models.ColumnDataProfile) bool {
	// Check for any unix timestamp pattern detection
	for _, pattern := range profile.DetectedPatterns {
		switch pattern.PatternName {
		case models.PatternUnixSeconds, models.PatternUnixMillis, models.PatternUnixMicros, models.PatternUnixNanos:
			// Validate at least 80% match rate
			if pattern.MatchRate >= 0.80 {
				// Validate that matched values convert to reasonable dates
				if validateUnixTimestamps(pattern.MatchedValues, pattern.PatternName) {
					return true
				}
			}
		}
	}
	return false
}

// validateUnixTimestamps checks if unix timestamp values convert to dates between 1970 and 2100.
func validateUnixTimestamps(values []string, patternName string) bool {
	if len(values) == 0 {
		return false
	}

	minYear := 1970
	maxYear := 2100
	validCount := 0

	for _, val := range values {
		ts, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			continue
		}

		// Convert to seconds based on scale
		var seconds int64
		switch patternName {
		case models.PatternUnixSeconds:
			seconds = ts
		case models.PatternUnixMillis:
			seconds = ts / 1000
		case models.PatternUnixMicros:
			seconds = ts / 1000000
		case models.PatternUnixNanos:
			seconds = ts / 1000000000
		}

		// Check if date is in reasonable range
		t := time.Unix(seconds, 0)
		year := t.Year()
		if year >= minYear && year <= maxYear {
			validCount++
		}
	}

	// At least half of the values should be valid timestamps
	return validCount > 0 && validCount >= len(values)/2
}

// matchesExternalIDPattern checks if the column matches known external service ID patterns.
func matchesExternalIDPattern(profile *models.ColumnDataProfile) bool {
	// Check for known external service patterns with high match rate
	externalPatterns := []string{
		models.PatternStripeID,
		models.PatternAWSSES,
		models.PatternTwilioSID,
	}

	for _, patternName := range externalPatterns {
		if profile.MatchesPatternWithThreshold(patternName, 0.80) {
			return true
		}
	}

	return false
}

// Type detection helper functions - these check data types not column names.
// Note: isTimestampType, isBooleanType, isIntegerType, isTextType are already defined
// in column_enrichment.go and ontology_finalization.go in this package.

// isUUIDTypeForClassification checks if the data type is a UUID type.
// Named differently to avoid collision with other type checks in the package.
func isUUIDTypeForClassification(dataType string) bool {
	lower := strings.ToLower(dataType)
	return lower == "uuid"
}

// isJSONTypeForClassification checks if the data type is a JSON type.
// Named differently to avoid collision with other type checks in the package.
func isJSONTypeForClassification(dataType string) bool {
	lower := strings.ToLower(dataType)
	return lower == "json" || lower == "jsonb"
}

// Ensure the service implements the dag.ColumnFeatureExtractionMethods interface.
var _ dag.ColumnFeatureExtractionMethods = (*columnFeatureExtractionService)(nil)

// ============================================================================
// Phase 2: Column Classification (Parallel LLM)
// ============================================================================

// Phase2Result contains classification results and queues for follow-up phases.
type Phase2Result struct {
	Features []*models.ColumnFeatures

	// Queues for subsequent phases (populated based on classification results)
	Phase3EnumQueue        []uuid.UUID // Columns needing enum value analysis
	Phase4FKQueue          []uuid.UUID // Columns needing FK resolution
	Phase5CrossColumnQueue []string    // Tables needing cross-column analysis
}

// runPhase2ColumnClassification classifies each column with a focused LLM request.
// Each column gets ONE focused LLM request. Progress updates after each completion.
func (s *columnFeatureExtractionService) runPhase2ColumnClassification(
	ctx context.Context,
	projectID uuid.UUID,
	profiles []*models.ColumnDataProfile,
	progressCallback dag.ProgressCallback,
) (*Phase2Result, error) {
	if len(profiles) == 0 {
		return &Phase2Result{
			Features:               make([]*models.ColumnFeatures, 0),
			Phase3EnumQueue:        make([]uuid.UUID, 0),
			Phase4FKQueue:          make([]uuid.UUID, 0),
			Phase5CrossColumnQueue: make([]string, 0),
		}, nil
	}

	// Check if LLM support is available
	if s.llmFactory == nil || s.workerPool == nil {
		return nil, fmt.Errorf("phase 2 requires LLM support: use NewColumnFeatureExtractionServiceWithLLM constructor")
	}

	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, len(profiles), "Classifying columns")
	}

	// Build work items - ONE LLM request per column
	workItems := make([]llm.WorkItem[*models.ColumnFeatures], 0, len(profiles))
	for _, profile := range profiles {
		p := profile // capture for closure
		workItems = append(workItems, llm.WorkItem[*models.ColumnFeatures]{
			ID: p.ColumnID.String(),
			Execute: func(ctx context.Context) (*models.ColumnFeatures, error) {
				return s.classifySingleColumn(ctx, projectID, p)
			},
		})
	}

	// Process in parallel with progress updates
	results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total, "Classifying columns")
		}
	})

	// Collect results and build queues for next phases
	result := &Phase2Result{
		Features:               make([]*models.ColumnFeatures, 0, len(results)),
		Phase3EnumQueue:        make([]uuid.UUID, 0),
		Phase4FKQueue:          make([]uuid.UUID, 0),
		Phase5CrossColumnQueue: make([]string, 0),
	}

	tablesNeedingCrossColumn := make(map[string]bool)

	// Track failures for logging
	var failedColumns []string
	for _, r := range results {
		if r.Err != nil {
			s.logger.Error("Column classification failed",
				zap.String("column_id", r.ID),
				zap.Error(r.Err))
			failedColumns = append(failedColumns, r.ID)
			continue
		}

		features := r.Result
		result.Features = append(result.Features, features)

		// Find the profile to get table name for cross-column queue
		var tableName string
		for _, p := range profiles {
			if p.ColumnID.String() == r.ID {
				tableName = p.TableName
				break
			}
		}

		// Enqueue follow-up work based on classification
		if features.NeedsEnumAnalysis {
			result.Phase3EnumQueue = append(result.Phase3EnumQueue, features.ColumnID)
		}
		if features.NeedsFKResolution {
			result.Phase4FKQueue = append(result.Phase4FKQueue, features.ColumnID)
		}
		if features.NeedsCrossColumnCheck && tableName != "" {
			tablesNeedingCrossColumn[tableName] = true
		}
	}

	// Convert map to slice
	for table := range tablesNeedingCrossColumn {
		result.Phase5CrossColumnQueue = append(result.Phase5CrossColumnQueue, table)
	}

	s.logger.Info("Column classification complete",
		zap.Int("total_columns", len(profiles)),
		zap.Int("classified", len(result.Features)),
		zap.Int("failed", len(failedColumns)),
		zap.Int("enum_candidates", len(result.Phase3EnumQueue)),
		zap.Int("fk_candidates", len(result.Phase4FKQueue)),
		zap.Int("cross_column_tables", len(result.Phase5CrossColumnQueue)))

	// Report final progress with summary
	if progressCallback != nil {
		summary := fmt.Sprintf("Classified %d columns. Found %d enums, %d FK candidates",
			len(result.Features), len(result.Phase3EnumQueue), len(result.Phase4FKQueue))
		progressCallback(len(profiles), len(profiles), summary)
	}

	return result, nil
}

// classifySingleColumn sends ONE focused LLM request for ONE column.
// It delegates to the path-specific classifier based on the column's classification path.
func (s *columnFeatureExtractionService) classifySingleColumn(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
) (*models.ColumnFeatures, error) {
	classifier := s.getClassifier(profile.ClassificationPath)
	return classifier.Classify(ctx, projectID, profile, s.llmFactory, s.getTenantCtx)
}

// getClassifier returns the classifier for a given classification path.
// Classifiers are cached and reused.
func (s *columnFeatureExtractionService) getClassifier(path models.ClassificationPath) ColumnClassifier {
	s.classifiersMu.RLock()
	if classifier, ok := s.classifiers[path]; ok {
		s.classifiersMu.RUnlock()
		return classifier
	}
	s.classifiersMu.RUnlock()

	// Create classifier under write lock
	s.classifiersMu.Lock()
	defer s.classifiersMu.Unlock()

	// Double-check after acquiring write lock
	if classifier, ok := s.classifiers[path]; ok {
		return classifier
	}

	// Create the appropriate classifier
	var classifier ColumnClassifier
	switch path {
	case models.ClassificationPathTimestamp:
		classifier = &timestampClassifier{logger: s.logger}
	case models.ClassificationPathBoolean:
		classifier = &booleanClassifier{logger: s.logger}
	case models.ClassificationPathEnum:
		classifier = &enumClassifier{logger: s.logger}
	case models.ClassificationPathUUID:
		classifier = &uuidClassifier{logger: s.logger}
	case models.ClassificationPathExternalID:
		classifier = &externalIDClassifier{logger: s.logger}
	case models.ClassificationPathNumeric:
		classifier = &numericClassifier{logger: s.logger}
	case models.ClassificationPathText:
		classifier = &textClassifier{logger: s.logger}
	case models.ClassificationPathJSON:
		classifier = &jsonClassifier{logger: s.logger}
	default:
		classifier = &unknownClassifier{logger: s.logger}
	}

	s.classifiers[path] = classifier
	return classifier
}

// ============================================================================
// Column Classifier Interface and Implementations
// ============================================================================

// ColumnClassifier interface for path-specific classification.
// Each classifier builds a focused prompt for its classification path.
type ColumnClassifier interface {
	// Classify classifies a single column and returns its features.
	Classify(
		ctx context.Context,
		projectID uuid.UUID,
		profile *models.ColumnDataProfile,
		llmFactory llm.LLMClientFactory,
		getTenantCtx TenantContextFunc,
	) (*models.ColumnFeatures, error)
}

// ============================================================================
// Timestamp Classifier
// ============================================================================

type timestampClassifier struct {
	logger *zap.Logger
}

func (c *timestampClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	// Build the focused prompt for timestamp classification
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	// Get LLM client
	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM with low temperature for deterministic classification
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse the response
	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *timestampClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify timestamp columns based on their data characteristics.
Focus on the DATA patterns (null rate, precision) not column names. Column names are provided for context only.
Respond with valid JSON only.`
}

func (c *timestampClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# Timestamp Column Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Null rate:** %.1f%%\n", profile.NullRate*100))
	sb.WriteString(fmt.Sprintf("**Row count:** %d\n", profile.RowCount))

	// Detect timestamp scale for bigint columns
	timestampScale := ""
	for _, pattern := range profile.DetectedPatterns {
		switch pattern.PatternName {
		case models.PatternUnixSeconds:
			timestampScale = "seconds"
		case models.PatternUnixMillis:
			timestampScale = "milliseconds"
		case models.PatternUnixMicros:
			timestampScale = "microseconds"
		case models.PatternUnixNanos:
			timestampScale = "nanoseconds"
		}
	}
	if timestampScale != "" {
		sb.WriteString(fmt.Sprintf("**Timestamp scale:** %s (bigint stored as Unix timestamp)\n", timestampScale))
	}

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Sample values:**\n")
		for i, val := range profile.SampleValues {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", val))
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Based on the DATA characteristics (especially null rate), determine the timestamp's purpose.\n\n")

	sb.WriteString("**Classification rules:**\n")
	sb.WriteString("- **90-100% NULL:** Likely soft delete or optional event timestamp\n")
	sb.WriteString("- **0-5% NULL:** Likely required audit field (created_at, updated_at) or event time\n")
	sb.WriteString("- **5-90% NULL:** Conditional timestamp (populated under certain conditions)\n")
	if timestampScale == "nanoseconds" {
		sb.WriteString("- **Nanosecond precision:** Suggests cursor/pagination use (high precision for ordering)\n")
	}

	sb.WriteString("\n**Possible purposes:**\n")
	sb.WriteString("- `audit_created`: Records when the row was created\n")
	sb.WriteString("- `audit_updated`: Records when the row was last modified\n")
	sb.WriteString("- `soft_delete`: Records when the row was soft-deleted (high null rate)\n")
	sb.WriteString("- `event_time`: Records when a business event occurred\n")
	sb.WriteString("- `scheduled_time`: Records when something is scheduled\n")
	sb.WriteString("- `expiration`: Records when something expires\n")
	sb.WriteString("- `cursor`: Used for pagination/ordering\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"purpose\": \"audit_created\",\n")
	sb.WriteString("  \"confidence\": 0.85,\n")
	sb.WriteString("  \"is_soft_delete\": false,\n")
	sb.WriteString("  \"is_audit_field\": true,\n")
	sb.WriteString("  \"description\": \"Records when the record was created.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// timestampClassificationResponse is the expected JSON response from the LLM.
type timestampClassificationResponse struct {
	Purpose      string  `json:"purpose"`
	Confidence   float64 `json:"confidence"`
	IsSoftDelete bool    `json:"is_soft_delete"`
	IsAuditField bool    `json:"is_audit_field"`
	Description  string  `json:"description"`
}

func (c *timestampClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[timestampClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse timestamp classification response: %w", err)
	}

	// Determine timestamp scale from detected patterns
	timestampScale := ""
	for _, pattern := range profile.DetectedPatterns {
		switch pattern.PatternName {
		case models.PatternUnixSeconds:
			timestampScale = models.TimestampScaleSeconds
		case models.PatternUnixMillis:
			timestampScale = models.TimestampScaleMilliseconds
		case models.PatternUnixMicros:
			timestampScale = models.TimestampScaleMicroseconds
		case models.PatternUnixNanos:
			timestampScale = models.TimestampScaleNanoseconds
		}
	}

	features := &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathTimestamp,
		Purpose:            models.PurposeTimestamp,
		SemanticType:       response.Purpose,
		Role:               models.RoleAttribute,
		Description:        response.Description,
		Confidence:         response.Confidence,
		TimestampFeatures: &models.TimestampFeatures{
			TimestampPurpose: response.Purpose,
			TimestampScale:   timestampScale,
			IsSoftDelete:     response.IsSoftDelete,
			IsAuditField:     response.IsAuditField,
		},
		AnalyzedAt:   time.Now(),
		LLMModelUsed: model,
	}

	// Soft delete timestamps may need cross-column validation
	if response.IsSoftDelete {
		features.NeedsCrossColumnCheck = true
	}

	return features, nil
}

// ============================================================================
// Boolean Classifier
// ============================================================================

type booleanClassifier struct {
	logger *zap.Logger
}

func (c *booleanClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *booleanClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify boolean columns.
Focus on the value distribution and context to determine meaning.
Respond with valid JSON only.`
}

func (c *booleanClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# Boolean Column Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))

	if len(profile.SampleValues) > 0 {
		sb.WriteString(fmt.Sprintf("**Distinct values:** %v\n", profile.SampleValues))
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Determine what true and false values mean for this column.\n\n")

	sb.WriteString("**Boolean types:**\n")
	sb.WriteString("- `feature_flag`: Controls feature availability\n")
	sb.WriteString("- `status_indicator`: Indicates active/inactive, enabled/disabled state\n")
	sb.WriteString("- `permission`: Grants or denies access\n")
	sb.WriteString("- `preference`: User preference setting\n")
	sb.WriteString("- `state`: Part of a state machine or process\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"true_meaning\": \"Account is active and can log in\",\n")
	sb.WriteString("  \"false_meaning\": \"Account is deactivated\",\n")
	sb.WriteString("  \"boolean_type\": \"status_indicator\",\n")
	sb.WriteString("  \"confidence\": 0.9,\n")
	sb.WriteString("  \"description\": \"Indicates whether the account is currently active.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

type booleanClassificationResponse struct {
	TrueMeaning  string  `json:"true_meaning"`
	FalseMeaning string  `json:"false_meaning"`
	BooleanType  string  `json:"boolean_type"`
	Confidence   float64 `json:"confidence"`
	Description  string  `json:"description"`
}

func (c *booleanClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[booleanClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse boolean classification response: %w", err)
	}

	return &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathBoolean,
		Purpose:            models.PurposeFlag,
		SemanticType:       response.BooleanType,
		Role:               models.RoleAttribute,
		Description:        response.Description,
		Confidence:         response.Confidence,
		BooleanFeatures: &models.BooleanFeatures{
			TrueMeaning:  response.TrueMeaning,
			FalseMeaning: response.FalseMeaning,
			BooleanType:  response.BooleanType,
		},
		AnalyzedAt:   time.Now(),
		LLMModelUsed: model,
	}, nil
}

// ============================================================================
// Enum Classifier
// ============================================================================

type enumClassifier struct {
	logger *zap.Logger
}

func (c *enumClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *enumClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify enum/categorical columns.
Determine if the values represent a state machine or simple categories.
Respond with valid JSON only.`
}

func (c *enumClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# Enum/Categorical Column Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Distinct values:** %d\n", profile.DistinctCount))

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Values found:**\n")
		for _, val := range profile.SampleValues {
			sb.WriteString(fmt.Sprintf("- `%s`\n", val))
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Analyze these values to determine:\n")
	sb.WriteString("1. Are they a state machine (ordered progression) or simple categories?\n")
	sb.WriteString("2. What does each value mean?\n\n")

	sb.WriteString("**State machine indicators:**\n")
	sb.WriteString("- Values suggest progression (pending → processing → complete)\n")
	sb.WriteString("- Some values are terminal states (cannot transition further)\n")
	sb.WriteString("- Some values are error/exception states\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"is_state_machine\": true,\n")
	sb.WriteString("  \"state_description\": \"Order processing workflow\",\n")
	sb.WriteString("  \"needs_detailed_analysis\": true,\n")
	sb.WriteString("  \"confidence\": 0.85,\n")
	sb.WriteString("  \"description\": \"Tracks the order fulfillment status.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

type enumClassificationResponse struct {
	IsStateMachine        bool    `json:"is_state_machine"`
	StateDescription      string  `json:"state_description"`
	NeedsDetailedAnalysis bool    `json:"needs_detailed_analysis"`
	Confidence            float64 `json:"confidence"`
	Description           string  `json:"description"`
}

func (c *enumClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[enumClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse enum classification response: %w", err)
	}

	features := &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathEnum,
		Purpose:            models.PurposeEnum,
		SemanticType:       "enum",
		Role:               models.RoleAttribute,
		Description:        response.Description,
		Confidence:         response.Confidence,
		EnumFeatures: &models.EnumFeatures{
			IsStateMachine:   response.IsStateMachine,
			StateDescription: response.StateDescription,
		},
		AnalyzedAt:   time.Now(),
		LLMModelUsed: model,
	}

	// Flag for detailed enum analysis in Phase 3
	if response.NeedsDetailedAnalysis || response.IsStateMachine {
		features.NeedsEnumAnalysis = true
	}

	return features, nil
}

// ============================================================================
// UUID Classifier
// ============================================================================

type uuidClassifier struct {
	logger *zap.Logger
}

func (c *uuidClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *uuidClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify UUID identifier columns.
Determine if it's a primary key, foreign key, or other identifier type.
Respond with valid JSON only.`
}

func (c *uuidClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# UUID Column Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Is Primary Key:** %v\n", profile.IsPrimaryKey))
	sb.WriteString(fmt.Sprintf("**Is Unique:** %v\n", profile.IsUnique))
	sb.WriteString(fmt.Sprintf("**Cardinality:** %.2f%%\n", profile.Cardinality*100))

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Classify this UUID column:\n\n")

	sb.WriteString("**Identifier types:**\n")
	sb.WriteString("- `primary_key`: This table's primary identifier\n")
	sb.WriteString("- `foreign_key`: Reference to another entity (high cardinality, not PK)\n")
	sb.WriteString("- `internal_uuid`: Internal system identifier\n")
	sb.WriteString("- `external_uuid`: External reference (e.g., from partner system)\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"identifier_type\": \"foreign_key\",\n")
	sb.WriteString("  \"entity_referenced\": \"user\",\n")
	sb.WriteString("  \"needs_fk_resolution\": true,\n")
	sb.WriteString("  \"confidence\": 0.8,\n")
	sb.WriteString("  \"description\": \"References the user who created this record.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

type uuidClassificationResponse struct {
	IdentifierType    string  `json:"identifier_type"`
	EntityReferenced  string  `json:"entity_referenced"`
	NeedsFKResolution bool    `json:"needs_fk_resolution"`
	Confidence        float64 `json:"confidence"`
	Description       string  `json:"description"`
}

func (c *uuidClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[uuidClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse UUID classification response: %w", err)
	}

	// Determine role
	role := models.RoleAttribute
	if profile.IsPrimaryKey || response.IdentifierType == models.IdentifierTypePrimaryKey {
		role = models.RolePrimaryKey
	} else if response.IdentifierType == models.IdentifierTypeForeignKey {
		role = models.RoleForeignKey
	}

	features := &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathUUID,
		Purpose:            models.PurposeIdentifier,
		SemanticType:       response.IdentifierType,
		Role:               role,
		Description:        response.Description,
		Confidence:         response.Confidence,
		IdentifierFeatures: &models.IdentifierFeatures{
			IdentifierType:   response.IdentifierType,
			EntityReferenced: response.EntityReferenced,
		},
		AnalyzedAt:   time.Now(),
		LLMModelUsed: model,
	}

	// Flag for FK resolution in Phase 4
	if response.NeedsFKResolution && response.IdentifierType == models.IdentifierTypeForeignKey {
		features.NeedsFKResolution = true
	}

	return features, nil
}

// ============================================================================
// External ID Classifier
// ============================================================================

type externalIDClassifier struct {
	logger *zap.Logger
}

func (c *externalIDClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *externalIDClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify external service ID columns.
Identify the external service and what entity type the ID references.
Respond with valid JSON only.`
}

func (c *externalIDClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# External Service ID Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))

	// Show detected patterns
	for _, p := range profile.DetectedPatterns {
		if p.PatternName == models.PatternStripeID ||
			p.PatternName == models.PatternTwilioSID ||
			p.PatternName == models.PatternAWSSES {
			sb.WriteString(fmt.Sprintf("**Detected pattern:** %s (%.0f%% match)\n", p.PatternName, p.MatchRate*100))
			if len(p.MatchedValues) > 0 {
				sb.WriteString("**Sample values:**\n")
				for _, v := range p.MatchedValues {
					sb.WriteString(fmt.Sprintf("- `%s`\n", v))
				}
			}
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Identify the external service and entity type.\n\n")

	sb.WriteString("**Known services:**\n")
	sb.WriteString("- `stripe`: Payment processing (cus_, pi_, ch_, sub_, etc.)\n")
	sb.WriteString("- `twilio`: Communications (AC, SM, MM, etc.)\n")
	sb.WriteString("- `aws_ses`: Email service\n")
	sb.WriteString("- `other`: Unknown external service\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"external_service\": \"stripe\",\n")
	sb.WriteString("  \"entity_referenced\": \"customer\",\n")
	sb.WriteString("  \"confidence\": 0.95,\n")
	sb.WriteString("  \"description\": \"Stripe customer ID for payment processing.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

type externalIDClassificationResponse struct {
	ExternalService  string  `json:"external_service"`
	EntityReferenced string  `json:"entity_referenced"`
	Confidence       float64 `json:"confidence"`
	Description      string  `json:"description"`
}

func (c *externalIDClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[externalIDClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse external ID classification response: %w", err)
	}

	return &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathExternalID,
		Purpose:            models.PurposeIdentifier,
		SemanticType:       "external_service_id",
		Role:               models.RoleAttribute,
		Description:        response.Description,
		Confidence:         response.Confidence,
		IdentifierFeatures: &models.IdentifierFeatures{
			IdentifierType:   models.IdentifierTypeExternalService,
			ExternalService:  response.ExternalService,
			EntityReferenced: response.EntityReferenced,
		},
		AnalyzedAt:   time.Now(),
		LLMModelUsed: model,
	}, nil
}

// ============================================================================
// Numeric Classifier
// ============================================================================

type numericClassifier struct {
	logger *zap.Logger
}

func (c *numericClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *numericClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify numeric columns.
Determine if the column represents a measure (amount, count, quantity) or an identifier.
Respond with valid JSON only.`
}

func (c *numericClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# Numeric Column Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Is Primary Key:** %v\n", profile.IsPrimaryKey))
	sb.WriteString(fmt.Sprintf("**Is Unique:** %v\n", profile.IsUnique))
	sb.WriteString(fmt.Sprintf("**Cardinality:** %.2f%%\n", profile.Cardinality*100))

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Sample values:**\n")
		for i, val := range profile.SampleValues {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", val))
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Classify this numeric column:\n\n")

	sb.WriteString("**Numeric types:**\n")
	sb.WriteString("- `identifier`: Numeric ID (auto-increment, serial)\n")
	sb.WriteString("- `measure`: Quantitative value (amount, count, score)\n")
	sb.WriteString("- `monetary`: Money amount (may need currency pairing)\n")
	sb.WriteString("- `percentage`: Percentage or ratio\n")
	sb.WriteString("- `count`: Integer count of items\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"numeric_type\": \"monetary\",\n")
	sb.WriteString("  \"may_be_monetary\": true,\n")
	sb.WriteString("  \"confidence\": 0.75,\n")
	sb.WriteString("  \"description\": \"Transaction amount in cents.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

type numericClassificationResponse struct {
	NumericType   string  `json:"numeric_type"`
	MayBeMonetary bool    `json:"may_be_monetary"`
	Confidence    float64 `json:"confidence"`
	Description   string  `json:"description"`
}

func (c *numericClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[numericClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse numeric classification response: %w", err)
	}

	// Determine role and purpose
	role := models.RoleAttribute
	purpose := models.PurposeMeasure
	if profile.IsPrimaryKey || response.NumericType == "identifier" {
		role = models.RolePrimaryKey
		purpose = models.PurposeIdentifier
	}
	if response.NumericType == "measure" || response.NumericType == "monetary" || response.NumericType == "percentage" || response.NumericType == "count" {
		role = models.RoleMeasure
	}

	features := &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathNumeric,
		Purpose:            purpose,
		SemanticType:       response.NumericType,
		Role:               role,
		Description:        response.Description,
		Confidence:         response.Confidence,
		AnalyzedAt:         time.Now(),
		LLMModelUsed:       model,
	}

	// Flag for cross-column analysis if potentially monetary
	if response.MayBeMonetary {
		features.NeedsCrossColumnCheck = true
		features.MonetaryFeatures = &models.MonetaryFeatures{
			IsMonetary: false, // Will be confirmed in Phase 5
		}
	}

	return features, nil
}

// ============================================================================
// Text Classifier
// ============================================================================

type textClassifier struct {
	logger *zap.Logger
}

func (c *textClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *textClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify text columns.
Determine the type of text content based on length and patterns.
Respond with valid JSON only.`
}

func (c *textClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# Text Column Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Cardinality:** %.2f%%\n", profile.Cardinality*100))

	if profile.MinLength != nil && profile.MaxLength != nil {
		sb.WriteString(fmt.Sprintf("**Length range:** %d - %d characters\n", *profile.MinLength, *profile.MaxLength))
	}

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Sample values:**\n")
		for i, val := range profile.SampleValues {
			if i >= 5 {
				break
			}
			// Truncate long values
			if len(val) > 100 {
				val = val[:100] + "..."
			}
			sb.WriteString(fmt.Sprintf("- `%s`\n", val))
		}
	}

	// Show any detected patterns
	for _, p := range profile.DetectedPatterns {
		if p.PatternName == models.PatternEmail || p.PatternName == models.PatternURL {
			sb.WriteString(fmt.Sprintf("**Detected pattern:** %s (%.0f%% match)\n", p.PatternName, p.MatchRate*100))
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Classify this text column:\n\n")

	sb.WriteString("**Text types:**\n")
	sb.WriteString("- `name`: Person, company, or entity name\n")
	sb.WriteString("- `email`: Email address\n")
	sb.WriteString("- `url`: Web URL or link\n")
	sb.WriteString("- `code`: Code, abbreviation, or short identifier\n")
	sb.WriteString("- `description`: Free-form description or notes\n")
	sb.WriteString("- `content`: Long-form content (body, message, etc.)\n")
	sb.WriteString("- `structured`: Structured text (JSON, XML, etc.)\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"text_type\": \"email\",\n")
	sb.WriteString("  \"confidence\": 0.95,\n")
	sb.WriteString("  \"description\": \"User's primary email address.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

type textClassificationResponse struct {
	TextType    string  `json:"text_type"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description"`
}

func (c *textClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[textClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse text classification response: %w", err)
	}

	return &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathText,
		Purpose:            models.PurposeText,
		SemanticType:       response.TextType,
		Role:               models.RoleAttribute,
		Description:        response.Description,
		Confidence:         response.Confidence,
		AnalyzedAt:         time.Now(),
		LLMModelUsed:       model,
	}, nil
}

// ============================================================================
// JSON Classifier
// ============================================================================

type jsonClassifier struct {
	logger *zap.Logger
}

func (c *jsonClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	prompt := c.buildPrompt(profile)
	systemMsg := c.systemMessage()

	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return c.parseResponse(profile, result.Content, llmClient.GetModel())
}

func (c *jsonClassifier) systemMessage() string {
	return `You are a database schema analyst. Your task is to classify JSON/JSONB columns.
Analyze the sample values to understand the JSON structure and purpose.
Respond with valid JSON only.`
}

func (c *jsonClassifier) buildPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# JSON Column Classification\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Sample values:**\n")
		for i, val := range profile.SampleValues {
			if i >= 3 {
				break
			}
			// Truncate long values
			if len(val) > 200 {
				val = val[:200] + "..."
			}
			sb.WriteString(fmt.Sprintf("```json\n%s\n```\n", val))
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Classify this JSON column:\n\n")

	sb.WriteString("**JSON types:**\n")
	sb.WriteString("- `settings`: User or system configuration\n")
	sb.WriteString("- `metadata`: Additional unstructured data\n")
	sb.WriteString("- `payload`: Event or message payload\n")
	sb.WriteString("- `audit_data`: Audit trail or history\n")
	sb.WriteString("- `nested_entity`: Embedded entity data\n")

	sb.WriteString("\n## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"json_type\": \"settings\",\n")
	sb.WriteString("  \"confidence\": 0.85,\n")
	sb.WriteString("  \"description\": \"User preferences and application settings.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

type jsonClassificationResponse struct {
	JSONType    string  `json:"json_type"`
	Confidence  float64 `json:"confidence"`
	Description string  `json:"description"`
}

func (c *jsonClassifier) parseResponse(profile *models.ColumnDataProfile, content, model string) (*models.ColumnFeatures, error) {
	response, err := llm.ParseJSONResponse[jsonClassificationResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse JSON classification response: %w", err)
	}

	return &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathJSON,
		Purpose:            models.PurposeJSON,
		SemanticType:       response.JSONType,
		Role:               models.RoleAttribute,
		Description:        response.Description,
		Confidence:         response.Confidence,
		AnalyzedAt:         time.Now(),
		LLMModelUsed:       model,
	}, nil
}

// ============================================================================
// Unknown Classifier (fallback)
// ============================================================================

type unknownClassifier struct {
	logger *zap.Logger
}

func (c *unknownClassifier) Classify(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
) (*models.ColumnFeatures, error) {
	// For unknown types, create minimal features without LLM call
	return &models.ColumnFeatures{
		ColumnID:           profile.ColumnID,
		ClassificationPath: models.ClassificationPathUnknown,
		Purpose:            models.PurposeText,
		SemanticType:       "unknown",
		Role:               models.RoleAttribute,
		Description:        fmt.Sprintf("Column with unrecognized data type: %s", profile.DataType),
		Confidence:         0.5,
		AnalyzedAt:         time.Now(),
	}, nil
}

// ============================================================================
// Phase 3: Enum Value Analysis (Parallel LLM)
// ============================================================================

// EnumAnalysisResult holds the detailed enum value analysis from the LLM.
type EnumAnalysisResult struct {
	ColumnID         uuid.UUID
	IsStateMachine   bool
	StateDescription string
	Values           []models.ColumnEnumValue
	Description      string
	Confidence       float64
	LLMModelUsed     string
}

// runPhase3EnumAnalysis analyzes enum values for columns flagged in Phase 2.
// Only runs for columns with NeedsEnumAnalysis=true. Each enum column gets ONE LLM request.
func (s *columnFeatureExtractionService) runPhase3EnumAnalysis(
	ctx context.Context,
	projectID uuid.UUID,
	enumQueue []uuid.UUID,
	profiles []*models.ColumnDataProfile,
	features []*models.ColumnFeatures,
	progressCallback dag.ProgressCallback,
) error {
	if len(enumQueue) == 0 {
		s.logger.Info("Phase 3: No enum candidates, skipping")
		return nil // Skip phase if no enum candidates
	}

	s.logger.Info("Starting Phase 3: Enum Value Analysis",
		zap.Int("enum_candidates", len(enumQueue)))

	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, len(enumQueue), "Analyzing enum values")
	}

	// Build profile lookup for quick access
	profileByID := make(map[uuid.UUID]*models.ColumnDataProfile, len(profiles))
	for _, p := range profiles {
		profileByID[p.ColumnID] = p
	}

	// Build work items - ONE request per enum column
	workItems := make([]llm.WorkItem[*EnumAnalysisResult], 0, len(enumQueue))
	for _, columnID := range enumQueue {
		cid := columnID
		profile := profileByID[cid]
		if profile == nil {
			s.logger.Warn("Enum column profile not found, skipping",
				zap.String("column_id", cid.String()))
			continue
		}

		workItems = append(workItems, llm.WorkItem[*EnumAnalysisResult]{
			ID: cid.String(),
			Execute: func(ctx context.Context) (*EnumAnalysisResult, error) {
				return s.analyzeEnumColumn(ctx, projectID, profile)
			},
		})
	}

	if len(workItems) == 0 {
		s.logger.Info("Phase 3: No valid enum work items")
		return nil
	}

	// Process in parallel with progress updates
	results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total, "Analyzing enum values")
		}
	})

	// Track outcomes for logging
	var successCount, failureCount int
	for _, r := range results {
		if r.Err != nil {
			s.logger.Error("Enum analysis failed",
				zap.String("column_id", r.ID),
				zap.Error(r.Err))
			failureCount++
			continue
		}
		s.mergeEnumAnalysis(features, r.Result)
		successCount++
	}

	s.logger.Info("Phase 3 complete",
		zap.Int("analyzed", successCount),
		zap.Int("failed", failureCount))

	// Report final progress
	if progressCallback != nil {
		summary := fmt.Sprintf("Analyzed %d enum columns", successCount)
		progressCallback(len(enumQueue), len(enumQueue), summary)
	}

	return nil
}

// analyzeEnumColumn sends ONE focused LLM request to analyze enum values for a column.
// It queries the value distribution, correlates with timestamp columns for state machine
// detection, and asks the LLM "What do these values mean?".
func (s *columnFeatureExtractionService) analyzeEnumColumn(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
) (*EnumAnalysisResult, error) {
	prompt := s.buildEnumAnalysisPrompt(profile)
	systemMsg := `You are a database schema analyst. Your task is to analyze enum/categorical column values and provide human-readable labels for each value.

Focus on the DATA patterns (value distribution, frequency) to understand what each value represents.
If the values appear to form a state machine (workflow progression), identify initial, in-progress, and terminal states.
Respond with valid JSON only.`

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM with low temperature for deterministic classification
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse the response
	return s.parseEnumAnalysisResponse(profile, result.Content, llmClient.GetModel())
}

// buildEnumAnalysisPrompt creates a focused prompt for enum value analysis.
// It includes value distribution and correlation with timestamp columns for state machine detection.
func (s *columnFeatureExtractionService) buildEnumAnalysisPrompt(profile *models.ColumnDataProfile) string {
	var sb strings.Builder

	sb.WriteString("# Enum Value Analysis\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Distinct values:** %d\n", profile.DistinctCount))

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Values found in data:**\n")
		for _, val := range profile.SampleValues {
			sb.WriteString(fmt.Sprintf("- `%s`\n", val))
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Analyze these enum values to determine:\n")
	sb.WriteString("1. What does each value mean in business terms?\n")
	sb.WriteString("2. Are these values part of a state machine (ordered workflow progression)?\n")
	sb.WriteString("3. If it's a state machine, which values are initial, in-progress, and terminal states?\n\n")

	sb.WriteString("**State machine indicators:**\n")
	sb.WriteString("- Values suggest progression (e.g., pending → processing → complete)\n")
	sb.WriteString("- Some values are terminal states (cannot transition further)\n")
	sb.WriteString("- Some values are error/exception states\n")
	sb.WriteString("- Numeric values that increase suggest workflow stages\n\n")

	sb.WriteString("**Value category definitions:**\n")
	sb.WriteString("- `initial`: First state in a workflow (e.g., pending, new, draft)\n")
	sb.WriteString("- `in_progress`: Intermediate processing state\n")
	sb.WriteString("- `terminal`: Final state that cannot transition (general)\n")
	sb.WriteString("- `terminal_success`: Successfully completed state\n")
	sb.WriteString("- `terminal_error`: Error or failed state\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"is_state_machine\": true,\n")
	sb.WriteString("  \"state_description\": \"Order processing workflow from creation to fulfillment\",\n")
	sb.WriteString("  \"values\": [\n")
	sb.WriteString("    {\"value\": \"0\", \"label\": \"pending\", \"category\": \"initial\"},\n")
	sb.WriteString("    {\"value\": \"1\", \"label\": \"processing\", \"category\": \"in_progress\"},\n")
	sb.WriteString("    {\"value\": \"2\", \"label\": \"completed\", \"category\": \"terminal_success\"},\n")
	sb.WriteString("    {\"value\": \"3\", \"label\": \"failed\", \"category\": \"terminal_error\"}\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"confidence\": 0.85,\n")
	sb.WriteString("  \"description\": \"Tracks order status from creation through fulfillment or failure.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// enumAnalysisLLMResponse is the expected JSON response from the LLM for enum analysis.
type enumAnalysisLLMResponse struct {
	IsStateMachine   bool                   `json:"is_state_machine"`
	StateDescription string                 `json:"state_description"`
	Values           []enumValueLLMResponse `json:"values"`
	Confidence       float64                `json:"confidence"`
	Description      string                 `json:"description"`
}

// enumValueLLMResponse represents a single enum value in the LLM response.
type enumValueLLMResponse struct {
	Value    string `json:"value"`
	Label    string `json:"label"`
	Category string `json:"category,omitempty"`
}

// parseEnumAnalysisResponse parses the LLM response into an EnumAnalysisResult.
func (s *columnFeatureExtractionService) parseEnumAnalysisResponse(
	profile *models.ColumnDataProfile,
	content string,
	model string,
) (*EnumAnalysisResult, error) {
	response, err := llm.ParseJSONResponse[enumAnalysisLLMResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse enum analysis response: %w", err)
	}

	// Convert LLM response values to ColumnEnumValue
	values := make([]models.ColumnEnumValue, 0, len(response.Values))
	for _, v := range response.Values {
		values = append(values, models.ColumnEnumValue{
			Value:    v.Value,
			Label:    v.Label,
			Category: v.Category,
			// Count and Percentage would be populated if we had value distribution data
		})
	}

	return &EnumAnalysisResult{
		ColumnID:         profile.ColumnID,
		IsStateMachine:   response.IsStateMachine,
		StateDescription: response.StateDescription,
		Values:           values,
		Description:      response.Description,
		Confidence:       response.Confidence,
		LLMModelUsed:     model,
	}, nil
}

// mergeEnumAnalysis merges the enum analysis results into the existing column features.
func (s *columnFeatureExtractionService) mergeEnumAnalysis(
	features []*models.ColumnFeatures,
	result *EnumAnalysisResult,
) {
	for _, f := range features {
		if f.ColumnID == result.ColumnID {
			// Update enum features with detailed analysis
			if f.EnumFeatures == nil {
				f.EnumFeatures = &models.EnumFeatures{}
			}
			f.EnumFeatures.IsStateMachine = result.IsStateMachine
			f.EnumFeatures.StateDescription = result.StateDescription
			f.EnumFeatures.Values = result.Values

			// Update description if we got a better one
			if result.Description != "" {
				f.Description = result.Description
			}

			// Update confidence if higher
			if result.Confidence > f.Confidence {
				f.Confidence = result.Confidence
			}

			// Mark enum analysis as complete
			f.NeedsEnumAnalysis = false

			s.logger.Debug("Merged enum analysis",
				zap.String("column_id", result.ColumnID.String()),
				zap.Bool("is_state_machine", result.IsStateMachine),
				zap.Int("value_count", len(result.Values)))
			return
		}
	}

	s.logger.Warn("Could not find feature to merge enum analysis",
		zap.String("column_id", result.ColumnID.String()))
}

// ============================================================================
// Phase 4: FK Resolution (Parallel LLM)
// ============================================================================

// FKResolutionResult contains the result of FK resolution for a single column.
type FKResolutionResult struct {
	ColumnID       uuid.UUID `json:"column_id"`
	FKTargetTable  string    `json:"fk_target_table"`
	FKTargetColumn string    `json:"fk_target_column"`
	FKConfidence   float64   `json:"fk_confidence"`
	LLMModelUsed   string    `json:"llm_model_used"`
}

// phase4FKCandidate represents a potential FK target with overlap statistics for Phase 4.
type phase4FKCandidate struct {
	Schema         string
	Table          string
	Column         string
	ColumnID       uuid.UUID
	DataType       string
	OverlapRate    float64 // Match rate from CheckValueOverlap (0.0-1.0)
	MatchedCount   int64   // Number of source values that matched target
	TargetDistinct int64   // Distinct values in target column
}

// runPhase4FKResolution resolves FK targets for columns flagged in Phase 2.
// Only runs for columns with NeedsFKResolution=true. Each FK candidate gets ONE LLM request
// after running data overlap queries to gather evidence.
func (s *columnFeatureExtractionService) runPhase4FKResolution(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	fkQueue []uuid.UUID,
	profiles []*models.ColumnDataProfile,
	features []*models.ColumnFeatures,
	progressCallback dag.ProgressCallback,
) error {
	if len(fkQueue) == 0 {
		s.logger.Info("Phase 4: No FK candidates, skipping")
		return nil
	}

	s.logger.Info("Starting Phase 4: FK Resolution",
		zap.Int("fk_candidates", len(fkQueue)))

	// Check if we have datasource dependencies (required for overlap queries)
	if s.datasourceService == nil || s.adapterFactory == nil {
		s.logger.Warn("Phase 4: Datasource dependencies not configured, using LLM-only resolution")
		return s.runPhase4FKResolutionLLMOnly(ctx, projectID, fkQueue, profiles, features, progressCallback)
	}

	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, len(fkQueue), "Resolving FK targets")
	}

	// Build profile lookup for quick access
	profileByID := make(map[uuid.UUID]*models.ColumnDataProfile, len(profiles))
	for _, p := range profiles {
		profileByID[p.ColumnID] = p
	}

	// Get datasource to create schema discoverer for overlap queries
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("get datasource: %w", err)
	}

	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return fmt.Errorf("create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Get all PK columns as potential FK targets
	pkColumns, err := s.schemaRepo.GetPrimaryKeyColumns(ctx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("get primary key columns: %w", err)
	}

	// Get all tables to build tableID -> table lookup
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}
	tableByID := make(map[uuid.UUID]*models.SchemaTable, len(tables))
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	// Build work items - ONE request per FK candidate column
	workItems := make([]llm.WorkItem[*FKResolutionResult], 0, len(fkQueue))
	for _, columnID := range fkQueue {
		cid := columnID
		profile := profileByID[cid]
		if profile == nil {
			s.logger.Warn("FK candidate profile not found, skipping",
				zap.String("column_id", cid.String()))
			continue
		}

		// Get the table info for the source column
		sourceTable := tableByID[profile.TableID]
		if sourceTable == nil {
			s.logger.Warn("FK candidate table not found, skipping",
				zap.String("column_id", cid.String()))
			continue
		}

		workItems = append(workItems, llm.WorkItem[*FKResolutionResult]{
			ID: cid.String(),
			Execute: func(ctx context.Context) (*FKResolutionResult, error) {
				return s.resolveFKTarget(ctx, projectID, profile, sourceTable, pkColumns, tableByID, discoverer)
			},
		})
	}

	if len(workItems) == 0 {
		s.logger.Info("Phase 4: No valid FK work items")
		return nil
	}

	// Process in parallel with progress updates
	results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total, "Resolving FK targets")
		}
	})

	// Track outcomes for logging
	var successCount, failureCount, noTargetCount int
	for _, r := range results {
		if r.Err != nil {
			s.logger.Error("FK resolution failed",
				zap.String("column_id", r.ID),
				zap.Error(r.Err))
			failureCount++
			continue
		}
		if r.Result.FKTargetTable == "" {
			noTargetCount++
			continue
		}
		s.mergeFKResolution(features, r.Result)
		successCount++
	}

	s.logger.Info("Phase 4 complete",
		zap.Int("resolved", successCount),
		zap.Int("no_target", noTargetCount),
		zap.Int("failed", failureCount))

	// Report final progress
	if progressCallback != nil {
		summary := fmt.Sprintf("Resolved %d FK targets", successCount)
		progressCallback(len(fkQueue), len(fkQueue), summary)
	}

	return nil
}

// runPhase4FKResolutionLLMOnly resolves FK targets using only LLM analysis (no overlap queries).
// This is a fallback when datasource dependencies are not configured.
func (s *columnFeatureExtractionService) runPhase4FKResolutionLLMOnly(
	ctx context.Context,
	projectID uuid.UUID,
	fkQueue []uuid.UUID,
	profiles []*models.ColumnDataProfile,
	features []*models.ColumnFeatures,
	progressCallback dag.ProgressCallback,
) error {
	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, len(fkQueue), "Resolving FK targets (LLM-only)")
	}

	// Build profile lookup
	profileByID := make(map[uuid.UUID]*models.ColumnDataProfile, len(profiles))
	for _, p := range profiles {
		profileByID[p.ColumnID] = p
	}

	// Build work items
	workItems := make([]llm.WorkItem[*FKResolutionResult], 0, len(fkQueue))
	for _, columnID := range fkQueue {
		cid := columnID
		profile := profileByID[cid]
		if profile == nil {
			continue
		}

		workItems = append(workItems, llm.WorkItem[*FKResolutionResult]{
			ID: cid.String(),
			Execute: func(ctx context.Context) (*FKResolutionResult, error) {
				return s.resolveFKTargetLLMOnly(ctx, projectID, profile)
			},
		})
	}

	if len(workItems) == 0 {
		return nil
	}

	// Process in parallel
	results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total, "Resolving FK targets (LLM-only)")
		}
	})

	// Merge results
	var successCount int
	for _, r := range results {
		if r.Err != nil || r.Result.FKTargetTable == "" {
			continue
		}
		s.mergeFKResolution(features, r.Result)
		successCount++
	}

	if progressCallback != nil {
		progressCallback(len(fkQueue), len(fkQueue), fmt.Sprintf("Resolved %d FK targets", successCount))
	}

	return nil
}

// resolveFKTarget resolves the FK target for a single column using overlap queries and LLM analysis.
func (s *columnFeatureExtractionService) resolveFKTarget(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	sourceTable *models.SchemaTable,
	pkColumns []*models.SchemaColumn,
	tableByID map[uuid.UUID]*models.SchemaTable,
	discoverer datasource.SchemaDiscoverer,
) (*FKResolutionResult, error) {
	// Find candidate PK columns with compatible types
	candidates := make([]phase4FKCandidate, 0)

	for _, pkCol := range pkColumns {
		// Skip self-reference (same table)
		if pkCol.SchemaTableID == profile.TableID {
			continue
		}

		// Skip incompatible types
		if !areTypesCompatibleForFK(profile.DataType, pkCol.DataType) {
			continue
		}

		// Get table info for this PK
		pkTable := tableByID[pkCol.SchemaTableID]
		if pkTable == nil {
			continue
		}

		// Run value overlap query
		overlap, err := discoverer.CheckValueOverlap(ctx,
			sourceTable.SchemaName, sourceTable.TableName, profile.ColumnName,
			pkTable.SchemaName, pkTable.TableName, pkCol.ColumnName,
			1000) // Sample up to 1000 values
		if err != nil {
			s.logger.Debug("Overlap check failed, skipping candidate",
				zap.String("source", fmt.Sprintf("%s.%s", sourceTable.TableName, profile.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s", pkTable.TableName, pkCol.ColumnName)),
				zap.Error(err))
			continue
		}

		// Only consider candidates with meaningful overlap
		if overlap.MatchRate < 0.5 {
			continue
		}

		candidates = append(candidates, phase4FKCandidate{
			Schema:         pkTable.SchemaName,
			Table:          pkTable.TableName,
			Column:         pkCol.ColumnName,
			ColumnID:       pkCol.ID,
			DataType:       pkCol.DataType,
			OverlapRate:    overlap.MatchRate,
			MatchedCount:   overlap.MatchedCount,
			TargetDistinct: overlap.TargetDistinct,
		})
	}

	// No candidates with meaningful overlap
	if len(candidates) == 0 {
		s.logger.Debug("No FK candidates with sufficient overlap",
			zap.String("column", fmt.Sprintf("%s.%s", sourceTable.TableName, profile.ColumnName)))
		return &FKResolutionResult{
			ColumnID: profile.ColumnID,
		}, nil
	}

	// Sort by overlap rate (highest first)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].OverlapRate > candidates[j].OverlapRate
	})

	// If only one candidate with high overlap (>90%), use it directly
	if len(candidates) == 1 && candidates[0].OverlapRate >= 0.9 {
		return &FKResolutionResult{
			ColumnID:       profile.ColumnID,
			FKTargetTable:  candidates[0].Table,
			FKTargetColumn: candidates[0].Column,
			FKConfidence:   candidates[0].OverlapRate,
			LLMModelUsed:   "data_overlap",
		}, nil
	}

	// Multiple candidates or uncertain - use LLM to decide
	return s.resolveFKTargetWithLLM(ctx, projectID, profile, candidates)
}

// resolveFKTargetWithLLM uses LLM to choose among FK candidates with overlap evidence.
func (s *columnFeatureExtractionService) resolveFKTargetWithLLM(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
	candidates []phase4FKCandidate,
) (*FKResolutionResult, error) {
	prompt := s.buildFKResolutionPrompt(profile, candidates)
	systemMsg := `You are a database schema analyst. Your task is to identify the most likely foreign key target for a column based on data overlap analysis.

Focus on:
1. Match rate (higher is better - indicates data compatibility)
2. Column naming conventions (user_id typically references users.id)
3. Business logic (what makes sense semantically)

Respond with valid JSON only.`

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM with low temperature for deterministic choice
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.1, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse the response
	return s.parseFKResolutionResponse(profile, result.Content, llmClient.GetModel(), candidates)
}

// resolveFKTargetLLMOnly resolves FK target using only LLM analysis (no overlap data).
func (s *columnFeatureExtractionService) resolveFKTargetLLMOnly(
	ctx context.Context,
	projectID uuid.UUID,
	profile *models.ColumnDataProfile,
) (*FKResolutionResult, error) {
	prompt := s.buildFKResolutionPromptLLMOnly(profile)
	systemMsg := `You are a database schema analyst. Your task is to infer the most likely foreign key target based on column naming conventions and business logic.

If you cannot determine the target with reasonable confidence, respond with an empty target.

Respond with valid JSON only.`

	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.1, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	return s.parseFKResolutionResponseLLMOnly(profile, result.Content, llmClient.GetModel())
}

// buildFKResolutionPrompt creates a prompt for FK resolution with overlap evidence.
func (s *columnFeatureExtractionService) buildFKResolutionPrompt(
	profile *models.ColumnDataProfile,
	candidates []phase4FKCandidate,
) string {
	var sb strings.Builder

	sb.WriteString("# FK Target Resolution\n\n")
	sb.WriteString(fmt.Sprintf("**Source Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Source Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data Type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Distinct Values:** %d\n", profile.DistinctCount))

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Sample Values:**\n")
		for i, val := range profile.SampleValues {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("- `%s`\n", val))
		}
	}

	sb.WriteString("\n## Candidate FK Targets\n\n")
	sb.WriteString("The following tables have primary key columns with data overlap:\n\n")

	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("### Candidate %d: %s.%s\n", i+1, c.Table, c.Column))
		sb.WriteString(fmt.Sprintf("- **Match Rate:** %.1f%% of source values found in target\n", c.OverlapRate*100))
		sb.WriteString(fmt.Sprintf("- **Matched Count:** %d values\n", c.MatchedCount))
		sb.WriteString(fmt.Sprintf("- **Target Distinct:** %d values\n", c.TargetDistinct))
		sb.WriteString(fmt.Sprintf("- **Data Type:** %s\n\n", c.DataType))
	}

	sb.WriteString("## Task\n\n")
	sb.WriteString("Based on the overlap analysis and naming conventions, determine which candidate is the most likely FK target.\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"target_table\": \"users\",\n")
	sb.WriteString("  \"target_column\": \"id\",\n")
	sb.WriteString("  \"confidence\": 0.95,\n")
	sb.WriteString("  \"reasoning\": \"High match rate (98%) and column naming (user_id → users.id) strongly suggest this FK relationship.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// buildFKResolutionPromptLLMOnly creates a prompt for FK resolution without overlap data.
func (s *columnFeatureExtractionService) buildFKResolutionPromptLLMOnly(
	profile *models.ColumnDataProfile,
) string {
	var sb strings.Builder

	sb.WriteString("# FK Target Inference\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n", profile.TableName))
	sb.WriteString(fmt.Sprintf("**Column:** %s\n", profile.ColumnName))
	sb.WriteString(fmt.Sprintf("**Data Type:** %s\n", profile.DataType))
	sb.WriteString(fmt.Sprintf("**Is Primary Key:** %v\n", profile.IsPrimaryKey))

	if len(profile.SampleValues) > 0 {
		sb.WriteString("\n**Sample Values:**\n")
		for i, val := range profile.SampleValues {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("- `%s`\n", val))
		}
	}

	sb.WriteString("\n## Task\n\n")
	sb.WriteString("Based on naming conventions and the data samples, infer the most likely FK target table and column.\n")
	sb.WriteString("Common patterns:\n")
	sb.WriteString("- `user_id` → `users.id`\n")
	sb.WriteString("- `order_id` → `orders.id`\n")
	sb.WriteString("- `product_uuid` → `products.id`\n\n")

	sb.WriteString("If you cannot determine the target with reasonable confidence, return empty strings.\n\n")

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"target_table\": \"users\",\n")
	sb.WriteString("  \"target_column\": \"id\",\n")
	sb.WriteString("  \"confidence\": 0.7,\n")
	sb.WriteString("  \"reasoning\": \"Column name 'user_id' follows standard FK naming convention pointing to users table.\"\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// fkResolutionLLMResponse is the expected JSON response from the LLM for FK resolution.
type fkResolutionLLMResponse struct {
	TargetTable  string  `json:"target_table"`
	TargetColumn string  `json:"target_column"`
	Confidence   float64 `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
}

// parseFKResolutionResponse parses the LLM response for FK resolution.
func (s *columnFeatureExtractionService) parseFKResolutionResponse(
	profile *models.ColumnDataProfile,
	content string,
	model string,
	candidates []phase4FKCandidate,
) (*FKResolutionResult, error) {
	response, err := llm.ParseJSONResponse[fkResolutionLLMResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse FK resolution response: %w", err)
	}

	// Validate that the response matches one of the candidates
	var matchedCandidate *phase4FKCandidate
	for i := range candidates {
		if strings.EqualFold(candidates[i].Table, response.TargetTable) &&
			strings.EqualFold(candidates[i].Column, response.TargetColumn) {
			matchedCandidate = &candidates[i]
			break
		}
	}

	// If LLM chose something not in candidates, use highest overlap candidate
	if matchedCandidate == nil && len(candidates) > 0 {
		s.logger.Warn("LLM chose target not in candidates, using highest overlap",
			zap.String("llm_choice", fmt.Sprintf("%s.%s", response.TargetTable, response.TargetColumn)),
			zap.String("best_candidate", fmt.Sprintf("%s.%s", candidates[0].Table, candidates[0].Column)))
		matchedCandidate = &candidates[0]
	}

	if matchedCandidate == nil {
		return &FKResolutionResult{
			ColumnID: profile.ColumnID,
		}, nil
	}

	return &FKResolutionResult{
		ColumnID:       profile.ColumnID,
		FKTargetTable:  matchedCandidate.Table,
		FKTargetColumn: matchedCandidate.Column,
		FKConfidence:   response.Confidence,
		LLMModelUsed:   model,
	}, nil
}

// parseFKResolutionResponseLLMOnly parses the LLM response for LLM-only FK resolution.
func (s *columnFeatureExtractionService) parseFKResolutionResponseLLMOnly(
	profile *models.ColumnDataProfile,
	content string,
	model string,
) (*FKResolutionResult, error) {
	response, err := llm.ParseJSONResponse[fkResolutionLLMResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse FK resolution response: %w", err)
	}

	if response.TargetTable == "" || response.TargetColumn == "" {
		return &FKResolutionResult{
			ColumnID: profile.ColumnID,
		}, nil
	}

	return &FKResolutionResult{
		ColumnID:       profile.ColumnID,
		FKTargetTable:  response.TargetTable,
		FKTargetColumn: response.TargetColumn,
		FKConfidence:   response.Confidence,
		LLMModelUsed:   model,
	}, nil
}

// mergeFKResolution merges the FK resolution results into the existing column features.
func (s *columnFeatureExtractionService) mergeFKResolution(
	features []*models.ColumnFeatures,
	result *FKResolutionResult,
) {
	for _, f := range features {
		if f.ColumnID == result.ColumnID {
			// Update identifier features with FK resolution
			if f.IdentifierFeatures == nil {
				f.IdentifierFeatures = &models.IdentifierFeatures{}
			}
			f.IdentifierFeatures.FKTargetTable = result.FKTargetTable
			f.IdentifierFeatures.FKTargetColumn = result.FKTargetColumn
			f.IdentifierFeatures.FKConfidence = result.FKConfidence

			// Update the identifier type to foreign_key if we found a target
			if result.FKTargetTable != "" {
				f.IdentifierFeatures.IdentifierType = models.IdentifierTypeForeignKey
				f.Role = models.RoleForeignKey
			}

			// Mark FK resolution as complete
			f.NeedsFKResolution = false

			s.logger.Debug("Merged FK resolution",
				zap.String("column_id", result.ColumnID.String()),
				zap.String("target", fmt.Sprintf("%s.%s", result.FKTargetTable, result.FKTargetColumn)),
				zap.Float64("confidence", result.FKConfidence))
			return
		}
	}

	s.logger.Warn("Could not find feature to merge FK resolution",
		zap.String("column_id", result.ColumnID.String()))
}

// ============================================================================
// Phase 5: Cross-Column Analysis (Parallel LLM)
// ============================================================================

// CrossColumnResult contains the result of cross-column analysis for a single table.
// This phase validates monetary pairings and soft delete timestamps by analyzing
// relationships between columns in the same table.
type CrossColumnResult struct {
	TableName string `json:"table_name"`

	// Monetary column pairings discovered
	MonetaryPairings []MonetaryPairing `json:"monetary_pairings,omitempty"`

	// Soft delete validation results
	SoftDeleteValidations []SoftDeleteValidation `json:"soft_delete_validations,omitempty"`

	LLMModelUsed string `json:"llm_model_used"`
}

// MonetaryPairing represents a validated pairing between a numeric amount column
// and a currency column in the same table.
type MonetaryPairing struct {
	AmountColumnID     uuid.UUID `json:"amount_column_id"`
	AmountColumnName   string    `json:"amount_column_name"`
	CurrencyColumnName string    `json:"currency_column_name"`
	CurrencyUnit       string    `json:"currency_unit"`      // "cents", "dollars", "basis_points"
	AmountDescription  string    `json:"amount_description"` // What this amount represents
	Confidence         float64   `json:"confidence"`
}

// SoftDeleteValidation represents the validation result for a suspected soft delete column.
type SoftDeleteValidation struct {
	ColumnID       uuid.UUID `json:"column_id"`
	ColumnName     string    `json:"column_name"`
	IsSoftDelete   bool      `json:"is_soft_delete"`   // Confirmed as soft delete?
	NonNullMeaning string    `json:"non_null_meaning"` // What does a non-NULL value indicate?
	Description    string    `json:"description"`
	Confidence     float64   `json:"confidence"`
}

// runPhase5CrossColumnAnalysis analyzes column relationships per table.
// Only runs for tables flagged in Phase 2 with NeedsCrossColumnCheck=true.
// Each table gets ONE LLM request that analyzes:
// - Monetary pairings: Which numeric amounts pair with which currency column?
// - Soft delete validation: Is this really a soft delete? What does non-NULL indicate?
func (s *columnFeatureExtractionService) runPhase5CrossColumnAnalysis(
	ctx context.Context,
	projectID uuid.UUID,
	tableQueue []string,
	profiles []*models.ColumnDataProfile,
	features []*models.ColumnFeatures,
	progressCallback dag.ProgressCallback,
) error {
	if len(tableQueue) == 0 {
		s.logger.Info("Phase 5: No tables need cross-column analysis, skipping")
		return nil
	}

	s.logger.Info("Starting Phase 5: Cross-Column Analysis",
		zap.Int("tables_to_analyze", len(tableQueue)))

	// Report initial progress
	if progressCallback != nil {
		progressCallback(0, len(tableQueue), "Analyzing column relationships")
	}

	// Build profile lookup by table name
	profilesByTable := make(map[string][]*models.ColumnDataProfile)
	for _, p := range profiles {
		profilesByTable[p.TableName] = append(profilesByTable[p.TableName], p)
	}

	// Build features lookup by column ID
	featuresByColumnID := make(map[uuid.UUID]*models.ColumnFeatures)
	for _, f := range features {
		featuresByColumnID[f.ColumnID] = f
	}

	// Build work items - ONE request per table
	workItems := make([]llm.WorkItem[*CrossColumnResult], 0, len(tableQueue))
	for _, tableName := range tableQueue {
		tn := tableName
		tableProfiles := profilesByTable[tn]
		if len(tableProfiles) == 0 {
			s.logger.Warn("No profiles found for table, skipping",
				zap.String("table", tn))
			continue
		}

		workItems = append(workItems, llm.WorkItem[*CrossColumnResult]{
			ID: tn,
			Execute: func(ctx context.Context) (*CrossColumnResult, error) {
				return s.analyzeCrossColumn(ctx, projectID, tn, tableProfiles, featuresByColumnID)
			},
		})
	}

	if len(workItems) == 0 {
		s.logger.Info("Phase 5: No valid cross-column work items")
		return nil
	}

	// Process in parallel with progress updates
	results := llm.Process(ctx, s.workerPool, workItems, func(completed, total int) {
		if progressCallback != nil {
			progressCallback(completed, total, "Analyzing column relationships")
		}
	})

	// Track outcomes for logging
	var successCount, failureCount int
	var monetaryPairingsFound, softDeleteValidations int

	for _, r := range results {
		if r.Err != nil {
			s.logger.Error("Cross-column analysis failed",
				zap.String("table", r.ID),
				zap.Error(r.Err))
			failureCount++
			continue
		}

		s.mergeCrossColumnAnalysis(features, r.Result)
		successCount++
		monetaryPairingsFound += len(r.Result.MonetaryPairings)
		softDeleteValidations += len(r.Result.SoftDeleteValidations)
	}

	s.logger.Info("Phase 5 complete",
		zap.Int("tables_analyzed", successCount),
		zap.Int("failed", failureCount),
		zap.Int("monetary_pairings", monetaryPairingsFound),
		zap.Int("soft_delete_validations", softDeleteValidations))

	// Report final progress
	if progressCallback != nil {
		summary := fmt.Sprintf("Analyzed %d tables for column relationships", successCount)
		progressCallback(len(tableQueue), len(tableQueue), summary)
	}

	return nil
}

// analyzeCrossColumn sends ONE focused LLM request to analyze cross-column relationships
// for a single table. It gathers table context and sends a request analyzing:
// - Monetary pairing: which numeric amounts pair with which currency column
// - Soft delete validation: is this really a soft delete timestamp
func (s *columnFeatureExtractionService) analyzeCrossColumn(
	ctx context.Context,
	projectID uuid.UUID,
	tableName string,
	tableProfiles []*models.ColumnDataProfile,
	featuresByColumnID map[uuid.UUID]*models.ColumnFeatures,
) (*CrossColumnResult, error) {
	// Gather context: find columns that need cross-column analysis
	var potentialMonetaryColumns []*models.ColumnDataProfile
	var potentialSoftDeleteColumns []*models.ColumnDataProfile
	var currencyColumns []*models.ColumnDataProfile

	for _, profile := range tableProfiles {
		features := featuresByColumnID[profile.ColumnID]
		if features == nil {
			continue
		}

		// Check for potential monetary columns
		if features.NeedsCrossColumnCheck && features.MonetaryFeatures != nil {
			potentialMonetaryColumns = append(potentialMonetaryColumns, profile)
		}

		// Check for potential soft delete columns
		if features.NeedsCrossColumnCheck && features.TimestampFeatures != nil && features.TimestampFeatures.IsSoftDelete {
			potentialSoftDeleteColumns = append(potentialSoftDeleteColumns, profile)
		}

		// Identify currency columns (text with ISO 4217 pattern)
		if profile.MatchesPatternWithThreshold(models.PatternISO4217, 0.8) {
			currencyColumns = append(currencyColumns, profile)
		}
	}

	// If nothing to analyze, return empty result
	if len(potentialMonetaryColumns) == 0 && len(potentialSoftDeleteColumns) == 0 {
		return &CrossColumnResult{
			TableName: tableName,
		}, nil
	}

	prompt := s.buildCrossColumnPrompt(tableName, tableProfiles, potentialMonetaryColumns, potentialSoftDeleteColumns, currencyColumns)
	systemMsg := `You are a database schema analyst. Your task is to analyze relationships between columns in the same table.

Focus on:
1. Monetary pairing: Which numeric columns represent monetary amounts and which currency column do they pair with?
2. Soft delete validation: For high-null-rate timestamp columns, confirm if they are soft delete markers and what non-NULL values mean.

Base your analysis on DATA patterns (value distributions, null rates), not column names. Column names are provided for context only.
Respond with valid JSON only.`

	// Get LLM client
	llmClient, err := s.llmFactory.CreateForProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	// Call LLM with low temperature for deterministic analysis
	result, err := llmClient.GenerateResponse(ctx, prompt, systemMsg, 0.2, false)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse the response
	return s.parseCrossColumnResponse(tableName, result.Content, llmClient.GetModel(), potentialMonetaryColumns, potentialSoftDeleteColumns)
}

// buildCrossColumnPrompt creates a focused prompt for cross-column analysis.
func (s *columnFeatureExtractionService) buildCrossColumnPrompt(
	tableName string,
	allProfiles []*models.ColumnDataProfile,
	monetaryColumns []*models.ColumnDataProfile,
	softDeleteColumns []*models.ColumnDataProfile,
	currencyColumns []*models.ColumnDataProfile,
) string {
	var sb strings.Builder

	sb.WriteString("# Cross-Column Analysis\n\n")
	sb.WriteString(fmt.Sprintf("**Table:** %s\n\n", tableName))

	// List all columns for context
	sb.WriteString("## All Columns in Table\n\n")
	sb.WriteString("| Column | Type | Null Rate | Distinct Count |\n")
	sb.WriteString("|--------|------|-----------|----------------|\n")
	for _, p := range allProfiles {
		sb.WriteString(fmt.Sprintf("| %s | %s | %.1f%% | %d |\n",
			p.ColumnName, p.DataType, p.NullRate*100, p.DistinctCount))
	}
	sb.WriteString("\n")

	// Monetary analysis section
	if len(monetaryColumns) > 0 {
		sb.WriteString("## Monetary Column Analysis\n\n")
		sb.WriteString("The following numeric columns may represent monetary amounts:\n\n")

		for _, col := range monetaryColumns {
			sb.WriteString(fmt.Sprintf("### %s\n", col.ColumnName))
			sb.WriteString(fmt.Sprintf("- **Data type:** %s\n", col.DataType))
			if len(col.SampleValues) > 0 {
				sb.WriteString("- **Sample values:** ")
				samples := col.SampleValues
				if len(samples) > 5 {
					samples = samples[:5]
				}
				sb.WriteString(strings.Join(samples, ", "))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		if len(currencyColumns) > 0 {
			sb.WriteString("### Potential Currency Columns\n\n")
			for _, col := range currencyColumns {
				sb.WriteString(fmt.Sprintf("- **%s** (type: %s)", col.ColumnName, col.DataType))
				if len(col.SampleValues) > 0 {
					samples := col.SampleValues
					if len(samples) > 5 {
						samples = samples[:5]
					}
					sb.WriteString(fmt.Sprintf(" - values: %s", strings.Join(samples, ", ")))
				}
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		sb.WriteString("**Task:** For each numeric column, determine:\n")
		sb.WriteString("1. Is it a monetary amount? (not a percentage, count, or ID)\n")
		sb.WriteString("2. What currency column (if any) does it pair with?\n")
		sb.WriteString("3. What unit is the amount in? (cents, dollars, basis_points)\n")
		sb.WriteString("4. What does this amount represent?\n\n")
	}

	// Soft delete analysis section
	if len(softDeleteColumns) > 0 {
		sb.WriteString("## Soft Delete Validation\n\n")
		sb.WriteString("The following timestamp columns have high null rates and may be soft delete markers:\n\n")

		for _, col := range softDeleteColumns {
			sb.WriteString(fmt.Sprintf("### %s\n", col.ColumnName))
			sb.WriteString(fmt.Sprintf("- **Data type:** %s\n", col.DataType))
			sb.WriteString(fmt.Sprintf("- **Null rate:** %.1f%%\n", col.NullRate*100))
			if len(col.SampleValues) > 0 {
				sb.WriteString("- **Sample non-NULL values:** ")
				samples := col.SampleValues
				if len(samples) > 3 {
					samples = samples[:3]
				}
				sb.WriteString(strings.Join(samples, ", "))
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
		}

		sb.WriteString("**Task:** For each timestamp column, determine:\n")
		sb.WriteString("1. Is this truly a soft delete marker? (or could it be something else like an optional event time?)\n")
		sb.WriteString("2. What does a non-NULL value indicate? (e.g., \"record was deleted\", \"user unsubscribed\")\n\n")
	}

	sb.WriteString("## Response Format\n\n")
	sb.WriteString("```json\n")
	sb.WriteString("{\n")
	sb.WriteString("  \"monetary_pairings\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"amount_column\": \"total_amount\",\n")
	sb.WriteString("      \"currency_column\": \"currency_code\",\n")
	sb.WriteString("      \"currency_unit\": \"cents\",\n")
	sb.WriteString("      \"amount_description\": \"Total transaction amount including taxes\",\n")
	sb.WriteString("      \"confidence\": 0.9\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ],\n")
	sb.WriteString("  \"soft_delete_validations\": [\n")
	sb.WriteString("    {\n")
	sb.WriteString("      \"column_name\": \"deleted_at\",\n")
	sb.WriteString("      \"is_soft_delete\": true,\n")
	sb.WriteString("      \"non_null_meaning\": \"Record was soft-deleted at this timestamp\",\n")
	sb.WriteString("      \"description\": \"Soft delete marker for logical deletion without removing the row\",\n")
	sb.WriteString("      \"confidence\": 0.95\n")
	sb.WriteString("    }\n")
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	sb.WriteString("```\n")

	return sb.String()
}

// crossColumnLLMResponse is the expected JSON response from the LLM for cross-column analysis.
type crossColumnLLMResponse struct {
	MonetaryPairings      []monetaryPairingLLMResponse      `json:"monetary_pairings"`
	SoftDeleteValidations []softDeleteValidationLLMResponse `json:"soft_delete_validations"`
}

type monetaryPairingLLMResponse struct {
	AmountColumn      string  `json:"amount_column"`
	CurrencyColumn    string  `json:"currency_column"`
	CurrencyUnit      string  `json:"currency_unit"`
	AmountDescription string  `json:"amount_description"`
	Confidence        float64 `json:"confidence"`
}

type softDeleteValidationLLMResponse struct {
	ColumnName     string  `json:"column_name"`
	IsSoftDelete   bool    `json:"is_soft_delete"`
	NonNullMeaning string  `json:"non_null_meaning"`
	Description    string  `json:"description"`
	Confidence     float64 `json:"confidence"`
}

// parseCrossColumnResponse parses the LLM response into a CrossColumnResult.
func (s *columnFeatureExtractionService) parseCrossColumnResponse(
	tableName string,
	content string,
	model string,
	monetaryColumns []*models.ColumnDataProfile,
	softDeleteColumns []*models.ColumnDataProfile,
) (*CrossColumnResult, error) {
	response, err := llm.ParseJSONResponse[crossColumnLLMResponse](content)
	if err != nil {
		return nil, fmt.Errorf("parse cross-column analysis response: %w", err)
	}

	// Build column name to ID lookup
	columnIDByName := make(map[string]uuid.UUID)
	for _, col := range monetaryColumns {
		columnIDByName[col.ColumnName] = col.ColumnID
	}
	for _, col := range softDeleteColumns {
		columnIDByName[col.ColumnName] = col.ColumnID
	}

	result := &CrossColumnResult{
		TableName:    tableName,
		LLMModelUsed: model,
	}

	// Convert monetary pairings
	for _, mp := range response.MonetaryPairings {
		columnID, ok := columnIDByName[mp.AmountColumn]
		if !ok {
			s.logger.Warn("Monetary pairing references unknown column",
				zap.String("table", tableName),
				zap.String("column", mp.AmountColumn))
			continue
		}

		result.MonetaryPairings = append(result.MonetaryPairings, MonetaryPairing{
			AmountColumnID:     columnID,
			AmountColumnName:   mp.AmountColumn,
			CurrencyColumnName: mp.CurrencyColumn,
			CurrencyUnit:       mp.CurrencyUnit,
			AmountDescription:  mp.AmountDescription,
			Confidence:         mp.Confidence,
		})
	}

	// Convert soft delete validations
	for _, sd := range response.SoftDeleteValidations {
		columnID, ok := columnIDByName[sd.ColumnName]
		if !ok {
			s.logger.Warn("Soft delete validation references unknown column",
				zap.String("table", tableName),
				zap.String("column", sd.ColumnName))
			continue
		}

		result.SoftDeleteValidations = append(result.SoftDeleteValidations, SoftDeleteValidation{
			ColumnID:       columnID,
			ColumnName:     sd.ColumnName,
			IsSoftDelete:   sd.IsSoftDelete,
			NonNullMeaning: sd.NonNullMeaning,
			Description:    sd.Description,
			Confidence:     sd.Confidence,
		})
	}

	return result, nil
}

// mergeCrossColumnAnalysis merges the cross-column analysis results into the existing column features.
func (s *columnFeatureExtractionService) mergeCrossColumnAnalysis(
	features []*models.ColumnFeatures,
	result *CrossColumnResult,
) {
	// Build feature lookup by column ID
	featureByID := make(map[uuid.UUID]*models.ColumnFeatures)
	for _, f := range features {
		featureByID[f.ColumnID] = f
	}

	// Merge monetary pairings
	for _, mp := range result.MonetaryPairings {
		f := featureByID[mp.AmountColumnID]
		if f == nil {
			s.logger.Warn("Could not find feature for monetary pairing",
				zap.String("table", result.TableName),
				zap.String("column", mp.AmountColumnName))
			continue
		}

		// Update or create monetary features
		if f.MonetaryFeatures == nil {
			f.MonetaryFeatures = &models.MonetaryFeatures{}
		}
		f.MonetaryFeatures.IsMonetary = true
		f.MonetaryFeatures.CurrencyUnit = mp.CurrencyUnit
		f.MonetaryFeatures.PairedCurrencyColumn = mp.CurrencyColumnName
		f.MonetaryFeatures.AmountDescription = mp.AmountDescription

		// Update semantic type and role
		f.SemanticType = "monetary"
		f.Role = models.RoleMeasure

		// Update description if we got a better one
		if mp.AmountDescription != "" && f.Description == "" {
			f.Description = mp.AmountDescription
		}

		// Update confidence if higher
		if mp.Confidence > f.Confidence {
			f.Confidence = mp.Confidence
		}

		// Mark cross-column check as complete
		f.NeedsCrossColumnCheck = false

		s.logger.Debug("Merged monetary pairing",
			zap.String("table", result.TableName),
			zap.String("amount_column", mp.AmountColumnName),
			zap.String("currency_column", mp.CurrencyColumnName),
			zap.String("unit", mp.CurrencyUnit))
	}

	// Merge soft delete validations
	for _, sd := range result.SoftDeleteValidations {
		f := featureByID[sd.ColumnID]
		if f == nil {
			s.logger.Warn("Could not find feature for soft delete validation",
				zap.String("table", result.TableName),
				zap.String("column", sd.ColumnName))
			continue
		}

		// Update timestamp features
		if f.TimestampFeatures == nil {
			f.TimestampFeatures = &models.TimestampFeatures{}
		}
		f.TimestampFeatures.IsSoftDelete = sd.IsSoftDelete

		// Update semantic type based on validation
		if sd.IsSoftDelete {
			f.SemanticType = models.TimestampPurposeSoftDelete
			f.TimestampFeatures.TimestampPurpose = models.TimestampPurposeSoftDelete
		} else {
			// Soft delete was rejected - clear the soft delete semantic type if it was set
			if f.SemanticType == models.TimestampPurposeSoftDelete {
				f.SemanticType = models.TimestampPurposeEventTime // Revert to generic timestamp purpose
			}
			if f.TimestampFeatures.TimestampPurpose == models.TimestampPurposeSoftDelete {
				f.TimestampFeatures.TimestampPurpose = models.TimestampPurposeEventTime
			}
		}

		// Update description
		if sd.Description != "" {
			f.Description = sd.Description
		}

		// Update confidence if higher
		if sd.Confidence > f.Confidence {
			f.Confidence = sd.Confidence
		}

		// Mark cross-column check as complete
		f.NeedsCrossColumnCheck = false

		s.logger.Debug("Merged soft delete validation",
			zap.String("table", result.TableName),
			zap.String("column", sd.ColumnName),
			zap.Bool("is_soft_delete", sd.IsSoftDelete))
	}
}
