package services

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AuditPageService provides operations for the audit page.
type AuditPageService interface {
	ListQueryExecutions(ctx context.Context, projectID uuid.UUID, filters models.QueryExecutionFilters) ([]*models.QueryExecutionRow, int, error)
	ListOntologyChanges(ctx context.Context, projectID uuid.UUID, filters models.OntologyChangeFilters) ([]*models.AuditLogEntry, int, error)
	ListSchemaChanges(ctx context.Context, projectID uuid.UUID, filters models.SchemaChangeFilters) ([]*models.PendingChange, int, error)
	ListQueryApprovals(ctx context.Context, projectID uuid.UUID, filters models.QueryApprovalFilters) ([]*models.Query, int, error)
	GetSummary(ctx context.Context, projectID uuid.UUID) (*models.AuditSummary, error)
	ListMCPEvents(ctx context.Context, projectID uuid.UUID, filters models.MCPAuditEventFilters) ([]*models.MCPAuditEvent, int, error)
}

type auditPageService struct {
	repo         repositories.AuditPageRepository
	mcpAuditRepo repositories.MCPAuditRepository
	logger       *zap.Logger
}

func NewAuditPageService(repo repositories.AuditPageRepository, mcpAuditRepo repositories.MCPAuditRepository, logger *zap.Logger) AuditPageService {
	return &auditPageService{
		repo:         repo,
		mcpAuditRepo: mcpAuditRepo,
		logger:       logger.Named("audit-page-service"),
	}
}

var _ AuditPageService = (*auditPageService)(nil)

func (s *auditPageService) ListQueryExecutions(ctx context.Context, projectID uuid.UUID, filters models.QueryExecutionFilters) ([]*models.QueryExecutionRow, int, error) {
	results, total, err := s.repo.ListQueryExecutions(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list query executions",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return results, total, nil
}

func (s *auditPageService) ListOntologyChanges(ctx context.Context, projectID uuid.UUID, filters models.OntologyChangeFilters) ([]*models.AuditLogEntry, int, error) {
	results, total, err := s.repo.ListOntologyChanges(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list ontology changes",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return results, total, nil
}

func (s *auditPageService) ListSchemaChanges(ctx context.Context, projectID uuid.UUID, filters models.SchemaChangeFilters) ([]*models.PendingChange, int, error) {
	results, total, err := s.repo.ListSchemaChanges(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list schema changes",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return results, total, nil
}

func (s *auditPageService) ListQueryApprovals(ctx context.Context, projectID uuid.UUID, filters models.QueryApprovalFilters) ([]*models.Query, int, error) {
	results, total, err := s.repo.ListQueryApprovals(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list query approvals",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return results, total, nil
}

func (s *auditPageService) GetSummary(ctx context.Context, projectID uuid.UUID) (*models.AuditSummary, error) {
	summary, err := s.repo.GetSummary(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get audit summary",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return summary, nil
}

func (s *auditPageService) ListMCPEvents(ctx context.Context, projectID uuid.UUID, filters models.MCPAuditEventFilters) ([]*models.MCPAuditEvent, int, error) {
	events, total, err := s.mcpAuditRepo.List(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list MCP events",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return events, total, nil
}
