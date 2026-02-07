package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AlertService provides operations for audit alerts.
type AlertService interface {
	ListAlerts(ctx context.Context, projectID uuid.UUID, filters models.AlertFilters) ([]*models.AuditAlert, int, error)
	GetAlertByID(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID) (*models.AuditAlert, error)
	CreateAlert(ctx context.Context, alert *models.AuditAlert) error
	ResolveAlert(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID, resolvedBy string, resolution string, notes string) error
}

type alertService struct {
	repo   repositories.AlertRepository
	logger *zap.Logger
}

func NewAlertService(repo repositories.AlertRepository, logger *zap.Logger) AlertService {
	return &alertService{
		repo:   repo,
		logger: logger.Named("alert-service"),
	}
}

var _ AlertService = (*alertService)(nil)

func (s *alertService) ListAlerts(ctx context.Context, projectID uuid.UUID, filters models.AlertFilters) ([]*models.AuditAlert, int, error) {
	// Validate status filter if provided
	if filters.Status != "" {
		switch filters.Status {
		case models.AlertStatusOpen, models.AlertStatusResolved, models.AlertStatusDismissed:
			// valid
		default:
			return nil, 0, fmt.Errorf("invalid status filter: %s", filters.Status)
		}
	}

	// Validate severity filter if provided
	if filters.Severity != "" && !models.ValidAlertSeverity(filters.Severity) {
		return nil, 0, fmt.Errorf("invalid severity filter: %s", filters.Severity)
	}

	alerts, total, err := s.repo.ListAlerts(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list alerts",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return alerts, total, nil
}

func (s *alertService) GetAlertByID(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID) (*models.AuditAlert, error) {
	alert, err := s.repo.GetAlertByID(ctx, projectID, alertID)
	if err != nil {
		s.logger.Error("Failed to get alert",
			zap.String("project_id", projectID.String()),
			zap.String("alert_id", alertID.String()),
			zap.Error(err))
		return nil, err
	}
	return alert, nil
}

func (s *alertService) CreateAlert(ctx context.Context, alert *models.AuditAlert) error {
	if alert.Title == "" {
		return fmt.Errorf("alert title is required")
	}
	if alert.AlertType == "" {
		return fmt.Errorf("alert type is required")
	}
	if !models.ValidAlertSeverity(alert.Severity) {
		return fmt.Errorf("invalid severity: %s", alert.Severity)
	}
	if alert.Status == "" {
		alert.Status = models.AlertStatusOpen
	}

	if err := s.repo.CreateAlert(ctx, alert); err != nil {
		s.logger.Error("Failed to create alert",
			zap.String("project_id", alert.ProjectID.String()),
			zap.String("alert_type", alert.AlertType),
			zap.Error(err))
		return err
	}
	return nil
}

func (s *alertService) ResolveAlert(ctx context.Context, projectID uuid.UUID, alertID uuid.UUID, resolvedBy string, resolution string, notes string) error {
	if !models.ValidAlertResolution(resolution) {
		return fmt.Errorf("invalid resolution: %s (must be 'resolved' or 'dismissed')", resolution)
	}
	if resolvedBy == "" {
		return fmt.Errorf("resolved_by is required")
	}

	if err := s.repo.ResolveAlert(ctx, projectID, alertID, resolvedBy, resolution, notes); err != nil {
		s.logger.Error("Failed to resolve alert",
			zap.String("project_id", projectID.String()),
			zap.String("alert_id", alertID.String()),
			zap.Error(err))
		return err
	}
	return nil
}
