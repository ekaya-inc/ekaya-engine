package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// Discovery configuration constants
const (
	DefaultMatchThreshold   = 0.95 // 95% match (allow 5% for data issues)
	DefaultSampleLimit      = 1000 // Max values to sample for overlap check
	MaxColumnsPerStatsQuery = 25   // Batch size for column stats

	// CardinalityUniqueThreshold allows 10% tolerance for uniqueness detection
	// to account for minor data inconsistencies or sampling variance.
	CardinalityUniqueThreshold = 1.1

	// Review candidate configuration - more aggressive discovery for orphan tables
	// ReviewMinCardinalityRatio is the minimum distinct/total ratio for review candidates.
	// Columns with lower ratios look like counts or status codes, not join keys.
	ReviewMinCardinalityRatio = 0.10 // 10%

	// ReviewMinDistinctCount is the minimum distinct values for review candidates.
	// Small counts (1-100) are likely enums or status codes.
	ReviewMinDistinctCount = 100

	// ReviewMaxOrphanRate for review candidates (must be 0 for strict matching).
	// All source values must exist in target column.
	ReviewMaxOrphanRate = 0.0 // 0% - all values must exist in target
)

// Column data types excluded from join key consideration.
// These types are unsuitable for relationship discovery via value overlap.
var excludedJoinTypes = map[string]bool{
	// Temporal types - dates/times don't represent relationships
	"timestamp": true, "timestamptz": true, "date": true,
	"time": true, "timetz": true, "interval": true,
	// Boolean - too few values, causes false positives
	"boolean": true, "bool": true,
	// Binary/LOB types - not comparable
	"bytea": true, "blob": true, "binary": true,
	// Structured data types - not comparable
	"json": true, "jsonb": true, "xml": true,
	// Geometry types - spatial data not suitable for FK inference
	"point": true, "line": true, "polygon": true, "geometry": true,
	// Variable-length strings - varying lengths cause false positives
	"character varying": true, "varchar": true,
}

// isNumericType checks if a type is numeric (integers, decimals, floats).
// Numeric types are excluded from value-overlap inference because:
// 1. Auto-increment IDs naturally overlap across unrelated tables
// 2. Counts/amounts falsely match other numeric columns
// 3. Real numeric FK relationships are already captured via DB FK constraints
func isNumericType(dataType string) bool {
	t := normalizeType(dataType)
	switch t {
	// Integer types
	case "integer", "int", "int4", "bigint", "int8", "smallint", "int2":
		return true
	// Serial types (auto-increment integers)
	case "serial", "bigserial", "smallserial":
		return true
	// Decimal/float types
	case "numeric", "decimal", "real", "double precision", "float", "float4", "float8":
		return true
	}
	return false
}

// RelationshipDiscoveryService handles automated relationship discovery.
type RelationshipDiscoveryService interface {
	// DiscoverRelationships runs the full discovery algorithm.
	DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DiscoveryResults, error)
}

type relationshipDiscoveryService struct {
	schemaRepo     repositories.SchemaRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	logger         *zap.Logger
}

// Compile-time check that relationshipDiscoveryService implements RelationshipDiscoveryService.
var _ RelationshipDiscoveryService = (*relationshipDiscoveryService)(nil)

// NewRelationshipDiscoveryService creates a new relationship discovery service.
func NewRelationshipDiscoveryService(
	schemaRepo repositories.SchemaRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) RelationshipDiscoveryService {
	return &relationshipDiscoveryService{
		schemaRepo:     schemaRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		logger:         logger,
	}
}

