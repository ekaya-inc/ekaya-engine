package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ColumnMetadataRepository provides data access for column metadata.
type ColumnMetadataRepository interface {
	// Upsert creates or updates column metadata based on project_id + table_name + column_name.
	Upsert(ctx context.Context, meta *models.ColumnMetadata) error

	// GetByTableColumn retrieves column metadata by table and column name.
	GetByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) (*models.ColumnMetadata, error)

	// GetByTable retrieves all column metadata for a table.
	GetByTable(ctx context.Context, projectID uuid.UUID, tableName string) ([]*models.ColumnMetadata, error)

	// GetByProject retrieves all column metadata for a project.
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error)

	// Delete removes column metadata by ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// DeleteByTableColumn removes column metadata by table and column name.
	DeleteByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) error
}

type columnMetadataRepository struct{}

// NewColumnMetadataRepository creates a new ColumnMetadataRepository.
func NewColumnMetadataRepository() ColumnMetadataRepository {
	return &columnMetadataRepository{}
}

var _ ColumnMetadataRepository = (*columnMetadataRepository)(nil)

func (r *columnMetadataRepository) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()

	// Default created_by to 'inference' if not set
	if meta.CreatedBy == "" {
		meta.CreatedBy = models.ProvenanceInferred
	}

	// Convert enum_values to JSON
	var enumValuesJSON []byte
	var err error
	if len(meta.EnumValues) > 0 {
		enumValuesJSON, err = json.Marshal(meta.EnumValues)
		if err != nil {
			return fmt.Errorf("failed to marshal enum_values: %w", err)
		}
	}

	query := `
		INSERT INTO engine_ontology_column_metadata (
			project_id, table_name, column_name,
			description, entity, role, enum_values,
			created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (project_id, table_name, column_name)
		DO UPDATE SET
			description = COALESCE(EXCLUDED.description, engine_ontology_column_metadata.description),
			entity = COALESCE(EXCLUDED.entity, engine_ontology_column_metadata.entity),
			role = COALESCE(EXCLUDED.role, engine_ontology_column_metadata.role),
			enum_values = COALESCE(EXCLUDED.enum_values, engine_ontology_column_metadata.enum_values),
			updated_by = $10,
			updated_at = $11
		RETURNING id, created_at, updated_at`

	err = scope.Conn.QueryRow(ctx, query,
		meta.ProjectID,
		meta.TableName,
		meta.ColumnName,
		meta.Description,
		meta.Entity,
		meta.Role,
		enumValuesJSON,
		meta.CreatedBy,
		now,
		meta.UpdatedBy,
		now,
	).Scan(&meta.ID, &meta.CreatedAt, &meta.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert column metadata: %w", err)
	}

	return nil
}

func (r *columnMetadataRepository) GetByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) (*models.ColumnMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, table_name, column_name,
		       description, entity, role, enum_values,
		       created_by, updated_by, created_at, updated_at
		FROM engine_ontology_column_metadata
		WHERE project_id = $1 AND table_name = $2 AND column_name = $3`

	row := scope.Conn.QueryRow(ctx, query, projectID, tableName, columnName)
	return scanColumnMetadata(row)
}

func (r *columnMetadataRepository) GetByTable(ctx context.Context, projectID uuid.UUID, tableName string) ([]*models.ColumnMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, table_name, column_name,
		       description, entity, role, enum_values,
		       created_by, updated_by, created_at, updated_at
		FROM engine_ontology_column_metadata
		WHERE project_id = $1 AND table_name = $2
		ORDER BY column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to query column metadata: %w", err)
	}
	defer rows.Close()

	return scanColumnMetadataRows(rows)
}

func (r *columnMetadataRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, table_name, column_name,
		       description, entity, role, enum_values,
		       created_by, updated_by, created_at, updated_at
		FROM engine_ontology_column_metadata
		WHERE project_id = $1
		ORDER BY table_name, column_name`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query column metadata: %w", err)
	}
	defer rows.Close()

	return scanColumnMetadataRows(rows)
}

func (r *columnMetadataRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_column_metadata WHERE id = $1`
	_, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete column metadata: %w", err)
	}

	return nil
}

func (r *columnMetadataRepository) DeleteByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_column_metadata WHERE project_id = $1 AND table_name = $2 AND column_name = $3`
	_, err := scope.Conn.Exec(ctx, query, projectID, tableName, columnName)
	if err != nil {
		return fmt.Errorf("failed to delete column metadata: %w", err)
	}

	return nil
}

func scanColumnMetadata(row pgx.Row) (*models.ColumnMetadata, error) {
	var m models.ColumnMetadata
	var enumValues []byte

	err := row.Scan(
		&m.ID, &m.ProjectID, &m.TableName, &m.ColumnName,
		&m.Description, &m.Entity, &m.Role, &enumValues,
		&m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan column metadata: %w", err)
	}

	// Unmarshal enum_values
	if len(enumValues) > 0 && string(enumValues) != "null" {
		if err := json.Unmarshal(enumValues, &m.EnumValues); err != nil {
			return nil, fmt.Errorf("failed to unmarshal enum_values: %w", err)
		}
	}

	return &m, nil
}

func scanColumnMetadataRows(rows pgx.Rows) ([]*models.ColumnMetadata, error) {
	var result []*models.ColumnMetadata
	for rows.Next() {
		var m models.ColumnMetadata
		var enumValues []byte

		err := rows.Scan(
			&m.ID, &m.ProjectID, &m.TableName, &m.ColumnName,
			&m.Description, &m.Entity, &m.Role, &enumValues,
			&m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan column metadata: %w", err)
		}

		// Unmarshal enum_values
		if len(enumValues) > 0 && string(enumValues) != "null" {
			if err := json.Unmarshal(enumValues, &m.EnumValues); err != nil {
				return nil, fmt.Errorf("failed to unmarshal enum_values: %w", err)
			}
		}

		result = append(result, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating column metadata: %w", err)
	}

	return result, nil
}
