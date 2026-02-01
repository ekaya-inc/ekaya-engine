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
	logger         *zap.Logger
}

// NewRelationshipCandidateCollector creates a new RelationshipCandidateCollector.
func NewRelationshipCandidateCollector(
	schemaRepo repositories.SchemaRepository,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) RelationshipCandidateCollector {
	return &relationshipCandidateCollector{
		schemaRepo:     schemaRepo,
		adapterFactory: adapterFactory,
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

// CollectCandidates gathers all potential FK relationship candidates using deterministic criteria.
// This method orchestrates the full candidate collection process:
// 1. Identify FK sources (columns that could be foreign keys)
// 2. Identify FK targets (primary keys and unique columns)
// 3. Generate candidate pairs with type compatibility checks
// 4. Collect join statistics and sample values for each candidate
//
// This implementation is a stub - full implementation is in Task 2.5.
func (c *relationshipCandidateCollector) CollectCandidates(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	progressCallback dag.ProgressCallback,
) ([]*RelationshipCandidate, error) {
	// Step 1: Identify FK sources
	if progressCallback != nil {
		progressCallback(0, 5, "Loading schema metadata")
	}

	sources, err := c.identifyFKSources(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("identify FK sources: %w", err)
	}

	if progressCallback != nil {
		progressCallback(1, 5, fmt.Sprintf("Found %d potential FK sources", len(sources)))
	}

	// TODO: Task 2.2 - Identify FK targets (PKs and unique columns)
	// TODO: Task 2.3 - Generate candidate pairs with type compatibility
	// TODO: Task 2.4 - Collect join statistics for each candidate
	// TODO: Task 2.5 - Wire together and complete this method

	c.logger.Info("candidate collection complete (stub)",
		zap.Int("sources", len(sources)),
		zap.String("project_id", projectID.String()),
	)

	return nil, nil
}
