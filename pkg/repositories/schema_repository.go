package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TableKey identifies a table uniquely within a datasource.
type TableKey struct {
	SchemaName string
	TableName  string
}

// SchemaRepository provides data access for schema discovery tables.
type SchemaRepository interface {
	// Tables
	ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error)
	GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error)
	GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error)
	// FindTableByName finds a table by name within a datasource (schema-agnostic).
	// Use this when the schema prefix is not known or relevant (e.g., ontology layer).
	FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error)
	UpsertTable(ctx context.Context, table *models.SchemaTable) error
	SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []TableKey) (int64, error)
	UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error
	UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error

	// Columns
	ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error)
	ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error)
	GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error)
	GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error)
	UpsertColumn(ctx context.Context, column *models.SchemaColumn) error
	SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error)
	UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error
	UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount *int64) error
	UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error

	// Relationships
	ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error)
	GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error)
	GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error)
	UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error
	UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error
	SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error
	SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error)

	// Relationship Discovery
	GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error)
	GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error)
	GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error)
	UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error
	GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error)
	UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount *int64, isJoinable *bool, joinabilityReason *string) error
	GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error)
	GetRelationshipCandidates(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipCandidate, error)
	// GetNonPKColumnsByExactType returns non-primary-key columns with exact data type match for review candidate discovery.
	GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error)
}

type schemaRepository struct{}

// NewSchemaRepository creates a new SchemaRepository.
func NewSchemaRepository() SchemaRepository {
	return &schemaRepository{}
}

var _ SchemaRepository = (*schemaRepository)(nil)

// ============================================================================
// Table Methods
// ============================================================================

func (r *schemaRepository) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name,
		       is_selected, row_count, business_name, description, metadata,
		       created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
		ORDER BY schema_name, table_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list tables: %w", err)
	}
	defer rows.Close()

	tables := make([]*models.SchemaTable, 0)
	for rows.Next() {
		t, err := scanSchemaTable(rows)
		if err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tables: %w", err)
	}

	return tables, nil
}

func (r *schemaRepository) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name,
		       is_selected, row_count, business_name, description, metadata,
		       created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, query, projectID, tableID)
	t, err := scanSchemaTableRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("table not found")
		}
		return nil, fmt.Errorf("failed to get table: %w", err)
	}

	return t, nil
}

func (r *schemaRepository) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name,
		       is_selected, row_count, business_name, description, metadata,
		       created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND datasource_id = $2
		  AND schema_name = $3 AND table_name = $4 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, query, projectID, datasourceID, schemaName, tableName)
	t, err := scanSchemaTableRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("table not found")
		}
		return nil, fmt.Errorf("failed to get table: %w", err)
	}

	return t, nil
}

func (r *schemaRepository) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name,
		       is_selected, row_count, business_name, description, metadata,
		       created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND datasource_id = $2
		  AND table_name = $3 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, query, projectID, datasourceID, tableName)
	t, err := scanSchemaTableRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("table not found: %s", tableName)
		}
		return nil, fmt.Errorf("failed to find table: %w", err)
	}

	return t, nil
}

func (r *schemaRepository) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	table.UpdatedAt = now
	if table.ID == uuid.Nil {
		table.ID = uuid.New()
		table.CreatedAt = now
	}

	metadata, err := json.Marshal(table.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if table.Metadata == nil {
		metadata = []byte("{}")
	}

	// First, try to reactivate a soft-deleted record
	reactivateQuery := `
		UPDATE engine_schema_tables
		SET deleted_at = NULL,
		    row_count = $5,
		    metadata = $6,
		    updated_at = $7
		WHERE project_id = $1
		  AND datasource_id = $2
		  AND schema_name = $3
		  AND table_name = $4
		  AND deleted_at IS NOT NULL
		RETURNING id, created_at, is_selected, business_name, description`

	var existingID uuid.UUID
	var existingCreatedAt time.Time
	var existingIsSelected bool
	var existingBusinessName, existingDescription *string
	err = scope.Conn.QueryRow(ctx, reactivateQuery,
		table.ProjectID, table.DatasourceID, table.SchemaName, table.TableName,
		table.RowCount, metadata, now,
	).Scan(&existingID, &existingCreatedAt, &existingIsSelected, &existingBusinessName, &existingDescription)

	if err == nil {
		// Reactivated soft-deleted record - preserve user metadata
		table.ID = existingID
		table.CreatedAt = existingCreatedAt
		table.IsSelected = existingIsSelected
		table.BusinessName = existingBusinessName
		table.Description = existingDescription
		return nil
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("failed to reactivate table: %w", err)
	}

	// No soft-deleted record, do standard upsert on active records
	upsertQuery := `
		INSERT INTO engine_schema_tables (
			id, project_id, datasource_id, schema_name, table_name,
			is_selected, row_count, business_name, description, metadata,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (project_id, datasource_id, schema_name, table_name)
			WHERE deleted_at IS NULL
		DO UPDATE SET
			row_count = EXCLUDED.row_count,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, is_selected, business_name, description`

	err = scope.Conn.QueryRow(ctx, upsertQuery,
		table.ID, table.ProjectID, table.DatasourceID, table.SchemaName, table.TableName,
		table.IsSelected, table.RowCount, table.BusinessName, table.Description, metadata,
		table.CreatedAt, table.UpdatedAt,
	).Scan(&table.ID, &table.CreatedAt, &table.IsSelected, &table.BusinessName, &table.Description)

	if err != nil {
		return fmt.Errorf("failed to upsert table: %w", err)
	}

	return nil
}

