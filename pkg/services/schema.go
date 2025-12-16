package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// SchemaService orchestrates schema operations between adapters and repositories.
type SchemaService interface {
	// RefreshDatasourceSchema syncs tables, columns, and FK relationships from the datasource.
	// Creates SchemaDiscoverer internally, caller does not manage adapter lifecycle.
	RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RefreshResult, error)

	// GetDatasourceSchema returns the complete schema for a datasource.
	GetDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error)

	// GetDatasourceTable returns a single table with its columns.
	GetDatasourceTable(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.DatasourceTable, error)
}

type schemaService struct {
	schemaRepo     repositories.SchemaRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	logger         *zap.Logger
}

// NewSchemaService creates a new schema service with dependencies.
func NewSchemaService(
	schemaRepo repositories.SchemaRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) SchemaService {
	return &schemaService{
		schemaRepo:     schemaRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		logger:         logger,
	}
}

// RefreshDatasourceSchema syncs the schema from the datasource into our repository.
func (s *schemaService) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RefreshResult, error) {
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

	result := &models.RefreshResult{}

	// Discover and sync tables
	discoveredTables, err := discoverer.DiscoverTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover tables: %w", err)
	}

	// Build active table keys for soft-delete cleanup
	activeTableKeys := make([]repositories.TableKey, len(discoveredTables))

	// Upsert each discovered table
	for i, dt := range discoveredTables {
		activeTableKeys[i] = repositories.TableKey{
			SchemaName: dt.SchemaName,
			TableName:  dt.TableName,
		}

		table := &models.SchemaTable{
			ProjectID:    projectID,
			DatasourceID: datasourceID,
			SchemaName:   dt.SchemaName,
			TableName:    dt.TableName,
			RowCount:     &dt.RowCount,
		}

		if err := s.schemaRepo.UpsertTable(ctx, table); err != nil {
			return nil, fmt.Errorf("failed to upsert table %s.%s: %w", dt.SchemaName, dt.TableName, err)
		}
		result.TablesUpserted++

		// Discover and sync columns for this table
		columnsUpserted, columnsDeleted, err := s.syncColumnsForTable(ctx, discoverer, projectID, table)
		if err != nil {
			return nil, fmt.Errorf("failed to sync columns for table %s.%s: %w", dt.SchemaName, dt.TableName, err)
		}
		result.ColumnsUpserted += columnsUpserted
		result.ColumnsDeleted += columnsDeleted
	}

	// Soft-delete tables no longer in datasource
	tablesDeleted, err := s.schemaRepo.SoftDeleteRemovedTables(ctx, projectID, datasourceID, activeTableKeys)
	if err != nil {
		return nil, fmt.Errorf("failed to soft-delete removed tables: %w", err)
	}
	result.TablesDeleted = tablesDeleted

	// Sync foreign key relationships if supported
	if discoverer.SupportsForeignKeys() {
		relationshipsCreated, err := s.syncForeignKeys(ctx, discoverer, projectID, datasourceID)
		if err != nil {
			return nil, fmt.Errorf("failed to sync foreign keys: %w", err)
		}
		result.RelationshipsCreated = relationshipsCreated
	}

	// Soft-delete orphaned relationships (where source or target column was deleted)
	relationshipsDeleted, err := s.schemaRepo.SoftDeleteOrphanedRelationships(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to soft-delete orphaned relationships: %w", err)
	}
	result.RelationshipsDeleted = relationshipsDeleted

	s.logger.Info("Schema refresh completed",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()),
		zap.Int("tables_upserted", result.TablesUpserted),
		zap.Int64("tables_deleted", result.TablesDeleted),
		zap.Int("columns_upserted", result.ColumnsUpserted),
		zap.Int64("columns_deleted", result.ColumnsDeleted),
		zap.Int("relationships_created", result.RelationshipsCreated),
		zap.Int64("relationships_deleted", result.RelationshipsDeleted),
	)

	return result, nil
}

