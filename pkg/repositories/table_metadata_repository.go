package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TableMetadataRepository provides data access for table metadata.
type TableMetadataRepository interface {
	// Upsert creates or updates table metadata based on project_id + datasource_id + table_name.
	Upsert(ctx context.Context, meta *models.TableMetadata) error

	// Get retrieves table metadata by project, datasource, and table name.
	Get(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.TableMetadata, error)

	// List retrieves all table metadata for a datasource.
	List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.TableMetadata, error)

	// Delete removes table metadata by project, datasource, and table name.
	Delete(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) error
}

type tableMetadataRepository struct{}

// NewTableMetadataRepository creates a new TableMetadataRepository.
func NewTableMetadataRepository() TableMetadataRepository {
	return &tableMetadataRepository{}
}

var _ TableMetadataRepository = (*tableMetadataRepository)(nil)

func (r *tableMetadataRepository) Upsert(ctx context.Context, meta *models.TableMetadata) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Get provenance from context
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		return fmt.Errorf("provenance context required")
	}

	now := time.Now()

	// Default source to provenance source if not set
	if meta.Source == "" {
		meta.Source = prov.Source.String()
	}

	query := `
		INSERT INTO engine_table_metadata (
			project_id, datasource_id, table_name,
			description, usage_notes, is_ephemeral, preferred_alternative,
			source, created_by, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (project_id, datasource_id, table_name)
		DO UPDATE SET
			description = COALESCE(EXCLUDED.description, engine_table_metadata.description),
			usage_notes = COALESCE(EXCLUDED.usage_notes, engine_table_metadata.usage_notes),
			is_ephemeral = EXCLUDED.is_ephemeral,
			preferred_alternative = COALESCE(EXCLUDED.preferred_alternative, engine_table_metadata.preferred_alternative),
			last_edit_source = $11,
			updated_by = $12,
			updated_at = $13
		RETURNING id, created_at, updated_at`

	err := scope.Conn.QueryRow(ctx, query,
		meta.ProjectID,
		meta.DatasourceID,
		meta.TableName,
		meta.Description,
		meta.UsageNotes,
		meta.IsEphemeral,
		meta.PreferredAlternative,
		meta.Source,
		prov.UserID,
		now,
		prov.Source.String(),
		prov.UserID,
		now,
	).Scan(&meta.ID, &meta.CreatedAt, &meta.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to upsert table metadata: %w", err)
	}

	// Set provenance fields on the returned object
	meta.CreatedBy = &prov.UserID
	meta.LastEditSource = nil // Only set on actual update
	meta.UpdatedBy = nil

	return nil
}

func (r *tableMetadataRepository) Get(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.TableMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, table_name,
		       description, usage_notes, is_ephemeral, preferred_alternative,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_table_metadata
		WHERE project_id = $1 AND datasource_id = $2 AND table_name = $3`

	row := scope.Conn.QueryRow(ctx, query, projectID, datasourceID, tableName)
	return scanTableMetadata(row)
}

func (r *tableMetadataRepository) List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.TableMetadata, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, datasource_id, table_name,
		       description, usage_notes, is_ephemeral, preferred_alternative,
		       source, last_edit_source, created_by, updated_by, created_at, updated_at
		FROM engine_table_metadata
		WHERE project_id = $1 AND datasource_id = $2
		ORDER BY table_name`

	rows, err := scope.Conn.Query(ctx, query, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to query table metadata: %w", err)
	}
	defer rows.Close()

	return scanTableMetadataRows(rows)
}

func (r *tableMetadataRepository) Delete(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_table_metadata WHERE project_id = $1 AND datasource_id = $2 AND table_name = $3`
	_, err := scope.Conn.Exec(ctx, query, projectID, datasourceID, tableName)
	if err != nil {
		return fmt.Errorf("failed to delete table metadata: %w", err)
	}

	return nil
}

func scanTableMetadata(row pgx.Row) (*models.TableMetadata, error) {
	var m models.TableMetadata

	err := row.Scan(
		&m.ID, &m.ProjectID, &m.DatasourceID, &m.TableName,
		&m.Description, &m.UsageNotes, &m.IsEphemeral, &m.PreferredAlternative,
		&m.Source, &m.LastEditSource, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan table metadata: %w", err)
	}

	return &m, nil
}

func scanTableMetadataRows(rows pgx.Rows) ([]*models.TableMetadata, error) {
	var result []*models.TableMetadata
	for rows.Next() {
		var m models.TableMetadata

		err := rows.Scan(
			&m.ID, &m.ProjectID, &m.DatasourceID, &m.TableName,
			&m.Description, &m.UsageNotes, &m.IsEphemeral, &m.PreferredAlternative,
			&m.Source, &m.LastEditSource, &m.CreatedBy, &m.UpdatedBy, &m.CreatedAt, &m.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan table metadata: %w", err)
		}

		result = append(result, &m)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating table metadata: %w", err)
	}

	return result, nil
}
