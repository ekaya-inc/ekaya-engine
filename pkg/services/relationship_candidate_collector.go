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
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// RelationshipCandidateCollector collects potential FK relationship candidates
// for LLM validation. This is Phase 1 of the relationship discovery pipeline.
type RelationshipCandidateCollector interface {
	// CollectCandidates gathers all potential FK relationship candidates
	// using deterministic criteria. The candidates are then passed to
	// the LLM validation phase.
	CollectCandidates(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) ([]*RelationshipCandidate, error)
}

type relationshipCandidateCollector struct {
	schemaRepo     repositories.SchemaRepository
	adapterFactory datasource.DatasourceAdapterFactory
	dsSvc          DatasourceService
	logger         *zap.Logger
}

// NewRelationshipCandidateCollector creates a new RelationshipCandidateCollector.
func NewRelationshipCandidateCollector(
	schemaRepo repositories.SchemaRepository,
	adapterFactory datasource.DatasourceAdapterFactory,
	dsSvc DatasourceService,
	logger *zap.Logger,
) RelationshipCandidateCollector {
	return &relationshipCandidateCollector{
		schemaRepo:     schemaRepo,
		adapterFactory: adapterFactory,
		dsSvc:          dsSvc,
		logger:         logger.Named("relationship-candidate-collector"),
	}
}

// FKSourceColumn represents a column identified as a potential FK source.
// It includes both schema metadata and ColumnFeatures data.
type FKSourceColumn struct {
	Column   *models.SchemaColumn
	Features *models.ColumnFeatures
	// TableName is cached for convenience
	TableName string
}

// identifyFKSources returns columns that are potential FK sources based on ColumnFeatures data.
// A column qualifies as an FK source if:
//   - ColumnFeatures role = 'foreign_key', OR
//   - ColumnFeatures purpose = 'identifier' (identifiers often reference other tables), OR
//   - is_joinable = true in column statistics
//
// Columns are EXCLUDED if:
//   - They are primary keys (PKs are targets, not sources)
//   - They are timestamp columns (classification_path = 'timestamp')
//   - They are boolean columns (classification_path = 'boolean')
//   - They are JSON columns (classification_path = 'json')
//
// Per CLAUDE.md rule #5: We do NOT filter by column name patterns (e.g., _id suffix).
// All classification is based on ColumnFeatures data and explicit schema metadata.
func (c *relationshipCandidateCollector) identifyFKSources(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
) ([]*FKSourceColumn, error) {
	// Get all columns with their features
	columnsByTable, err := c.schemaRepo.GetColumnsWithFeaturesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get columns with features: %w", err)
	}

	// Also get all columns to catch joinable columns without features
	allColumns, err := c.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list all columns: %w", err)
	}

	// Build a map of column IDs to table names for all columns
	columnTableMap := make(map[uuid.UUID]string)
	tableIDToName := make(map[uuid.UUID]string)

	// Get tables to resolve table names
	tables, err := c.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	for _, t := range tables {
		tableIDToName[t.ID] = t.TableName
	}
	for _, col := range allColumns {
		if tableName, ok := tableIDToName[col.SchemaTableID]; ok {
			columnTableMap[col.ID] = tableName
		}
	}

	var sources []*FKSourceColumn

	// Process columns with ColumnFeatures
	for tableName, columns := range columnsByTable {
		for _, col := range columns {
			if c.shouldExcludeFromFKSources(col) {
				continue
			}

			features := col.GetColumnFeatures()
			if features == nil {
				continue
			}

			// Check if this column qualifies as an FK source based on features
			if c.isQualifiedFKSource(col, features) {
				sources = append(sources, &FKSourceColumn{
					Column:    col,
					Features:  features,
					TableName: tableName,
				})
			}
		}
	}

	// Also check columns marked as joinable but without features (fallback)
	// This catches columns that were marked joinable during earlier analysis
	seenColumns := make(map[uuid.UUID]bool)
	for _, src := range sources {
		seenColumns[src.Column.ID] = true
	}

	for _, col := range allColumns {
		// Skip if already added
		if seenColumns[col.ID] {
			continue
		}

		// Skip columns that should be excluded
		if c.shouldExcludeFromFKSources(col) {
			continue
		}

		// Include if marked as joinable
		if col.IsJoinable != nil && *col.IsJoinable {
			tableName, ok := columnTableMap[col.ID]
			if !ok || tableName == "" {
				// Skip columns where table name cannot be resolved
				continue
			}
			sources = append(sources, &FKSourceColumn{
				Column:    col,
				Features:  col.GetColumnFeatures(), // May be nil
				TableName: tableName,
			})
		}
	}

	c.logger.Info("identified FK source candidates",
		zap.Int("count", len(sources)),
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()),
	)

	return sources, nil
}

