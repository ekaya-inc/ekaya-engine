package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
)

// AlertTriggerRepository provides counting queries used by the alert trigger service
// to evaluate volume-based alert conditions.
type AlertTriggerRepository interface {
	// CountUserEventsInWindow counts MCP audit events for a user within a time window.
	CountUserEventsInWindow(ctx context.Context, projectID uuid.UUID, userID string, since time.Time) (int, error)

	// CountUserErrorsWithMessage counts error events with a matching error message substring
	// from a specific user within a time window.
	CountUserErrorsWithMessage(ctx context.Context, projectID uuid.UUID, userID string, errSubstring string, since time.Time) (int, error)

	// GetUserFirstEventTime returns the timestamp of the user's earliest MCP audit event for a project.
	// Returns nil if the user has no events.
	GetUserFirstEventTime(ctx context.Context, projectID uuid.UUID, userID string) (*time.Time, error)

	// HasOpenAlertForUserAndType checks whether an open alert of the given type exists
	// for the specified user within the given time window (for idempotency).
	HasOpenAlertForUserAndType(ctx context.Context, projectID uuid.UUID, userID string, alertType string, since time.Time) (bool, error)

	// HasOpenAlertForType checks whether an open alert of the given type exists
	// (user-agnostic) within the given time window.
	HasOpenAlertForType(ctx context.Context, projectID uuid.UUID, alertType string, since time.Time) (bool, error)
}

type alertTriggerRepository struct{}

func NewAlertTriggerRepository() AlertTriggerRepository {
	return &alertTriggerRepository{}
}

var _ AlertTriggerRepository = (*alertTriggerRepository)(nil)

func (r *alertTriggerRepository) CountUserEventsInWindow(ctx context.Context, projectID uuid.UUID, userID string, since time.Time) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	var count int
	err := scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM engine_mcp_audit_log
		WHERE project_id = $1 AND user_id = $2 AND created_at >= $3`,
		projectID, userID, since,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count user events: %w", err)
	}
	return count, nil
}

func (r *alertTriggerRepository) CountUserErrorsWithMessage(ctx context.Context, projectID uuid.UUID, userID string, errSubstring string, since time.Time) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	var count int
	err := scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM engine_mcp_audit_log
		WHERE project_id = $1 AND user_id = $2
		  AND was_successful = false
		  AND error_message ILIKE '%' || $3 || '%'
		  AND created_at >= $4`,
		projectID, userID, errSubstring, since,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count user errors: %w", err)
	}
	return count, nil
}

func (r *alertTriggerRepository) GetUserFirstEventTime(ctx context.Context, projectID uuid.UUID, userID string) (*time.Time, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	var firstTime *time.Time
	err := scope.Conn.QueryRow(ctx, `
		SELECT MIN(created_at)
		FROM engine_mcp_audit_log
		WHERE project_id = $1 AND user_id = $2`,
		projectID, userID,
	).Scan(&firstTime)
	if err != nil {
		return nil, fmt.Errorf("failed to get user first event time: %w", err)
	}
	return firstTime, nil
}

func (r *alertTriggerRepository) HasOpenAlertForUserAndType(ctx context.Context, projectID uuid.UUID, userID string, alertType string, since time.Time) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	var exists bool
	err := scope.Conn.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM engine_mcp_audit_alerts
			WHERE project_id = $1
			  AND affected_user_id = $2
			  AND alert_type = $3
			  AND status = 'open'
			  AND created_at >= $4
		)`,
		projectID, userID, alertType, since,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check existing alert: %w", err)
	}
	return exists, nil
}

func (r *alertTriggerRepository) HasOpenAlertForType(ctx context.Context, projectID uuid.UUID, alertType string, since time.Time) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	var exists bool
	err := scope.Conn.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM engine_mcp_audit_alerts
			WHERE project_id = $1
			  AND alert_type = $2
			  AND status = 'open'
			  AND created_at >= $3
		)`,
		projectID, alertType, since,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check existing alert: %w", err)
	}
	return exists, nil
}
