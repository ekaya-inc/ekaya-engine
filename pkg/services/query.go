package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// QueryService orchestrates query management and execution.
type QueryService interface {
	// CRUD Operations
	Create(ctx context.Context, projectID, datasourceID uuid.UUID, req *CreateQueryRequest) (*models.Query, error)
	Get(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error)
	List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error)
	Update(ctx context.Context, projectID, queryID uuid.UUID, req *UpdateQueryRequest) (*models.Query, error)
	Delete(ctx context.Context, projectID, queryID uuid.UUID) error

	// Filtering
	ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error)

	// Status Management
	SetEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error

	// Query Execution
	Execute(ctx context.Context, projectID, queryID uuid.UUID, req *ExecuteQueryRequest) (*datasource.QueryExecutionResult, error)
	Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *TestQueryRequest) (*datasource.QueryExecutionResult, error)
	Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) error
}

// CreateQueryRequest contains fields for creating a new query.
type CreateQueryRequest struct {
	NaturalLanguagePrompt string `json:"natural_language_prompt"`
	AdditionalContext     string `json:"additional_context,omitempty"`
	SQLQuery              string `json:"sql_query"`
	Dialect               string `json:"dialect"`
	IsEnabled             bool   `json:"is_enabled"`
}

// UpdateQueryRequest contains fields for updating a query.
// All fields are optional - only non-nil values are updated.
type UpdateQueryRequest struct {
	NaturalLanguagePrompt *string `json:"natural_language_prompt,omitempty"`
	AdditionalContext     *string `json:"additional_context,omitempty"`
	SQLQuery              *string `json:"sql_query,omitempty"`
	Dialect               *string `json:"dialect,omitempty"`
	IsEnabled             *bool   `json:"is_enabled,omitempty"`
}

// ExecuteQueryRequest contains options for executing a saved query.
type ExecuteQueryRequest struct {
	Limit int `json:"limit,omitempty"` // 0 = no limit
}

// TestQueryRequest contains a SQL query to test without saving.
type TestQueryRequest struct {
	SQLQuery string `json:"sql_query"`
	Limit    int    `json:"limit,omitempty"` // 0 = no limit
}

type queryService struct {
	queryRepo      repositories.QueryRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	logger         *zap.Logger
}

// NewQueryService creates a new query service with dependencies.
func NewQueryService(
	queryRepo repositories.QueryRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	logger *zap.Logger,
) QueryService {
	return &queryService{
		queryRepo:      queryRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		logger:         logger,
	}
}

// Create creates a new saved query.
func (s *queryService) Create(ctx context.Context, projectID, datasourceID uuid.UUID, req *CreateQueryRequest) (*models.Query, error) {
	// Validate request
	if strings.TrimSpace(req.NaturalLanguagePrompt) == "" {
		return nil, fmt.Errorf("natural language prompt is required")
	}
	if strings.TrimSpace(req.SQLQuery) == "" {
		return nil, fmt.Errorf("SQL query is required")
	}
	if strings.TrimSpace(req.Dialect) == "" {
		return nil, fmt.Errorf("dialect is required")
	}

	// Create query model
	query := &models.Query{
		ProjectID:             projectID,
		DatasourceID:          datasourceID,
		NaturalLanguagePrompt: req.NaturalLanguagePrompt,
		SQLQuery:              req.SQLQuery,
		Dialect:               req.Dialect,
		IsEnabled:             req.IsEnabled,
		UsageCount:            0,
	}

	if req.AdditionalContext != "" {
		query.AdditionalContext = &req.AdditionalContext
	}

	if err := s.queryRepo.Create(ctx, query); err != nil {
		return nil, fmt.Errorf("failed to create query: %w", err)
	}

	s.logger.Info("Created query",
		zap.String("id", query.ID.String()),
		zap.String("project_id", projectID.String()),
		zap.String("datasource_id", datasourceID.String()),
	)

	return query, nil
}

// Get retrieves a query by ID.
func (s *queryService) Get(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error) {
	query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get query: %w", err)
	}
	return query, nil
}