func (r *schemaRepository) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []TableKey) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	if len(activeTableKeys) == 0 {
		// Soft-delete ALL active tables for this datasource
		query := `
			UPDATE engine_schema_tables
			SET deleted_at = NOW()
			WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL`
		result, err := scope.Conn.Exec(ctx, query, projectID, datasourceID)
		if err != nil {
			return 0, fmt.Errorf("failed to soft-delete tables: %w", err)
		}
		return result.RowsAffected(), nil
	}

	// Use CTE with parallel arrays for NOT EXISTS check
	query := `
		WITH active_tables AS (
			SELECT unnest($3::text[]) as schema_name,
			       unnest($4::text[]) as table_name
		)
		UPDATE engine_schema_tables t
		SET deleted_at = NOW()
		WHERE t.project_id = $1
		  AND t.datasource_id = $2
		  AND t.deleted_at IS NULL
		  AND NOT EXISTS (
			  SELECT 1 FROM active_tables a
			  WHERE a.schema_name = t.schema_name AND a.table_name = t.table_name
		  )`

	schemaNames := make([]string, len(activeTableKeys))
	tableNames := make([]string, len(activeTableKeys))
	for i, k := range activeTableKeys {
		schemaNames[i] = k.SchemaName
		tableNames[i] = k.TableName
	}

	result, err := scope.Conn.Exec(ctx, query, projectID, datasourceID, schemaNames, tableNames)
	if err != nil {
		return 0, fmt.Errorf("failed to soft-delete tables: %w", err)
	}

	return result.RowsAffected(), nil
}

func (r *schemaRepository) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_tables
		SET is_selected = $3, updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, projectID, tableID, isSelected)
	if err != nil {
		return fmt.Errorf("failed to update table selection: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("table not found")
	}

	return nil
}

func (r *schemaRepository) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Use COALESCE to preserve existing values when nil is passed
	query := `
		UPDATE engine_schema_tables
		SET business_name = COALESCE($3, business_name),
		    description = COALESCE($4, description),
		    updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, projectID, tableID, businessName, description)
	if err != nil {
		return fmt.Errorf("failed to update table metadata: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("table not found")
	}

	return nil
}

// ============================================================================
// Column Methods
// ============================================================================

func (r *schemaRepository) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id, column_name, data_type,
		       is_nullable, is_primary_key, is_unique, is_selected, ordinal_position,
		       default_value, distinct_count, null_count, business_name, description, metadata,
		       created_at, updated_at
		FROM engine_schema_columns
		WHERE project_id = $1 AND schema_table_id = $2 AND deleted_at IS NULL
		ORDER BY ordinal_position`

	rows, err := scope.Conn.Query(ctx, query, projectID, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns: %w", err)
	}
	defer rows.Close()

	columns := make([]*models.SchemaColumn, 0)
	for rows.Next() {
		c, err := scanSchemaColumn(rows)
		if err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating columns: %w", err)
	}

	return columns, nil
}

