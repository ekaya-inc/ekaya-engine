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

// NOTE: As of migration 020_schema_columns_refactor, engine_schema_columns no longer has:
// - business_name, description, metadata, is_sensitive, sample_values
// These semantic fields now live in engine_ontology_column_metadata.

// TableKey identifies a table uniquely within a datasource.
type TableKey struct {
	SchemaName string
	TableName  string
}

// SchemaRepository provides data access for schema discovery tables.
type SchemaRepository interface {
	// Tables
	// ListTablesByDatasource returns selected tables for a datasource.
	ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error)
	// ListAllTablesByDatasource returns all tables (including non-selected) for a datasource.
	// Use this only for schema management operations (refresh, UI display).
	ListAllTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error)
	GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error)
	GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error)
	// FindTableByName finds a table by name within a datasource (schema-agnostic).
	// Use this when the schema prefix is not known or relevant (e.g., ontology layer).
	FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error)
	UpsertTable(ctx context.Context, table *models.SchemaTable) error
	SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []TableKey) (int64, error)
	UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error
	// GetTablesByNames returns selected tables for a project filtered by table names, keyed by table name.
	// Used by ontology context to retrieve table-level metadata (e.g., row_count).
	GetTablesByNames(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string]*models.SchemaTable, error)

	// Columns
	// ListColumnsByTable returns selected columns for a specific table.
	ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error)
	// ListAllColumnsByTable returns all columns (including non-selected) for a specific table.
	// Use this only for schema management operations (sync, UI display, selection management).
	ListAllColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error)
	ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error)
	// GetColumnsByTables returns selected columns for multiple tables, grouped by table name.
	GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error)
	GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error)
	// GetTableCountByProject returns the count of selected, non-deleted tables for a project.
	GetTableCountByProject(ctx context.Context, projectID uuid.UUID) (int, error)
	// GetSelectedTableNamesByProject returns the names of all selected, non-deleted tables for a project.
	GetSelectedTableNamesByProject(ctx context.Context, projectID uuid.UUID) ([]string, error)
	GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error)
	GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error)
	UpsertColumn(ctx context.Context, column *models.SchemaColumn) error
	SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error)
	UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error
	UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64) error

	// Relationships
	ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error)
	GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error)
	GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error)
	UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error
	UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error
	SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error
	SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error)

	// GetRelationshipsByMethod returns relationships filtered by inference method.
	// Use this to query relationships discovered by a specific algorithm (e.g., "pk_match", "column_features", "fk").
	GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error)

	// Relationship Discovery
	GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error)
	GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error)
	GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error)
	UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error
	GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error)
	UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error
	GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error)
	// GetNonPKColumnsByExactType returns non-primary-key columns with exact data type match for review candidate discovery.
	GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error)

	// SelectAllTablesAndColumns marks all tables and columns for a datasource as selected.
	// Used after schema refresh to auto-select newly discovered tables.
	SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error

	// DeleteInferredRelationshipsByProject hard-deletes all relationships for a project.
	// This includes both inferred relationships (column_features, pk_match) and DB-declared FKs.
	// Returns the count of deleted relationships.
	// Used when deleting ontology to give a clean slate - DB-declared FKs will be
	// re-imported during the next schema refresh.
	DeleteInferredRelationshipsByProject(ctx context.Context, projectID uuid.UUID) (int64, error)
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