// shouldExcludeFromFKSources returns true if a column should be excluded from FK source consideration.
// Exclusion criteria:
//   - Primary keys (they are targets, not sources)
//   - Timestamp columns
//   - Boolean columns
//   - JSON columns
func (c *relationshipCandidateCollector) shouldExcludeFromFKSources(col *models.SchemaColumn) bool {
	// Exclude primary keys - they are FK targets, not sources
	if col.IsPrimaryKey {
		return true
	}

	// Exclude based on data type (timestamps, booleans, JSON)
	dataType := strings.ToLower(col.DataType)

	// Timestamp types
	if strings.Contains(dataType, "timestamp") ||
		strings.Contains(dataType, "datetime") ||
		dataType == "date" ||
		dataType == "time" {
		return true
	}

	// Boolean types
	if dataType == "boolean" || dataType == "bool" {
		return true
	}

	// JSON types
	if dataType == "json" || dataType == "jsonb" {
		return true
	}

	// Also check ColumnFeatures classification path for more precise exclusion
	features := col.GetColumnFeatures()
	if features != nil {
		switch features.ClassificationPath {
		case models.ClassificationPathTimestamp,
			models.ClassificationPathBoolean,
			models.ClassificationPathJSON:
			return true
		}
	}

	return false
}

// isQualifiedFKSource returns true if a column qualifies as an FK source based on its features.
// Qualification criteria (any of these):
//   - ColumnFeatures role = 'foreign_key'
//   - ColumnFeatures purpose = 'identifier' (identifiers often reference other tables)
//   - ClassificationPath = 'uuid' (UUIDs are high-priority FK candidates per design doc)
func (c *relationshipCandidateCollector) isQualifiedFKSource(_ *models.SchemaColumn, features *models.ColumnFeatures) bool {
	// Role explicitly marked as foreign_key
	if features.Role == models.RoleForeignKey {
		return true
	}

	// Purpose is identifier (identifiers reference other tables)
	if features.Purpose == models.PurposeIdentifier {
		return true
	}

	// UUID columns are high-priority FK candidates per design doc
	// UUIDs are almost always identifiers that reference something
	if features.ClassificationPath == models.ClassificationPathUUID {
		return true
	}

	// External ID columns might reference external systems, not internal tables
	// However, they should still be considered as potential FK sources
	// since some external IDs do map to internal tables
	if features.ClassificationPath == models.ClassificationPathExternalID {
		return true
	}

	return false
}

// FKTargetColumn represents a column identified as a valid FK target.
// FK targets must be either primary keys or unique columns.
type FKTargetColumn struct {
	Column    *models.SchemaColumn
	TableName string
	IsUnique  bool // true if target is unique (includes PKs)
}