func (r *schemaRepository) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.business_name, c.description, c.metadata,
		       c.created_at, c.updated_at
		FROM engine_schema_columns c
		JOIN engine_schema_tables t ON c.schema_table_id = t.id
		WHERE c.project_id = $1 AND t.datasource_id = $2
		  AND c.deleted_at IS NULL AND t.deleted_at IS NULL
		ORDER BY t.schema_name, t.table_name, c.ordinal_position`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list columns: %w", err)
	}
	defer rows.Close()

	columns := make([]*models.SchemaColumn, 0)
	for rows.Next() {
		c, err := scanSchemaColumn(rows)
		if err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating columns: %w", err)
	}

	return columns, nil
}

func (r *schemaRepository) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id, column_name, data_type,
		       is_nullable, is_primary_key, is_unique, is_selected, ordinal_position,
		       default_value, distinct_count, null_count, business_name, description, metadata,
		       created_at, updated_at
		FROM engine_schema_columns
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, query, projectID, columnID)
	c, err := scanSchemaColumnRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get column: %w", err)
	}

	return c, nil
}

func (r *schemaRepository) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id, column_name, data_type,
		       is_nullable, is_primary_key, is_unique, is_selected, ordinal_position,
		       default_value, distinct_count, null_count, business_name, description, metadata,
		       created_at, updated_at
		FROM engine_schema_columns
		WHERE schema_table_id = $1 AND column_name = $2 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, query, tableID, columnName)
	c, err := scanSchemaColumnRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get column: %w", err)
	}

	return c, nil
}

func (r *schemaRepository) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	column.UpdatedAt = now
	if column.ID == uuid.Nil {
		column.ID = uuid.New()
		column.CreatedAt = now
	}

	metadata, err := json.Marshal(column.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}
	if column.Metadata == nil {
		metadata = []byte("{}")
	}

	// First, try to reactivate a soft-deleted record
	reactivateQuery := `
		UPDATE engine_schema_columns
		SET deleted_at = NULL,
		    data_type = $4,
		    is_nullable = $5,
		    is_primary_key = $6,
		    is_unique = $7,
		    ordinal_position = $8,
		    default_value = $9,
		    metadata = $10,
		    updated_at = $11
		WHERE schema_table_id = $1
		  AND column_name = $2
		  AND project_id = $3
		  AND deleted_at IS NOT NULL
		RETURNING id, created_at, is_selected, distinct_count, null_count, business_name, description`

	var existingID uuid.UUID
	var existingCreatedAt time.Time
	var existingIsSelected bool
	var existingDistinctCount, existingNullCount *int64
	var existingBusinessName, existingDescription *string
	err = scope.Conn.QueryRow(ctx, reactivateQuery,
		column.SchemaTableID, column.ColumnName, column.ProjectID,
		column.DataType, column.IsNullable, column.IsPrimaryKey, column.IsUnique, column.OrdinalPosition,
		column.DefaultValue, metadata, now,
	).Scan(&existingID, &existingCreatedAt, &existingIsSelected,
		&existingDistinctCount, &existingNullCount, &existingBusinessName, &existingDescription)

	if err == nil {
		// Reactivated soft-deleted record - preserve user metadata and stats
		column.ID = existingID
		column.CreatedAt = existingCreatedAt
		column.IsSelected = existingIsSelected
		column.DistinctCount = existingDistinctCount
		column.NullCount = existingNullCount
		column.BusinessName = existingBusinessName
		column.Description = existingDescription
		return nil
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("failed to reactivate column: %w", err)
	}

	// No soft-deleted record, do standard upsert on active records
	upsertQuery := `
		INSERT INTO engine_schema_columns (
			id, project_id, schema_table_id, column_name, data_type,
			is_nullable, is_primary_key, is_unique, is_selected, ordinal_position,
			default_value, distinct_count, null_count, business_name, description, metadata,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		ON CONFLICT (schema_table_id, column_name)
			WHERE deleted_at IS NULL
		DO UPDATE SET
			data_type = EXCLUDED.data_type,
			is_nullable = EXCLUDED.is_nullable,
			is_primary_key = EXCLUDED.is_primary_key,
			is_unique = EXCLUDED.is_unique,
			ordinal_position = EXCLUDED.ordinal_position,
			default_value = EXCLUDED.default_value,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, is_selected, distinct_count, null_count, business_name, description`

	err = scope.Conn.QueryRow(ctx, upsertQuery,
		column.ID, column.ProjectID, column.SchemaTableID, column.ColumnName, column.DataType,
		column.IsNullable, column.IsPrimaryKey, column.IsUnique, column.IsSelected, column.OrdinalPosition,
		column.DefaultValue, column.DistinctCount, column.NullCount, column.BusinessName, column.Description, metadata,
		column.CreatedAt, column.UpdatedAt,
	).Scan(&column.ID, &column.CreatedAt, &column.IsSelected,
		&column.DistinctCount, &column.NullCount, &column.BusinessName, &column.Description)

	if err != nil {
		return fmt.Errorf("failed to upsert column: %w", err)
	}

	return nil
}

func (r *schemaRepository) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	if len(activeColumnNames) == 0 {
		// Soft-delete ALL active columns for this table
		query := `
			UPDATE engine_schema_columns
			SET deleted_at = NOW()
			WHERE schema_table_id = $1 AND deleted_at IS NULL`
		result, err := scope.Conn.Exec(ctx, query, tableID)
		if err != nil {
			return 0, fmt.Errorf("failed to soft-delete columns: %w", err)
		}
		return result.RowsAffected(), nil
	}

	// Soft-delete columns NOT in the active list
	query := `
		UPDATE engine_schema_columns
		SET deleted_at = NOW()
		WHERE schema_table_id = $1
		  AND deleted_at IS NULL
		  AND column_name != ALL($2::text[])`

	result, err := scope.Conn.Exec(ctx, query, tableID, activeColumnNames)
	if err != nil {
		return 0, fmt.Errorf("failed to soft-delete columns: %w", err)
	}

	return result.RowsAffected(), nil
}

func (r *schemaRepository) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_columns
		SET is_selected = $3, updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, projectID, columnID, isSelected)
	if err != nil {
		return fmt.Errorf("failed to update column selection: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

func (r *schemaRepository) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount *int64) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_columns
		SET distinct_count = $2, null_count = $3, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, columnID, distinctCount, nullCount)
	if err != nil {
		return fmt.Errorf("failed to update column stats: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

func (r *schemaRepository) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_columns
		SET business_name = COALESCE($3, business_name),
		    description = COALESCE($4, description),
		    updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, projectID, columnID, businessName, description)
	if err != nil {
		return fmt.Errorf("failed to update column metadata: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

// ============================================================================
// Relationship Methods
// ============================================================================

func (r *schemaRepository) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT r.id, r.project_id, r.source_table_id, r.source_column_id,
		       r.target_table_id, r.target_column_id, r.relationship_type,
		       r.cardinality, r.confidence, r.inference_method, r.is_validated,
		       r.validation_results, r.is_approved, r.created_at, r.updated_at
		FROM engine_schema_relationships r
		JOIN engine_schema_tables st ON r.source_table_id = st.id
		WHERE r.project_id = $1 AND st.datasource_id = $2
		  AND r.deleted_at IS NULL AND st.deleted_at IS NULL
		ORDER BY r.created_at`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list relationships: %w", err)
	}
	defer rows.Close()

	relationships := make([]*models.SchemaRelationship, 0)
	for rows.Next() {
		rel, err := scanSchemaRelationship(rows)
		if err != nil {
			return nil, err
		}
		relationships = append(relationships, rel)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating relationships: %w", err)
	}

	return relationships, nil
}

