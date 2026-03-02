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

// PendingChangeRepository provides data access for pending ontology changes.
type PendingChangeRepository interface {
	// Create inserts a single pending change.
	Create(ctx context.Context, change *models.PendingChange) error

	// CreateBatch inserts multiple pending changes efficiently.
	CreateBatch(ctx context.Context, changes []*models.PendingChange) error

	// List returns pending changes for a project filtered by status.
	List(ctx context.Context, projectID uuid.UUID, status string, limit int) ([]*models.PendingChange, error)

	// ListByType returns pending changes filtered by change type.
	ListByType(ctx context.Context, projectID uuid.UUID, changeType string, limit int) ([]*models.PendingChange, error)

	// GetByID returns a single pending change by ID.
	GetByID(ctx context.Context, changeID uuid.UUID) (*models.PendingChange, error)

	// UpdateStatus updates the status of a pending change.
	UpdateStatus(ctx context.Context, changeID uuid.UUID, status, reviewedBy string) error

	// DeleteByProject removes all pending changes for a project.
	DeleteByProject(ctx context.Context, projectID uuid.UUID) error

	// CountByStatus returns counts of pending changes grouped by status.
	CountByStatus(ctx context.Context, projectID uuid.UUID) (map[string]int, error)
}

type pendingChangeRepository struct{}

// NewPendingChangeRepository creates a new PendingChangeRepository.
func NewPendingChangeRepository() PendingChangeRepository {
	return &pendingChangeRepository{}
}

var _ PendingChangeRepository = (*pendingChangeRepository)(nil)

