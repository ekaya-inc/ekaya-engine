package services

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// MCPAuditService provides operations for MCP audit event logging and querying.
type MCPAuditService interface {
	Record(ctx context.Context, event *models.MCPAuditEvent) error
	List(ctx context.Context, projectID uuid.UUID, filters models.MCPAuditEventFilters) ([]*models.MCPAuditEvent, int, error)
}

type mcpAuditService struct {
	repo   repositories.MCPAuditRepository
	logger *zap.Logger
}

func NewMCPAuditService(repo repositories.MCPAuditRepository, logger *zap.Logger) MCPAuditService {
	return &mcpAuditService{
		repo:   repo,
		logger: logger.Named("mcp-audit-service"),
	}
}

var _ MCPAuditService = (*mcpAuditService)(nil)

func (s *mcpAuditService) Record(ctx context.Context, event *models.MCPAuditEvent) error {
	err := s.repo.Create(ctx, event)
	if err != nil {
		s.logger.Error("Failed to record MCP audit event",
			zap.String("project_id", event.ProjectID.String()),
			zap.String("event_type", event.EventType),
			zap.Error(err))
		return err
	}
	return nil
}

func (s *mcpAuditService) List(ctx context.Context, projectID uuid.UUID, filters models.MCPAuditEventFilters) ([]*models.MCPAuditEvent, int, error) {
	events, total, err := s.repo.List(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list MCP audit events",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return events, total, nil
}