func (r *schemaRepository) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, source_table_id, source_column_id,
		       target_table_id, target_column_id, relationship_type,
		       cardinality, confidence, inference_method, is_validated,
		       validation_results, is_approved, created_at, updated_at,
		       match_rate, source_distinct, target_distinct, matched_count, rejection_reason
		FROM engine_schema_relationships
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, query, projectID, relationshipID)
	rel, err := scanSchemaRelationshipRowWithDiscovery(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("relationship not found")
		}
		return nil, fmt.Errorf("failed to get relationship: %w", err)
	}

	return rel, nil
}

func (r *schemaRepository) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, source_table_id, source_column_id,
		       target_table_id, target_column_id, relationship_type,
		       cardinality, confidence, inference_method, is_validated,
		       validation_results, is_approved, created_at, updated_at,
		       match_rate, source_distinct, target_distinct, matched_count, rejection_reason
		FROM engine_schema_relationships
		WHERE source_column_id = $1 AND target_column_id = $2 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, query, sourceColumnID, targetColumnID)
	rel, err := scanSchemaRelationshipRowWithDiscovery(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found is not an error for this lookup
		}
		return nil, fmt.Errorf("failed to get relationship: %w", err)
	}

	return rel, nil
}

func (r *schemaRepository) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	rel.UpdatedAt = now
	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
		rel.CreatedAt = now
	}

	var validationResultsJSON []byte
	if rel.ValidationResults != nil {
		var err error
		validationResultsJSON, err = json.Marshal(rel.ValidationResults)
		if err != nil {
			return fmt.Errorf("failed to marshal validation_results: %w", err)
		}
	}

	// First, try to reactivate a soft-deleted record
	reactivateQuery := `
		UPDATE engine_schema_relationships
		SET deleted_at = NULL,
		    relationship_type = $3,
		    cardinality = $4,
		    confidence = $5,
		    inference_method = $6,
		    is_validated = $7,
		    validation_results = $8,
		    is_approved = $9,
		    rejection_reason = $10,
		    updated_at = $11
		WHERE source_column_id = $1
		  AND target_column_id = $2
		  AND deleted_at IS NOT NULL
		RETURNING id, project_id, source_table_id, target_table_id, created_at`

	var existingID, existingProjectID, existingSourceTableID, existingTargetTableID uuid.UUID
	var existingCreatedAt time.Time
	err := scope.Conn.QueryRow(ctx, reactivateQuery,
		rel.SourceColumnID, rel.TargetColumnID,
		rel.RelationshipType, rel.Cardinality, rel.Confidence, rel.InferenceMethod,
		rel.IsValidated, validationResultsJSON, rel.IsApproved, rel.RejectionReason, now,
	).Scan(&existingID, &existingProjectID, &existingSourceTableID, &existingTargetTableID, &existingCreatedAt)

	if err == nil {
		// Reactivated soft-deleted record
		rel.ID = existingID
		rel.ProjectID = existingProjectID
		rel.SourceTableID = existingSourceTableID
		rel.TargetTableID = existingTargetTableID
		rel.CreatedAt = existingCreatedAt
		return nil
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("failed to reactivate relationship: %w", err)
	}

	// No soft-deleted record, do standard upsert on active records
	upsertQuery := `
		INSERT INTO engine_schema_relationships (
			id, project_id, source_table_id, source_column_id,
			target_table_id, target_column_id, relationship_type,
			cardinality, confidence, inference_method, is_validated,
			validation_results, is_approved, rejection_reason, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (source_column_id, target_column_id)
			WHERE deleted_at IS NULL
		DO UPDATE SET
			relationship_type = EXCLUDED.relationship_type,
			cardinality = EXCLUDED.cardinality,
			confidence = EXCLUDED.confidence,
			inference_method = EXCLUDED.inference_method,
			is_validated = EXCLUDED.is_validated,
			validation_results = EXCLUDED.validation_results,
			is_approved = EXCLUDED.is_approved,
			rejection_reason = EXCLUDED.rejection_reason,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at`

	err = scope.Conn.QueryRow(ctx, upsertQuery,
		rel.ID, rel.ProjectID, rel.SourceTableID, rel.SourceColumnID,
		rel.TargetTableID, rel.TargetColumnID, rel.RelationshipType,
		rel.Cardinality, rel.Confidence, rel.InferenceMethod, rel.IsValidated,
		validationResultsJSON, rel.IsApproved, rel.RejectionReason, rel.CreatedAt, rel.UpdatedAt,
	).Scan(&rel.ID, &rel.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert relationship: %w", err)
	}

	return nil
}

