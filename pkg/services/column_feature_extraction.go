package services

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

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
	schemaRepo   repositories.SchemaRepository
	llmFactory   llm.LLMClientFactory
	workerPool   *llm.WorkerPool
	getTenantCtx TenantContextFunc
	logger       *zap.Logger

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

		// TODO: Phase 3-6 will be implemented in subsequent tasks
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
