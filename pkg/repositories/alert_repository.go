package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AlertRepository provides data access for audit alerts.
type AlertRepository interface {
	ListAlerts(ctx context.Context, projectID uuid.UUID, filters models.AlertFilters) ([]*models.AuditAlert, int, error)
	GetAlertByID(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID) (*models.AuditAlert, error)
	CreateAlert(ctx context.Context, alert *models.AuditAlert) error
	ResolveAlert(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID, resolvedBy string, status string, notes string) error
}

type alertRepository struct{}

func NewAlertRepository() AlertRepository {
	return &alertRepository{}
}

var _ AlertRepository = (*alertRepository)(nil)

func (r *alertRepository) ListAlerts(ctx context.Context, projectID uuid.UUID, filters models.AlertFilters) ([]*models.AuditAlert, int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, 0, fmt.Errorf("no tenant scope in context")
	}

	limit, offset := normalizePageParams(filters.Limit, filters.Offset)

	conditions := []string{"project_id = $1"}
	args := []any{projectID}
	argIdx := 2

	if filters.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Severity != "" {
		conditions = append(conditions, fmt.Sprintf("severity = $%d", argIdx))
		args = append(args, filters.Severity)
		argIdx++
	}
	if filters.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}
	if filters.Until != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_mcp_audit_alerts WHERE %s`, where)
	var total int
	if err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count alerts: %w", err)
	}

	// Data
	dataQuery := fmt.Sprintf(`
		SELECT id, project_id, alert_type, severity, title, description,
		       affected_user_id, related_audit_ids, status, resolved_by,
		       resolved_at, resolution_notes, created_at, updated_at
		FROM engine_mcp_audit_alerts
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := scope.Conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []*models.AuditAlert
	for rows.Next() {
		alert, err := scanAlert(rows)
		if err != nil {
			return nil, 0, err
		}
		alerts = append(alerts, alert)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating alerts: %w", err)
	}

	return alerts, total, nil
}

func (r *alertRepository) GetAlertByID(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID) (*models.AuditAlert, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	row := scope.Conn.QueryRow(ctx, `
		SELECT id, project_id, alert_type, severity, title, description,
		       affected_user_id, related_audit_ids, status, resolved_by,
		       resolved_at, resolution_notes, created_at, updated_at
		FROM engine_mcp_audit_alerts
		WHERE project_id = $1 AND id = $2`, projectID, alertID)

	alert := &models.AuditAlert{}
	err := row.Scan(
		&alert.ID, &alert.ProjectID, &alert.AlertType, &alert.Severity,
		&alert.Title, &alert.Description, &alert.AffectedUserID, &alert.RelatedAuditIDs,
		&alert.Status, &alert.ResolvedBy, &alert.ResolvedAt, &alert.ResolutionNotes,
		&alert.CreatedAt, &alert.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get alert: %w", err)
	}

	return alert, nil
}

func (r *alertRepository) CreateAlert(ctx context.Context, alert *models.AuditAlert) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	err := scope.Conn.QueryRow(ctx, `
		INSERT INTO engine_mcp_audit_alerts (
			project_id, alert_type, severity, title, description,
			affected_user_id, related_audit_ids, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		alert.ProjectID, alert.AlertType, alert.Severity, alert.Title,
		alert.Description, alert.AffectedUserID, alert.RelatedAuditIDs, alert.Status,
	).Scan(&alert.ID, &alert.CreatedAt, &alert.UpdatedAt)

	if err != nil {
		return fmt.Errorf("failed to create alert: %w", err)
	}

	return nil
}

func (r *alertRepository) ResolveAlert(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID, resolvedBy string, status string, notes string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	tag, err := scope.Conn.Exec(ctx, `
		UPDATE engine_mcp_audit_alerts
		SET status = $3, resolved_by = $4, resolved_at = $5, resolution_notes = $6, updated_at = $5
		WHERE project_id = $1 AND id = $2 AND status = 'open'`,
		projectID, alertID, status, resolvedBy, now, notes,
	)
	if err != nil {
		return fmt.Errorf("failed to resolve alert: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("alert not found or already resolved")
	}

	return nil
}

func scanAlert(rows pgx.Rows) (*models.AuditAlert, error) {
	alert := &models.AuditAlert{}
	err := rows.Scan(
		&alert.ID, &alert.ProjectID, &alert.AlertType, &alert.Severity,
		&alert.Title, &alert.Description, &alert.AffectedUserID, &alert.RelatedAuditIDs,
		&alert.Status, &alert.ResolvedBy, &alert.ResolvedAt, &alert.ResolutionNotes,
		&alert.CreatedAt, &alert.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan alert: %w", err)
	}
	return alert, nil
}
