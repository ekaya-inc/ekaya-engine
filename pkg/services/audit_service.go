package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AuditService provides operations for logging changes to ontology objects.
// It automatically extracts provenance (source, user) from context.
type AuditService interface {
	// LogCreate logs the creation of an entity.
	LogCreate(ctx context.Context, projectID uuid.UUID, entityType string, entityID uuid.UUID) error

	// LogUpdate logs an update to an entity with the changed fields.
	LogUpdate(ctx context.Context, projectID uuid.UUID, entityType string, entityID uuid.UUID, changes map[string]models.FieldChange) error

	// LogDelete logs the deletion of an entity.
	LogDelete(ctx context.Context, projectID uuid.UUID, entityType string, entityID uuid.UUID) error

	// GetByProject returns audit log entries for a project.
	GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.AuditLogEntry, error)

	// GetByEntity returns audit log entries for a specific entity.
	GetByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]*models.AuditLogEntry, error)
}

type auditService struct {
	repo   repositories.AuditRepository
	logger *zap.Logger
}

// NewAuditService creates a new AuditService.
func NewAuditService(repo repositories.AuditRepository, logger *zap.Logger) AuditService {
	return &auditService{
		repo:   repo,
		logger: logger.Named("audit-service"),
	}
}

var _ AuditService = (*auditService)(nil)

func (s *auditService) LogCreate(ctx context.Context, projectID uuid.UUID, entityType string, entityID uuid.UUID) error {
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		// Log a warning but don't fail the operation - audit logging shouldn't break the main operation
		s.logger.Warn("No provenance context for audit log",
			zap.String("entity_type", entityType),
			zap.String("entity_id", entityID.String()),
			zap.String("action", models.AuditActionCreate))
		return nil
	}

	entry := &models.AuditLogEntry{
		ProjectID:  projectID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     models.AuditActionCreate,
		Source:     string(prov.Source),
		UserID:     &prov.UserID,
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		s.logger.Error("Failed to create audit log entry",
			zap.String("entity_type", entityType),
			zap.String("entity_id", entityID.String()),
			zap.String("action", models.AuditActionCreate),
			zap.Error(err))
		return fmt.Errorf("create audit log entry: %w", err)
	}

	return nil
}

func (s *auditService) LogUpdate(ctx context.Context, projectID uuid.UUID, entityType string, entityID uuid.UUID, changes map[string]models.FieldChange) error {
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		s.logger.Warn("No provenance context for audit log",
			zap.String("entity_type", entityType),
			zap.String("entity_id", entityID.String()),
			zap.String("action", models.AuditActionUpdate))
		return nil
	}

	entry := &models.AuditLogEntry{
		ProjectID:     projectID,
		EntityType:    entityType,
		EntityID:      entityID,
		Action:        models.AuditActionUpdate,
		Source:        string(prov.Source),
		UserID:        &prov.UserID,
		ChangedFields: changes,
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		s.logger.Error("Failed to create audit log entry",
			zap.String("entity_type", entityType),
			zap.String("entity_id", entityID.String()),
			zap.String("action", models.AuditActionUpdate),
			zap.Error(err))
		return fmt.Errorf("create audit log entry: %w", err)
	}

	return nil
}

func (s *auditService) LogDelete(ctx context.Context, projectID uuid.UUID, entityType string, entityID uuid.UUID) error {
	prov, ok := models.GetProvenance(ctx)
	if !ok {
		s.logger.Warn("No provenance context for audit log",
			zap.String("entity_type", entityType),
			zap.String("entity_id", entityID.String()),
			zap.String("action", models.AuditActionDelete))
		return nil
	}

	entry := &models.AuditLogEntry{
		ProjectID:  projectID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     models.AuditActionDelete,
		Source:     string(prov.Source),
		UserID:     &prov.UserID,
	}

	if err := s.repo.Create(ctx, entry); err != nil {
		s.logger.Error("Failed to create audit log entry",
			zap.String("entity_type", entityType),
			zap.String("entity_id", entityID.String()),
			zap.String("action", models.AuditActionDelete),
			zap.Error(err))
		return fmt.Errorf("create audit log entry: %w", err)
	}

	return nil
}

func (s *auditService) GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.AuditLogEntry, error) {
	if limit <= 0 {
		limit = 100 // Default limit
	}

	entries, err := s.repo.GetByProject(ctx, projectID, limit)
	if err != nil {
		s.logger.Error("Failed to get audit log entries by project",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get audit log entries: %w", err)
	}

	return entries, nil
}

func (s *auditService) GetByEntity(ctx context.Context, entityType string, entityID uuid.UUID) ([]*models.AuditLogEntry, error) {
	entries, err := s.repo.GetByEntity(ctx, entityType, entityID)
	if err != nil {
		s.logger.Error("Failed to get audit log entries by entity",
			zap.String("entity_type", entityType),
			zap.String("entity_id", entityID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("get audit log entries: %w", err)
	}

	return entries, nil
}