func (r *schemaRepository) listTablesByDatasourceInternal(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Build query - uuid.Nil means "all datasources"
	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name,
		       is_selected, row_count, created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1 AND deleted_at IS NULL`

	var args []any
	args = append(args, projectID)

	// Filter by datasource unless uuid.Nil (which means all datasources)
	if datasourceID != uuid.Nil {
		query += " AND datasource_id = $2"
		args = append(args, datasourceID)
	}

	if selectedOnly {
		query += " AND is_selected = true"
	}
	query += " ORDER BY schema_name, table_name"

	rows, err := scope.Conn.Query(ctx, query, args...)
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

// ListTablesByDatasource returns selected tables for a datasource (safe default).
func (r *schemaRepository) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return r.listTablesByDatasourceInternal(ctx, projectID, datasourceID, true)
}

// ListAllTablesByDatasource returns all tables (including non-selected) for a datasource.
func (r *schemaRepository) ListAllTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return r.listTablesByDatasourceInternal(ctx, projectID, datasourceID, false)
}

func (r *schemaRepository) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name,
		       is_selected, row_count, created_at, updated_at
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
		       is_selected, row_count, created_at, updated_at
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
		       is_selected, row_count, created_at, updated_at
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

	// First, try to reactivate a soft-deleted record
	reactivateQuery := `
		UPDATE engine_schema_tables
		SET deleted_at = NULL,
		    is_selected = $5,
		    row_count = $6,
		    updated_at = $7
		WHERE project_id = $1
		  AND datasource_id = $2
		  AND schema_name = $3
		  AND table_name = $4
		  AND deleted_at IS NOT NULL
		RETURNING id, created_at`

	var existingID uuid.UUID
	var existingCreatedAt time.Time
	err := scope.Conn.QueryRow(ctx, reactivateQuery,
		table.ProjectID, table.DatasourceID, table.SchemaName, table.TableName,
		table.IsSelected, table.RowCount, now,
	).Scan(&existingID, &existingCreatedAt)

	if err == nil {
		// Reactivated soft-deleted record
		table.ID = existingID
		table.CreatedAt = existingCreatedAt
		return nil
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("failed to reactivate table: %w", err)
	}

	// No soft-deleted record, do standard upsert on active records
	upsertQuery := `
		INSERT INTO engine_schema_tables (
			id, project_id, datasource_id, schema_name, table_name,
			is_selected, row_count, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (project_id, datasource_id, schema_name, table_name)
			WHERE deleted_at IS NULL
		DO UPDATE SET
			row_count = EXCLUDED.row_count,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, is_selected`

	err = scope.Conn.QueryRow(ctx, upsertQuery,
		table.ID, table.ProjectID, table.DatasourceID, table.SchemaName, table.TableName,
		table.IsSelected, table.RowCount, table.CreatedAt, table.UpdatedAt,
	).Scan(&table.ID, &table.CreatedAt, &table.IsSelected)

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

func (r *schemaRepository) GetTablesByNames(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string]*models.SchemaTable, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if len(tableNames) == 0 {
		return make(map[string]*models.SchemaTable), nil
	}

	query := `
		SELECT id, project_id, datasource_id, schema_name, table_name, is_selected,
		       row_count, created_at, updated_at
		FROM engine_schema_tables
		WHERE project_id = $1
		  AND table_name = ANY($2)
		  AND deleted_at IS NULL
		  AND is_selected = true
		ORDER BY table_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get tables by names: %w", err)
	}
	defer rows.Close()

	result := make(map[string]*models.SchemaTable)
	for rows.Next() {
		var t models.SchemaTable
		err := rows.Scan(
			&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
			&t.IsSelected, &t.RowCount, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan table: %w", err)
		}
		result[t.TableName] = &t
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating tables: %w", err)
	}

	return result, nil
}

// ============================================================================
// Column Methods
// ============================================================================

// ListColumnsByTable returns selected columns for a table.
func (r *schemaRepository) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return r.listColumnsByTableInternal(ctx, projectID, tableID, true)
}

// ListAllColumnsByTable returns all columns (including non-selected) for a table.
func (r *schemaRepository) ListAllColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return r.listColumnsByTableInternal(ctx, projectID, tableID, false)
}

