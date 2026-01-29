package services

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

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
	schemaRepo repositories.SchemaRepository
	logger     *zap.Logger
}

// NewColumnFeatureExtractionService creates a new column feature extraction service.
func NewColumnFeatureExtractionService(
	schemaRepo repositories.SchemaRepository,
	logger *zap.Logger,
) ColumnFeatureExtractionService {
	return &columnFeatureExtractionService{
		schemaRepo: schemaRepo,
		logger:     logger.Named("column-feature-extraction"),
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
// This implements Phase 1 of the column feature extraction pipeline.
func (s *columnFeatureExtractionService) ExtractColumnFeatures(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) (int, error) {
	s.logger.Info("Starting column feature extraction (Phase 1)",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()))

	// Run Phase 1: Data Collection (deterministic, no LLM)
	phase1Result, err := s.runPhase1DataCollection(ctx, projectID, datasourceID, progressCallback)
	if err != nil {
		return 0, fmt.Errorf("phase 1 data collection failed: %w", err)
	}

	s.logger.Info("Column feature extraction Phase 1 complete",
		zap.Int("columns_processed", phase1Result.TotalColumns))

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