// DiscoverRelationships runs the complete discovery algorithm.
func (s *relationshipDiscoveryService) DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DiscoveryResults, error) {
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user ID not found in context: %w", err)
	}

	// Get datasource with decrypted config
	ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create schema discoverer with identity parameters for connection pooling
	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	results := &models.DiscoveryResults{
		EmptyTableNames:  make([]string, 0),
		OrphanTableNames: make([]string, 0),
	}

	// Phase 1: Get all tables and columns
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	results.TablesAnalyzed = len(tables)

	// Build table lookup map
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	// Phase 2: Gather column stats and classify joinability
	columnsAnalyzed := 0
	pkColumnsByTable := make(map[uuid.UUID][]*models.SchemaColumn) // Table ID -> PK columns
	allJoinableColumns := make([]*columnWithTable, 0)

	for _, table := range tables {
		// Skip empty tables
		if table.RowCount == nil || *table.RowCount == 0 {
			results.EmptyTables++
			results.EmptyTableNames = append(results.EmptyTableNames, table.TableName)
			continue
		}

		// Extract row count for use in inner loop (defensive - nil already checked above)
		tableRowCount := *table.RowCount

		columns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID, false)
		if err != nil {
			return nil, fmt.Errorf("failed to list columns for %s: %w", table.TableName, err)
		}

		// Batch analyze column stats
		columnNames := make([]string, len(columns))
		for i, c := range columns {
			columnNames[i] = c.ColumnName
		}

		stats, err := s.analyzeColumnStats(ctx, discoverer, table, columnNames)
		if err != nil {
			s.logger.Error("Failed to analyze column stats, skipping table",
				zap.String("table", table.TableName),
				zap.Error(err))
			continue
		}

		// Build stats lookup
		statsMap := make(map[string]*datasource.ColumnStats)
		for i := range stats {
			statsMap[stats[i].ColumnName] = &stats[i]
		}

		// Classify joinability and update columns
		for _, col := range columns {
			columnsAnalyzed++
			st := statsMap[col.ColumnName]

			isJoinable, reason := s.classifyJoinability(col, st, tableRowCount)

			// Update column joinability in database
			var rowCount, nonNullCount, distinctCount *int64
			if st != nil {
				rowCount = &st.RowCount
				nonNullCount = &st.NonNullCount
				distinctCount = &st.DistinctCount
			}

			if err := s.schemaRepo.UpdateColumnJoinability(ctx, col.ID, rowCount, nonNullCount, distinctCount, &isJoinable, &reason); err != nil {
				s.logger.Error("Failed to update column joinability",
					zap.String("column", col.ColumnName),
					zap.Error(err))
			}

			// Track joinable columns
			if isJoinable {
				cwt := &columnWithTable{
					column: col,
					table:  table,
				}
				allJoinableColumns = append(allJoinableColumns, cwt)

				if col.IsPrimaryKey {
					pkColumnsByTable[table.ID] = append(pkColumnsByTable[table.ID], col)
				}
			}
		}
	}

	results.ColumnsAnalyzed = columnsAnalyzed

	// Phase 3: Find relationship candidates
	// Get existing relationships to avoid duplicates
	existingRels, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing relationships: %w", err)
	}

	existingRelSet := make(map[string]bool)
	tablesWithOutbound := make(map[uuid.UUID]bool)
	tablesWithInbound := make(map[uuid.UUID]bool)

	for _, rel := range existingRels {
		key := rel.SourceColumnID.String() + "->" + rel.TargetColumnID.String()
		existingRelSet[key] = true
		tablesWithOutbound[rel.SourceTableID] = true
		tablesWithInbound[rel.TargetTableID] = true
	}

	// Find candidate pairs
	candidates := make([]*relationshipCandidate, 0)

	for _, source := range allJoinableColumns {
		// Skip tables that already have outbound relationships (for this column)
		// Actually, we want to discover even if table has some relationships - focus on columns without outbound

		for _, pkCols := range pkColumnsByTable {
			for _, targetPK := range pkCols {
				// Skip self-references
				if source.table.ID == targetPK.SchemaTableID {
					continue
				}

				// Skip if relationship already exists
				key := source.column.ID.String() + "->" + targetPK.ID.String()
				if existingRelSet[key] {
					continue
				}

				// Skip ALL numeric-to-numeric joins to prevent false positives from:
				// 1. Auto-increment IDs naturally overlapping across unrelated tables
				// 2. Count/amount columns falsely matching other numeric columns
				// 3. Statistical coincidence in numeric data (e.g., order_total matching user_id)
				//
				// Real numeric FK relationships are captured via database FK constraints
				// (imported as 'fk' type), so we lose no valid relationships here.
				if isNumericType(source.column.DataType) && isNumericType(targetPK.DataType) {
					continue
				}

				// Skip if types are incompatible
				if !s.areTypesCompatible(source.column.DataType, targetPK.DataType) {
					continue
				}

				// Skip if semantic validation fails:
				// - *_id columns should reference their expected table
				// - Attribute columns (email, password, etc.) are never FK sources
				if !shouldCreateCandidate(source.column.ColumnName, tableByID[targetPK.SchemaTableID].TableName) {
					continue
				}

				candidates = append(candidates, &relationshipCandidate{
					sourceColumn: source.column,
					sourceTable:  source.table,
					targetColumn: targetPK,
					targetTable:  tableByID[targetPK.SchemaTableID],
				})
			}
		}
	}

	s.logger.Info("Found relationship candidates",
		zap.Int("candidates", len(candidates)))

	// Phase 4: Verify candidates via value overlap and join analysis
	relationshipsCreated := 0

	for i, candidate := range candidates {
		// Check for context cancellation periodically (every 100 candidates)
		if i%100 == 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
		}

		// Skip if target table doesn't exist (shouldn't happen but safety check)
		if candidate.targetTable == nil {
			continue
		}

		// Check value overlap
		overlap, err := discoverer.CheckValueOverlap(
			ctx,
			candidate.sourceTable.SchemaName, candidate.sourceTable.TableName, candidate.sourceColumn.ColumnName,
			candidate.targetTable.SchemaName, candidate.targetTable.TableName, candidate.targetColumn.ColumnName,
			DefaultSampleLimit,
		)
		if err != nil {
			s.logger.Debug("Value overlap check failed",
				zap.String("source", candidate.sourceTable.TableName+"."+candidate.sourceColumn.ColumnName),
				zap.String("target", candidate.targetTable.TableName+"."+candidate.targetColumn.ColumnName),
				zap.Error(err))
			continue
		}

		// Check match threshold
		if overlap.MatchRate < DefaultMatchThreshold {
			// Record as rejected candidate
			s.recordRejectedCandidate(ctx, projectID, candidate, overlap, models.RejectionLowMatchRate)
			continue
		}

		// Validate FK direction: source (FK) should have fewer or equal distinct values than target (PK)
		// If source has more distinct values, it's likely the PK side, indicating a reversed relationship
		if rejection := validateFKDirection(overlap, candidate.sourceTable, candidate.targetTable); rejection != "" {
			s.recordRejectedCandidate(ctx, projectID, candidate, overlap, rejection)
			continue
		}

		// Verify via join analysis
		joinAnalysis, err := discoverer.AnalyzeJoin(
			ctx,
			candidate.sourceTable.SchemaName, candidate.sourceTable.TableName, candidate.sourceColumn.ColumnName,
			candidate.targetTable.SchemaName, candidate.targetTable.TableName, candidate.targetColumn.ColumnName,
		)
		if err != nil {
			s.logger.Debug("Join analysis failed",
				zap.String("source", candidate.sourceTable.TableName+"."+candidate.sourceColumn.ColumnName),
				zap.String("target", candidate.targetTable.TableName+"."+candidate.targetColumn.ColumnName),
				zap.Error(err))
			s.recordRejectedCandidate(ctx, projectID, candidate, overlap, models.RejectionJoinFailed)
			continue
		}

		// Zero-orphan requirement for inferred relationships
		// Real FK relationships should have 0% orphans - all source values must exist in target.
		// This prevents false positives from coincidental value overlap.
		if joinAnalysis.OrphanCount > 0 {
			s.recordRejectedCandidate(ctx, projectID, candidate, overlap, models.RejectionOrphanIntegrity)
			continue
		}

		// Phase 5: Create verified relationship
		cardinality := s.inferCardinality(joinAnalysis)
		inferenceMethod := models.InferenceMethodValueOverlap

		rel := &models.SchemaRelationship{
			ProjectID:        projectID,
			SourceTableID:    candidate.sourceTable.ID,
			SourceColumnID:   candidate.sourceColumn.ID,
			TargetTableID:    candidate.targetTable.ID,
			TargetColumnID:   candidate.targetColumn.ID,
			RelationshipType: models.RelationshipTypeInferred,
			Cardinality:      cardinality,
			Confidence:       overlap.MatchRate,
			InferenceMethod:  &inferenceMethod,
			IsValidated:      true,
		}

		metrics := &models.DiscoveryMetrics{
			MatchRate:      overlap.MatchRate,
			SourceDistinct: overlap.SourceDistinct,
			TargetDistinct: overlap.TargetDistinct,
			MatchedCount:   overlap.MatchedCount,
		}

		if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
			s.logger.Error("Failed to create relationship",
				zap.String("source", candidate.sourceTable.TableName+"."+candidate.sourceColumn.ColumnName),
				zap.String("target", candidate.targetTable.TableName+"."+candidate.targetColumn.ColumnName),
				zap.Error(err))
			continue
		}

		relationshipsCreated++
		tablesWithOutbound[candidate.sourceTable.ID] = true
		tablesWithInbound[candidate.targetTable.ID] = true

		s.logger.Info("Created inferred relationship",
			zap.String("source", candidate.sourceTable.TableName+"."+candidate.sourceColumn.ColumnName),
			zap.String("target", candidate.targetTable.TableName+"."+candidate.targetColumn.ColumnName),
			zap.Float64("match_rate", overlap.MatchRate),
			zap.String("cardinality", cardinality))
	}

	results.RelationshipsCreated = relationshipsCreated

	// Find orphan tables (non-empty tables with no relationships)
	orphanTables := make([]*models.SchemaTable, 0)
	for _, table := range tables {
		if table.RowCount != nil && *table.RowCount > 0 {
			if !tablesWithOutbound[table.ID] && !tablesWithInbound[table.ID] {
				results.TablesWithoutRelationships++
				results.OrphanTableNames = append(results.OrphanTableNames, table.TableName)
				orphanTables = append(orphanTables, table)
			}
		}
	}

	// Phase 6: Find review candidates for orphan tables
	// These are numeric-to-numeric relationships that we skipped earlier but may be valid FKs
	if len(orphanTables) > 0 {
		reviewCreated, err := s.findReviewCandidates(ctx, discoverer, projectID, datasourceID, orphanTables, tableByID, pkColumnsByTable, existingRelSet)
		if err != nil {
			s.logger.Error("Failed to find review candidates", zap.Error(err))
			// Continue - this is not fatal
		} else {
			results.ReviewCandidatesCreated = reviewCreated
		}
	}

	s.logger.Info("Relationship discovery completed",
		zap.String("datasource_id", datasourceID.String()),
		zap.Int("tables_analyzed", results.TablesAnalyzed),
		zap.Int("columns_analyzed", results.ColumnsAnalyzed),
		zap.Int("relationships_created", results.RelationshipsCreated),
		zap.Int("review_candidates_created", results.ReviewCandidatesCreated),
		zap.Int("empty_tables", results.EmptyTables),
		zap.Int("orphan_tables", results.TablesWithoutRelationships))

	return results, nil
}

