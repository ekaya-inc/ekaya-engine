package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// SchemaService orchestrates schema operations between adapters and repositories.
type SchemaService interface {
	// RefreshDatasourceSchema syncs tables, columns, and FK relationships from the datasource.
	// Creates SchemaDiscoverer internally, caller does not manage adapter lifecycle.
	// If autoSelect is true, newly created tables and columns will have IsSelected set to true.
	RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID, autoSelect bool) (*models.RefreshResult, error)

	// GetDatasourceSchema returns the complete schema for a datasource.
	GetDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error)

	// GetDatasourceTable returns a single table with its columns.
	GetDatasourceTable(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.DatasourceTable, error)

	// AddManualRelationship creates a user-defined relationship between two columns.
	AddManualRelationship(ctx context.Context, projectID, datasourceID uuid.UUID, req *models.AddRelationshipRequest) (*models.SchemaRelationship, error)

	// RemoveRelationship marks a relationship as removed (is_approved=false).
	// The relationship remains in the database to prevent re-inference.
	RemoveRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error

	// GetRelationshipsForDatasource returns all relationships for a datasource.
	GetRelationshipsForDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error)

	// GetRelationshipsResponse returns enriched relationships with table/column details and empty/orphan tables.
	GetRelationshipsResponse(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RelationshipsResponse, error)

	// UpdateTableMetadata updates business_name and/or description for a table.
	UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error

	// UpdateColumnMetadata updates business_name and/or description for a column.
	UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error

	// SaveSelections updates is_selected flags for tables and columns.
	// tableSelections maps table UUIDs to selection status.
	// columnSelections maps table UUIDs to lists of selected column UUIDs.
	SaveSelections(ctx context.Context, projectID, datasourceID uuid.UUID, tableSelections map[uuid.UUID]bool, columnSelections map[uuid.UUID][]uuid.UUID) error

	// GetSelectedDatasourceSchema returns only selected tables and columns.
	GetSelectedDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error)

	// GetDatasourceSchemaForPrompt returns schema formatted for LLM context.
	GetDatasourceSchemaForPrompt(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error)

	// GetDatasourceSchemaWithEntities returns schema enriched with entity/role semantic information.
	// This includes entity names and roles for columns that represent domain entities.
	GetDatasourceSchemaWithEntities(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error)

	// SelectAllTables marks all tables and columns for this datasource as selected.
	// Used after schema refresh to auto-select newly discovered tables.
	SelectAllTables(ctx context.Context, projectID, datasourceID uuid.UUID) error
}

type schemaService struct {
	schemaRepo     repositories.SchemaRepository
	entityRepo     repositories.OntologyEntityRepository
	ontologyRepo   repositories.OntologyRepository
	entityRelRepo  repositories.EntityRelationshipRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	logger         *zap.Logger
}

// NewSchemaService creates a new schema service with dependencies.
func NewSchemaService(
	schemaRepo repositories.SchemaRepository,
	entityRepo repositories.OntologyEntityRepository,
	ontologyRepo repositories.OntologyRepository,
	entityRelRepo repositories.EntityRelationshipRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) SchemaService {
	return &schemaService{
		schemaRepo:     schemaRepo,
		entityRepo:     entityRepo,
		ontologyRepo:   ontologyRepo,
		entityRelRepo:  entityRelRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		logger:         logger,
	}
}

