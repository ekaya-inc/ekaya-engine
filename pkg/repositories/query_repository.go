package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// QueryRepository provides data access for saved queries.
type QueryRepository interface {
	// CRUD operations
	Create(ctx context.Context, query *models.Query) error
	GetByID(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error)
	ListByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error)
	Update(ctx context.Context, query *models.Query) error
	SoftDelete(ctx context.Context, projectID, queryID uuid.UUID) error

	// Filtering
	ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error)
	// HasEnabledQueries efficiently checks if any enabled queries exist (uses LIMIT 1).
	HasEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (bool, error)

	// Status management
	UpdateEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error

	// Usage tracking
	IncrementUsageCount(ctx context.Context, queryID uuid.UUID) error
}

type queryRepository struct{}

// NewQueryRepository creates a new QueryRepository.
func NewQueryRepository() QueryRepository {
	return &queryRepository{}
}

var _ QueryRepository = (*queryRepository)(nil)

func (r *queryRepository) Create(ctx context.Context, query *models.Query) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	query.ID = uuid.New()
	query.CreatedAt = now
	query.UpdatedAt = now

	sql := `
		INSERT INTO engine_queries (
			id, project_id, datasource_id, natural_language_prompt, additional_context,
			sql_query, dialect, is_enabled, parameters, output_columns, constraints,
			status, suggested_by, suggestion_context,
			usage_count, last_used_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)`

	_, err := scope.Conn.Exec(ctx, sql,
		query.ID, query.ProjectID, query.DatasourceID, query.NaturalLanguagePrompt, query.AdditionalContext,
		query.SQLQuery, query.Dialect, query.IsEnabled, query.Parameters, query.OutputColumns, query.Constraints,
		query.Status, query.SuggestedBy, query.SuggestionContext,
		query.UsageCount, query.LastUsedAt, query.CreatedAt, query.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create query: %w", err)
	}

	return nil
}

func (r *queryRepository) GetByID(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	sql := `
		SELECT id, project_id, datasource_id, natural_language_prompt, additional_context,
		       sql_query, dialect, is_enabled, parameters, output_columns, constraints,
		       status, suggested_by, suggestion_context,
		       usage_count, last_used_at, created_at, updated_at
		FROM engine_queries
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	row := scope.Conn.QueryRow(ctx, sql, projectID, queryID)
	q, err := scanQueryRow(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("query not found")
		}
		return nil, fmt.Errorf("failed to get query: %w", err)
	}

	return q, nil
}

func (r *queryRepository) ListByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	sql := `
		SELECT id, project_id, datasource_id, natural_language_prompt, additional_context,
		       sql_query, dialect, is_enabled, parameters, output_columns, constraints,
		       status, suggested_by, suggestion_context,
		       usage_count, last_used_at, created_at, updated_at
		FROM engine_queries
		WHERE project_id = $1 AND datasource_id = $2 AND deleted_at IS NULL
		ORDER BY created_at DESC`

	rows, err := scope.Conn.Query(ctx, sql, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list queries: %w", err)
	}
	defer rows.Close()

	queries := make([]*models.Query, 0)
	for rows.Next() {
		q, err := scanQuery(rows)
		if err != nil {
			return nil, err
		}
		queries = append(queries, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating queries: %w", err)
	}

	return queries, nil
}

func (r *queryRepository) Update(ctx context.Context, query *models.Query) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query.UpdatedAt = time.Now()

	sql := `
		UPDATE engine_queries
		SET natural_language_prompt = $3,
		    additional_context = $4,
		    sql_query = $5,
		    dialect = $6,
		    is_enabled = $7,
		    parameters = $8,
		    output_columns = $9,
		    constraints = $10,
		    status = $11,
		    suggested_by = $12,
		    suggestion_context = $13,
		    updated_at = $14
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, sql,
		query.ProjectID, query.ID,
		query.NaturalLanguagePrompt, query.AdditionalContext,
		query.SQLQuery, query.Dialect, query.IsEnabled, query.Parameters,
		query.OutputColumns, query.Constraints,
		query.Status, query.SuggestedBy, query.SuggestionContext,
		query.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to update query: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("query not found")
	}

	return nil
}

func (r *queryRepository) SoftDelete(ctx context.Context, projectID, queryID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	sql := `
		UPDATE engine_queries
		SET deleted_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, sql, projectID, queryID)
	if err != nil {
		return fmt.Errorf("failed to soft-delete query: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("query not found")
	}

	return nil
}

func (r *queryRepository) ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	sql := `
		SELECT id, project_id, datasource_id, natural_language_prompt, additional_context,
		       sql_query, dialect, is_enabled, parameters, output_columns, constraints,
		       status, suggested_by, suggestion_context,
		       usage_count, last_used_at, created_at, updated_at
		FROM engine_queries
		WHERE project_id = $1 AND datasource_id = $2 AND is_enabled = true AND deleted_at IS NULL
		ORDER BY created_at DESC`

	rows, err := scope.Conn.Query(ctx, sql, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled queries: %w", err)
	}
	defer rows.Close()

	queries := make([]*models.Query, 0)
	for rows.Next() {
		q, err := scanQuery(rows)
		if err != nil {
			return nil, err
		}
		queries = append(queries, q)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating queries: %w", err)
	}

	return queries, nil
}

func (r *queryRepository) HasEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	sql := `
		SELECT 1 FROM engine_queries
		WHERE project_id = $1 AND datasource_id = $2 AND is_enabled = true AND deleted_at IS NULL
		LIMIT 1`

	var exists int
	err := scope.Conn.QueryRow(ctx, sql, projectID, datasourceID).Scan(&exists)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to check enabled queries: %w", err)
	}

	return true, nil
}

func (r *queryRepository) UpdateEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	sql := `
		UPDATE engine_queries
		SET is_enabled = $3, updated_at = NOW()
		WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, sql, projectID, queryID, isEnabled)
	if err != nil {
		return fmt.Errorf("failed to update query enabled status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("query not found")
	}

	return nil
}

func (r *queryRepository) IncrementUsageCount(ctx context.Context, queryID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	sql := `
		UPDATE engine_queries
		SET usage_count = usage_count + 1,
		    last_used_at = NOW(),
		    updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := scope.Conn.Exec(ctx, sql, queryID)
	if err != nil {
		return fmt.Errorf("failed to increment usage count: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("query not found")
	}

	return nil
}

// ============================================================================
// Helper Functions - Scan
// ============================================================================

func scanQuery(rows pgx.Rows) (*models.Query, error) {
	var q models.Query
	err := rows.Scan(
		&q.ID, &q.ProjectID, &q.DatasourceID, &q.NaturalLanguagePrompt, &q.AdditionalContext,
		&q.SQLQuery, &q.Dialect, &q.IsEnabled, &q.Parameters, &q.OutputColumns, &q.Constraints,
		&q.Status, &q.SuggestedBy, &q.SuggestionContext,
		&q.UsageCount, &q.LastUsedAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan query: %w", err)
	}
	return &q, nil
}

func scanQueryRow(row pgx.Row) (*models.Query, error) {
	var q models.Query
	err := row.Scan(
		&q.ID, &q.ProjectID, &q.DatasourceID, &q.NaturalLanguagePrompt, &q.AdditionalContext,
		&q.SQLQuery, &q.Dialect, &q.IsEnabled, &q.Parameters, &q.OutputColumns, &q.Constraints,
		&q.Status, &q.SuggestedBy, &q.SuggestionContext,
		&q.UsageCount, &q.LastUsedAt, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &q, nil
}