// Helper types

type columnWithTable struct {
	column *models.SchemaColumn
	table  *models.SchemaTable
}

type relationshipCandidate struct {
	sourceColumn *models.SchemaColumn
	sourceTable  *models.SchemaTable
	targetColumn *models.SchemaColumn
	targetTable  *models.SchemaTable
}

// Helper methods

func (s *relationshipDiscoveryService) analyzeColumnStats(
	ctx context.Context,
	discoverer datasource.SchemaDiscoverer,
	table *models.SchemaTable,
	columnNames []string,
) ([]datasource.ColumnStats, error) {
	// Process in batches if needed
	if len(columnNames) <= MaxColumnsPerStatsQuery {
		return discoverer.AnalyzeColumnStats(ctx, table.SchemaName, table.TableName, columnNames)
	}

	allStats := make([]datasource.ColumnStats, 0, len(columnNames))
	for i := 0; i < len(columnNames); i += MaxColumnsPerStatsQuery {
		end := i + MaxColumnsPerStatsQuery
		if end > len(columnNames) {
			end = len(columnNames)
		}

		batch, err := discoverer.AnalyzeColumnStats(ctx, table.SchemaName, table.TableName, columnNames[i:end])
		if err != nil {
			return nil, err
		}
		allStats = append(allStats, batch...)
	}

	return allStats, nil
}

