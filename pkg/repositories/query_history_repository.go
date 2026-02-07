package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// QueryHistoryRepository provides data access for the MCP query history.
type QueryHistoryRepository interface {
	Create(ctx context.Context, entry *models.QueryHistoryEntry) error
	List(ctx context.Context, projectID uuid.UUID, filters models.QueryHistoryFilters) ([]*models.QueryHistoryEntry, int, error)
	UpdateFeedback(ctx context.Context, projectID uuid.UUID, entryID uuid.UUID, userID string, feedback string, comment *string) error
	DeleteOlderThan(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error)
}

type queryHistoryRepository struct{}

func NewQueryHistoryRepository() QueryHistoryRepository {
	return &queryHistoryRepository{}
}

var _ QueryHistoryRepository = (*queryHistoryRepository)(nil)

func (r *queryHistoryRepository) Create(ctx context.Context, entry *models.QueryHistoryEntry) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	if entry.ID == uuid.Nil {
		entry.ID = uuid.New()
	}

	timeFiltersJSON, err := marshalJSONBany(entry.TimeFilters)
	if err != nil {
		return fmt.Errorf("failed to marshal time_filters: %w", err)
	}

	query := `
		INSERT INTO engine_mcp_query_history (
			id, project_id, user_id,
			natural_language, sql,
			executed_at, execution_duration_ms, row_count,
			user_feedback, feedback_comment,
			query_type, tables_used, aggregations_used, time_filters
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

	_, err = scope.Conn.Exec(ctx, query,
		entry.ID,
		entry.ProjectID,
		entry.UserID,
		entry.NaturalLanguage,
		entry.SQL,
		entry.ExecutedAt,
		entry.ExecutionDurationMs,
		entry.RowCount,
		entry.UserFeedback,
		entry.FeedbackComment,
		entry.QueryType,
		entry.TablesUsed,
		entry.AggregationsUsed,
		timeFiltersJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to create query history entry: %w", err)
	}

	return nil
}

func (r *queryHistoryRepository) List(ctx context.Context, projectID uuid.UUID, filters models.QueryHistoryFilters) ([]*models.QueryHistoryEntry, int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, 0, fmt.Errorf("no tenant scope in context")
	}

	limit := filters.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	conditions := []string{"project_id = $1"}
	args := []any{projectID}
	argIdx := 2

	// User ID is always required for privacy scoping
	if filters.UserID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		args = append(args, filters.UserID)
		argIdx++
	}

	if filters.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}

	if len(filters.TablesUsed) > 0 {
		conditions = append(conditions, fmt.Sprintf("tables_used && $%d", argIdx))
		args = append(args, filters.TablesUsed)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_mcp_query_history WHERE %s`, where)
	var total int
	if err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count query history entries: %w", err)
	}

	// Data
	dataQuery := fmt.Sprintf(`
		SELECT id, project_id, user_id,
		       natural_language, sql,
		       executed_at, execution_duration_ms, row_count,
		       user_feedback, feedback_comment,
		       query_type, tables_used, aggregations_used, time_filters,
		       created_at
		FROM engine_mcp_query_history
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d`, where, argIdx)

	args = append(args, limit)

	rows, err := scope.Conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list query history entries: %w", err)
	}
	defer rows.Close()

	var entries []*models.QueryHistoryEntry
	for rows.Next() {
		var entry models.QueryHistoryEntry
		var timeFiltersJSON []byte

		err := rows.Scan(
			&entry.ID,
			&entry.ProjectID,
			&entry.UserID,
			&entry.NaturalLanguage,
			&entry.SQL,
			&entry.ExecutedAt,
			&entry.ExecutionDurationMs,
			&entry.RowCount,
			&entry.UserFeedback,
			&entry.FeedbackComment,
			&entry.QueryType,
			&entry.TablesUsed,
			&entry.AggregationsUsed,
			&timeFiltersJSON,
			&entry.CreatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan query history entry: %w", err)
		}

		if len(timeFiltersJSON) > 0 && string(timeFiltersJSON) != "null" {
			var tf map[string]any
			if jsonErr := json.Unmarshal(timeFiltersJSON, &tf); jsonErr == nil {
				entry.TimeFilters = tf
			}
		}

		entries = append(entries, &entry)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating query history entries: %w", err)
	}

	return entries, total, nil
}

func (r *queryHistoryRepository) UpdateFeedback(ctx context.Context, projectID uuid.UUID, entryID uuid.UUID, userID string, feedback string, comment *string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_mcp_query_history
		SET user_feedback = $1, feedback_comment = $2
		WHERE id = $3 AND project_id = $4 AND user_id = $5`

	tag, err := scope.Conn.Exec(ctx, query, feedback, comment, entryID, projectID, userID)
	if err != nil {
		return fmt.Errorf("failed to update query history feedback: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return fmt.Errorf("query history entry not found or not owned by user")
	}

	return nil
}

func (r *queryHistoryRepository) DeleteOlderThan(ctx context.Context, projectID uuid.UUID, cutoff time.Time) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_mcp_query_history WHERE project_id = $1 AND created_at < $2`
	tag, err := scope.Conn.Exec(ctx, query, projectID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete old query history entries: %w", err)
	}

	return tag.RowsAffected(), nil
}

// marshalJSONBany marshals any value to JSON bytes, returning nil for nil values.
func marshalJSONBany(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	return json.Marshal(v)
}