func (r *schemaRepository) listColumnsByTableInternal(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id, column_name, data_type,
		       is_nullable, is_primary_key, is_unique, is_selected, ordinal_position,
		       default_value, distinct_count, null_count, min_length, max_length,
		       created_at, updated_at
		FROM engine_schema_columns
		WHERE project_id = $1 AND schema_table_id = $2 AND deleted_at IS NULL`
	if selectedOnly {
		query += ` AND is_selected = true`
	}
	query += ` ORDER BY ordinal_position`

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

	// Include discovery fields (is_joinable, row_count, etc.) for relationship discovery algorithms
	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
		       c.created_at, c.updated_at,
		       c.row_count, c.non_null_count, c.is_joinable, c.joinability_reason, c.stats_updated_at
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
		c, err := scanSchemaColumnWithDiscovery(rows)
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

func (r *schemaRepository) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if len(tableNames) == 0 {
		return make(map[string][]*models.SchemaColumn), nil
	}

	query := `
		SELECT c.id, c.project_id, c.schema_table_id, c.column_name, c.data_type,
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
		       c.created_at, c.updated_at,
		       t.table_name
		FROM engine_schema_columns c
		JOIN engine_schema_tables t ON c.schema_table_id = t.id
		WHERE c.project_id = $1
		  AND t.table_name = ANY($2)
		  AND c.deleted_at IS NULL
		  AND t.deleted_at IS NULL
		  AND c.is_selected = true
		  AND t.is_selected = true
		ORDER BY t.table_name, c.ordinal_position`

	rows, err := scope.Conn.Query(ctx, query, projectID, tableNames)
	if err != nil {
		return nil, fmt.Errorf("failed to get columns by tables: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]*models.SchemaColumn)
	for rows.Next() {
		var c models.SchemaColumn
		var tableName string
		err := rows.Scan(
			&c.ID, &c.ProjectID, &c.SchemaTableID, &c.ColumnName, &c.DataType,
			&c.IsNullable, &c.IsPrimaryKey, &c.IsUnique, &c.IsSelected, &c.OrdinalPosition,
			&c.DefaultValue, &c.DistinctCount, &c.NullCount, &c.MinLength, &c.MaxLength,
			&c.CreatedAt, &c.UpdatedAt,
			&tableName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan column: %w", err)
		}
		result[tableName] = append(result[tableName], &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating columns: %w", err)
	}

	return result, nil
}

func (r *schemaRepository) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT COUNT(*)
		FROM engine_schema_columns c
		JOIN engine_schema_tables t ON c.schema_table_id = t.id
		WHERE c.project_id = $1
		  AND c.deleted_at IS NULL
		  AND t.deleted_at IS NULL`

	var count int
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get column count: %w", err)
	}

	return count, nil
}

func (r *schemaRepository) GetTableCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT COUNT(*)
		FROM engine_schema_tables
		WHERE project_id = $1
		  AND deleted_at IS NULL
		  AND is_selected = true`

	var count int
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get table count: %w", err)
	}

	return count, nil
}

func (r *schemaRepository) GetSelectedTableNamesByProject(ctx context.Context, projectID uuid.UUID) ([]string, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT table_name
		FROM engine_schema_tables
		WHERE project_id = $1
		  AND deleted_at IS NULL
		  AND is_selected = true
		ORDER BY table_name`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get selected table names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan table name: %w", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating table names: %w", err)
	}

	return names, nil
}

func (r *schemaRepository) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, schema_table_id, column_name, data_type,
		       is_nullable, is_primary_key, is_unique, is_selected, ordinal_position,
		       default_value, distinct_count, null_count, min_length, max_length,
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
		       default_value, distinct_count, null_count, min_length, max_length,
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
		    is_selected = $10,
		    updated_at = $11
		WHERE schema_table_id = $1
		  AND column_name = $2
		  AND project_id = $3
		  AND deleted_at IS NOT NULL
		RETURNING id, created_at, distinct_count, null_count`

	var existingID uuid.UUID
	var existingCreatedAt time.Time
	var existingDistinctCount, existingNullCount *int64
	err := scope.Conn.QueryRow(ctx, reactivateQuery,
		column.SchemaTableID, column.ColumnName, column.ProjectID,
		column.DataType, column.IsNullable, column.IsPrimaryKey, column.IsUnique, column.OrdinalPosition,
		column.DefaultValue, column.IsSelected, now,
	).Scan(&existingID, &existingCreatedAt,
		&existingDistinctCount, &existingNullCount)

	if err == nil {
		// Reactivated soft-deleted record - preserve stats
		column.ID = existingID
		column.CreatedAt = existingCreatedAt
		column.DistinctCount = existingDistinctCount
		column.NullCount = existingNullCount
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
			default_value, distinct_count, null_count,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (schema_table_id, column_name)
			WHERE deleted_at IS NULL
		DO UPDATE SET
			data_type = EXCLUDED.data_type,
			is_nullable = EXCLUDED.is_nullable,
			is_primary_key = EXCLUDED.is_primary_key,
			is_unique = EXCLUDED.is_unique,
			ordinal_position = EXCLUDED.ordinal_position,
			default_value = EXCLUDED.default_value,
			updated_at = EXCLUDED.updated_at
		RETURNING id, created_at, is_selected, distinct_count, null_count`

	err = scope.Conn.QueryRow(ctx, upsertQuery,
		column.ID, column.ProjectID, column.SchemaTableID, column.ColumnName, column.DataType,
		column.IsNullable, column.IsPrimaryKey, column.IsUnique, column.IsSelected, column.OrdinalPosition,
		column.DefaultValue, column.DistinctCount, column.NullCount,
		column.CreatedAt, column.UpdatedAt,
	).Scan(&column.ID, &column.CreatedAt, &column.IsSelected,
		&column.DistinctCount, &column.NullCount)

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