func (r *schemaRepository) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_relationships
		SET is_approved = $3, updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, projectID, relationshipID, isApproved)
	if err != nil {
		return fmt.Errorf("failed to update relationship approval: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("relationship not found")
	}

	return nil
}

func (r *schemaRepository) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_relationships
		SET deleted_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, projectID, relationshipID)
	if err != nil {
		return fmt.Errorf("failed to soft-delete relationship: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("relationship not found")
	}

	return nil
}

func (r *schemaRepository) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	// Soft-delete relationships where source or target column is soft-deleted
	query := `
		UPDATE engine_schema_relationships r
		SET deleted_at = NOW()
		WHERE r.project_id = $1
		  AND r.deleted_at IS NULL
		  AND (
			  EXISTS (
				  SELECT 1 FROM engine_schema_columns c
				  WHERE c.id = r.source_column_id AND c.deleted_at IS NOT NULL
			  )
			  OR EXISTS (
				  SELECT 1 FROM engine_schema_columns c
				  WHERE c.id = r.target_column_id AND c.deleted_at IS NOT NULL
			  )
		  )`

	result, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return 0, fmt.Errorf("failed to soft-delete orphaned relationships: %w", err)
	}

	return result.RowsAffected(), nil
}

// ============================================================================
// Relationship Discovery Methods
// ============================================================================

func (r *schemaRepository) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT
			r.id,
			st.table_name as source_table_name,
			sc.column_name as source_column_name,
			sc.data_type as source_column_type,
			tt.table_name as target_table_name,
			tc.column_name as target_column_name,
			tc.data_type as target_column_type,
			r.relationship_type,
			r.cardinality,
			r.confidence,
			r.inference_method,
			r.is_validated,
			r.is_approved,
			r.created_at,
			r.updated_at
		FROM engine_schema_relationships r
		JOIN engine_schema_columns sc ON r.source_column_id = sc.id
		JOIN engine_schema_tables st ON r.source_table_id = st.id
		JOIN engine_schema_columns tc ON r.target_column_id = tc.id
		JOIN engine_schema_tables tt ON r.target_table_id = tt.id
		WHERE r.project_id = $1
		  AND st.datasource_id = $2
		  AND r.deleted_at IS NULL
		  AND sc.deleted_at IS NULL
		  AND st.deleted_at IS NULL
		  AND tc.deleted_at IS NULL
		  AND tt.deleted_at IS NULL
		  AND r.rejection_reason IS NULL
		ORDER BY st.schema_name, st.table_name, sc.column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationship details: %w", err)
	}
	defer rows.Close()

	details := make([]*models.RelationshipDetail, 0)
	for rows.Next() {
		var d models.RelationshipDetail
		err := rows.Scan(
			&d.ID,
			&d.SourceTableName, &d.SourceColumnName, &d.SourceColumnType,
			&d.TargetTableName, &d.TargetColumnName, &d.TargetColumnType,
			&d.RelationshipType, &d.Cardinality, &d.Confidence,
			&d.InferenceMethod, &d.IsValidated, &d.IsApproved,
			&d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan relationship detail: %w", err)
		}
		details = append(details, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating relationship details: %w", err)
	}

	return details, nil
}

func (r *schemaRepository) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT table_name
		FROM engine_schema_tables
		WHERE project_id = $1
		  AND datasource_id = $2
		  AND deleted_at IS NULL
		  AND (row_count IS NULL OR row_count = 0)
		ORDER BY table_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get empty tables: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan empty table: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating empty tables: %w", err)
	}

	return tables, nil
}

