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

// InstalledAppRepository defines the interface for installed app data access.
type InstalledAppRepository interface {
	// List returns all installed apps for a project.
	List(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error)

	// Get returns a specific installed app (nil if not installed).
	Get(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error)

	// IsInstalled checks if an app is installed.
	IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error)

	// Install adds an app to the project.
	Install(ctx context.Context, app *models.InstalledApp) error

	// Uninstall removes an app from the project.
	Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error

	// UpdateSettings updates app-specific settings.
	UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error
}

// installedAppRepository implements InstalledAppRepository using PostgreSQL.
type installedAppRepository struct{}

// NewInstalledAppRepository creates a new installed app repository.
func NewInstalledAppRepository() InstalledAppRepository {
	return &installedAppRepository{}
}

// List returns all installed apps for a project.
func (r *installedAppRepository) List(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, app_id, installed_at, installed_by, settings
		FROM engine_installed_apps
		WHERE project_id = $1
		ORDER BY installed_at ASC`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed apps: %w", err)
	}
	defer rows.Close()

	var apps []*models.InstalledApp
	for rows.Next() {
		app, err := r.scanApp(rows)
		if err != nil {
			return nil, err
		}
		apps = append(apps, app)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating installed apps: %w", err)
	}

	return apps, nil
}

// Get returns a specific installed app (nil if not installed).
func (r *installedAppRepository) Get(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, app_id, installed_at, installed_by, settings
		FROM engine_installed_apps
		WHERE project_id = $1 AND app_id = $2`

	row := scope.Conn.QueryRow(ctx, query, projectID, appID)
	app, err := r.scanAppRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get installed app: %w", err)
	}

	return app, nil
}

// IsInstalled checks if an app is installed.
func (r *installedAppRepository) IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT EXISTS(
			SELECT 1 FROM engine_installed_apps
			WHERE project_id = $1 AND app_id = $2
		)`

	var exists bool
	err := scope.Conn.QueryRow(ctx, query, projectID, appID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check if app is installed: %w", err)
	}

	return exists, nil
}

// Install adds an app to the project.
func (r *installedAppRepository) Install(ctx context.Context, app *models.InstalledApp) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	settingsJSON, err := json.Marshal(app.Settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		INSERT INTO engine_installed_apps (id, project_id, app_id, installed_at, installed_by, settings)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err = scope.Conn.Exec(ctx, query,
		app.ID,
		app.ProjectID,
		app.AppID,
		app.InstalledAt,
		app.InstalledBy,
		settingsJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to install app: %w", err)
	}

	return nil
}

// Uninstall removes an app from the project.
func (r *installedAppRepository) Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_installed_apps WHERE project_id = $1 AND app_id = $2`

	result, err := scope.Conn.Exec(ctx, query, projectID, appID)
	if err != nil {
		return fmt.Errorf("failed to uninstall app: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("app not installed")
	}

	return nil
}

// UpdateSettings updates app-specific settings.
func (r *installedAppRepository) UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	query := `
		UPDATE engine_installed_apps
		SET settings = $3
		WHERE project_id = $1 AND app_id = $2`

	result, err := scope.Conn.Exec(ctx, query, projectID, appID, settingsJSON)
	if err != nil {
		return fmt.Errorf("failed to update settings: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("app not installed")
	}

	return nil
}

// scanApp scans a row from rows into an InstalledApp.
func (r *installedAppRepository) scanApp(rows pgx.Rows) (*models.InstalledApp, error) {
	var app models.InstalledApp
	var installedBy *string
	var settingsJSON []byte

	err := rows.Scan(
		&app.ID,
		&app.ProjectID,
		&app.AppID,
		&app.InstalledAt,
		&installedBy,
		&settingsJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan installed app: %w", err)
	}

	if installedBy != nil {
		app.InstalledBy = *installedBy
	}

	if err := json.Unmarshal(settingsJSON, &app.Settings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	return &app, nil
}

// scanAppRow scans a single row into an InstalledApp.
func (r *installedAppRepository) scanAppRow(row pgx.Row) (*models.InstalledApp, error) {
	var app models.InstalledApp
	var installedBy *string
	var settingsJSON []byte

	err := row.Scan(
		&app.ID,
		&app.ProjectID,
		&app.AppID,
		&app.InstalledAt,
		&installedBy,
		&settingsJSON,
	)
	if err != nil {
		return nil, err
	}

	if installedBy != nil {
		app.InstalledBy = *installedBy
	}

	if err := json.Unmarshal(settingsJSON, &app.Settings); err != nil {
		return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	return &app, nil
}

// Ensure installedAppRepository implements InstalledAppRepository at compile time.
var _ InstalledAppRepository = (*installedAppRepository)(nil)