// identifyFKTargets returns columns that are valid FK targets.
// Per the plan, FK targets are ONLY:
//   - Primary key columns (is_primary_key = true)
//   - Unique columns (is_unique = true)
//
// This is a key change from the old approach which allowed any high-cardinality
// column (distinctCount >= 20) as a target, causing ~90% incorrect inferences.
func (c *relationshipCandidateCollector) identifyFKTargets(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
) ([]*FKTargetColumn, error) {
	// Get tables first to resolve table names
	tables, err := c.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	tableIDToName := make(map[uuid.UUID]string)
	for _, t := range tables {
		tableIDToName[t.ID] = t.TableName
	}

	// Get all columns for the datasource
	allColumns, err := c.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}

	var targets []*FKTargetColumn

	for _, col := range allColumns {
		// FK targets must be PKs or unique columns
		if !col.IsPrimaryKey && !col.IsUnique {
			continue
		}

		tableName, ok := tableIDToName[col.SchemaTableID]
		if !ok || tableName == "" {
			// Skip columns where table name cannot be resolved
			continue
		}

		targets = append(targets, &FKTargetColumn{
			Column:    col,
			TableName: tableName,
			IsUnique:  col.IsPrimaryKey || col.IsUnique, // Both PKs and unique columns are "unique"
		})
	}

	c.logger.Info("identified FK target candidates",
		zap.Int("count", len(targets)),
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()),
	)

	return targets, nil
}

// areTypesCompatible checks if two data types are compatible for a FK relationship.
// Compatible pairs include:
//   - Same types (uuid→uuid, int→int, text→text)
//   - Integer variants (int4→int8, smallint→bigint, integer→bigint)
//   - String variants (varchar→text, char→text, character varying→text)
//
// Returns false for clearly incompatible pairs:
//   - text→int, bool→uuid, timestamp→text, etc.
//   - Unknown types (conservative approach - better to reject than create bad FK)
func areTypesCompatible(sourceType, targetType string) bool {
	// Normalize types to lowercase for comparison
	source := strings.ToLower(strings.TrimSpace(sourceType))
	target := strings.ToLower(strings.TrimSpace(targetType))

	// Group types into categories
	sourceCategory := categorizeDataType(source)
	targetCategory := categorizeDataType(target)

	// Unknown types are not compatible with anything (including themselves)
	// This is conservative - better to not create a bad relationship
	if sourceCategory == "" || targetCategory == "" {
		return false
	}

	// Types are compatible if they're in the same category
	return sourceCategory == targetCategory
}

// categorizeDataType returns a category string for the given data type.
// Types in the same category are considered compatible for FK relationships.
func categorizeDataType(dataType string) string {
	// UUID types
	if dataType == "uuid" {
		return "uuid"
	}

	// Integer types
	integerTypes := []string{
		"int", "int2", "int4", "int8",
		"integer", "smallint", "bigint",
		"serial", "smallserial", "bigserial",
		"tinyint", // MSSQL/MySQL
	}
	for _, t := range integerTypes {
		if dataType == t {
			return "integer"
		}
	}

	// String types
	stringTypes := []string{
		"text", "varchar", "char", "character", "character varying",
		"bpchar",                     // PostgreSQL blank-padded char
		"nvarchar", "nchar", "ntext", // MSSQL unicode strings
		"string", // Some databases
	}
	for _, t := range stringTypes {
		if dataType == t || strings.HasPrefix(dataType, t+"(") {
			return "string"
		}
	}
	// Handle varchar(n), char(n) patterns
	if strings.HasPrefix(dataType, "varchar") ||
		strings.HasPrefix(dataType, "char") ||
		strings.HasPrefix(dataType, "character") ||
		strings.HasPrefix(dataType, "nvarchar") ||
		strings.HasPrefix(dataType, "nchar") {
		return "string"
	}

	// Numeric types (decimal, numeric, float, double)
	numericTypes := []string{
		"numeric", "decimal", "float", "float4", "float8",
		"real", "double precision", "double", "money",
	}
	for _, t := range numericTypes {
		if dataType == t || strings.HasPrefix(dataType, t+"(") {
			return "numeric"
		}
	}

	// Boolean types - these should NOT match anything else
	if dataType == "boolean" || dataType == "bool" || dataType == "bit" {
		return "boolean"
	}

	// Timestamp types - these should NOT match anything else
	timestampTypes := []string{
		"timestamp", "timestamptz", "timestamp with time zone",
		"timestamp without time zone", "datetime", "datetime2",
		"date", "time", "timetz", "time with time zone",
	}
	for _, t := range timestampTypes {
		if dataType == t || strings.HasPrefix(dataType, t) {
			return "timestamp"
		}
	}

	// JSON types - these should NOT match anything else
	if dataType == "json" || dataType == "jsonb" {
		return "json"
	}

	// Unknown type - return empty string (not compatible with anything)
	return ""
}