func (r *schemaRepository) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Tables with data (row_count > 0) but no active relationships
	query := `
		SELECT t.table_name
		FROM engine_schema_tables t
		WHERE t.project_id = $1
		  AND t.datasource_id = $2
		  AND t.deleted_at IS NULL
		  AND t.row_count IS NOT NULL
		  AND t.row_count > 0
		  AND NOT EXISTS (
			  SELECT 1 FROM engine_schema_relationships r
			  WHERE r.deleted_at IS NULL
			    AND r.rejection_reason IS NULL
			    AND (r.source_table_id = t.id OR r.target_table_id = t.id)
		  )
		ORDER BY t.table_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get orphan tables: %w", err)
	}
	defer rows.Close()

	tables := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan orphan table: %w", err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating orphan tables: %w", err)
	}

	return tables, nil
}

func (r *schemaRepository) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	rel.UpdatedAt = now
	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
		rel.CreatedAt = now
	}

	var validationResultsJSON []byte
	if rel.ValidationResults != nil {
		var err error
		validationResultsJSON, err = json.Marshal(rel.ValidationResults)
		if err != nil {
			return fmt.Errorf("failed to marshal validation_results: %w", err)
		}
	}

	// Set discovery metrics on the relationship
	if metrics != nil {
		rel.MatchRate = &metrics.MatchRate
		rel.SourceDistinct = &metrics.SourceDistinct
		rel.TargetDistinct = &metrics.TargetDistinct
		rel.MatchedCount = &metrics.MatchedCount
	}

	// First, try to reactivate a soft-deleted record
	reactivateQuery := `
		UPDATE engine_schema_relationships
		SET deleted_at = NULL,
		    relationship_type = $3,
		    cardinality = $4,
		    confidence = $5,
		    inference_method = $6,
		    is_validated = $7,
		    validation_results = $8,
		    is_approved = $9,
		    match_rate = $10,
		    source_distinct = $11,
		    target_distinct = $12,
		    matched_count = $13,
		    rejection_reason = $14,
		    updated_at = $15
		WHERE source_column_id = $1
		  AND target_column_id = $2
		  AND deleted_at IS NOT NULL
		RETURNING id, project_id, source_table_id, target_table_id, created_at`

	var existingID, existingProjectID, existingSourceTableID, existingTargetTableID uuid.UUID
	var existingCreatedAt time.Time
	err := scope.Conn.QueryRow(ctx, reactivateQuery,
		rel.SourceColumnID, rel.TargetColumnID,
		rel.RelationshipType, rel.Cardinality, rel.Confidence, rel.InferenceMethod,
		rel.IsValidated, validationResultsJSON, rel.IsApproved,
		rel.MatchRate, rel.SourceDistinct, rel.TargetDistinct, rel.MatchedCount,
		rel.RejectionReason, now,
	).Scan(&existingID, &existingProjectID, &existingSourceTableID, &existingTargetTableID, &existingCreatedAt)

	if err == nil {
		rel.ID = existingID
		rel.ProjectID = existingProjectID
		rel.SourceTableID = existingSourceTableID
		rel.TargetTableID = existingTargetTableID
		rel.CreatedAt = existingCreatedAt
		return nil
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("failed to reactivate relationship: %w", err)
	}

	// No soft-deleted record, do standard upsert on active records
	upsertQuery := `
		INSERT INTO engine_schema_relationships (
			id, project_id, source_table_id, source_column_id,
			target_table_id, target_column_id, relationship_type,
			cardinality, confidence, inference_method, is_validated,
			validation_results, is_approved, match_rate, source_distinct,
			target_distinct, matched_count, rejection_reason,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20)
		ON CONFLICT (source_column_id, target_column_id)
			WHERE deleted_at IS NULL
		DO UPDATE SET
			relationship_type = EXCLUDED.relationship_type,
			cardinality = EXCLUDED.cardinality,
			confidence = EXCLUDED.confidence,
			inference_method = EXCLUDED.inference_method,
			is_validated = EXCLUDED.is_validated,
			validation_results = EXCLUDED.validation_results,
			is_approved = EXCLUDED.is_approved,
			match_rate = EXCLUDED.match_rate,
			source_distinct = EXCLUDED.source_distinct,
			target_distinct = EXCLUDED.target_distinct,
			matched_count = EXCLUDED.matched_count,
			rejection_reason = EXCLUDED.rejection_reason,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at`

	err = scope.Conn.QueryRow(ctx, upsertQuery,
		rel.ID, rel.ProjectID, rel.SourceTableID, rel.SourceColumnID,
		rel.TargetTableID, rel.TargetColumnID, rel.RelationshipType,
		rel.Cardinality, rel.Confidence, rel.InferenceMethod, rel.IsValidated,
		validationResultsJSON, rel.IsApproved, rel.MatchRate, rel.SourceDistinct,
		rel.TargetDistinct, rel.MatchedCount, rel.RejectionReason,
		rel.CreatedAt, rel.UpdatedAt,
	).Scan(&rel.ID, &rel.CreatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert relationship with metrics: %w", err)
	}

	return nil
}

func (r *schemaRepository) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id, column_name, data_type,
		       is_nullable, is_primary_key, is_selected, ordinal_position,
		       distinct_count, null_count, business_name, description, metadata,
		       created_at, updated_at,
		       row_count, non_null_count, is_joinable, joinability_reason, stats_updated_at
		FROM engine_schema_columns
		WHERE project_id = $1
		  AND schema_table_id = $2
		  AND deleted_at IS NULL
		  AND is_joinable = true
		ORDER BY ordinal_position`

	rows, err := scope.Conn.Query(ctx, query, projectID, tableID)
	if err != nil {
		return nil, fmt.Errorf("failed to get joinable columns: %w", err)
	}
	defer rows.Close()

	columns := make([]*models.SchemaColumn, 0)
	for rows.Next() {
		c, err := scanSchemaColumnWithDiscovery(rows)
		if err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating joinable columns: %w", err)
	}

	return columns, nil
}

