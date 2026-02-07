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

	// RunScheduler starts a background goroutine that prunes all projects on the given interval.
	// It runs immediately on startup, then repeats every interval.
	// Cancel the context to stop the scheduler.
	RunScheduler(ctx context.Context, interval time.Duration)
}

type retentionService struct {
	db               *database.DB
	queryHistoryRepo repositories.QueryHistoryRepository
	mcpAuditRepo     repositories.MCPAuditRepository
	mcpConfigRepo    repositories.MCPConfigRepository
	logger           *zap.Logger
}

func NewRetentionService(
	db *database.DB,
	queryHistoryRepo repositories.QueryHistoryRepository,
	mcpAuditRepo repositories.MCPAuditRepository,
	mcpConfigRepo repositories.MCPConfigRepository,
	logger *zap.Logger,
) RetentionService {
	return &retentionService{
		db:               db,
		queryHistoryRepo: queryHistoryRepo,
		mcpAuditRepo:     mcpAuditRepo,
		mcpConfigRepo:    mcpConfigRepo,
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

// RunScheduler starts a background loop that prunes old data for all projects.
func (s *retentionService) RunScheduler(ctx context.Context, interval time.Duration) {
	go func() {
		s.logger.Info("Retention scheduler started",
			zap.Duration("interval", interval),
			zap.Int("default_retention_days", DefaultRetentionDays))

		// Run immediately on startup, then at each interval
		s.pruneAllProjects(ctx)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				s.logger.Info("Retention scheduler stopped")
				return
			case <-ticker.C:
				s.pruneAllProjects(ctx)
			}
		}
	}()
}

// pruneAllProjects iterates all projects and prunes each one using its configured retention period.
func (s *retentionService) pruneAllProjects(ctx context.Context) {
	// List all project IDs using a connection without tenant scope (so RLS returns all rows)
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		s.logger.Error("Retention scheduler: failed to acquire connection", zap.Error(err))
		return
	}

	rows, err := scope.Conn.Query(ctx, `SELECT project_id, audit_retention_days FROM engine_mcp_config`)
	if err != nil {
		scope.Close()
		s.logger.Error("Retention scheduler: failed to list projects", zap.Error(err))
		return
	}

	type projectRetention struct {
		ID            uuid.UUID
		RetentionDays *int
	}
	var projects []projectRetention
	for rows.Next() {
		var pr projectRetention
		if err := rows.Scan(&pr.ID, &pr.RetentionDays); err != nil {
			s.logger.Error("Retention scheduler: failed to scan project", zap.Error(err))
			continue
		}
		projects = append(projects, pr)
	}
	rows.Close()
	scope.Close()

	if len(projects) == 0 {
		return
	}

	s.logger.Debug("Retention scheduler: pruning projects", zap.Int("count", len(projects)))

	for _, pr := range projects {
		if ctx.Err() != nil {
			return
		}

		days := DefaultRetentionDays
		if pr.RetentionDays != nil && *pr.RetentionDays > 0 {
			days = *pr.RetentionDays
		}

		if _, err := s.PruneProject(ctx, pr.ID, days); err != nil {
			s.logger.Error("Retention scheduler: failed to prune project",
				zap.String("project_id", pr.ID.String()),
				zap.Error(err))
		}
	}
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
