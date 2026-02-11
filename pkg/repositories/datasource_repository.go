package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// DatasourceRepository defines the interface for datasource data access.
// Config is stored as encrypted TEXT - encryption/decryption is handled by the service layer.
type DatasourceRepository interface {
	// Create inserts a new datasource. Returns error if name already exists for project.
	Create(ctx context.Context, ds *models.Datasource, encryptedConfig string) error

	// GetByID retrieves a datasource by ID within a project. Returns the model and encrypted config.
	GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, string, error)

	// GetByName retrieves a datasource by project and name. Returns the model and encrypted config.
	GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, string, error)

	// List retrieves all datasources for a project. Returns models and their encrypted configs.
	List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, []string, error)

	// Update modifies an existing datasource.
	Update(ctx context.Context, id uuid.UUID, name, dsType, provider, encryptedConfig string) error

	// Rename updates only the name of a datasource.
	Rename(ctx context.Context, id uuid.UUID, name string) error

	// Delete removes a datasource by ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// GetProjectID retrieves the project_id for a datasource by ID.
	GetProjectID(ctx context.Context, id uuid.UUID) (uuid.UUID, error)
}

// datasourceRepository implements DatasourceRepository using PostgreSQL.
type datasourceRepository struct{}

// NewDatasourceRepository creates a new datasource repository.
func NewDatasourceRepository() DatasourceRepository {
	return &datasourceRepository{}
}

// Create inserts a new datasource.
// Currently enforces a one-datasource-per-project limit within a transaction.
func (r *datasourceRepository) Create(ctx context.Context, ds *models.Datasource, encryptedConfig string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	ds.CreatedAt = now
	ds.UpdatedAt = now

	// Use a transaction to check limit and insert atomically
	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback on defer is best-effort

	// Check if a datasource already exists for this project (one-datasource-per-project policy)
	var count int
	err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM engine_datasources WHERE project_id = $1", ds.ProjectID).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check existing datasources: %w", err)
	}
	if count > 0 {
		return apperrors.ErrDatasourceLimitReached
	}

	// Insert the new datasource
	query := `
		INSERT INTO engine_datasources (project_id, name, datasource_type, provider, datasource_config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`

	err = tx.QueryRow(ctx, query,
		ds.ProjectID,
		ds.Name,
		ds.DatasourceType,
		ds.Provider,
		encryptedConfig,
		ds.CreatedAt,
		ds.UpdatedAt,
	).Scan(&ds.ID)
	if err != nil {
		// Check for unique constraint violation (PostgreSQL error code 23505)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperrors.ErrConflict
		}
		return fmt.Errorf("failed to create datasource: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByID retrieves a datasource by ID within a project.
func (r *datasourceRepository) GetByID(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, string, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, "", fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, name, datasource_type, provider, datasource_config, created_at, updated_at
		FROM engine_datasources
		WHERE project_id = $1 AND id = $2`

	var ds models.Datasource
	var encryptedConfig string
	var provider *string
	err := scope.Conn.QueryRow(ctx, query, projectID, id).Scan(
		&ds.ID,
		&ds.ProjectID,
		&ds.Name,
		&ds.DatasourceType,
		&provider,
		&encryptedConfig,
		&ds.CreatedAt,
		&ds.UpdatedAt,
	)
	if provider != nil {
		ds.Provider = *provider
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, "", fmt.Errorf("datasource not found")
		}
		return nil, "", fmt.Errorf("failed to get datasource: %w", err)
	}

	return &ds, encryptedConfig, nil
}

// GetByName retrieves a datasource by project and name.
func (r *datasourceRepository) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, string, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, "", fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, name, datasource_type, provider, datasource_config, created_at, updated_at
		FROM engine_datasources
		WHERE project_id = $1 AND name = $2`

	var ds models.Datasource
	var encryptedConfig string
	var provider *string
	err := scope.Conn.QueryRow(ctx, query, projectID, name).Scan(
		&ds.ID,
		&ds.ProjectID,
		&ds.Name,
		&ds.DatasourceType,
		&provider,
		&encryptedConfig,
		&ds.CreatedAt,
		&ds.UpdatedAt,
	)
	if provider != nil {
		ds.Provider = *provider
	}
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, "", fmt.Errorf("datasource not found")
		}
		return nil, "", fmt.Errorf("failed to get datasource: %w", err)
	}

	return &ds, encryptedConfig, nil
}

// List retrieves all datasources for a project.
func (r *datasourceRepository) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, []string, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, name, datasource_type, provider, datasource_config, created_at, updated_at
		FROM engine_datasources
		WHERE project_id = $1
		ORDER BY created_at DESC`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list datasources: %w", err)
	}
	defer rows.Close()

	var datasources []*models.Datasource
	var encryptedConfigs []string
	for rows.Next() {
		var ds models.Datasource
		var encryptedConfig string
		var provider *string
		err := rows.Scan(
			&ds.ID,
			&ds.ProjectID,
			&ds.Name,
			&ds.DatasourceType,
			&provider,
			&encryptedConfig,
			&ds.CreatedAt,
			&ds.UpdatedAt,
		)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to scan datasource: %w", err)
		}
		if provider != nil {
			ds.Provider = *provider
		}
		datasources = append(datasources, &ds)
		encryptedConfigs = append(encryptedConfigs, encryptedConfig)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating datasources: %w", err)
	}

	return datasources, encryptedConfigs, nil
}

// Update modifies an existing datasource.
func (r *datasourceRepository) Update(ctx context.Context, id uuid.UUID, name, dsType, provider, encryptedConfig string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_datasources
		SET name = $2, datasource_type = $3, provider = $4, datasource_config = $5, updated_at = $6
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, name, dsType, provider, encryptedConfig, time.Now())
	if err != nil {
		return fmt.Errorf("failed to update datasource: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("datasource not found")
	}

	return nil
}

// Rename updates only the name of a datasource.
func (r *datasourceRepository) Rename(ctx context.Context, id uuid.UUID, name string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `UPDATE engine_datasources SET name = $2, updated_at = $3 WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id, name, time.Now())
	if err != nil {
		return fmt.Errorf("failed to rename datasource: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("datasource not found")
	}

	return nil
}

// Delete removes a datasource by ID.
func (r *datasourceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_datasources WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete datasource: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("datasource not found")
	}

	return nil
}

// GetProjectID retrieves the project_id for a datasource by ID.
func (r *datasourceRepository) GetProjectID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return uuid.Nil, fmt.Errorf("no tenant scope in context")
	}

	query := `SELECT project_id FROM engine_datasources WHERE id = $1`

	var projectID uuid.UUID
	err := scope.Conn.QueryRow(ctx, query, id).Scan(&projectID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return uuid.Nil, fmt.Errorf("datasource not found")
		}
		return uuid.Nil, fmt.Errorf("failed to get project_id: %w", err)
	}

	return projectID, nil
}

// Ensure datasourceRepository implements DatasourceRepository at compile time.
var _ DatasourceRepository = (*datasourceRepository)(nil)
