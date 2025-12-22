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
)

// Discovery configuration constants
const (
	DefaultMatchThreshold      = 0.70 // 70% value overlap required
	DefaultOrphanRateThreshold = 0.50 // 50% max orphan rate
	DefaultSampleLimit         = 1000 // Max values to sample for overlap check
	MaxColumnsPerStatsQuery    = 25   // Batch size for column stats
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
	// Get datasource with decrypted config
	ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create schema discoverer
	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	results := &models.DiscoveryResults{
		EmptyTableNames:  make([]string, 0),
		OrphanTableNames: make([]string, 0),
	}

	// Phase 1: Get all tables and columns
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
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

		columns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID)
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
			s.logger.Warn("Failed to analyze column stats, skipping table",
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

			isJoinable, reason := s.classifyJoinability(col, st, *table.RowCount)

			// Update column joinability in database
			var rowCount, nonNullCount *int64
			if st != nil {
				rowCount = &st.RowCount
				nonNullCount = &st.NonNullCount
			}

			if err := s.schemaRepo.UpdateColumnJoinability(ctx, col.ID, rowCount, nonNullCount, &isJoinable, &reason); err != nil {
				s.logger.Warn("Failed to update column joinability",
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

				// Skip ALL numeric-to-numeric joins - these cause false positives.
				// Auto-increment IDs, counts, and amounts naturally overlap across
				// unrelated tables. Real numeric FK relationships are already
				// captured via database FK constraints (imported as 'fk' type).
				if isNumericType(source.column.DataType) && isNumericType(targetPK.DataType) {
					continue
				}

				// Skip if types are incompatible
				if !s.areTypesCompatible(source.column.DataType, targetPK.DataType) {
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

	for _, candidate := range candidates {
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

		// Check orphan rate
		var orphanRate float64
		if joinAnalysis.SourceMatched > 0 {
			orphanRate = float64(joinAnalysis.OrphanCount) / float64(joinAnalysis.SourceMatched+joinAnalysis.OrphanCount)
		}

		if orphanRate > DefaultOrphanRateThreshold {
			s.recordRejectedCandidate(ctx, projectID, candidate, overlap, models.RejectionHighOrphanRate)
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
			s.logger.Warn("Failed to create relationship",
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
	for _, table := range tables {
		if table.RowCount != nil && *table.RowCount > 0 {
			if !tablesWithOutbound[table.ID] && !tablesWithInbound[table.ID] {
				results.TablesWithoutRelationships++
				results.OrphanTableNames = append(results.OrphanTableNames, table.TableName)
			}
		}
	}

	s.logger.Info("Relationship discovery completed",
		zap.String("datasource_id", datasourceID.String()),
		zap.Int("tables_analyzed", results.TablesAnalyzed),
		zap.Int("columns_analyzed", results.ColumnsAnalyzed),
		zap.Int("relationships_created", results.RelationshipsCreated),
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

	// Exclude certain data types
	baseType := strings.ToLower(col.DataType)
	// Strip any length/precision info like "varchar(255)"
	if idx := strings.Index(baseType, "("); idx > 0 {
		baseType = baseType[:idx]
	}
	// Handle array types like "integer[]"
	baseType = strings.TrimSuffix(baseType, "[]")

	if excludedJoinTypes[baseType] {
		return false, models.JoinabilityTypeExcluded
	}

	// Need stats for further classification
	if stats == nil || tableRowCount == 0 {
		return false, "no_stats"
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
	return true, "cardinality_ok"
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
	if sourceRatio <= 1.1 && targetRatio <= 1.1 {
		return models.Cardinality1To1
	}

	// N:1 - multiple source rows match one target (typical FK)
	if sourceRatio <= 1.1 && targetRatio > 1.1 {
		return models.CardinalityNTo1
	}

	// 1:N - one source matches multiple targets (reverse FK)
	if sourceRatio > 1.1 && targetRatio <= 1.1 {
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
		s.logger.Warn("Failed to record rejected candidate",
			zap.String("source", candidate.sourceTable.TableName+"."+candidate.sourceColumn.ColumnName),
			zap.String("target", candidate.targetTable.TableName+"."+candidate.targetColumn.ColumnName),
			zap.Error(err))
	}
}

// Ensure relationshipDiscoveryService implements RelationshipDiscoveryService at compile time.
var _ RelationshipDiscoveryService = (*relationshipDiscoveryService)(nil)
