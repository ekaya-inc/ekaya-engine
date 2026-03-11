package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ETLRepository provides data access for ETL load history.
type ETLRepository interface {
	// CreateLoadStatus inserts a new load status record.
	CreateLoadStatus(ctx context.Context, status *models.LoadStatus) error

	// UpdateLoadStatus updates an existing load status record.
	UpdateLoadStatus(ctx context.Context, status *models.LoadStatus) error

	// GetLoadStatus retrieves a load status by ID.
	GetLoadStatus(ctx context.Context, projectID, statusID uuid.UUID) (*models.LoadStatus, error)

	// ListLoadStatus returns load history for a project, optionally filtered by app ID.
	ListLoadStatus(ctx context.Context, projectID uuid.UUID, appID string, limit int) ([]*models.LoadStatus, error)
}

type etlRepository struct{}

// NewETLRepository creates a new ETL repository.
func NewETLRepository() ETLRepository {
	return &etlRepository{}
}

func (r *etlRepository) CreateLoadStatus(ctx context.Context, status *models.LoadStatus) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	if status.ID == uuid.Nil {
		status.ID = uuid.New()
	}

	errorsJSON, err := json.Marshal(status.Errors)
	if err != nil {
		errorsJSON = []byte("[]")
	}

	_, err = scope.Conn.Exec(ctx,
		`INSERT INTO engine_etl_load_history
			(id, project_id, app_id, file_name, table_name, rows_attempted, rows_loaded,
			 rows_skipped, errors, started_at, completed_at, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		status.ID, status.ProjectID, status.AppID, status.FileName, status.TableName,
		status.RowsAttempted, status.RowsLoaded, status.RowsSkipped, errorsJSON,
		status.StartedAt, status.CompletedAt, status.Status,
	)
	if err != nil {
		return fmt.Errorf("failed to create load status: %w", err)
	}
	return nil
}

func (r *etlRepository) UpdateLoadStatus(ctx context.Context, status *models.LoadStatus) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	errorsJSON, err := json.Marshal(status.Errors)
	if err != nil {
		errorsJSON = []byte("[]")
	}

	_, err = scope.Conn.Exec(ctx,
		`UPDATE engine_etl_load_history
		SET table_name = $1, rows_attempted = $2, rows_loaded = $3,
			rows_skipped = $4, errors = $5, completed_at = $6, status = $7
		WHERE id = $8 AND project_id = $9`,
		status.TableName, status.RowsAttempted, status.RowsLoaded,
		status.RowsSkipped, errorsJSON, status.CompletedAt, status.Status,
		status.ID, status.ProjectID,
	)
	if err != nil {
		return fmt.Errorf("failed to update load status: %w", err)
	}
	return nil
}

func (r *etlRepository) GetLoadStatus(ctx context.Context, projectID, statusID uuid.UUID) (*models.LoadStatus, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	row := scope.Conn.QueryRow(ctx,
		`SELECT id, project_id, app_id, file_name, table_name,
			rows_attempted, rows_loaded, rows_skipped, errors,
			started_at, completed_at, status
		FROM engine_etl_load_history
		WHERE id = $1 AND project_id = $2`,
		statusID, projectID,
	)

	return scanLoadStatus(row)
}

func (r *etlRepository) ListLoadStatus(ctx context.Context, projectID uuid.UUID, appID string, limit int) ([]*models.LoadStatus, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if limit <= 0 {
		limit = 50
	}

	var rows pgx.Rows
	var err error

	if appID != "" {
		rows, err = scope.Conn.Query(ctx,
			`SELECT id, project_id, app_id, file_name, table_name,
				rows_attempted, rows_loaded, rows_skipped, errors,
				started_at, completed_at, status
			FROM engine_etl_load_history
			WHERE project_id = $1 AND app_id = $2
			ORDER BY started_at DESC
			LIMIT $3`,
			projectID, appID, limit,
		)
	} else {
		rows, err = scope.Conn.Query(ctx,
			`SELECT id, project_id, app_id, file_name, table_name,
				rows_attempted, rows_loaded, rows_skipped, errors,
				started_at, completed_at, status
			FROM engine_etl_load_history
			WHERE project_id = $1
			ORDER BY started_at DESC
			LIMIT $2`,
			projectID, limit,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to list load status: %w", err)
	}
	defer rows.Close()

	var results []*models.LoadStatus
	for rows.Next() {
		status, err := scanLoadStatusRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, status)
	}
	return results, rows.Err()
}

func scanLoadStatus(row pgx.Row) (*models.LoadStatus, error) {
	var s models.LoadStatus
	var errorsJSON []byte
	err := row.Scan(
		&s.ID, &s.ProjectID, &s.AppID, &s.FileName, &s.TableName,
		&s.RowsAttempted, &s.RowsLoaded, &s.RowsSkipped, &errorsJSON,
		&s.StartedAt, &s.CompletedAt, &s.Status,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("load status not found")
		}
		return nil, fmt.Errorf("failed to scan load status: %w", err)
	}
	if len(errorsJSON) > 0 {
		_ = json.Unmarshal(errorsJSON, &s.Errors)
	}
	return &s, nil
}

func scanLoadStatusRow(rows pgx.Rows) (*models.LoadStatus, error) {
	var s models.LoadStatus
	var errorsJSON []byte
	err := rows.Scan(
		&s.ID, &s.ProjectID, &s.AppID, &s.FileName, &s.TableName,
		&s.RowsAttempted, &s.RowsLoaded, &s.RowsSkipped, &errorsJSON,
		&s.StartedAt, &s.CompletedAt, &s.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan load status: %w", err)
	}
	if len(errorsJSON) > 0 {
		_ = json.Unmarshal(errorsJSON, &s.Errors)
	}
	return &s, nil
}
