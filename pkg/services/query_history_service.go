package services

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// QueryHistoryService provides operations for MCP query history (learning).
type QueryHistoryService interface {
	Record(ctx context.Context, entry *models.QueryHistoryEntry) error
	List(ctx context.Context, projectID uuid.UUID, filters models.QueryHistoryFilters) ([]*models.QueryHistoryEntry, int, error)
	RecordFeedback(ctx context.Context, projectID uuid.UUID, entryID uuid.UUID, userID string, feedback string, comment *string) error
	PruneOlderThan(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error)
}

type queryHistoryService struct {
	repo   repositories.QueryHistoryRepository
	logger *zap.Logger
}

func NewQueryHistoryService(repo repositories.QueryHistoryRepository, logger *zap.Logger) QueryHistoryService {
	return &queryHistoryService{
		repo:   repo,
		logger: logger.Named("query-history-service"),
	}
}

var _ QueryHistoryService = (*queryHistoryService)(nil)

func (s *queryHistoryService) Record(ctx context.Context, entry *models.QueryHistoryEntry) error {
	// Classify the query before recording
	classifyQuery(entry)

	err := s.repo.Create(ctx, entry)
	if err != nil {
		s.logger.Error("Failed to record query history entry",
			zap.String("project_id", entry.ProjectID.String()),
			zap.String("user_id", entry.UserID),
			zap.Error(err))
		return err
	}
	return nil
}

func (s *queryHistoryService) List(ctx context.Context, projectID uuid.UUID, filters models.QueryHistoryFilters) ([]*models.QueryHistoryEntry, int, error) {
	entries, total, err := s.repo.List(ctx, projectID, filters)
	if err != nil {
		s.logger.Error("Failed to list query history entries",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, 0, err
	}
	return entries, total, nil
}

func (s *queryHistoryService) RecordFeedback(ctx context.Context, projectID uuid.UUID, entryID uuid.UUID, userID string, feedback string, comment *string) error {
	if feedback != "helpful" && feedback != "not_helpful" {
		return fmt.Errorf("invalid feedback value: %s (must be 'helpful' or 'not_helpful')", feedback)
	}

	err := s.repo.UpdateFeedback(ctx, projectID, entryID, userID, feedback, comment)
	if err != nil {
		s.logger.Error("Failed to record query feedback",
			zap.String("project_id", projectID.String()),
			zap.String("entry_id", entryID.String()),
			zap.Error(err))
		return err
	}
	return nil
}

func (s *queryHistoryService) PruneOlderThan(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error) {
	count, err := s.repo.DeleteOlderThan(ctx, projectID, cutoff)
	if err != nil {
		s.logger.Error("Failed to prune query history",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return 0, err
	}
	return count, nil
}

// classifyQuery extracts classification metadata from the SQL query.
// This sets query_type, tables_used, and aggregations_used on the entry.
func classifyQuery(entry *models.QueryHistoryEntry) {
	sql := strings.ToUpper(entry.SQL)

	// Extract tables used
	entry.TablesUsed = extractTablesFromSQL(entry.SQL)

	// Extract aggregations used
	entry.AggregationsUsed = extractAggregations(sql)

	// Classify query type
	queryType := classifyQueryType(sql)
	if queryType != "" {
		entry.QueryType = &queryType
	}
}

// extractTablesFromSQL extracts table names from SQL FROM and JOIN clauses.
var tableRefPattern = regexp.MustCompile(`(?i)\b(?:FROM|JOIN)\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)?)`)

func extractTablesFromSQL(sql string) []string {
	matches := tableRefPattern.FindAllStringSubmatch(sql, -1)
	seen := make(map[string]bool)
	var tables []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		tableName := strings.ToLower(match[1])
		// Skip subquery aliases and common SQL keywords
		if tableName == "select" || tableName == "lateral" {
			continue
		}
		if !seen[tableName] {
			seen[tableName] = true
			tables = append(tables, tableName)
		}
	}

	return tables
}

var aggregationPattern = regexp.MustCompile(`\b(COUNT|SUM|AVG|MIN|MAX|ARRAY_AGG|STRING_AGG|BOOL_AND|BOOL_OR)\s*\(`)

func extractAggregations(sqlUpper string) []string {
	matches := aggregationPattern.FindAllStringSubmatch(sqlUpper, -1)
	seen := make(map[string]bool)
	var aggs []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		agg := match[1]
		if !seen[agg] {
			seen[agg] = true
			aggs = append(aggs, agg)
		}
	}

	return aggs
}

func classifyQueryType(sqlUpper string) string {
	hasAgg := aggregationPattern.MatchString(sqlUpper)
	hasGroupBy := strings.Contains(sqlUpper, "GROUP BY")

	if hasAgg || hasGroupBy {
		return "aggregation"
	}

	hasWhere := strings.Contains(sqlUpper, "WHERE")
	hasLimit := strings.Contains(sqlUpper, "LIMIT")

	// Simple lookup: has specific WHERE conditions and returns limited results
	if hasWhere && hasLimit {
		return "lookup"
	}

	// Report: has ORDER BY but no LIMIT (implies full result set)
	hasOrderBy := strings.Contains(sqlUpper, "ORDER BY")
	if hasOrderBy && !hasLimit {
		return "report"
	}

	return "exploration"
}