// syncColumnsForTable discovers and syncs columns for a single table.
func (s *schemaService) syncColumnsForTable(
	ctx context.Context,
	discoverer datasource.SchemaDiscoverer,
	projectID uuid.UUID,
	table *models.SchemaTable,
) (int, int64, error) {
	discoveredColumns, err := discoverer.DiscoverColumns(ctx, table.SchemaName, table.TableName)
	if err != nil {
		return 0, 0, fmt.Errorf("discover columns: %w", err)
	}

	activeColumnNames := make([]string, len(discoveredColumns))
	columnsUpserted := 0

	for i, dc := range discoveredColumns {
		activeColumnNames[i] = dc.ColumnName

		column := &models.SchemaColumn{
			ProjectID:       projectID,
			SchemaTableID:   table.ID,
			ColumnName:      dc.ColumnName,
			DataType:        dc.DataType,
			IsNullable:      dc.IsNullable,
			IsPrimaryKey:    dc.IsPrimaryKey,
			OrdinalPosition: dc.OrdinalPosition,
		}

		if err := s.schemaRepo.UpsertColumn(ctx, column); err != nil {
			return 0, 0, fmt.Errorf("upsert column %s: %w", dc.ColumnName, err)
		}
		columnsUpserted++
	}

	// Soft-delete columns no longer in table
	columnsDeleted, err := s.schemaRepo.SoftDeleteRemovedColumns(ctx, table.ID, activeColumnNames)
	if err != nil {
		return 0, 0, fmt.Errorf("soft-delete removed columns: %w", err)
	}

	return columnsUpserted, columnsDeleted, nil
}

// syncForeignKeys discovers and syncs foreign key relationships.
func (s *schemaService) syncForeignKeys(
	ctx context.Context,
	discoverer datasource.SchemaDiscoverer,
	projectID, datasourceID uuid.UUID,
) (int, error) {
	fks, err := discoverer.DiscoverForeignKeys(ctx)
	if err != nil {
		return 0, fmt.Errorf("discover foreign keys: %w", err)
	}

	relationshipsCreated := 0

	for _, fk := range fks {
		// Look up source table and column
		sourceTable, err := s.schemaRepo.GetTableByName(ctx, projectID, datasourceID, fk.SourceSchema, fk.SourceTable)
		if err != nil {
			s.logger.Warn("FK source table not found, skipping",
				zap.String("constraint", fk.ConstraintName),
				zap.String("source_table", fk.SourceSchema+"."+fk.SourceTable),
				zap.Error(err),
			)
			continue
		}

		sourceColumn, err := s.schemaRepo.GetColumnByName(ctx, sourceTable.ID, fk.SourceColumn)
		if err != nil {
			s.logger.Warn("FK source column not found, skipping",
				zap.String("constraint", fk.ConstraintName),
				zap.String("source_column", fk.SourceColumn),
				zap.Error(err),
			)
			continue
		}

		// Look up target table and column
		targetTable, err := s.schemaRepo.GetTableByName(ctx, projectID, datasourceID, fk.TargetSchema, fk.TargetTable)
		if err != nil {
			s.logger.Warn("FK target table not found, skipping",
				zap.String("constraint", fk.ConstraintName),
				zap.String("target_table", fk.TargetSchema+"."+fk.TargetTable),
				zap.Error(err),
			)
			continue
		}

		targetColumn, err := s.schemaRepo.GetColumnByName(ctx, targetTable.ID, fk.TargetColumn)
		if err != nil {
			s.logger.Warn("FK target column not found, skipping",
				zap.String("constraint", fk.ConstraintName),
				zap.String("target_column", fk.TargetColumn),
				zap.Error(err),
			)
			continue
		}

		// Create/update relationship
		fkType := models.RelationshipTypeFK
		rel := &models.SchemaRelationship{
			ProjectID:        projectID,
			SourceTableID:    sourceTable.ID,
			SourceColumnID:   sourceColumn.ID,
			TargetTableID:    targetTable.ID,
			TargetColumnID:   targetColumn.ID,
			RelationshipType: fkType,
			Cardinality:      models.CardinalityNTo1, // FK typically represents N:1
			Confidence:       1.0,                    // FK constraints have 100% confidence
			InferenceMethod:  &fkType,
		}

		if err := s.schemaRepo.UpsertRelationship(ctx, rel); err != nil {
			return relationshipsCreated, fmt.Errorf("upsert relationship for FK %s: %w", fk.ConstraintName, err)
		}
		relationshipsCreated++
	}

	return relationshipsCreated, nil
}

