package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jinzhu/inflection"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// DataChangeDetectionService detects data-level changes that affect ontology accuracy.
// This includes new enum values, cardinality shifts, and potential FK patterns.
type DataChangeDetectionService interface {
	// ScanForChanges performs a full scan of all selected columns for data changes.
	// Returns the pending changes that were created.
	ScanForChanges(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.PendingChange, error)

	// ScanTables scans specific tables for data changes (for targeted refresh).
	ScanTables(ctx context.Context, projectID, datasourceID uuid.UUID, tableNames []string) ([]*models.PendingChange, error)
}

// DataChangeDetectionConfig configures the data change detection behavior.
type DataChangeDetectionConfig struct {
	// MaxDistinctValuesForEnum is the maximum number of distinct values for a column to be considered an enum.
	// Columns with more distinct values are not checked for enum changes.
	MaxDistinctValuesForEnum int

	// MaxEnumValueLength is the maximum string length for a value to be considered part of an enum.
	MaxEnumValueLength int

	// MinMatchRateForFK is the minimum match rate (0.0-1.0) for a column to be considered a potential FK.
	MinMatchRateForFK float64
}

// DefaultDataChangeDetectionConfig returns sensible defaults for data change detection.
func DefaultDataChangeDetectionConfig() DataChangeDetectionConfig {
	return DataChangeDetectionConfig{
		MaxDistinctValuesForEnum: 100,
		MaxEnumValueLength:       100,
		MinMatchRateForFK:        0.9,
	}
}

type dataChangeDetectionService struct {
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	pendingChangeRepo  repositories.PendingChangeRepository
	datasourceService  DatasourceService
	projectService     ProjectService
	adapterFactory     datasource.DatasourceAdapterFactory
	config             DataChangeDetectionConfig
	logger             *zap.Logger
}

// NewDataChangeDetectionService creates a new DataChangeDetectionService.
func NewDataChangeDetectionService(
	schemaRepo repositories.SchemaRepository,
	columnMetadataRepo repositories.ColumnMetadataRepository,
	pendingChangeRepo repositories.PendingChangeRepository,
	datasourceService DatasourceService,
	projectService ProjectService,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) DataChangeDetectionService {
	return &dataChangeDetectionService{
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		pendingChangeRepo:  pendingChangeRepo,
		datasourceService:  datasourceService,
		projectService:     projectService,
		adapterFactory:     adapterFactory,
		config:             DefaultDataChangeDetectionConfig(),
		logger:             logger,
	}
}

var _ DataChangeDetectionService = (*dataChangeDetectionService)(nil)

// ScanForChanges performs a full scan of all selected columns for data changes.
func (s *dataChangeDetectionService) ScanForChanges(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
) ([]*models.PendingChange, error) {
	// Get all selected tables for the datasource
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	// Extract table names
	tableNames := make([]string, 0, len(tables))
	for _, t := range tables {
		if t.IsSelected {
			tableNames = append(tableNames, t.TableName)
		}
	}

	return s.ScanTables(ctx, projectID, datasourceID, tableNames)
}

// ScanTables scans specific tables for data changes.
func (s *dataChangeDetectionService) ScanTables(
	ctx context.Context,
	projectID, datasourceID uuid.UUID,
	tableNames []string,
) ([]*models.PendingChange, error) {
	if len(tableNames) == 0 {
		return nil, nil
	}

	// Get datasource for connection
	ds, err := s.datasourceService.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create schema discoverer for data queries
	discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("failed to create schema discoverer: %w", err)
	}
	defer discoverer.Close()

	// Get tables for schema info
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	// Build table map for quick lookup
	tableMap := make(map[string]*models.SchemaTable)
	for _, t := range tables {
		tableMap[t.TableName] = t
	}

	var changes []*models.PendingChange

	// Process each table
	for _, tableName := range tableNames {
		table, ok := tableMap[tableName]
		if !ok || !table.IsSelected {
			continue
		}

		// Get columns for the table
		columns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID)
		if err != nil {
			s.logger.Warn("Failed to list columns for table",
				zap.String("table", tableName),
				zap.Error(err))
			continue
		}

		// Fetch column metadata for this table to get existing enum values
		columnIDs := make([]uuid.UUID, 0, len(columns))
		for _, col := range columns {
			if col.IsSelected {
				columnIDs = append(columnIDs, col.ID)
			}
		}
		metadataList, err := s.columnMetadataRepo.GetBySchemaColumnIDs(ctx, columnIDs)
		if err != nil {
			s.logger.Warn("Failed to fetch column metadata for enum check",
				zap.String("table", tableName),
				zap.Error(err))
		}
		metadataByColID := make(map[uuid.UUID]*models.ColumnMetadata)
		for _, meta := range metadataList {
			metadataByColID[meta.SchemaColumnID] = meta
		}

		// Check each column for enum changes
		for _, col := range columns {
			if !col.IsSelected {
				continue
			}

			// Get existing enum values from column metadata
			var existingEnumValues []string
			if meta, ok := metadataByColID[col.ID]; ok {
				if enumFeatures := meta.GetEnumFeatures(); enumFeatures != nil && len(enumFeatures.Values) > 0 {
					for _, ev := range enumFeatures.Values {
						existingEnumValues = append(existingEnumValues, ev.Value)
					}
				}
			}

			// Detect enum value changes
			enumChange, err := s.detectEnumChanges(ctx, discoverer, table, col, existingEnumValues)
			if err != nil {
				s.logger.Warn("Failed to detect enum changes",
					zap.String("table", tableName),
					zap.String("column", col.ColumnName),
					zap.Error(err))
				continue
			}
			if enumChange != nil {
				enumChange.ProjectID = projectID
				changes = append(changes, enumChange)
			}
		}

		// Detect potential FK patterns for non-FK columns
		// Reuse metadataByColID built earlier for enum detection
		fkChanges, err := s.detectPotentialFKs(ctx, discoverer, table, columns, tables, metadataByColID)
		if err != nil {
			s.logger.Warn("Failed to detect FK patterns",
				zap.String("table", tableName),
				zap.Error(err))
		} else {
			for _, c := range fkChanges {
				c.ProjectID = projectID
				changes = append(changes, c)
			}
		}
	}

	// Persist changes if any
	if len(changes) > 0 {
		if err := s.pendingChangeRepo.CreateBatch(ctx, changes); err != nil {
			return nil, fmt.Errorf("failed to persist pending changes: %w", err)
		}

		s.logger.Info("Created pending changes from data scan",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Int("total_changes", len(changes)),
		)
	}

	return changes, nil
}