func (s *relationshipDiscoveryService) classifyJoinability(col *models.SchemaColumn, stats *datasource.ColumnStats, tableRowCount int64) (bool, string) {
	// Primary keys are always joinable
	if col.IsPrimaryKey {
		return true, models.JoinabilityPK
	}

	// Exclude certain data types (use shared normalizeType function)
	baseType := normalizeType(col.DataType)

	if excludedJoinTypes[baseType] {
		return false, models.JoinabilityTypeExcluded
	}

	// Need stats for further classification
	if stats == nil || tableRowCount == 0 {
		return false, models.JoinabilityNoStats
	}

	// Check for unique values (potential FK target)
	if stats.DistinctCount == stats.NonNullCount && stats.NonNullCount > 0 {
		return true, models.JoinabilityUniqueValues
	}

	// Check for low cardinality (< 1% distinct)
	distinctRatio := float64(stats.DistinctCount) / float64(tableRowCount)
	if distinctRatio < 0.01 {
		return false, models.JoinabilityLowCardinality
	}

	// Default: joinable if it has reasonable cardinality
	return true, models.JoinabilityCardinalityOK
}

func (s *relationshipDiscoveryService) areTypesCompatible(sourceType, targetType string) bool {
	// Normalize types for comparison
	source := normalizeType(sourceType)
	target := normalizeType(targetType)

	// After numeric types are skipped (earlier in the pipeline), the remaining
	// joinable types are uuid and text. These require exact type match.
	// Same normalized type = compatible
	return source == target
}

