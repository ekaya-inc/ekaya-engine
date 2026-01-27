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

// AuditRepository provides data access for the unified audit log.
type AuditRepository interface {
	// Create inserts a new audit log entry.
	Create(ctx context.Context, entry *models.AuditLogEntry) error

	// GetByProject returns all audit log entries for a project, ordered by time (newest first).
	GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.AuditLogEntry, error)

	// GetByEntity returns all audit log entries for a specific entity.
	GetByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]*models.AuditLogEntry, error)

	// GetByUser returns all audit log entries triggered by a specific user.
	GetByUser(ctx context.Context, projectID uuid.UUID, userID uuid.UUID, limit int) ([]*models.AuditLogEntry, error)
}

type auditRepository struct{}

// NewAuditRepository creates a new AuditRepository.
func NewAuditRepository() AuditRepository {
	return &auditRepository{}
}

var _ AuditRepository = (*auditRepository)(nil)

func (r *auditRepository) Create(ctx context.Context, entry *models.AuditLogEntry) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}
	entry.CreatedAt = time.Now()

	// Convert changed_fields to JSONB
	var changedFieldsJSON []byte
	var err error
	if len(entry.ChangedFields) > 0 {
		changedFieldsJSON, err = json.Marshal(entry.ChangedFields)
		if err != nil {
			return fmt.Errorf("failed to marshal changed_fields: %w", err)
		}
	}

	query := `
		INSERT INTO engine_audit_log (
			id, project_id, entity_type, entity_id, action, source, user_id, changed_fields, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = scope.Conn.Exec(ctx, query,
		entry.ID,
		entry.ProjectID,
		entry.EntityType,
		entry.EntityID,
		entry.Action,
		entry.Source,
		entry.UserID,
		changedFieldsJSON,
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create audit log entry: %w", err)
	}

	return nil
}

func (r *auditRepository) GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.AuditLogEntry, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, entity_type, entity_id, action, source, user_id, changed_fields, created_at
		FROM engine_audit_log
		WHERE project_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := scope.Conn.Query(ctx, query, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log: %w", err)
	}
	defer rows.Close()

	var entries []*models.AuditLogEntry
	for rows.Next() {
		entry, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log entries: %w", err)
	}

	return entries, nil
}

func (r *auditRepository) GetByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]*models.AuditLogEntry, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, entity_type, entity_id, action, source, user_id, changed_fields, created_at
		FROM engine_audit_log
		WHERE entity_type = $1 AND entity_id = $2
		ORDER BY created_at DESC`

	rows, err := scope.Conn.Query(ctx, query, entityType, entityID)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log by entity: %w", err)
	}
	defer rows.Close()

	var entries []*models.AuditLogEntry
	for rows.Next() {
		entry, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log entries: %w", err)
	}

	return entries, nil
}

func (r *auditRepository) GetByUser(ctx context.Context, projectID uuid.UUID, userID uuid.UUID, limit int) ([]*models.AuditLogEntry, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, project_id, entity_type, entity_id, action, source, user_id, changed_fields, created_at
		FROM engine_audit_log
		WHERE project_id = $1 AND user_id = $2
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := scope.Conn.Query(ctx, query, projectID, userID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log by user: %w", err)
	}
	defer rows.Close()

	var entries []*models.AuditLogEntry
	for rows.Next() {
		entry, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log entries: %w", err)
	}

	return entries, nil
}

func scanAuditLogEntry(row pgx.Row) (*models.AuditLogEntry, error) {
	var entry models.AuditLogEntry
	var changedFieldsJSON []byte

	err := row.Scan(
		&entry.ID,
		&entry.ProjectID,
		&entry.EntityType,
		&entry.EntityID,
		&entry.Action,
		&entry.Source,
		&entry.UserID,
		&changedFieldsJSON,
		&entry.CreatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, err
		}
		return nil, fmt.Errorf("failed to scan audit log entry: %w", err)
	}

	// Unmarshal changed_fields if present
	if len(changedFieldsJSON) > 0 && string(changedFieldsJSON) != "null" {
		if err := json.Unmarshal(changedFieldsJSON, &entry.ChangedFields); err != nil {
			return nil, fmt.Errorf("failed to unmarshal changed_fields: %w", err)
		}
	}

	return &entry, nil
}