// GetDatasourceSchema returns the complete schema for a datasource.
func (s *schemaService) GetDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	// Get all tables
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}

	// Get all columns for the datasource (avoids N+1 queries)
	allColumns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns: %w", err)
	}

	// Build column map by table ID
	columnsByTable := make(map[uuid.UUID][]*models.SchemaColumn)
	for _, col := range allColumns {
		columnsByTable[col.SchemaTableID] = append(columnsByTable[col.SchemaTableID], col)
	}

	// Get all relationships
	relationships, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list relationships: %w", err)
	}

	// Build table name map for relationship enrichment
	tableNameByID := make(map[uuid.UUID]string)
	for _, t := range tables {
		tableNameByID[t.ID] = t.SchemaName + "." + t.TableName
	}

	// Build column name map for relationship enrichment
	columnNameByID := make(map[uuid.UUID]string)
	for _, c := range allColumns {
		columnNameByID[c.ID] = c.ColumnName
	}

	// Convert to service-layer types
	schema := &models.DatasourceSchema{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		Tables:       make([]*models.DatasourceTable, len(tables)),
	}

	for i, t := range tables {
		dt := &models.DatasourceTable{
			ID:         t.ID,
			SchemaName: t.SchemaName,
			TableName:  t.TableName,
			IsSelected: t.IsSelected,
		}
		if t.BusinessName != nil {
			dt.BusinessName = *t.BusinessName
		}
		if t.Description != nil {
			dt.Description = *t.Description
		}
		if t.RowCount != nil {
			dt.RowCount = *t.RowCount
		}

		// Add columns
		cols := columnsByTable[t.ID]
		dt.Columns = make([]*models.DatasourceColumn, len(cols))
		for j, c := range cols {
			dc := &models.DatasourceColumn{
				ID:              c.ID,
				ColumnName:      c.ColumnName,
				DataType:        c.DataType,
				IsNullable:      c.IsNullable,
				IsPrimaryKey:    c.IsPrimaryKey,
				IsSelected:      c.IsSelected,
				OrdinalPosition: c.OrdinalPosition,
				DistinctCount:   c.DistinctCount,
				NullCount:       c.NullCount,
			}
			if c.BusinessName != nil {
				dc.BusinessName = *c.BusinessName
			}
			if c.Description != nil {
				dc.Description = *c.Description
			}
			dt.Columns[j] = dc
		}

		schema.Tables[i] = dt
	}

	// Convert relationships
	schema.Relationships = make([]*models.DatasourceRelationship, len(relationships))
	for i, r := range relationships {
		dr := &models.DatasourceRelationship{
			ID:               r.ID,
			SourceTableID:    r.SourceTableID,
			SourceTableName:  tableNameByID[r.SourceTableID],
			SourceColumnID:   r.SourceColumnID,
			SourceColumnName: columnNameByID[r.SourceColumnID],
			TargetTableID:    r.TargetTableID,
			TargetTableName:  tableNameByID[r.TargetTableID],
			TargetColumnID:   r.TargetColumnID,
			TargetColumnName: columnNameByID[r.TargetColumnID],
			RelationshipType: r.RelationshipType,
			Cardinality:      r.Cardinality,
			Confidence:       r.Confidence,
			IsApproved:       r.IsApproved,
		}
		schema.Relationships[i] = dr
	}

	return schema, nil
}

// GetDatasourceTable returns a single table with its columns.
func (s *schemaService) GetDatasourceTable(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.DatasourceTable, error) {
	// Parse schema.table format if provided, otherwise assume public schema
	schemaName := "public"
	tblName := tableName
	for i := 0; i < len(tableName); i++ {
		if tableName[i] == '.' {
			schemaName = tableName[:i]
			tblName = tableName[i+1:]
			break
		}
	}

	// Get table
	table, err := s.schemaRepo.GetTableByName(ctx, projectID, datasourceID, schemaName, tblName)
	if err != nil {
		return nil, fmt.Errorf("table not found: %w", err)
	}

	// Get columns
	columns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns: %w", err)
	}

	// Convert to service-layer type
	dt := &models.DatasourceTable{
		ID:         table.ID,
		SchemaName: table.SchemaName,
		TableName:  table.TableName,
		IsSelected: table.IsSelected,
	}
	if table.BusinessName != nil {
		dt.BusinessName = *table.BusinessName
	}
	if table.Description != nil {
		dt.Description = *table.Description
	}
	if table.RowCount != nil {
		dt.RowCount = *table.RowCount
	}

	dt.Columns = make([]*models.DatasourceColumn, len(columns))
	for i, c := range columns {
		dc := &models.DatasourceColumn{
			ID:              c.ID,
			ColumnName:      c.ColumnName,
			DataType:        c.DataType,
			IsNullable:      c.IsNullable,
			IsPrimaryKey:    c.IsPrimaryKey,
			IsSelected:      c.IsSelected,
			OrdinalPosition: c.OrdinalPosition,
			DistinctCount:   c.DistinctCount,
			NullCount:       c.NullCount,
		}
		if c.BusinessName != nil {
			dc.BusinessName = *c.BusinessName
		}
		if c.Description != nil {
			dc.Description = *c.Description
		}
		dt.Columns[i] = dc
	}

	return dt, nil
}

// Ensure schemaService implements SchemaService at compile time.
var _ SchemaService = (*schemaService)(nil)