// attributeColumnPatterns contains column names that represent data attributes,
// not foreign key references. These should never be FK sources.
var attributeColumnPatterns = []string{
	"email",
	"password",
	"name",
	"description",
	"status",
	"type",
}

// shouldCreateCandidate validates whether a source column should be considered
// as a foreign key candidate pointing to the target column. This applies semantic
// validation beyond type compatibility to reduce false positive relationships.
//
// Rules:
// 1. *_id columns should reference their expected table (user_id → users)
// 2. Attribute columns (email, password, etc.) are never FK sources
func shouldCreateCandidate(sourceColumnName, targetTableName string) bool {
	sourceLower := strings.ToLower(sourceColumnName)

	// Rule 2: Attribute columns are not FK sources
	// Check if source column name contains any attribute pattern
	for _, attr := range attributeColumnPatterns {
		if strings.Contains(sourceLower, attr) {
			return false // emails, passwords, statuses aren't FKs
		}
	}

	// Rule 1: *_id columns should match their expected table
	if strings.HasSuffix(sourceLower, "_id") {
		// Extract the entity name from column (e.g., "user_id" → "user")
		entityName := strings.TrimSuffix(sourceLower, "_id")

		// Target table should be the entity name or its plural form
		targetLower := strings.ToLower(targetTableName)

		// Handle common plural patterns:
		// user_id → users, user
		// category_id → categories, category
		// status_id → statuses, status
		expectedPlural := entityName + "s"
		expectedPluralIES := ""
		if strings.HasSuffix(entityName, "y") {
			// category → categories (drop y, add ies)
			expectedPluralIES = entityName[:len(entityName)-1] + "ies"
		}

		if targetLower != entityName &&
			targetLower != expectedPlural &&
			(expectedPluralIES == "" || targetLower != expectedPluralIES) {
			return false // user_id shouldn't point to channels table
		}
	}

	return true
}