// List retrieves all queries for a datasource.
func (s *queryService) List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	queries, err := s.queryRepo.ListByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list queries: %w", err)
	}
	return queries, nil
}

// Update updates an existing query.
func (s *queryService) Update(ctx context.Context, projectID, queryID uuid.UUID, req *UpdateQueryRequest) (*models.Query, error) {
	// Get existing query
	query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get query: %w", err)
	}

	// Apply updates
	if req.NaturalLanguagePrompt != nil {
		query.NaturalLanguagePrompt = *req.NaturalLanguagePrompt
	}
	if req.AdditionalContext != nil {
		query.AdditionalContext = req.AdditionalContext
	}
	if req.SQLQuery != nil {
		query.SQLQuery = *req.SQLQuery
	}
	if req.Dialect != nil {
		query.Dialect = *req.Dialect
	}
	if req.IsEnabled != nil {
		query.IsEnabled = *req.IsEnabled
	}

	if err := s.queryRepo.Update(ctx, query); err != nil {
		return nil, fmt.Errorf("failed to update query: %w", err)
	}

	s.logger.Info("Updated query",
		zap.String("id", queryID.String()),
		zap.String("project_id", projectID.String()),
	)

	return query, nil
}

// Delete soft-deletes a query.
func (s *queryService) Delete(ctx context.Context, projectID, queryID uuid.UUID) error {
	// Verify query exists
	_, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("failed to get query: %w", err)
	}

	if err := s.queryRepo.SoftDelete(ctx, projectID, queryID); err != nil {
		return fmt.Errorf("failed to delete query: %w", err)
	}

	s.logger.Info("Deleted query",
		zap.String("id", queryID.String()),
		zap.String("project_id", projectID.String()),
	)

	return nil
}

// ListEnabled retrieves only enabled queries for a datasource.
func (s *queryService) ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	queries, err := s.queryRepo.ListEnabled(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled queries: %w", err)
	}
	return queries, nil
}

// SetEnabledStatus updates the enabled status of a query.
func (s *queryService) SetEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error {
	if err := s.queryRepo.UpdateEnabledStatus(ctx, projectID, queryID, isEnabled); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("failed to update enabled status: %w", err)
	}

	s.logger.Info("Updated query enabled status",
		zap.String("id", queryID.String()),
		zap.Bool("is_enabled", isEnabled),
	)

	return nil
}

// Execute runs a saved query and tracks usage.
func (s *queryService) Execute(ctx context.Context, projectID, queryID uuid.UUID, req *ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	// Get query
	query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get query: %w", err)
	}

	// Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, query.DatasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create query executor
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// Execute query
	limit := 0
	if req != nil {
		limit = req.Limit
	}

	result, err := executor.ExecuteQuery(ctx, query.SQLQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// Increment usage count (fire-and-forget, log warning on failure)
	if err := s.queryRepo.IncrementUsageCount(ctx, queryID); err != nil {
		s.logger.Warn("Failed to increment usage count",
			zap.String("query_id", queryID.String()),
			zap.Error(err),
		)
	}

	return result, nil
}

// Test executes a SQL query without saving it.
func (s *queryService) Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *TestQueryRequest) (*datasource.QueryExecutionResult, error) {
	// Validate request
	if strings.TrimSpace(req.SQLQuery) == "" {
		return nil, fmt.Errorf("SQL query is required")
	}

	// Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create query executor
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// Execute query
	result, err := executor.ExecuteQuery(ctx, req.SQLQuery, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	return result, nil
}

// Validate checks if a SQL query is syntactically valid.
func (s *queryService) Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) error {
	// Validate input
	if strings.TrimSpace(sqlQuery) == "" {
		return fmt.Errorf("SQL query is required")
	}

	// Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create query executor
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config)
	if err != nil {
		return fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// Validate query
	if err := executor.ValidateQuery(ctx, sqlQuery); err != nil {
		return err
	}

	return nil
}