// detectEnumChanges checks for new enum values or potential enum columns.
func (s *dataChangeDetectionService) detectEnumChanges(
	ctx context.Context,
	discoverer datasource.SchemaDiscoverer,
	table *models.SchemaTable,
	col *models.SchemaColumn,
	existingEnumValues []string,
) (*models.PendingChange, error) {
	// Skip non-string columns for enum detection
	dataTypeLower := strings.ToLower(col.DataType)
	if !isStringType(dataTypeLower) {
		return nil, nil
	}

	// Get distinct values from the database
	values, err := discoverer.GetDistinctValues(ctx, table.SchemaName, table.TableName, col.ColumnName, s.config.MaxDistinctValuesForEnum+1)
	if err != nil {
		return nil, fmt.Errorf("failed to get distinct values: %w", err)
	}

	// If more than max distinct values, not an enum
	if len(values) > s.config.MaxDistinctValuesForEnum {
		return nil, nil
	}

	// Check max value length
	for _, v := range values {
		if len(v) > s.config.MaxEnumValueLength {
			return nil, nil
		}
	}

	// If column already has enum values, check for new ones
	if len(existingEnumValues) > 0 {
		existingSet := make(map[string]bool)
		for _, v := range existingEnumValues {
			existingSet[v] = true
		}

		var newValues []string
		for _, v := range values {
			if !existingSet[v] {
				newValues = append(newValues, v)
			}
		}

		if len(newValues) > 0 {
			return &models.PendingChange{
				ChangeType:      models.ChangeTypeNewEnumValue,
				ChangeSource:    models.ChangeSourceDataScan,
				TableName:       table.TableName,
				ColumnName:      col.ColumnName,
				OldValue:        map[string]any{"enum_values": existingEnumValues},
				NewValue:        map[string]any{"new_values": newValues, "all_values": values},
				SuggestedAction: models.SuggestedActionUpdateColumnMetadata,
				SuggestedPayload: map[string]any{
					"table":       table.TableName,
					"column":      col.ColumnName,
					"enum_values": values,
				},
				Status: models.ChangeStatusPending,
			}, nil
		}
		return nil, nil
	}

	// Column doesn't have enum values yet - check if it looks like an enum
	// Only flag columns with reasonable number of distinct values (e.g., 2-50)
	if len(values) >= 2 && len(values) <= 50 {
		return &models.PendingChange{
			ChangeType:      models.ChangeTypeNewEnumValue,
			ChangeSource:    models.ChangeSourceDataScan,
			TableName:       table.TableName,
			ColumnName:      col.ColumnName,
			NewValue:        map[string]any{"distinct_count": len(values), "sample_values": values},
			SuggestedAction: models.SuggestedActionUpdateColumnMetadata,
			SuggestedPayload: map[string]any{
				"table":       table.TableName,
				"column":      col.ColumnName,
				"enum_values": values,
			},
			Status: models.ChangeStatusPending,
		}, nil
	}

	return nil, nil
}