// FKDirectionOrphanThreshold defines the maximum orphan rate (5%) for FK direction validation.
// If more than 5% of source values don't exist in the target, the relationship is likely invalid.
const FKDirectionOrphanThreshold = 0.05

// validateFKDirection validates that the relationship direction is correct based on cardinality patterns.
// Returns a rejection reason if the direction is wrong, empty string if valid.
//
// FK pattern rules:
// 1. Source (FK) should have fewer or equal distinct values than target (PK)
//   - If source has more distinct values, it's likely the PK side (reversed direction)
//
// 2. Child table (FK holder) typically has more or equal rows than parent (PK holder)
//   - This isn't strictly enforced as some valid patterns (e.g., 1:1) may violate this
//
// This prevents reversed relationships like accounts.account_id → account_password_resets.account_id
// where the direction should be account_password_resets.account_id → accounts.account_id
func validateFKDirection(overlap *datasource.ValueOverlapResult, sourceTable, targetTable *models.SchemaTable) string {
	// Rule 1: FK column should have fewer or equal distinct values than PK column
	// If source has significantly more distinct values, this suggests the source is the PK side
	// Allow a 10% tolerance for data quality issues
	if overlap.SourceDistinct > 0 && overlap.TargetDistinct > 0 {
		if float64(overlap.SourceDistinct) > float64(overlap.TargetDistinct)*1.1 {
			return models.RejectionWrongDirection
		}
	}

	// Rule 2: Validate row count pattern (soft check with tolerance)
	// In typical FK relationships, the child table (FK holder) has more or equal rows
	// However, this is a soft check because:
	// - 1:1 relationships may have equal rows
	// - Optional FKs may have fewer rows on either side
	// - We use a 50% tolerance to avoid false rejections
	if sourceTable.RowCount != nil && targetTable.RowCount != nil {
		sourceRows := *sourceTable.RowCount
		targetRows := *targetTable.RowCount

		// If target (PK table) has significantly more rows than source (FK table),
		// this could indicate a reversed relationship, but only flag extreme cases
		// where target has 3x more rows (very unusual for valid FK)
		if targetRows > 0 && sourceRows > 0 {
			if float64(targetRows) > float64(sourceRows)*3 {
				// This is suspicious but not definitive - log it but don't reject
				// The orphan check (which happens after join analysis) will catch invalid relationships
			}
		}
	}

	return "" // Valid direction
}

func normalizeType(t string) string {
	t = strings.ToLower(t)
	// Strip length/precision info
	if idx := strings.Index(t, "("); idx > 0 {
		t = t[:idx]
	}
	// Strip array suffix
	t = strings.TrimSuffix(t, "[]")
	return t
}

func (s *relationshipDiscoveryService) inferCardinality(join *datasource.JoinAnalysis) string {
	if join.SourceMatched == 0 || join.TargetMatched == 0 {
		return models.CardinalityUnknown
	}

	// Ratio of join rows to source/target matched
	sourceRatio := float64(join.JoinCount) / float64(join.SourceMatched)
	targetRatio := float64(join.JoinCount) / float64(join.TargetMatched)

	// 1:1 - both sides have unique matches
	if sourceRatio <= CardinalityUniqueThreshold && targetRatio <= CardinalityUniqueThreshold {
		return models.Cardinality1To1
	}

	// N:1 - multiple source rows match one target (typical FK)
	if sourceRatio <= CardinalityUniqueThreshold && targetRatio > CardinalityUniqueThreshold {
		return models.CardinalityNTo1
	}

	// 1:N - one source matches multiple targets (reverse FK)
	if sourceRatio > CardinalityUniqueThreshold && targetRatio <= CardinalityUniqueThreshold {
		return models.Cardinality1ToN
	}

	// N:M - many-to-many
	return models.CardinalityNToM
}