// tableExclusionPatterns contains compiled regex patterns for tables that should
// not be auto-selected during schema discovery (test/sample/temporary tables).
var tableExclusionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^s\d+_`),   // s1_, s2_, etc. (sample tables)
	regexp.MustCompile(`^test_`),   // test_* (test tables)
	regexp.MustCompile(`^tmp_`),    // tmp_* (temporary tables)
	regexp.MustCompile(`^temp_`),   // temp_* (temporary tables)
	regexp.MustCompile(`_test$`),   // *_test (test tables)
	regexp.MustCompile(`_backup$`), // *_backup (backup tables)
}

// shouldAutoSelect determines if a table should be auto-selected during schema discovery.
// Returns true by default, but returns false for tables matching test/sample patterns.
func shouldAutoSelect(tableName string) bool {
	lowerName := strings.ToLower(tableName)
	for _, pattern := range tableExclusionPatterns {
		if pattern.MatchString(lowerName) {
			return false
		}
	}
	return true
}

// RefreshDatasourceSchema syncs the schema from the datasource into our repository.
// If autoSelect is true, newly created tables and columns will have IsSelected set to true.
func (s *schemaService) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID, autoSelect bool) (*models.RefreshResult, error) {
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user ID not found in context: %w", err)
	}

	// Get existing tables before refresh to track what's new/removed
	existingTables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing tables: %w", err)
	}
	existingTableNames := make(map[string]bool)
	for _, t := range existingTables {
		existingTableNames[t.SchemaName+"."+t.TableName] = true
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

	result := &models.RefreshResult{
		NewTableNames:     make([]string, 0),
		RemovedTableNames: make([]string, 0),
		NewColumns:        make([]models.RefreshColumnChange, 0),
		RemovedColumns:    make([]models.RefreshColumnChange, 0),
		ModifiedColumns:   make([]models.RefreshColumnModification, 0),
	}

	// Discover and sync tables
	discoveredTables, err := discoverer.DiscoverTables(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to discover tables: %w", err)
	}

	// Build active table keys for soft-delete cleanup and track discovered table names
	activeTableKeys := make([]repositories.TableKey, len(discoveredTables))
	discoveredTableNames := make(map[string]bool)

	// Upsert each discovered table
	for i, dt := range discoveredTables {
		tableFQN := dt.SchemaName + "." + dt.TableName
		activeTableKeys[i] = repositories.TableKey{
			SchemaName: dt.SchemaName,
			TableName:  dt.TableName,
		}
		discoveredTableNames[tableFQN] = true

		// Check if this is a new table
		isNewTable := !existingTableNames[tableFQN]
		if isNewTable {
			result.NewTableNames = append(result.NewTableNames, tableFQN)
		}

		// Determine if this table should be auto-selected
		// Only auto-select new tables when autoSelect is true AND table doesn't match exclusion patterns
		tableAutoSelect := isNewTable && autoSelect && shouldAutoSelect(dt.TableName)

		table := &models.SchemaTable{
			ProjectID:    projectID,
			DatasourceID: datasourceID,
			SchemaName:   dt.SchemaName,
			TableName:    dt.TableName,
			RowCount:     &dt.RowCount,
			IsSelected:   tableAutoSelect,
		}

		if err := s.schemaRepo.UpsertTable(ctx, table); err != nil {
			return nil, fmt.Errorf("failed to upsert table %s.%s: %w", dt.SchemaName, dt.TableName, err)
		}
		result.TablesUpserted++

		// Discover and sync columns for this table
		// Auto-select columns only if the table is auto-selected
		autoSelectColumns := tableAutoSelect
		colResult, err := s.syncColumnsForTable(ctx, discoverer, projectID, table, autoSelectColumns)
		if err != nil {
			return nil, fmt.Errorf("failed to sync columns for table %s.%s: %w", dt.SchemaName, dt.TableName, err)
		}
		result.ColumnsUpserted += colResult.ColumnsUpserted
		result.ColumnsDeleted += colResult.ColumnsDeleted
		result.NewColumns = append(result.NewColumns, colResult.NewColumns...)
		result.RemovedColumns = append(result.RemovedColumns, colResult.RemovedColumns...)
		result.ModifiedColumns = append(result.ModifiedColumns, colResult.ModifiedColumns...)
	}

	// Identify removed tables (existed before but not discovered now)
	for tableFQN := range existingTableNames {
		if !discoveredTableNames[tableFQN] {
			result.RemovedTableNames = append(result.RemovedTableNames, tableFQN)
		}
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

// columnSyncResult holds detailed results from syncing columns for a table.
type columnSyncResult struct {
	ColumnsUpserted int
	ColumnsDeleted  int64
	NewColumns      []models.RefreshColumnChange
	RemovedColumns  []models.RefreshColumnChange
	ModifiedColumns []models.RefreshColumnModification
}

// syncColumnsForTable discovers and syncs columns for a single table.
// Returns detailed change information for change detection.
// If autoSelect is true, newly created columns will have IsSelected set to true.
func (s *schemaService) syncColumnsForTable(
	ctx context.Context,
	discoverer datasource.SchemaDiscoverer,
	projectID uuid.UUID,
	table *models.SchemaTable,
	autoSelect bool,
) (*columnSyncResult, error) {
	result := &columnSyncResult{
		NewColumns:      make([]models.RefreshColumnChange, 0),
		RemovedColumns:  make([]models.RefreshColumnChange, 0),
		ModifiedColumns: make([]models.RefreshColumnModification, 0),
	}

	tableFQN := table.SchemaName + "." + table.TableName

	// Get existing columns before sync to detect changes
	existingColumns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID, false)
	if err != nil {
		return nil, fmt.Errorf("list existing columns: %w", err)
	}

	// Build map of existing columns by name for comparison
	existingByName := make(map[string]*models.SchemaColumn, len(existingColumns))
	for _, col := range existingColumns {
		existingByName[col.ColumnName] = col
	}

	discoveredColumns, err := discoverer.DiscoverColumns(ctx, table.SchemaName, table.TableName)
	if err != nil {
		return nil, fmt.Errorf("discover columns: %w", err)
	}

	activeColumnNames := make([]string, len(discoveredColumns))
	discoveredByName := make(map[string]bool)

	for i, dc := range discoveredColumns {
		activeColumnNames[i] = dc.ColumnName
		discoveredByName[dc.ColumnName] = true

		// Check if this is a new column or type change
		existing, found := existingByName[dc.ColumnName]
		isNewColumn := !found

		column := &models.SchemaColumn{
			ProjectID:       projectID,
			SchemaTableID:   table.ID,
			ColumnName:      dc.ColumnName,
			DataType:        dc.DataType,
			IsNullable:      dc.IsNullable,
			IsPrimaryKey:    dc.IsPrimaryKey,
			IsUnique:        dc.IsUnique,
			OrdinalPosition: dc.OrdinalPosition,
			DefaultValue:    dc.DefaultValue,
			IsSelected:      isNewColumn && autoSelect, // Auto-select new columns if requested
		}

		if isNewColumn {
			// New column
			result.NewColumns = append(result.NewColumns, models.RefreshColumnChange{
				TableName:  tableFQN,
				ColumnName: dc.ColumnName,
				DataType:   dc.DataType,
			})
		} else if existing.DataType != dc.DataType {
			// Type changed
			result.ModifiedColumns = append(result.ModifiedColumns, models.RefreshColumnModification{
				TableName:  tableFQN,
				ColumnName: dc.ColumnName,
				OldType:    existing.DataType,
				NewType:    dc.DataType,
			})
		}

		if err := s.schemaRepo.UpsertColumn(ctx, column); err != nil {
			return nil, fmt.Errorf("upsert column %s: %w", dc.ColumnName, err)
		}
		result.ColumnsUpserted++
	}

	// Identify removed columns (existed before but not discovered now)
	for colName, col := range existingByName {
		if !discoveredByName[colName] {
			result.RemovedColumns = append(result.RemovedColumns, models.RefreshColumnChange{
				TableName:  tableFQN,
				ColumnName: colName,
				DataType:   col.DataType,
			})
		}
	}

	// Soft-delete columns no longer in table
	columnsDeleted, err := s.schemaRepo.SoftDeleteRemovedColumns(ctx, table.ID, activeColumnNames)
	if err != nil {
		return nil, fmt.Errorf("soft-delete removed columns: %w", err)
	}
	result.ColumnsDeleted = columnsDeleted

	return result, nil
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
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID, false)
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
				IsUnique:        c.IsUnique,
				IsSelected:      c.IsSelected,
				OrdinalPosition: c.OrdinalPosition,
				DefaultValue:    c.DefaultValue,
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
	columns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID, false)
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
			IsUnique:        c.IsUnique,
			IsSelected:      c.IsSelected,
			OrdinalPosition: c.OrdinalPosition,
			DefaultValue:    c.DefaultValue,
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

// parseTableName splits "schema.table" into schema and table name.
// If no dot is present, defaults to "public" schema.
func parseTableName(tableName string) (schemaName, tblName string) {
	schemaName = "public"
	tblName = tableName
	for i := 0; i < len(tableName); i++ {
		if tableName[i] == '.' {
			schemaName = tableName[:i]
			tblName = tableName[i+1:]
			break
		}
	}
	return
}

// AddManualRelationship creates a user-defined relationship between two columns.
func (s *schemaService) AddManualRelationship(ctx context.Context, projectID, datasourceID uuid.UUID, req *models.AddRelationshipRequest) (*models.SchemaRelationship, error) {
	// Validate request
	if req.SourceTableName == "" || req.SourceColumnName == "" {
		return nil, fmt.Errorf("source table and column are required")
	}
	if req.TargetTableName == "" || req.TargetColumnName == "" {
		return nil, fmt.Errorf("target table and column are required")
	}

	// Parse and validate source table
	sourceSchema, sourceTableName := parseTableName(req.SourceTableName)
	sourceTable, err := s.schemaRepo.GetTableByName(ctx, projectID, datasourceID, sourceSchema, sourceTableName)
	if err != nil {
		return nil, fmt.Errorf("source table not found: %w", err)
	}

	// Validate source column
	sourceColumn, err := s.schemaRepo.GetColumnByName(ctx, sourceTable.ID, req.SourceColumnName)
	if err != nil {
		return nil, fmt.Errorf("source column not found: %w", err)
	}

	// Parse and validate target table
	targetSchema, targetTableName := parseTableName(req.TargetTableName)
	targetTable, err := s.schemaRepo.GetTableByName(ctx, projectID, datasourceID, targetSchema, targetTableName)
	if err != nil {
		return nil, fmt.Errorf("target table not found: %w", err)
	}

	// Validate target column
	targetColumn, err := s.schemaRepo.GetColumnByName(ctx, targetTable.ID, req.TargetColumnName)
	if err != nil {
		return nil, fmt.Errorf("target column not found: %w", err)
	}

	// Check if relationship already exists
	existing, err := s.schemaRepo.GetRelationshipByColumns(ctx, sourceColumn.ID, targetColumn.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing relationship: %w", err)
	}
	if existing != nil {
		return nil, apperrors.ErrConflict
	}

	// Create relationship
	isApproved := true
	manualType := models.RelationshipTypeManual
	rel := &models.SchemaRelationship{
		ProjectID:        projectID,
		SourceTableID:    sourceTable.ID,
		SourceColumnID:   sourceColumn.ID,
		TargetTableID:    targetTable.ID,
		TargetColumnID:   targetColumn.ID,
		RelationshipType: manualType,
		Cardinality:      models.CardinalityUnknown,
		Confidence:       1.0,
		InferenceMethod:  &manualType,
		IsApproved:       &isApproved,
	}

	if err := s.schemaRepo.UpsertRelationship(ctx, rel); err != nil {
		return nil, fmt.Errorf("failed to create relationship: %w", err)
	}

	s.logger.Info("Created manual relationship",
		zap.String("project_id", projectID.String()),
		zap.String("relationship_id", rel.ID.String()),
		zap.String("source", req.SourceTableName+"."+req.SourceColumnName),
		zap.String("target", req.TargetTableName+"."+req.TargetColumnName),
	)

	// If ontology exists, also create entity relationship for immediate visibility
	if s.ontologyRepo != nil && s.entityRelRepo != nil && s.entityRepo != nil {
		s.createEntityRelationshipForManual(ctx, projectID, sourceTable, sourceColumn, targetTable, targetColumn)
	}

	return rel, nil
}

// createEntityRelationshipForManual creates an entity relationship for a manual schema relationship.
// This is best-effort - if ontology or entities don't exist, it silently skips.
func (s *schemaService) createEntityRelationshipForManual(
	ctx context.Context,
	projectID uuid.UUID,
	sourceTable *models.SchemaTable,
	sourceColumn *models.SchemaColumn,
	targetTable *models.SchemaTable,
	targetColumn *models.SchemaColumn,
) {
	// Get active ontology
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil || ontology == nil {
		s.logger.Debug("No active ontology for manual relationship, will be created on next extraction",
			zap.String("project_id", projectID.String()))
		return
	}

	// Get entities for source and target tables
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		s.logger.Debug("Failed to get entities for manual relationship",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return
	}

	// Build entity lookup by primary table
	entityByTable := make(map[string]*models.OntologyEntity)
	for _, e := range entities {
		if e.PrimaryTable != "" {
			entityByTable[e.PrimaryTable] = e
		}
	}

	// Find source and target entities
	sourceKey := fmt.Sprintf("%s.%s", sourceTable.SchemaName, sourceTable.TableName)
	targetKey := fmt.Sprintf("%s.%s", targetTable.SchemaName, targetTable.TableName)
	sourceEntity := entityByTable[sourceKey]
	targetEntity := entityByTable[targetKey]

	if sourceEntity == nil || targetEntity == nil {
		s.logger.Debug("Entities not found for manual relationship tables",
			zap.String("source_table", sourceKey),
			zap.String("target_table", targetKey),
			zap.Bool("source_entity_found", sourceEntity != nil),
			zap.Bool("target_entity_found", targetEntity != nil))
		return
	}

	// Don't create self-referencing relationships
	if sourceEntity.ID == targetEntity.ID {
		return
	}

	// Create entity relationship
	entityRel := &models.EntityRelationship{
		OntologyID:         ontology.ID,
		SourceEntityID:     sourceEntity.ID,
		TargetEntityID:     targetEntity.ID,
		SourceColumnSchema: sourceTable.SchemaName,
		SourceColumnTable:  sourceTable.TableName,
		SourceColumnName:   sourceColumn.ColumnName,
		TargetColumnSchema: targetTable.SchemaName,
		TargetColumnTable:  targetTable.TableName,
		TargetColumnName:   targetColumn.ColumnName,
		DetectionMethod:    models.DetectionMethodManual,
		Confidence:         1.0,
		Status:             models.RelationshipStatusConfirmed,
	}

	if err := s.entityRelRepo.Create(ctx, entityRel); err != nil {
		s.logger.Debug("Failed to create entity relationship for manual relationship (may already exist)",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return
	}

	s.logger.Info("Created entity relationship for manual relationship",
		zap.String("project_id", projectID.String()),
		zap.String("source_entity", sourceEntity.Name),
		zap.String("target_entity", targetEntity.Name))
}

// RemoveRelationship marks a relationship as removed (is_approved=false).
func (s *schemaService) RemoveRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	// Verify relationship exists and belongs to project
	rel, err := s.schemaRepo.GetRelationshipByID(ctx, projectID, relationshipID)
	if err != nil {
		return apperrors.ErrNotFound
	}

	// Double-check project ownership (security)
	if rel.ProjectID != projectID {
		return apperrors.ErrNotFound
	}

	// Set is_approved=false to mark as removed
	if err := s.schemaRepo.UpdateRelationshipApproval(ctx, projectID, relationshipID, false); err != nil {
		return fmt.Errorf("failed to update relationship: %w", err)
	}

	s.logger.Info("Removed relationship",
		zap.String("project_id", projectID.String()),
		zap.String("relationship_id", relationshipID.String()),
	)

	return nil
}

// GetRelationshipsForDatasource returns all relationships for a datasource.
func (s *schemaService) GetRelationshipsForDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	relationships, err := s.schemaRepo.ListRelationshipsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list relationships: %w", err)
	}
	return relationships, nil
}

// GetRelationshipsResponse returns enriched relationships with table/column details and empty/orphan tables.
func (s *schemaService) GetRelationshipsResponse(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RelationshipsResponse, error) {
	// Get enriched relationship details (includes table/column names and types)
	details, err := s.schemaRepo.GetRelationshipDetails(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationship details: %w", err)
	}

	// Get empty tables (row_count = 0)
	emptyTables, err := s.schemaRepo.GetEmptyTables(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get empty tables: %w", err)
	}

	// Get orphan tables (has data but no relationships)
	orphanTables, err := s.schemaRepo.GetOrphanTables(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get orphan tables: %w", err)
	}

	return &models.RelationshipsResponse{
		Relationships: details,
		TotalCount:    len(details),
		EmptyTables:   emptyTables,
		OrphanTables:  orphanTables,
	}, nil
}

// UpdateTableMetadata updates business_name and/or description for a table.
func (s *schemaService) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	if err := s.schemaRepo.UpdateTableMetadata(ctx, projectID, tableID, businessName, description); err != nil {
		return fmt.Errorf("failed to update table metadata: %w", err)
	}

	s.logger.Info("Updated table metadata",
		zap.String("project_id", projectID.String()),
		zap.String("table_id", tableID.String()),
	)

	return nil
}

// UpdateColumnMetadata updates business_name and/or description for a column.
func (s *schemaService) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	if err := s.schemaRepo.UpdateColumnMetadata(ctx, projectID, columnID, businessName, description); err != nil {
		return fmt.Errorf("failed to update column metadata: %w", err)
	}

	s.logger.Info("Updated column metadata",
		zap.String("project_id", projectID.String()),
		zap.String("column_id", columnID.String()),
	)

	return nil
}

// SaveSelections updates is_selected flags for tables and columns using their UUIDs.
func (s *schemaService) SaveSelections(ctx context.Context, projectID, datasourceID uuid.UUID, tableSelections map[uuid.UUID]bool, columnSelections map[uuid.UUID][]uuid.UUID) error {
	// Update table selections directly using table IDs
	for tableID, isSelected := range tableSelections {
		if err := s.schemaRepo.UpdateTableSelection(ctx, projectID, tableID, isSelected); err != nil {
			s.logger.Warn("Failed to update table selection, skipping",
				zap.String("table_id", tableID.String()),
				zap.Error(err),
			)
			continue
		}
	}

	// Update column selections
	for tableID, selectedColumnIDs := range columnSelections {
		// Get all columns for this table to determine which should be deselected
		columns, err := s.schemaRepo.ListColumnsByTable(ctx, projectID, tableID, false)
		if err != nil {
			return fmt.Errorf("failed to list columns for table %s: %w", tableID.String(), err)
		}

		// Build set of selected column IDs
		selectedSet := make(map[uuid.UUID]bool)
		for _, colID := range selectedColumnIDs {
			selectedSet[colID] = true
		}

		// Update each column's selection status
		for _, col := range columns {
			isSelected := selectedSet[col.ID]
			if err := s.schemaRepo.UpdateColumnSelection(ctx, projectID, col.ID, isSelected); err != nil {
				return fmt.Errorf("failed to update column selection for %s: %w", col.ID.String(), err)
			}
		}
	}

	s.logger.Info("Saved selections",
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()),
		zap.Int("tables", len(tableSelections)),
		zap.Int("column_tables", len(columnSelections)),
	)

	return nil
}

// GetSelectedDatasourceSchema returns only selected tables and columns.
func (s *schemaService) GetSelectedDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	// Get full schema
	fullSchema, err := s.GetDatasourceSchema(ctx, projectID, datasourceID)
	if err != nil {
		return nil, err
	}

	// Build set of selected table IDs for relationship filtering
	selectedTableIDs := make(map[uuid.UUID]bool)

	// Filter tables
	selectedTables := make([]*models.DatasourceTable, 0)
	for _, table := range fullSchema.Tables {
		if !table.IsSelected {
			continue
		}

		selectedTableIDs[table.ID] = true

		// Filter columns
		selectedColumns := make([]*models.DatasourceColumn, 0)
		for _, col := range table.Columns {
			if col.IsSelected {
				selectedColumns = append(selectedColumns, col)
			}
		}
		table.Columns = selectedColumns

		selectedTables = append(selectedTables, table)
	}

	// Filter relationships - only include if both source and target tables are selected
	selectedRelationships := make([]*models.DatasourceRelationship, 0)
	for _, rel := range fullSchema.Relationships {
		if selectedTableIDs[rel.SourceTableID] && selectedTableIDs[rel.TargetTableID] {
			selectedRelationships = append(selectedRelationships, rel)
		}
	}

	return &models.DatasourceSchema{
		ProjectID:     projectID,
		DatasourceID:  datasourceID,
		Tables:        selectedTables,
		Relationships: selectedRelationships,
	}, nil
}

// GetDatasourceSchemaForPrompt returns schema formatted for LLM context.
func (s *schemaService) GetDatasourceSchemaForPrompt(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error) {
	var schema *models.DatasourceSchema
	var err error

	if selectedOnly {
		schema, err = s.GetSelectedDatasourceSchema(ctx, projectID, datasourceID)
	} else {
		schema, err = s.GetDatasourceSchema(ctx, projectID, datasourceID)
	}
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	sb.WriteString("DATABASE SCHEMA:\n")

	for _, table := range schema.Tables {
		sb.WriteString("\nTable: ")
		sb.WriteString(table.TableName)
		sb.WriteString("\n")

		if table.Description != "" {
			sb.WriteString("Description: ")
			sb.WriteString(table.Description)
			sb.WriteString("\n")
		}

		if table.RowCount > 0 {
			sb.WriteString(fmt.Sprintf("Row count: %d\n", table.RowCount))
		}

		sb.WriteString("Columns:\n")
		for _, col := range table.Columns {
			sb.WriteString("  - ")
			sb.WriteString(col.ColumnName)
			sb.WriteString(": ")
			sb.WriteString(col.DataType)

			// Add column attributes
			attrs := make([]string, 0)
			if col.IsPrimaryKey {
				attrs = append(attrs, "PRIMARY KEY")
			}
			if !col.IsNullable {
				attrs = append(attrs, "NOT NULL")
			}
			if len(attrs) > 0 {
				sb.WriteString(" [")
				sb.WriteString(strings.Join(attrs, ", "))
				sb.WriteString("]")
			}
			sb.WriteString("\n")
		}
	}

	if len(schema.Relationships) > 0 {
		sb.WriteString("\nRELATIONSHIPS:\n")
		for _, rel := range schema.Relationships {
			sb.WriteString("  ")
			sb.WriteString(rel.SourceTableName)
			sb.WriteString(".")
			sb.WriteString(rel.SourceColumnName)
			sb.WriteString(" -> ")
			sb.WriteString(rel.TargetTableName)
			sb.WriteString(".")
			sb.WriteString(rel.TargetColumnName)
			if rel.Cardinality != "" && rel.Cardinality != "unknown" {
				sb.WriteString(" (")
				sb.WriteString(rel.Cardinality)
				sb.WriteString(")")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// GetDatasourceSchemaWithEntities returns schema enriched with entity/role semantic information.
func (s *schemaService) GetDatasourceSchemaWithEntities(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error) {
	// Get base schema
	var schema *models.DatasourceSchema
	var err error

	if selectedOnly {
		schema, err = s.GetSelectedDatasourceSchema(ctx, projectID, datasourceID)
	} else {
		schema, err = s.GetDatasourceSchema(ctx, projectID, datasourceID)
	}
	if err != nil {
		return "", err
	}

	// Get entities for the project (from active ontology)
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get entities: %w", err)
	}

	// Filter entities to only include those from selected tables
	if selectedOnly {
		selectedTableNames := make(map[string]bool)
		for _, table := range schema.Tables {
			selectedTableNames[table.TableName] = true
		}

		filteredEntities := make([]*models.OntologyEntity, 0)
		for _, entity := range entities {
			if selectedTableNames[entity.PrimaryTable] {
				filteredEntities = append(filteredEntities, entity)
			}
		}
		entities = filteredEntities
	}

	// TODO: Compute occurrences from relationships (task 2.9 or later)
	// This function needs to be updated to query relationships and compute occurrences at runtime
	// similar to how entity_service.go and ontology_context.go were updated in tasks 2.5 and 2.6.
	// For now, entity/role annotations in schema output are disabled.

	// Build entity lookup maps
	entityByID := make(map[uuid.UUID]*models.OntologyEntity)
	for _, e := range entities {
		entityByID[e.ID] = e
	}

	// Build occurrence lookup map: schema.table.column -> []occurrence
	// TODO: Remove this empty map once the function is updated to compute occurrences from relationships
	occurrencesByColumn := make(map[string][]*models.OntologyEntityOccurrence)

	// Build schema context with entity information
	var sb strings.Builder
	sb.WriteString("DATABASE SCHEMA WITH ENTITY SEMANTICS:\n")
	sb.WriteString("=====================================\n\n")

	// First, list all entities
	if len(entities) > 0 {
		sb.WriteString("DOMAIN ENTITIES:\n")
		for _, entity := range entities {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", entity.Name, entity.Description))
			sb.WriteString(fmt.Sprintf("    Primary location: %s.%s.%s\n",
				entity.PrimarySchema, entity.PrimaryTable, entity.PrimaryColumn))
		}
		sb.WriteString("\n")
	}

	// Then list tables with entity annotations
	for _, table := range schema.Tables {
		sb.WriteString("\nTable: ")
		sb.WriteString(table.SchemaName)
		sb.WriteString(".")
		sb.WriteString(table.TableName)
		sb.WriteString("\n")

		if table.Description != "" {
			sb.WriteString("Description: ")
			sb.WriteString(table.Description)
			sb.WriteString("\n")
		}

		if table.RowCount > 0 {
			sb.WriteString(fmt.Sprintf("Row count: %d\n", table.RowCount))
		}

		sb.WriteString("Columns:\n")
		for _, col := range table.Columns {
			sb.WriteString("  - ")
			sb.WriteString(col.ColumnName)
			sb.WriteString(": ")
			sb.WriteString(col.DataType)

			// Add column attributes
			attrs := make([]string, 0)
			if col.IsPrimaryKey {
				attrs = append(attrs, "PRIMARY KEY")
			}
			if !col.IsNullable {
				attrs = append(attrs, "NOT NULL")
			}
			if len(attrs) > 0 {
				sb.WriteString(" [")
				sb.WriteString(strings.Join(attrs, ", "))
				sb.WriteString("]")
			}

			// Add entity/role information
			columnKey := fmt.Sprintf("%s.%s.%s", table.SchemaName, table.TableName, col.ColumnName)
			if occs, ok := occurrencesByColumn[columnKey]; ok {
				sb.WriteString(" => ")
				entityAnnotations := make([]string, 0, len(occs))
				for _, occ := range occs {
					if entity, found := entityByID[occ.EntityID]; found {
						if occ.Association != nil && *occ.Association != "" {
							entityAnnotations = append(entityAnnotations,
								fmt.Sprintf("%s (association: %s)", entity.Name, *occ.Association))
						} else {
							entityAnnotations = append(entityAnnotations, entity.Name)
						}
					}
				}
				sb.WriteString(strings.Join(entityAnnotations, ", "))
			}

			sb.WriteString("\n")
		}
	}

	// Add relationships with entity context
	if len(schema.Relationships) > 0 {
		sb.WriteString("\nRELATIONSHIPS (with entity context):\n")
		for _, rel := range schema.Relationships {
			sb.WriteString("  ")
			sb.WriteString(rel.SourceTableName)
			sb.WriteString(".")
			sb.WriteString(rel.SourceColumnName)

			// Add entity info for source
			var sourceKey string
			if idx := strings.Index(rel.SourceTableName, "."); idx != -1 {
				sourceKey = fmt.Sprintf("%s.%s.%s", rel.SourceTableName[:idx],
					rel.SourceTableName[idx+1:], rel.SourceColumnName)
			} else {
				sourceKey = fmt.Sprintf("public.%s.%s", rel.SourceTableName, rel.SourceColumnName)
			}

			if occs, ok := occurrencesByColumn[sourceKey]; ok && len(occs) > 0 {
				if entity, found := entityByID[occs[0].EntityID]; found {
					if occs[0].Association != nil && *occs[0].Association != "" {
						sb.WriteString(fmt.Sprintf(" [%s as %s]", entity.Name, *occs[0].Association))
					} else {
						sb.WriteString(fmt.Sprintf(" [%s]", entity.Name))
					}
				}
			}

			sb.WriteString(" -> ")
			sb.WriteString(rel.TargetTableName)
			sb.WriteString(".")
			sb.WriteString(rel.TargetColumnName)

			// Add entity info for target
			var targetKey string
			if idx := strings.Index(rel.TargetTableName, "."); idx != -1 {
				targetKey = fmt.Sprintf("%s.%s.%s", rel.TargetTableName[:idx],
					rel.TargetTableName[idx+1:], rel.TargetColumnName)
			} else {
				targetKey = fmt.Sprintf("public.%s.%s", rel.TargetTableName, rel.TargetColumnName)
			}

			if occs, ok := occurrencesByColumn[targetKey]; ok && len(occs) > 0 {
				if entity, found := entityByID[occs[0].EntityID]; found {
					if occs[0].Association != nil && *occs[0].Association != "" {
						sb.WriteString(fmt.Sprintf(" [%s as %s]", entity.Name, *occs[0].Association))
					} else {
						sb.WriteString(fmt.Sprintf(" [%s]", entity.Name))
					}
				}
			}

			if rel.Cardinality != "" && rel.Cardinality != "unknown" {
				sb.WriteString(" (")
				sb.WriteString(rel.Cardinality)
				sb.WriteString(")")
			}
			sb.WriteString("\n")
		}
	}

	return sb.String(), nil
}

// SelectAllTables marks all tables and columns for this datasource as selected.
func (s *schemaService) SelectAllTables(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	if err := s.schemaRepo.SelectAllTablesAndColumns(ctx, projectID, datasourceID); err != nil {
		return fmt.Errorf("failed to select all tables: %w", err)
	}
	return nil
}

// Ensure schemaService implements SchemaService at compile time.
var _ SchemaService = (*schemaService)(nil)