// generateCandidatePairs creates relationship candidates for all valid source→target pairs.
// For each source column, it pairs with each target column if:
//   - They are not the same column (no self-references)
//   - Their data types are compatible (uuid→uuid, int→int, etc.)
//
// The method populates ColumnFeatures-derived fields (SourcePurpose, SourceRole, etc.)
// from the source's ColumnFeatures data.
//
// No threshold-based filtering is applied - all type-compatible pairs become candidates
// for LLM validation in the next phase.
func (c *relationshipCandidateCollector) generateCandidatePairs(
	sources []*FKSourceColumn,
	targets []*FKTargetColumn,
) []*RelationshipCandidate {
	var candidates []*RelationshipCandidate

	for _, source := range sources {
		for _, target := range targets {
			// Skip self-references (same table.column)
			if source.TableName == target.TableName && source.Column.ColumnName == target.Column.ColumnName {
				continue
			}

			// Skip if data types are incompatible
			if !areTypesCompatible(source.Column.DataType, target.Column.DataType) {
				continue
			}

			candidate := &RelationshipCandidate{
				// Source column info
				SourceTable:    source.TableName,
				SourceColumn:   source.Column.ColumnName,
				SourceDataType: source.Column.DataType,
				SourceIsPK:     source.Column.IsPrimaryKey,
				SourceColumnID: source.Column.ID,

				// Target column info
				TargetTable:    target.TableName,
				TargetColumn:   target.Column.ColumnName,
				TargetDataType: target.Column.DataType,
				TargetIsPK:     target.Column.IsPrimaryKey,
				TargetColumnID: target.Column.ID,
			}

			// Populate ColumnFeatures-derived fields for source
			if source.Features != nil {
				candidate.SourcePurpose = source.Features.Purpose
				candidate.SourceRole = source.Features.Role
			}

			// Populate ColumnFeatures-derived fields for target
			targetFeatures := target.Column.GetColumnFeatures()
			if targetFeatures != nil {
				candidate.TargetPurpose = targetFeatures.Purpose
				candidate.TargetRole = targetFeatures.Role
			}

			candidates = append(candidates, candidate)
		}
	}

	c.logger.Debug("generated candidate pairs",
		zap.Int("source_count", len(sources)),
		zap.Int("target_count", len(targets)),
		zap.Int("candidate_count", len(candidates)),
	)

	return candidates
}

// collectJoinStatistics uses the datasource adapter to collect join analysis statistics
// for a relationship candidate. It populates the JoinCount, OrphanCount, ReverseOrphans,
// SourceMatched, and TargetMatched fields on the candidate.
//
// This uses the SchemaDiscoverer.AnalyzeJoin method which performs:
// - Join count and source matched count
// - Orphan count (source values not in target)
// - Reverse orphan count (target values not in source)
func (c *relationshipCandidateCollector) collectJoinStatistics(
	ctx context.Context,
	adapter datasource.SchemaDiscoverer,
	candidate *RelationshipCandidate,
) error {
	// Use the adapter's AnalyzeJoin method which handles the SQL generation
	// We pass empty schema name since our tables may be in different schemas
	// or the datasource may not use schemas
	joinAnalysis, err := adapter.AnalyzeJoin(
		ctx,
		"", candidate.SourceTable, candidate.SourceColumn,
		"", candidate.TargetTable, candidate.TargetColumn,
	)
	if err != nil {
		return fmt.Errorf("analyze join for %s.%s -> %s.%s: %w",
			candidate.SourceTable, candidate.SourceColumn,
			candidate.TargetTable, candidate.TargetColumn, err)
	}

	// Populate candidate fields from join analysis
	candidate.JoinCount = joinAnalysis.JoinCount
	candidate.SourceMatched = joinAnalysis.SourceMatched
	candidate.TargetMatched = joinAnalysis.TargetMatched
	candidate.OrphanCount = joinAnalysis.OrphanCount
	candidate.ReverseOrphans = joinAnalysis.ReverseOrphanCount

	return nil
}