func (s *relationshipDiscoveryService) recordRejectedCandidate(
	ctx context.Context,
	projectID uuid.UUID,
	candidate *relationshipCandidate,
	overlap *datasource.ValueOverlapResult,
	rejectionReason string,
) {
	// Create a rejected relationship record for tracking
	rel := &models.SchemaRelationship{
		ProjectID:        projectID,
		SourceTableID:    candidate.sourceTable.ID,
		SourceColumnID:   candidate.sourceColumn.ID,
		TargetTableID:    candidate.targetTable.ID,
		TargetColumnID:   candidate.targetColumn.ID,
		RelationshipType: models.RelationshipTypeInferred,
		Cardinality:      models.CardinalityUnknown,
		Confidence:       overlap.MatchRate,
		IsValidated:      false,
		RejectionReason:  &rejectionReason,
	}

	metrics := &models.DiscoveryMetrics{
		MatchRate:      overlap.MatchRate,
		SourceDistinct: overlap.SourceDistinct,
		TargetDistinct: overlap.TargetDistinct,
		MatchedCount:   overlap.MatchedCount,
	}

	if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
		s.logger.Error("Failed to record rejected candidate",
			zap.String("source", candidate.sourceTable.TableName+"."+candidate.sourceColumn.ColumnName),
			zap.String("target", candidate.targetTable.TableName+"."+candidate.targetColumn.ColumnName),
			zap.Error(err))
	}
}

// findReviewCandidates discovers potential numeric FK relationships for orphan tables.
// It finds tables that may reference the orphan table's PK via an FK column.
// These are stored as type='review' for LLM-assisted verification since numeric
// relationships were skipped by the deterministic algorithm to avoid false positives.
//
// Algorithm: For each orphan table, find non-PK columns in other tables that:
// 1. Have exact type match with the orphan's PK
// 2. Have high cardinality (not counts/status codes)
// 3. Have 100% of their values present in the orphan's PK (FK integrity)
//
// Relationships follow FK convention: source=FK holder, target=PK holder
// e.g., other_table.orphan_id -> orphan_table.id
func (s *relationshipDiscoveryService) findReviewCandidates(
	ctx context.Context,
	discoverer datasource.SchemaDiscoverer,
	projectID, datasourceID uuid.UUID,
	orphanTables []*models.SchemaTable,
	tableByID map[uuid.UUID]*models.SchemaTable,
	pkColumnsByTable map[uuid.UUID][]*models.SchemaColumn,
	existingRelSet map[string]bool,
) (int, error) {
	reviewCreated := 0

	for _, orphanTable := range orphanTables {
		// Get PK columns from orphan table (these are potential FK targets)
		pkColumns, err := s.getSourceColumnsForReview(ctx, projectID, orphanTable)
		if err != nil {
			s.logger.Error("Failed to get PK columns for review",
				zap.String("table", orphanTable.TableName),
				zap.Error(err))
			continue
		}

		for _, pkCol := range pkColumns {
			// Only consider numeric types for review candidates
			if !isNumericType(pkCol.DataType) {
				continue
			}

			// Find FK candidate columns (non-PK columns with exact type match)
			fkCandidates, err := s.schemaRepo.GetNonPKColumnsByExactType(ctx, projectID, datasourceID, pkCol.DataType)
			if err != nil {
				s.logger.Error("Failed to get FK candidate columns by type",
					zap.String("type", pkCol.DataType),
					zap.Error(err))
				continue
			}

			for _, fkCol := range fkCandidates {
				// Skip self-references
				if pkCol.SchemaTableID == fkCol.SchemaTableID {
					continue
				}

				// Skip if relationship already exists (FK -> PK direction)
				key := fkCol.ID.String() + "->" + pkCol.ID.String()
				if existingRelSet[key] {
					continue
				}

				// Get FK table
				fkTable := tableByID[fkCol.SchemaTableID]
				if fkTable == nil {
					continue
				}

				// Check FK column has high cardinality (looks like a join key, not a count)
				if !s.isHighCardinality(fkCol, fkTable) {
					continue
				}

				// Check FK integrity: all FK values must exist in PK
				// Source = FK column, Target = PK column
				overlap, err := discoverer.CheckValueOverlap(
					ctx,
					fkTable.SchemaName, fkTable.TableName, fkCol.ColumnName,
					orphanTable.SchemaName, orphanTable.TableName, pkCol.ColumnName,
					DefaultSampleLimit,
				)
				if err != nil {
					s.logger.Debug("Value overlap check failed for review candidate",
						zap.String("fk", fkTable.TableName+"."+fkCol.ColumnName),
						zap.String("pk", orphanTable.TableName+"."+pkCol.ColumnName),
						zap.Error(err))
					continue
				}

				// Require 100% match (no orphan FK values) for review candidates
				// This is strict by design - review candidates should have high confidence
				if overlap.MatchRate < 1.0-ReviewMaxOrphanRate {
					continue
				}

				// Create review relationship following FK convention: source=FK, target=PK
				inferenceMethod := models.InferenceMethodValueOverlap
				rel := &models.SchemaRelationship{
					ProjectID:        projectID,
					SourceTableID:    fkTable.ID,     // FK holder
					SourceColumnID:   fkCol.ID,       // FK column
					TargetTableID:    orphanTable.ID, // PK holder
					TargetColumnID:   pkCol.ID,       // PK column
					RelationshipType: models.RelationshipTypeReview,
					Cardinality:      models.CardinalityUnknown, // Will be determined on approval
					Confidence:       overlap.MatchRate,
					InferenceMethod:  &inferenceMethod,
					IsValidated:      false,
					IsApproved:       nil, // Pending review
				}

				metrics := &models.DiscoveryMetrics{
					MatchRate:      overlap.MatchRate,
					SourceDistinct: overlap.SourceDistinct,
					TargetDistinct: overlap.TargetDistinct,
					MatchedCount:   overlap.MatchedCount,
				}

				if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
					s.logger.Error("Failed to create review relationship",
						zap.String("fk", fkTable.TableName+"."+fkCol.ColumnName),
						zap.String("pk", orphanTable.TableName+"."+pkCol.ColumnName),
						zap.Error(err))
					continue
				}

				reviewCreated++
				existingRelSet[key] = true // Prevent duplicates

				s.logger.Info("Created review candidate",
					zap.String("fk", fkTable.TableName+"."+fkCol.ColumnName),
					zap.String("pk", orphanTable.TableName+"."+pkCol.ColumnName),
					zap.Float64("match_rate", overlap.MatchRate))
			}
		}
	}

	return reviewCreated, nil
}

