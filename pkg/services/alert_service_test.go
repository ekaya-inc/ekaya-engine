package services

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockAlertRepo implements repositories.AlertRepository for testing.
type mockAlertRepo struct {
	alerts    []*models.AuditAlert
	createErr error
	resolveErr error
	listErr   error
	getErr    error
}

func (m *mockAlertRepo) ListAlerts(_ context.Context, projectID uuid.UUID, filters models.AlertFilters) ([]*models.AuditAlert, int, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}

	var result []*models.AuditAlert
	for _, a := range m.alerts {
		if a.ProjectID != projectID {
			continue
		}
		if filters.Status != "" && a.Status != filters.Status {
			continue
		}
		if filters.Severity != "" && a.Severity != filters.Severity {
			continue
		}
		result = append(result, a)
	}
	return result, len(result), nil
}

func (m *mockAlertRepo) GetAlertByID(_ context.Context, projectID uuid.UUID, alertID uuid.UUID) (*models.AuditAlert, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	for _, a := range m.alerts {
		if a.ProjectID == projectID && a.ID == alertID {
			return a, nil
		}
	}
	return nil, nil
}

func (m *mockAlertRepo) CreateAlert(_ context.Context, alert *models.AuditAlert) error {
	if m.createErr != nil {
		return m.createErr
	}
	alert.ID = uuid.New()
	alert.CreatedAt = time.Now()
	alert.UpdatedAt = time.Now()
	m.alerts = append(m.alerts, alert)
	return nil
}

func (m *mockAlertRepo) ResolveAlert(_ context.Context, projectID uuid.UUID, alertID uuid.UUID, resolvedBy string, status string, notes string) error {
	if m.resolveErr != nil {
		return m.resolveErr
	}
	for _, a := range m.alerts {
		if a.ProjectID == projectID && a.ID == alertID && a.Status == models.AlertStatusOpen {
			a.Status = status
			a.ResolvedBy = &resolvedBy
			now := time.Now()
			a.ResolvedAt = &now
			a.ResolutionNotes = &notes
			return nil
		}
	}
	return fmt.Errorf("alert not found or already resolved")
}

func TestAlertService_CreateAlert_Valid(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	alert := &models.AuditAlert{
		ProjectID: uuid.New(),
		AlertType: models.AlertTypeSQLInjection,
		Severity:  models.AlertSeverityCritical,
		Title:     "SQL injection detected",
		Status:    models.AlertStatusOpen,
	}

	err := svc.CreateAlert(context.Background(), alert)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, alert.ID)
	assert.Len(t, repo.alerts, 1)
}

func TestAlertService_CreateAlert_MissingTitle(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	alert := &models.AuditAlert{
		ProjectID: uuid.New(),
		AlertType: models.AlertTypeSQLInjection,
		Severity:  models.AlertSeverityCritical,
	}

	err := svc.CreateAlert(context.Background(), alert)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "title is required")
}

func TestAlertService_CreateAlert_MissingAlertType(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	alert := &models.AuditAlert{
		ProjectID: uuid.New(),
		Severity:  models.AlertSeverityCritical,
		Title:     "Test alert",
	}

	err := svc.CreateAlert(context.Background(), alert)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alert type is required")
}

func TestAlertService_CreateAlert_InvalidSeverity(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	alert := &models.AuditAlert{
		ProjectID: uuid.New(),
		AlertType: models.AlertTypeSQLInjection,
		Severity:  "invalid",
		Title:     "Test alert",
	}

	err := svc.CreateAlert(context.Background(), alert)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid severity")
}

func TestAlertService_CreateAlert_DefaultsStatusToOpen(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	alert := &models.AuditAlert{
		ProjectID: uuid.New(),
		AlertType: models.AlertTypeSQLInjection,
		Severity:  models.AlertSeverityCritical,
		Title:     "Test alert",
	}

	err := svc.CreateAlert(context.Background(), alert)
	require.NoError(t, err)
	assert.Equal(t, models.AlertStatusOpen, alert.Status)
}