// collectSampleValues uses the datasource adapter to collect sample values
// for both source and target columns of a relationship candidate.
// It populates the SourceSamples, SourceDistinctCount, TargetSamples, and
// TargetDistinctCount fields on the candidate.
func (c *relationshipCandidateCollector) collectSampleValues(
	ctx context.Context,
	adapter datasource.SchemaDiscoverer,
	candidate *RelationshipCandidate,
) error {
	const sampleLimit = 10

	// Get sample values from source column
	sourceSamples, err := adapter.GetDistinctValues(
		ctx,
		"", candidate.SourceTable, candidate.SourceColumn,
		sampleLimit,
	)
	if err != nil {
		return fmt.Errorf("get source samples for %s.%s: %w",
			candidate.SourceTable, candidate.SourceColumn, err)
	}
	candidate.SourceSamples = sourceSamples

	// Get sample values from target column
	targetSamples, err := adapter.GetDistinctValues(
		ctx,
		"", candidate.TargetTable, candidate.TargetColumn,
		sampleLimit,
	)
	if err != nil {
		return fmt.Errorf("get target samples for %s.%s: %w",
			candidate.TargetTable, candidate.TargetColumn, err)
	}
	candidate.TargetSamples = targetSamples

	return nil
}

// collectDistinctCounts collects the distinct count and null rate for source and target columns.
// This uses column statistics from the schema discoverer adapter.
func (c *relationshipCandidateCollector) collectDistinctCounts(
	ctx context.Context,
	adapter datasource.SchemaDiscoverer,
	candidate *RelationshipCandidate,
) error {
	// Analyze source column statistics
	sourceStats, err := adapter.AnalyzeColumnStats(
		ctx,
		"", candidate.SourceTable, []string{candidate.SourceColumn},
	)
	if err != nil {
		c.logger.Warn("failed to get source column stats",
			zap.String("table", candidate.SourceTable),
			zap.String("column", candidate.SourceColumn),
			zap.Error(err),
		)
	} else if len(sourceStats) > 0 {
		candidate.SourceDistinctCount = sourceStats[0].DistinctCount
		// Calculate null rate: (total - non-null) / total
		if sourceStats[0].RowCount > 0 {
			nullCount := sourceStats[0].RowCount - sourceStats[0].NonNullCount
			candidate.SourceNullRate = float64(nullCount) / float64(sourceStats[0].RowCount)
		}
	}

	// Analyze target column statistics
	targetStats, err := adapter.AnalyzeColumnStats(
		ctx,
		"", candidate.TargetTable, []string{candidate.TargetColumn},
	)
	if err != nil {
		c.logger.Warn("failed to get target column stats",
			zap.String("table", candidate.TargetTable),
			zap.String("column", candidate.TargetColumn),
			zap.Error(err),
		)
	} else if len(targetStats) > 0 {
		candidate.TargetDistinctCount = targetStats[0].DistinctCount
		// Calculate null rate: (total - non-null) / total
		if targetStats[0].RowCount > 0 {
			nullCount := targetStats[0].RowCount - targetStats[0].NonNullCount
			candidate.TargetNullRate = float64(nullCount) / float64(targetStats[0].RowCount)
		}
	}

	return nil
}