// getSourceColumnsForReview returns the columns to use as source for review candidate discovery.
// If the table has a primary key, only PK columns are returned (more restrictive).
// If the table has no primary key, all numeric columns are returned.
func (s *relationshipDiscoveryService) getSourceColumnsForReview(ctx context.Context, projectID uuid.UUID, table *models.SchemaTable) ([]*models.SchemaColumn, error) {
	columns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID, false)
	if err != nil {
		return nil, err
	}

	// Check if table has a primary key
	hasPK := false
	for _, col := range columns {
		if col.IsPrimaryKey {
			hasPK = true
			break
		}
	}

	result := make([]*models.SchemaColumn, 0)
	for _, col := range columns {
		if hasPK {
			// If table has PK, only use PK columns
			if col.IsPrimaryKey {
				result = append(result, col)
			}
		} else {
			// If no PK, use all numeric columns
			if isNumericType(col.DataType) {
				result = append(result, col)
			}
		}
	}

	return result, nil
}

// isHighCardinality checks if a column has high enough cardinality to be a join key.
// Low cardinality columns (counts, status codes) are excluded.
func (s *relationshipDiscoveryService) isHighCardinality(col *models.SchemaColumn, table *models.SchemaTable) bool {
	// Need distinct count stats
	if col.DistinctCount == nil {
		return false
	}

	distinctCount := *col.DistinctCount

	// Must have at least ReviewMinDistinctCount distinct values
	if distinctCount < ReviewMinDistinctCount {
		return false
	}

	// Check cardinality ratio if we have row count
	if table.RowCount != nil && *table.RowCount > 0 {
		ratio := float64(distinctCount) / float64(*table.RowCount)
		if ratio < ReviewMinCardinalityRatio {
			return false
		}
	}

	return true
}