func (r *schemaRepository) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount *int64, isJoinable *bool, joinabilityReason *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_columns
		SET row_count = $2,
		    non_null_count = $3,
		    is_joinable = $4,
		    joinability_reason = $5,
		    stats_updated_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, columnID, rowCount, nonNullCount, isJoinable, joinabilityReason)
	if err != nil {
		return fmt.Errorf("failed to update column joinability: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

func (r *schemaRepository) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_selected, c.ordinal_position,
		       c.distinct_count, c.null_count, c.business_name, c.description, c.metadata,
		       c.created_at, c.updated_at,
		       c.row_count, c.non_null_count, c.is_joinable, c.joinability_reason, c.stats_updated_at
		FROM engine_schema_columns c
		JOIN engine_schema_tables t ON c.schema_table_id = t.id
		WHERE c.project_id = $1
		  AND t.datasource_id = $2
		  AND c.deleted_at IS NULL
		  AND t.deleted_at IS NULL
		  AND c.is_primary_key = true
		ORDER BY t.schema_name, t.table_name, c.ordinal_position`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get primary key columns: %w", err)
	}
	defer rows.Close()

	columns := make([]*models.SchemaColumn, 0)
	for rows.Next() {
		c, err := scanSchemaColumnWithDiscovery(rows)
		if err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating primary key columns: %w", err)
	}

	return columns, nil
}

func (r *schemaRepository) GetRelationshipCandidates(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipCandidate, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Get all relationships including rejected ones (those with rejection_reason set)
	query := `
		SELECT
			r.id,
			st.schema_name || '.' || st.table_name as source_table,
			sc.column_name as source_column,
			tt.schema_name || '.' || tt.table_name as target_table,
			tc.column_name as target_column,
			COALESCE(r.match_rate, 0) as match_rate,
			CASE
				WHEN r.rejection_reason IS NOT NULL THEN 'rejected'
				WHEN r.is_approved = true THEN 'verified'
				ELSE 'pending'
			END as status,
			r.rejection_reason
		FROM engine_schema_relationships r
		JOIN engine_schema_columns sc ON r.source_column_id = sc.id
		JOIN engine_schema_tables st ON r.source_table_id = st.id
		JOIN engine_schema_columns tc ON r.target_column_id = tc.id
		JOIN engine_schema_tables tt ON r.target_table_id = tt.id
		WHERE r.project_id = $1
		  AND st.datasource_id = $2
		  AND r.deleted_at IS NULL
		  AND sc.deleted_at IS NULL
		  AND st.deleted_at IS NULL
		  AND tc.deleted_at IS NULL
		  AND tt.deleted_at IS NULL
		ORDER BY st.schema_name, st.table_name, sc.column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationship candidates: %w", err)
	}
	defer rows.Close()

	candidates := make([]*models.RelationshipCandidate, 0)
	for rows.Next() {
		var c models.RelationshipCandidate
		err := rows.Scan(
			&c.ID,
			&c.SourceTable, &c.SourceColumn,
			&c.TargetTable, &c.TargetColumn,
			&c.MatchRate, &c.Status, &c.RejectionReason,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan relationship candidate: %w", err)
		}
		candidates = append(candidates, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating relationship candidates: %w", err)
	}

	return candidates, nil
}

func (r *schemaRepository) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Returns non-primary-key columns with exact data type match for review candidate discovery.
	// Includes column stats and table info for cardinality filtering.
	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.business_name, c.description, c.metadata,
		       c.created_at, c.updated_at,
		       c.row_count, c.non_null_count, c.is_joinable, c.joinability_reason, c.stats_updated_at
		FROM engine_schema_columns c
		JOIN engine_schema_tables t ON c.schema_table_id = t.id
		WHERE c.project_id = $1
		  AND t.datasource_id = $2
		  AND c.deleted_at IS NULL
		  AND t.deleted_at IS NULL
		  AND c.is_primary_key = false
		  AND LOWER(c.data_type) = LOWER($3)
		ORDER BY t.schema_name, t.table_name, c.ordinal_position`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID, dataType)
	if err != nil {
		return nil, fmt.Errorf("failed to get non-PK columns by type: %w", err)
	}
	defer rows.Close()

	columns := make([]*models.SchemaColumn, 0)
	for rows.Next() {
		c, err := scanSchemaColumnWithDiscovery(rows)
		if err != nil {
			return nil, err
		}
		columns = append(columns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating non-PK columns: %w", err)
	}

	return columns, nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanSchemaTable(rows pgx.Rows) (*models.SchemaTable, error) {
	var t models.SchemaTable
	var metadata []byte
	err := rows.Scan(
		&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
		&t.IsSelected, &t.RowCount, &t.BusinessName, &t.Description, &metadata,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan table: %w", err)
	}
	if err := json.Unmarshal(metadata, &t.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &t, nil
}

func scanSchemaTableRow(row pgx.Row) (*models.SchemaTable, error) {
	var t models.SchemaTable
	var metadata []byte
	err := row.Scan(
		&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
		&t.IsSelected, &t.RowCount, &t.BusinessName, &t.Description, &metadata,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metadata, &t.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &t, nil
}

func scanSchemaColumn(rows pgx.Rows) (*models.SchemaColumn, error) {
	var c models.SchemaColumn
	var metadata []byte
	err := rows.Scan(
		&c.ID, &c.ProjectID, &c.SchemaTableID, &c.ColumnName, &c.DataType,
		&c.IsNullable, &c.IsPrimaryKey, &c.IsUnique, &c.IsSelected, &c.OrdinalPosition,
		&c.DefaultValue, &c.DistinctCount, &c.NullCount, &c.BusinessName, &c.Description, &metadata,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan column: %w", err)
	}
	if err := json.Unmarshal(metadata, &c.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &c, nil
}

func scanSchemaColumnRow(row pgx.Row) (*models.SchemaColumn, error) {
	var c models.SchemaColumn
	var metadata []byte
	err := row.Scan(
		&c.ID, &c.ProjectID, &c.SchemaTableID, &c.ColumnName, &c.DataType,
		&c.IsNullable, &c.IsPrimaryKey, &c.IsUnique, &c.IsSelected, &c.OrdinalPosition,
		&c.DefaultValue, &c.DistinctCount, &c.NullCount, &c.BusinessName, &c.Description, &metadata,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(metadata, &c.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &c, nil
}

func scanSchemaRelationship(rows pgx.Rows) (*models.SchemaRelationship, error) {
	var rel models.SchemaRelationship
	var validationResultsJSON []byte
	err := rows.Scan(
		&rel.ID, &rel.ProjectID, &rel.SourceTableID, &rel.SourceColumnID,
		&rel.TargetTableID, &rel.TargetColumnID, &rel.RelationshipType,
		&rel.Cardinality, &rel.Confidence, &rel.InferenceMethod, &rel.IsValidated,
		&validationResultsJSON, &rel.IsApproved, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan relationship: %w", err)
	}
	if len(validationResultsJSON) > 0 {
		rel.ValidationResults = &models.ValidationResults{}
		if err := json.Unmarshal(validationResultsJSON, rel.ValidationResults); err != nil {
			return nil, fmt.Errorf("failed to unmarshal validation_results: %w", err)
		}
	}
	return &rel, nil
}

func scanSchemaRelationshipRow(row pgx.Row) (*models.SchemaRelationship, error) {
	var rel models.SchemaRelationship
	var validationResultsJSON []byte
	err := row.Scan(
		&rel.ID, &rel.ProjectID, &rel.SourceTableID, &rel.SourceColumnID,
		&rel.TargetTableID, &rel.TargetColumnID, &rel.RelationshipType,
		&rel.Cardinality, &rel.Confidence, &rel.InferenceMethod, &rel.IsValidated,
		&validationResultsJSON, &rel.IsApproved, &rel.CreatedAt, &rel.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if len(validationResultsJSON) > 0 {
		rel.ValidationResults = &models.ValidationResults{}
		if err := json.Unmarshal(validationResultsJSON, rel.ValidationResults); err != nil {
			return nil, fmt.Errorf("failed to unmarshal validation_results: %w", err)
		}
	}
	return &rel, nil
}

// scanSchemaRelationshipRowWithDiscovery scans a relationship row including discovery fields.
func scanSchemaRelationshipRowWithDiscovery(row pgx.Row) (*models.SchemaRelationship, error) {
	var rel models.SchemaRelationship
	var validationResultsJSON []byte
	err := row.Scan(
		&rel.ID, &rel.ProjectID, &rel.SourceTableID, &rel.SourceColumnID,
		&rel.TargetTableID, &rel.TargetColumnID, &rel.RelationshipType,
		&rel.Cardinality, &rel.Confidence, &rel.InferenceMethod, &rel.IsValidated,
		&validationResultsJSON, &rel.IsApproved, &rel.CreatedAt, &rel.UpdatedAt,
		&rel.MatchRate, &rel.SourceDistinct, &rel.TargetDistinct, &rel.MatchedCount, &rel.RejectionReason,
	)
	if err != nil {
		return nil, err
	}
	if len(validationResultsJSON) > 0 {
		rel.ValidationResults = &models.ValidationResults{}
		if err := json.Unmarshal(validationResultsJSON, rel.ValidationResults); err != nil {
			return nil, fmt.Errorf("failed to unmarshal validation_results: %w", err)
		}
	}
	return &rel, nil
}

// scanSchemaColumnWithDiscovery scans a column row including discovery fields.
func scanSchemaColumnWithDiscovery(rows pgx.Rows) (*models.SchemaColumn, error) {
	var c models.SchemaColumn
	var metadata []byte
	err := rows.Scan(
		&c.ID, &c.ProjectID, &c.SchemaTableID, &c.ColumnName, &c.DataType,
		&c.IsNullable, &c.IsPrimaryKey, &c.IsSelected, &c.OrdinalPosition,
		&c.DistinctCount, &c.NullCount, &c.BusinessName, &c.Description, &metadata,
		&c.CreatedAt, &c.UpdatedAt,
		&c.RowCount, &c.NonNullCount, &c.IsJoinable, &c.JoinabilityReason, &c.StatsUpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan column with discovery: %w", err)
	}
	if err := json.Unmarshal(metadata, &c.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}
	return &c, nil
}