// CollectCandidates gathers all potential FK relationship candidates using deterministic criteria.
// This method orchestrates the full candidate collection process:
// 1. Get datasource and create adapter
// 2. Identify FK sources (columns that could be foreign keys)
// 3. Identify FK targets (primary keys and unique columns)
// 4. Generate candidate pairs with type compatibility checks
// 5. Collect join statistics and sample values for each candidate
//
// Error handling follows the fail-fast policy per CLAUDE.md:
// - Fatal errors (schema load, adapter creation): Return error immediately
// - Non-fatal errors (single candidate stats fail): Log warning and continue
func (c *relationshipCandidateCollector) CollectCandidates(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) ([]*RelationshipCandidate, error) {
	// Step 1: Get datasource and create adapter
	if progressCallback != nil {
		progressCallback(0, 5, "Loading schema metadata")
	}

	ds, err := c.dsSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer adapter for join analysis and sample value collection
	adapter, err := c.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Step 2: Identify FK sources
	sources, err := c.identifyFKSources(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("identify FK sources: %w", err)
	}

	if progressCallback != nil {
		progressCallback(1, 5, fmt.Sprintf("Found %d potential FK sources", len(sources)))
	}

	// Step 3: Identify FK targets (PKs and unique columns only)
	targets, err := c.identifyFKTargets(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("identify FK targets: %w", err)
	}

	if progressCallback != nil {
		progressCallback(2, 5, fmt.Sprintf("Found %d FK targets (PKs/unique)", len(targets)))
	}

	// Step 4: Generate candidate pairs with type compatibility
	candidates := c.generateCandidatePairs(sources, targets)

	if progressCallback != nil {
		progressCallback(3, 5, fmt.Sprintf("Generated %d candidate pairs", len(candidates)))
	}

	// Step 5: Collect join statistics for each candidate and filter aggressively
	// A valid FK relationship requires:
	// - At least one source value matches a target value (SourceMatched > 0)
	// - Zero orphans (all source values must exist in target for referential integrity)
	var validCandidates []*RelationshipCandidate
	var rejectedNoMatch, rejectedOrphans, rejectedError int

	for i, candidate := range candidates {
		// Collect join statistics (join count, orphans, etc.)
		if err := c.collectJoinStatistics(ctx, adapter, candidate); err != nil {
			c.logger.Debug("failed to collect join stats, rejecting candidate",
				zap.String("source", candidate.SourceTable+"."+candidate.SourceColumn),
				zap.String("target", candidate.TargetTable+"."+candidate.TargetColumn),
				zap.Error(err),
			)
			rejectedError++
			continue
		}

		// Filter: Reject if no source values match target (not a relationship)
		if candidate.SourceMatched == 0 {
			rejectedNoMatch++
			continue
		}

		// Filter: Reject if any orphans exist (violates referential integrity)
		if candidate.OrphanCount > 0 {
			rejectedOrphans++
			continue
		}

		// This candidate passed join analysis - collect additional data for LLM
		// Collect sample values for source and target columns
		if err := c.collectSampleValues(ctx, adapter, candidate); err != nil {
			c.logger.Warn("failed to collect sample values, continuing",
				zap.String("source", candidate.SourceTable+"."+candidate.SourceColumn),
				zap.String("target", candidate.TargetTable+"."+candidate.TargetColumn),
				zap.Error(err),
			)
			// Continue - missing samples is not fatal
		}

		// Collect distinct counts and null rates
		if err := c.collectDistinctCounts(ctx, adapter, candidate); err != nil {
			c.logger.Warn("failed to collect distinct counts, continuing",
				zap.String("source", candidate.SourceTable+"."+candidate.SourceColumn),
				zap.String("target", candidate.TargetTable+"."+candidate.TargetColumn),
				zap.Error(err),
			)
			// Continue - missing stats is not fatal
		}

		validCandidates = append(validCandidates, candidate)

		// Report progress for large candidate sets (every 100 candidates)
		if len(candidates) > 100 && i%100 == 0 && progressCallback != nil {
			progressCallback(4, 5, fmt.Sprintf("Analyzing candidates: %d/%d", i, len(candidates)))
		}
	}

	if progressCallback != nil {
		progressCallback(5, 5, fmt.Sprintf("Found %d valid candidates", len(validCandidates)))
	}

	c.logger.Info("candidate collection complete",
		zap.Int("sources", len(sources)),
		zap.Int("targets", len(targets)),
		zap.Int("initial_candidates", len(candidates)),
		zap.Int("valid_candidates", len(validCandidates)),
		zap.Int("rejected_no_match", rejectedNoMatch),
		zap.Int("rejected_orphans", rejectedOrphans),
		zap.Int("rejected_error", rejectedError),
		zap.String("project_id", projectID.String()),
	)

	return validCandidates, nil
}