func (r *pendingChangeRepository) Create(ctx context.Context, change *models.PendingChange) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		INSERT INTO engine_ontology_pending_changes (
			project_id, change_type, change_source, table_name, column_name,
			old_value, new_value, suggested_action, suggested_payload, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at`

	now := time.Now()
	err := scope.Conn.QueryRow(ctx, query,
		change.ProjectID,
		change.ChangeType,
		change.ChangeSource,
		nullableString(change.TableName),
		nullableString(change.ColumnName),
		jsonbValueMap(change.OldValue),
		jsonbValueMap(change.NewValue),
		nullableString(change.SuggestedAction),
		jsonbValueMap(change.SuggestedPayload),
		change.Status,
		now,
	).Scan(&change.ID, &change.CreatedAt)
	if err != nil {
		return fmt.Errorf("failed to create pending change: %w", err)
	}

	return nil
}

func (r *pendingChangeRepository) CreateBatch(ctx context.Context, changes []*models.PendingChange) error {
	if len(changes) == 0 {
		return nil
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()

	batch := &pgx.Batch{}
	query := `
		INSERT INTO engine_ontology_pending_changes (
			project_id, change_type, change_source, table_name, column_name,
			old_value, new_value, suggested_action, suggested_payload, status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at`

	for _, change := range changes {
		batch.Queue(query,
			change.ProjectID,
			change.ChangeType,
			change.ChangeSource,
			nullableString(change.TableName),
			nullableString(change.ColumnName),
			jsonbValueMap(change.OldValue),
			jsonbValueMap(change.NewValue),
			nullableString(change.SuggestedAction),
			jsonbValueMap(change.SuggestedPayload),
			change.Status,
			now,
		)
	}

	results := scope.Conn.SendBatch(ctx, batch)
	defer results.Close()

	for i := range changes {
		err := results.QueryRow().Scan(&changes[i].ID, &changes[i].CreatedAt)
		if err != nil {
			return fmt.Errorf("failed to create pending change %d: %w", i, err)
		}
	}

	return nil
}

func (r *pendingChangeRepository) List(ctx context.Context, projectID uuid.UUID, status string, limit int) ([]*models.PendingChange, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if limit <= 0 {
		limit = 10000
	}

	query := `
		SELECT id, project_id, change_type, change_source, table_name, column_name,
		       old_value, new_value, suggested_action, suggested_payload,
		       status, reviewed_by, reviewed_at, created_at
		FROM engine_ontology_pending_changes
		WHERE project_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := scope.Conn.Query(ctx, query, projectID, status, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending changes: %w", err)
	}
	defer rows.Close()

	return scanPendingChanges(rows)
}

func (r *pendingChangeRepository) ListByType(ctx context.Context, projectID uuid.UUID, changeType string, limit int) ([]*models.PendingChange, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	if limit <= 0 {
		limit = 10000
	}

	query := `
		SELECT id, project_id, change_type, change_source, table_name, column_name,
		       old_value, new_value, suggested_action, suggested_payload,
		       status, reviewed_by, reviewed_at, created_at
		FROM engine_ontology_pending_changes
		WHERE project_id = $1 AND change_type = $2
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := scope.Conn.Query(ctx, query, projectID, changeType, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending changes by type: %w", err)
	}
	defer rows.Close()

	return scanPendingChanges(rows)
}

func (r *pendingChangeRepository) GetByID(ctx context.Context, changeID uuid.UUID) (*models.PendingChange, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, change_type, change_source, table_name, column_name,
		       old_value, new_value, suggested_action, suggested_payload,
		       status, reviewed_by, reviewed_at, created_at
		FROM engine_ontology_pending_changes
		WHERE id = $1`

	row := scope.Conn.QueryRow(ctx, query, changeID)
	return scanPendingChange(row)
}

func (r *pendingChangeRepository) UpdateStatus(ctx context.Context, changeID uuid.UUID, status, reviewedBy string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	query := `
		UPDATE engine_ontology_pending_changes
		SET status = $2, reviewed_by = $3, reviewed_at = $4
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, changeID, status, reviewedBy, now)
	if err != nil {
		return fmt.Errorf("failed to update pending change status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("pending change not found")
	}

	return nil
}

func (r *pendingChangeRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_ontology_pending_changes WHERE project_id = $1`
	_, err := scope.Conn.Exec(ctx, query, projectID)
	if err != nil {
		return fmt.Errorf("failed to delete pending changes: %w", err)
	}

	return nil
}

func (r *pendingChangeRepository) CountByStatus(ctx context.Context, projectID uuid.UUID) (map[string]int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT status, COUNT(*) as count
		FROM engine_ontology_pending_changes
		WHERE project_id = $1
		GROUP BY status`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending changes: %w", err)
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan count: %w", err)
		}
		counts[status] = count
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating counts: %w", err)
	}

	return counts, nil
}

// Helper functions

func scanPendingChanges(rows pgx.Rows) ([]*models.PendingChange, error) {
	var changes []*models.PendingChange
	for rows.Next() {
		change, err := scanPendingChangeFromRows(rows)
		if err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating pending changes: %w", err)
	}

	return changes, nil
}

func scanPendingChangeFromRows(rows pgx.Rows) (*models.PendingChange, error) {
	var c models.PendingChange
	var tableName, columnName, suggestedAction, reviewedBy *string
	var oldValue, newValue, suggestedPayload []byte

	err := rows.Scan(
		&c.ID,
		&c.ProjectID,
		&c.ChangeType,
		&c.ChangeSource,
		&tableName,
		&columnName,
		&oldValue,
		&newValue,
		&suggestedAction,
		&suggestedPayload,
		&c.Status,
		&reviewedBy,
		&c.ReviewedAt,
		&c.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan pending change: %w", err)
	}

	// Handle nullable fields
	if tableName != nil {
		c.TableName = *tableName
	}
	if columnName != nil {
		c.ColumnName = *columnName
	}
	if suggestedAction != nil {
		c.SuggestedAction = *suggestedAction
	}
	if reviewedBy != nil {
		c.ReviewedBy = reviewedBy
	}

	// Unmarshal JSONB fields
	if len(oldValue) > 0 && string(oldValue) != "null" {
		if err := json.Unmarshal(oldValue, &c.OldValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal old_value: %w", err)
		}
	}
	if len(newValue) > 0 && string(newValue) != "null" {
		if err := json.Unmarshal(newValue, &c.NewValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal new_value: %w", err)
		}
	}
	if len(suggestedPayload) > 0 && string(suggestedPayload) != "null" {
		if err := json.Unmarshal(suggestedPayload, &c.SuggestedPayload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal suggested_payload: %w", err)
		}
	}

	return &c, nil
}

func scanPendingChange(row pgx.Row) (*models.PendingChange, error) {
	var c models.PendingChange
	var tableName, columnName, suggestedAction, reviewedBy *string
	var oldValue, newValue, suggestedPayload []byte

	err := row.Scan(
		&c.ID,
		&c.ProjectID,
		&c.ChangeType,
		&c.ChangeSource,
		&tableName,
		&columnName,
		&oldValue,
		&newValue,
		&suggestedAction,
		&suggestedPayload,
		&c.Status,
		&reviewedBy,
		&c.ReviewedAt,
		&c.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan pending change: %w", err)
	}

	// Handle nullable fields
	if tableName != nil {
		c.TableName = *tableName
	}
	if columnName != nil {
		c.ColumnName = *columnName
	}
	if suggestedAction != nil {
		c.SuggestedAction = *suggestedAction
	}
	if reviewedBy != nil {
		c.ReviewedBy = reviewedBy
	}

	// Unmarshal JSONB fields
	if len(oldValue) > 0 && string(oldValue) != "null" {
		if err := json.Unmarshal(oldValue, &c.OldValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal old_value: %w", err)
		}
	}
	if len(newValue) > 0 && string(newValue) != "null" {
		if err := json.Unmarshal(newValue, &c.NewValue); err != nil {
			return nil, fmt.Errorf("failed to unmarshal new_value: %w", err)
		}
	}
	if len(suggestedPayload) > 0 && string(suggestedPayload) != "null" {
		if err := json.Unmarshal(suggestedPayload, &c.SuggestedPayload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal suggested_payload: %w", err)
		}
	}

	return &c, nil
}

// jsonbValueMap converts a map to JSONB format for database insertion.
func jsonbValueMap(v map[string]any) any {
	if v == nil || len(v) == 0 {
		return nil
	}
	return v
}