func (r *schemaRepository) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_columns
		SET distinct_count = COALESCE($2, distinct_count),
		    null_count = COALESCE($3, null_count),
		    min_length = COALESCE($4, min_length),
		    max_length = COALESCE($5, max_length),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, columnID, distinctCount, nullCount, minLength, maxLength)
	if err != nil {
		return fmt.Errorf("failed to update column stats: %w", err)
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

	// Check if a soft-deleted record exists with the same column IDs.
	// If so, respect the user's deletion and skip the insert.
	// The reset mechanism is: if a column is deleted and re-added, it gets a new UUID,
	// so the soft-deleted record won't match and the relationship can be rediscovered.
	var softDeletedExists bool
	checkQuery := `
		SELECT EXISTS(
			SELECT 1 FROM engine_schema_relationships
			WHERE source_column_id = $1
			  AND target_column_id = $2
			  AND deleted_at IS NOT NULL
		)`
	if err := scope.Conn.QueryRow(ctx, checkQuery, rel.SourceColumnID, rel.TargetColumnID).Scan(&softDeletedExists); err != nil {
		return fmt.Errorf("failed to check for soft-deleted relationship: %w", err)
	}
	if softDeletedExists {
		// User explicitly deleted this relationship - don't recreate it
		return nil
	}

	// No soft-deleted record exists, do standard upsert on active records
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

	err := scope.Conn.QueryRow(ctx, upsertQuery,
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

func (r *schemaRepository) GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT r.id, r.project_id, r.source_table_id, r.source_column_id,
		       r.target_table_id, r.target_column_id, r.relationship_type,
		       r.cardinality, r.confidence, r.inference_method, r.is_validated,
		       r.validation_results, r.is_approved, r.created_at, r.updated_at,
		       r.match_rate, r.source_distinct, r.target_distinct, r.matched_count, r.rejection_reason
		FROM engine_schema_relationships r
		JOIN engine_schema_tables st ON r.source_table_id = st.id
		WHERE r.project_id = $1 AND st.datasource_id = $2
		  AND r.inference_method = $3
		  AND r.deleted_at IS NULL AND st.deleted_at IS NULL
		ORDER BY r.created_at`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID, method)
	if err != nil {
		return nil, fmt.Errorf("failed to get relationships by method: %w", err)
	}
	defer rows.Close()

	relationships := make([]*models.SchemaRelationship, 0)
	for rows.Next() {
		rel, err := scanSchemaRelationshipWithDiscovery(rows)
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

// ============================================================================
// Relationship Discovery Methods
// ============================================================================

func (r *schemaRepository) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	// Build query - uuid.Nil means "all datasources"
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
		  AND r.deleted_at IS NULL
		  AND sc.deleted_at IS NULL
		  AND st.deleted_at IS NULL
		  AND tc.deleted_at IS NULL
		  AND tt.deleted_at IS NULL
		  AND r.rejection_reason IS NULL`

	var args []any
	args = append(args, projectID)

	// Filter by datasource unless uuid.Nil (which means all datasources)
	if datasourceID != uuid.Nil {
		query += " AND st.datasource_id = $2"
		args = append(args, datasourceID)
	}

	query += " ORDER BY st.schema_name, st.table_name, sc.column_name"

	rows, err := scope.Conn.Query(ctx, query, args...)
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

	// Build query - uuid.Nil means "all datasources"
	query := `
		SELECT table_name
		FROM engine_schema_tables
		WHERE project_id = $1
		  AND deleted_at IS NULL
		  AND (row_count IS NULL OR row_count = 0)`

	var args []any
	args = append(args, projectID)

	// Filter by datasource unless uuid.Nil (which means all datasources)
	if datasourceID != uuid.Nil {
		query += " AND datasource_id = $2"
		args = append(args, datasourceID)
	}

	query += " ORDER BY table_name"

	rows, err := scope.Conn.Query(ctx, query, args...)
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

	// Build query - uuid.Nil means "all datasources"
	// Tables with data (row_count > 0) but no active relationships
	query := `
		SELECT t.table_name
		FROM engine_schema_tables t
		WHERE t.project_id = $1
		  AND t.deleted_at IS NULL
		  AND t.row_count IS NOT NULL
		  AND t.row_count > 0
		  AND NOT EXISTS (
			  SELECT 1 FROM engine_schema_relationships r
			  WHERE r.deleted_at IS NULL
			    AND r.rejection_reason IS NULL
			    AND (r.source_table_id = t.id OR r.target_table_id = t.id)
		  )`

	var args []any
	args = append(args, projectID)

	// Filter by datasource unless uuid.Nil (which means all datasources)
	if datasourceID != uuid.Nil {
		query += " AND t.datasource_id = $2"
		args = append(args, datasourceID)
	}

	query += " ORDER BY t.table_name"

	rows, err := scope.Conn.Query(ctx, query, args...)
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

	// Check if a soft-deleted record exists with the same column IDs.
	// If so, respect the user's deletion and skip the insert.
	// The reset mechanism is: if a column is deleted and re-added, it gets a new UUID,
	// so the soft-deleted record won't match and the relationship can be rediscovered.
	var softDeletedExists bool
	checkQuery := `
		SELECT EXISTS(
			SELECT 1 FROM engine_schema_relationships
			WHERE source_column_id = $1
			  AND target_column_id = $2
			  AND deleted_at IS NOT NULL
		)`
	if err := scope.Conn.QueryRow(ctx, checkQuery, rel.SourceColumnID, rel.TargetColumnID).Scan(&softDeletedExists); err != nil {
		return fmt.Errorf("failed to check for soft-deleted relationship: %w", err)
	}
	if softDeletedExists {
		// User explicitly deleted this relationship - don't recreate it
		return nil
	}

	// No soft-deleted record exists, do standard upsert on active records
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

	err := scope.Conn.QueryRow(ctx, upsertQuery,
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
		       is_nullable, is_primary_key, is_unique, is_selected, ordinal_position,
		       default_value, distinct_count, null_count, min_length, max_length,
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

func (r *schemaRepository) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_schema_columns
		SET row_count = $2,
		    non_null_count = $3,
		    distinct_count = $4,
		    is_joinable = $5,
		    joinability_reason = $6,
		    stats_updated_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, query, columnID, rowCount, nonNullCount, distinctCount, isJoinable, joinabilityReason)
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
		       c.is_nullable, c.is_primary_key, c.is_unique, c.is_selected, c.ordinal_position,
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
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
		       c.default_value, c.distinct_count, c.null_count, c.min_length, c.max_length,
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
	err := rows.Scan(
		&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
		&t.IsSelected, &t.RowCount, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan table: %w", err)
	}
	return &t, nil
}

func scanSchemaTableRow(row pgx.Row) (*models.SchemaTable, error) {
	var t models.SchemaTable
	err := row.Scan(
		&t.ID, &t.ProjectID, &t.DatasourceID, &t.SchemaName, &t.TableName,
		&t.IsSelected, &t.RowCount, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanSchemaColumn(rows pgx.Rows) (*models.SchemaColumn, error) {
	var c models.SchemaColumn
	err := rows.Scan(
		&c.ID, &c.ProjectID, &c.SchemaTableID, &c.ColumnName, &c.DataType,
		&c.IsNullable, &c.IsPrimaryKey, &c.IsUnique, &c.IsSelected, &c.OrdinalPosition,
		&c.DefaultValue, &c.DistinctCount, &c.NullCount, &c.MinLength, &c.MaxLength,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan column: %w", err)
	}
	return &c, nil
}

func scanSchemaColumnRow(row pgx.Row) (*models.SchemaColumn, error) {
	var c models.SchemaColumn
	err := row.Scan(
		&c.ID, &c.ProjectID, &c.SchemaTableID, &c.ColumnName, &c.DataType,
		&c.IsNullable, &c.IsPrimaryKey, &c.IsUnique, &c.IsSelected, &c.OrdinalPosition,
		&c.DefaultValue, &c.DistinctCount, &c.NullCount, &c.MinLength, &c.MaxLength,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
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

// scanSchemaRelationshipWithDiscovery scans a relationship from rows including discovery fields.
func scanSchemaRelationshipWithDiscovery(rows pgx.Rows) (*models.SchemaRelationship, error) {
	var rel models.SchemaRelationship
	var validationResultsJSON []byte
	err := rows.Scan(
		&rel.ID, &rel.ProjectID, &rel.SourceTableID, &rel.SourceColumnID,
		&rel.TargetTableID, &rel.TargetColumnID, &rel.RelationshipType,
		&rel.Cardinality, &rel.Confidence, &rel.InferenceMethod, &rel.IsValidated,
		&validationResultsJSON, &rel.IsApproved, &rel.CreatedAt, &rel.UpdatedAt,
		&rel.MatchRate, &rel.SourceDistinct, &rel.TargetDistinct, &rel.MatchedCount, &rel.RejectionReason,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan relationship with discovery: %w", err)
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
	err := rows.Scan(
		&c.ID, &c.ProjectID, &c.SchemaTableID, &c.ColumnName, &c.DataType,
		&c.IsNullable, &c.IsPrimaryKey, &c.IsUnique, &c.IsSelected, &c.OrdinalPosition,
		&c.DefaultValue, &c.DistinctCount, &c.NullCount, &c.MinLength, &c.MaxLength,
		&c.CreatedAt, &c.UpdatedAt,
		&c.RowCount, &c.NonNullCount, &c.IsJoinable, &c.JoinabilityReason, &c.StatsUpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan column with discovery: %w", err)
	}
	return &c, nil
}

// SelectAllTablesAndColumns marks all tables and their columns as selected for a datasource.
func (r *schemaRepository) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Update all tables to selected (explicit project_id filter in addition to RLS)
	_, err := scope.Conn.Exec(ctx, `
		UPDATE engine_schema_tables
		SET is_selected = true, updated_at = NOW()
		WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
	`, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("failed to select all tables: %w", err)
	}

	// Update all columns to selected for the tables in this datasource
	_, err = scope.Conn.Exec(ctx, `
		UPDATE engine_schema_columns
		SET is_selected = true, updated_at = NOW()
		WHERE schema_table_id IN (
			SELECT id FROM engine_schema_tables
			WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
		) AND deleted_at IS NULL
	`, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("failed to select all columns: %w", err)
	}

	return nil
}

// DeleteInferredRelationshipsByProject hard-deletes inferred relationships for a project.
// Only deletes relationships with relationship_type = 'inferred' or 'review'.
// Preserves:
//   - 'fk' (DB-declared foreign keys from schema discovery)
//   - 'manual' (user-created relationships)
//
// This is a hard delete (not soft delete) because inferred relationships are re-generated
// during ontology extraction. Soft-deleted records would block re-discovery due to upsert key conflicts.
func (r *schemaRepository) DeleteInferredRelationshipsByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	// Only delete inferred and review relationships.
	// FK and manual relationships are preserved because they represent:
	// - FK: Database-declared constraints (source of truth is the DB schema)
	// - Manual: User-specified relationships (should persist across re-extractions)
	query := `
		DELETE FROM engine_schema_relationships
		WHERE project_id = $1
		  AND relationship_type IN ('inferred', 'review')`

	result, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return 0, fmt.Errorf("delete inferred relationships: %w", err)
	}

	return result.RowsAffected(), nil
}