func TestAlertService_ResolveAlert_Valid(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	projectID := uuid.New()
	alertID := uuid.New()
	repo.alerts = append(repo.alerts, &models.AuditAlert{
		ID:        alertID,
		ProjectID: projectID,
		Status:    models.AlertStatusOpen,
	})

	err := svc.ResolveAlert(context.Background(), projectID, alertID, "user-123", "resolved", "Fixed the issue")
	require.NoError(t, err)
	assert.Equal(t, models.AlertStatusResolved, repo.alerts[0].Status)
}

func TestAlertService_ResolveAlert_Dismiss(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	projectID := uuid.New()
	alertID := uuid.New()
	repo.alerts = append(repo.alerts, &models.AuditAlert{
		ID:        alertID,
		ProjectID: projectID,
		Status:    models.AlertStatusOpen,
	})

	err := svc.ResolveAlert(context.Background(), projectID, alertID, "user-123", "dismissed", "False positive")
	require.NoError(t, err)
	assert.Equal(t, models.AlertStatusDismissed, repo.alerts[0].Status)
}

func TestAlertService_ResolveAlert_InvalidResolution(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	err := svc.ResolveAlert(context.Background(), uuid.New(), uuid.New(), "user-123", "invalid", "notes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid resolution")
}

func TestAlertService_ResolveAlert_MissingResolvedBy(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	err := svc.ResolveAlert(context.Background(), uuid.New(), uuid.New(), "", "resolved", "notes")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolved_by is required")
}

func TestAlertService_ListAlerts_FilterByStatus(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	projectID := uuid.New()
	repo.alerts = append(repo.alerts,
		&models.AuditAlert{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityCritical},
		&models.AuditAlert{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusResolved, Severity: models.AlertSeverityWarning},
		&models.AuditAlert{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityInfo},
	)

	alerts, total, err := svc.ListAlerts(context.Background(), projectID, models.AlertFilters{Status: models.AlertStatusOpen})
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, alerts, 2)
}

func TestAlertService_ListAlerts_FilterBySeverity(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	projectID := uuid.New()
	repo.alerts = append(repo.alerts,
		&models.AuditAlert{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityCritical},
		&models.AuditAlert{ID: uuid.New(), ProjectID: projectID, Status: models.AlertStatusOpen, Severity: models.AlertSeverityWarning},
	)

	alerts, total, err := svc.ListAlerts(context.Background(), projectID, models.AlertFilters{Severity: models.AlertSeverityCritical})
	require.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Len(t, alerts, 1)
}

func TestAlertService_ListAlerts_InvalidStatusFilter(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	_, _, err := svc.ListAlerts(context.Background(), uuid.New(), models.AlertFilters{Status: "invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid status filter")
}

func TestAlertService_ListAlerts_InvalidSeverityFilter(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	_, _, err := svc.ListAlerts(context.Background(), uuid.New(), models.AlertFilters{Severity: "invalid"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid severity filter")
}

func TestAlertService_GetAlertByID(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	projectID := uuid.New()
	alertID := uuid.New()
	repo.alerts = append(repo.alerts, &models.AuditAlert{
		ID:        alertID,
		ProjectID: projectID,
		Title:     "Test alert",
	})

	alert, err := svc.GetAlertByID(context.Background(), projectID, alertID)
	require.NoError(t, err)
	require.NotNil(t, alert)
	assert.Equal(t, alertID, alert.ID)
}

func TestAlertService_GetAlertByID_NotFound(t *testing.T) {
	repo := &mockAlertRepo{}
	svc := NewAlertService(repo, zap.NewNop())

	alert, err := svc.GetAlertByID(context.Background(), uuid.New(), uuid.New())
	require.NoError(t, err)
	assert.Nil(t, alert)
}
