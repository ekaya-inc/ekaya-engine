package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// DefaultRetentionDays is the default retention period for audit and history data.
const DefaultRetentionDays = 90

// RetentionService handles cleanup of old audit and history data.
type RetentionService interface {
	// PruneProject removes records older than the retention period for a project.
	// Returns total number of records deleted across all tables.
	PruneProject(ctx context.Context, projectID uuid.UUID, retentionDays int) (int64, error)
}

type retentionService struct {
	db               *database.DB
	queryHistoryRepo repositories.QueryHistoryRepository
	mcpAuditRepo     repositories.MCPAuditRepository
	logger           *zap.Logger
}

func NewRetentionService(
	db *database.DB,
	queryHistoryRepo repositories.QueryHistoryRepository,
	mcpAuditRepo repositories.MCPAuditRepository,
	logger *zap.Logger,
) RetentionService {
	return &retentionService{
		db:               db,
		queryHistoryRepo: queryHistoryRepo,
		mcpAuditRepo:     mcpAuditRepo,
		logger:           logger.Named("retention-service"),
	}
}

var _ RetentionService = (*retentionService)(nil)

func (s *retentionService) PruneProject(ctx context.Context, projectID uuid.UUID, retentionDays int) (int64, error) {
	if retentionDays <= 0 {
		retentionDays = DefaultRetentionDays
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	var totalDeleted int64

	// Acquire tenant scope
	scope, err := s.db.WithTenant(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("failed to acquire tenant scope: %w", err)
	}
	defer scope.Close()

	tenantCtx := database.SetTenantScope(ctx, scope)

	// Prune query history
	historyDeleted, err := s.queryHistoryRepo.DeleteOlderThan(tenantCtx, projectID, cutoff)
	if err != nil {
		s.logger.Error("Failed to prune query history",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return totalDeleted, fmt.Errorf("failed to prune query history: %w", err)
	}
	totalDeleted += historyDeleted

	// Prune MCP audit log
	auditDeleted, err := s.pruneMCPAuditLog(tenantCtx, projectID, cutoff)
	if err != nil {
		s.logger.Error("Failed to prune MCP audit log",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return totalDeleted, fmt.Errorf("failed to prune MCP audit log: %w", err)
	}
	totalDeleted += auditDeleted

	// Prune query executions
	execDeleted, err := s.pruneQueryExecutions(tenantCtx, projectID, cutoff)
	if err != nil {
		s.logger.Error("Failed to prune query executions",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return totalDeleted, fmt.Errorf("failed to prune query executions: %w", err)
	}
	totalDeleted += execDeleted

	if totalDeleted > 0 {
		s.logger.Info("Retention cleanup completed",
			zap.String("project_id", projectID.String()),
			zap.Int("retention_days", retentionDays),
			zap.Int64("total_deleted", totalDeleted),
			zap.Int64("history_deleted", historyDeleted),
			zap.Int64("audit_deleted", auditDeleted),
			zap.Int64("executions_deleted", execDeleted))
	}

	return totalDeleted, nil
}

func (s *retentionService) pruneMCPAuditLog(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	tag, err := scope.Conn.Exec(ctx,
		`DELETE FROM engine_mcp_audit_log WHERE project_id = $1 AND created_at < $2`,
		projectID, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *retentionService) pruneQueryExecutions(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	tag, err := scope.Conn.Exec(ctx,
		`DELETE FROM engine_query_executions WHERE project_id = $1 AND executed_at < $2`,
		projectID, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