// detectPotentialFKs looks for columns that look like FKs but aren't declared as such.
// Uses stored ColumnMetadata.Purpose to identify identifier columns as FK candidates.
func (s *dataChangeDetectionService) detectPotentialFKs(
	ctx context.Context,
	discoverer datasource.SchemaDiscoverer,
	table *models.SchemaTable,
	columns []*models.SchemaColumn,
	allTables []*models.SchemaTable,
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata,
) ([]*models.PendingChange, error) {
	var changes []*models.PendingChange

	// Build a map of table names to their info
	tableByName := make(map[string]*models.SchemaTable)
	for _, t := range allTables {
		tableByName[t.TableName] = t
		// Also map singular form for FK lookups (e.g., "categories" -> "category")
		singular := inflection.Singular(t.TableName)
		if singular != t.TableName {
			tableByName[singular] = t
		}
	}

	for _, col := range columns {
		// Skip if already an FK or PK
		if col.IsPrimaryKey {
			continue
		}

		// Check column metadata - only check columns classified as identifiers
		meta := metadataByColumnID[col.ID]
		if meta != nil && meta.Purpose != nil && *meta.Purpose != models.PurposeIdentifier {
			continue
		}

		// Also check columns ending in _id as a fallback (for columns without metadata)
		if meta == nil && !strings.HasSuffix(col.ColumnName, "_id") {
			continue
		}

		// Derive target table: prefer metadata FK target, fall back to name heuristic for new columns
		var targetTable *models.SchemaTable
		if meta != nil {
			if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.FKTargetTable != "" {
				targetTableKey := idFeatures.FKTargetTable
				// FKTargetTable may be schema-qualified; strip schema prefix for name-only lookup
				if strings.Contains(targetTableKey, ".") {
					parts := strings.SplitN(targetTableKey, ".", 2)
					targetTableKey = parts[1]
				}
				targetTable = tableByName[targetTableKey]
			}
		}
		if targetTable == nil {
			// Fallback for columns without metadata (newly added columns)
			var potentialTableBase string
			if strings.HasSuffix(col.ColumnName, "_id") {
				potentialTableBase = strings.TrimSuffix(col.ColumnName, "_id")
			} else {
				potentialTableBase = col.ColumnName
			}
			targetTable = tableByName[potentialTableBase]
			if targetTable == nil {
				targetTable = tableByName[potentialTableBase+"s"]
			}
		}
		if targetTable == nil {
			continue
		}

		// Don't create self-referential FK suggestions without explicit structure
		if targetTable.TableName == table.TableName {
			continue
		}

		// Find target column: prefer metadata FKTargetColumn, fall back to PK scan
		targetColumns, err := s.schemaRepo.ListColumnsByTable(ctx, targetTable.ProjectID, targetTable.ID)
		if err != nil {
			continue
		}

		var targetPKColumn *models.SchemaColumn
		if meta != nil {
			if idFeatures := meta.GetIdentifierFeatures(); idFeatures != nil && idFeatures.FKTargetColumn != "" {
				for _, tc := range targetColumns {
					if tc.ColumnName == idFeatures.FKTargetColumn {
						targetPKColumn = tc
						break
					}
				}
			}
		}
		if targetPKColumn == nil {
			for _, tc := range targetColumns {
				if tc.IsPrimaryKey {
					targetPKColumn = tc
					break
				}
			}
		}
		if targetPKColumn == nil {
			// Try finding an 'id' column
			for _, tc := range targetColumns {
				if tc.ColumnName == "id" {
					targetPKColumn = tc
					break
				}
			}
		}
		if targetPKColumn == nil {
			continue
		}

		// Check if values actually match target table's PK
		overlap, err := discoverer.CheckValueOverlap(
			ctx,
			table.SchemaName, table.TableName, col.ColumnName,
			targetTable.SchemaName, targetTable.TableName, targetPKColumn.ColumnName,
			1000,
		)
		if err != nil {
			s.logger.Debug("Failed to check value overlap",
				zap.String("source", fmt.Sprintf("%s.%s", table.TableName, col.ColumnName)),
				zap.String("target", fmt.Sprintf("%s.%s", targetTable.TableName, targetPKColumn.ColumnName)),
				zap.Error(err))
			continue
		}

		// If high match rate, suggest FK relationship
		if overlap.MatchRate >= s.config.MinMatchRateForFK {
			changes = append(changes, &models.PendingChange{
				ChangeType:   models.ChangeTypeNewFKPattern,
				ChangeSource: models.ChangeSourceDataScan,
				TableName:    table.TableName,
				ColumnName:   col.ColumnName,
				NewValue: map[string]any{
					"target_table":    targetTable.TableName,
					"target_column":   targetPKColumn.ColumnName,
					"match_rate":      overlap.MatchRate,
					"matched_count":   overlap.MatchedCount,
					"source_distinct": overlap.SourceDistinct,
				},
				SuggestedAction: models.SuggestedActionCreateRelationship,
				SuggestedPayload: map[string]any{
					"from_table":  table.TableName,
					"from_column": col.ColumnName,
					"to_table":    targetTable.TableName,
					"to_column":   targetPKColumn.ColumnName,
				},
				Status: models.ChangeStatusPending,
			})
		}
	}

	return changes, nil
}

// isStringType returns true if the data type is a string-like type.
func isStringType(dataType string) bool {
	stringTypes := []string{
		"text", "varchar", "character varying", "char", "character",
		"nvarchar", "nchar", "ntext", "string",
	}
	for _, st := range stringTypes {
		if strings.Contains(dataType, st) {
			return true
		}
	}
	return false
}
