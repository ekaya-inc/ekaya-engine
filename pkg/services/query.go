package services

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/audit"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	sqlpkg "github.com/ekaya-inc/ekaya-engine/pkg/sql"
	sqlvalidator "github.com/ekaya-inc/ekaya-engine/pkg/sql"
)

// DefaultPreviewLimit is the default row limit for query preview/test operations.
// This provides a sensible default for validating queries and seeing sample output
// without fetching excessive data. Users can override by specifying a limit.
const DefaultPreviewLimit = 100

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
	ListEnabledByTags(ctx context.Context, projectID, datasourceID uuid.UUID, tags []string) ([]*models.Query, error)
	// HasEnabledQueries efficiently checks if any enabled queries exist (uses LIMIT 1).
	HasEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (bool, error)

	// Status Management
	SetEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error

	// Query Execution
	Execute(ctx context.Context, projectID, queryID uuid.UUID, req *ExecuteQueryRequest) (*datasource.QueryExecutionResult, error)
	ExecuteWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any, req *ExecuteQueryRequest) (*datasource.QueryExecutionResult, error)
	// ExecuteModifyingWithParameters executes a data-modifying query (INSERT/UPDATE/DELETE/CALL).
	// Unlike ExecuteWithParameters which is for SELECT queries, this method:
	// - Does not apply row limits (modifying queries affect all matching rows)
	// - Returns RowsAffected for statements without RETURNING clause
	// - Returns result rows for statements with RETURNING clause
	ExecuteModifyingWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any) (*datasource.ExecuteResult, error)
	Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *TestQueryRequest) (*datasource.QueryExecutionResult, error)
	Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) (*ValidationResult, error)
	ValidateParameterizedQuery(sqlQuery string, params []models.QueryParameter) error

	// Approval Workflow - Business User Operations (creates pending records)
	// SuggestUpdate creates a pending update suggestion for an existing query.
	// The original query remains active until the suggestion is approved.
	SuggestUpdate(ctx context.Context, projectID uuid.UUID, req *SuggestUpdateRequest) (*models.Query, error)

	// Approval Workflow - Admin Operations (direct operations, no pending record)
	// DirectCreate creates a new query with status="approved" (no review required).
	DirectCreate(ctx context.Context, projectID, datasourceID uuid.UUID, req *CreateQueryRequest) (*models.Query, error)
	// DirectUpdate updates an existing query directly (no pending record).
	DirectUpdate(ctx context.Context, projectID, queryID uuid.UUID, req *UpdateQueryRequest) (*models.Query, error)
	// DeleteWithPendingRejection soft-deletes a query and auto-rejects any pending update suggestions.
	// Returns the count of pending suggestions that were auto-rejected.
	DeleteWithPendingRejection(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) (int, error)

	// Approval Workflow - Review Operations
	// ApproveQuery approves a pending query suggestion.
	// For new queries: sets status="approved" and enables the query.
	// For update suggestions: applies changes to original query and soft-deletes pending record.
	ApproveQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) error
	// RejectQuery rejects a pending query suggestion with a reason.
	RejectQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string, reason string) error
	// MoveToPending moves a rejected query back to pending status for re-review.
	MoveToPending(ctx context.Context, projectID, queryID uuid.UUID) error
	// ListPending returns all pending query suggestions for a project.
	ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error)
}

// CreateQueryRequest contains fields for creating a new query.
// Note: Dialect is derived from datasource type, not provided by caller.
type CreateQueryRequest struct {
	NaturalLanguagePrompt string                  `json:"natural_language_prompt"`
	AdditionalContext     string                  `json:"additional_context,omitempty"`
	SQLQuery              string                  `json:"sql_query"`
	IsEnabled             bool                    `json:"is_enabled"`
	Parameters            []models.QueryParameter `json:"parameters,omitempty"`
	OutputColumns         []models.OutputColumn   `json:"output_columns,omitempty"`
	Constraints           string                  `json:"constraints,omitempty"`
	Tags                  []string                `json:"tags,omitempty"`               // Tags for organizing queries
	Status                string                  `json:"status,omitempty"`             // pending, approved, rejected (default: "approved")
	SuggestedBy           string                  `json:"suggested_by,omitempty"`       // user, agent, admin
	SuggestionContext     map[string]any          `json:"suggestion_context,omitempty"` // validation results, etc.
	AllowsModification    bool                    `json:"allows_modification"`          // Allow INSERT/UPDATE/DELETE/CALL
}

// UpdateQueryRequest contains fields for updating a query.
// All fields are optional - only non-nil values are updated.
// Note: Dialect cannot be updated - it's derived from datasource type.
type UpdateQueryRequest struct {
	NaturalLanguagePrompt *string                  `json:"natural_language_prompt,omitempty"`
	AdditionalContext     *string                  `json:"additional_context,omitempty"`
	SQLQuery              *string                  `json:"sql_query,omitempty"`
	IsEnabled             *bool                    `json:"is_enabled,omitempty"`
	Parameters            *[]models.QueryParameter `json:"parameters,omitempty"`
	OutputColumns         *[]models.OutputColumn   `json:"output_columns,omitempty"`
	Constraints           *string                  `json:"constraints,omitempty"`
	Tags                  *[]string                `json:"tags,omitempty"`
	AllowsModification    *bool                    `json:"allows_modification,omitempty"` // Allow INSERT/UPDATE/DELETE/CALL
}

// ExecuteQueryRequest contains options for executing a saved query.
type ExecuteQueryRequest struct {
	Limit int `json:"limit,omitempty"` // 0 = no limit
}

// TestQueryRequest contains a SQL query to test without saving.
type TestQueryRequest struct {
	SQLQuery             string                  `json:"sql_query"`
	Limit                int                     `json:"limit,omitempty"` // 0 = use DefaultPreviewLimit (100)
	ParameterDefinitions []models.QueryParameter `json:"parameter_definitions,omitempty"`
	ParameterValues      map[string]any          `json:"parameter_values,omitempty"`
}

// SuggestUpdateRequest contains fields for suggesting an update to an existing query.
// The original query remains active until the suggestion is approved.
type SuggestUpdateRequest struct {
	QueryID                  uuid.UUID                `json:"query_id"`                             // ID of the original query to update
	NaturalLanguagePrompt    *string                  `json:"natural_language_prompt,omitempty"`    // Updated name/prompt
	AdditionalContext        *string                  `json:"additional_context,omitempty"`         // Updated context
	SQLQuery                 *string                  `json:"sql_query,omitempty"`                  // Updated SQL
	Parameters               *[]models.QueryParameter `json:"parameters,omitempty"`                 // Updated parameters
	OutputColumns            *[]models.OutputColumn   `json:"output_columns,omitempty"`             // Updated output columns
	Constraints              *string                  `json:"constraints,omitempty"`                // Updated constraints
	Tags                     *[]string                `json:"tags,omitempty"`                       // Updated tags
	AllowsModification       *bool                    `json:"allows_modification,omitempty"`        // Updated modification flag
	SuggestionContext        map[string]any           `json:"suggestion_context,omitempty"`         // Why this update is needed
	OutputColumnDescriptions map[string]string        `json:"output_column_descriptions,omitempty"` // For MCP tool compatibility
}

type queryService struct {
	queryRepo      repositories.QueryRepository
	datasourceSvc  DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	auditor        *audit.SecurityAuditor
	logger         *zap.Logger
}

// NewQueryService creates a new query service with dependencies.
func NewQueryService(
	queryRepo repositories.QueryRepository,
	datasourceSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	auditor *audit.SecurityAuditor,
	logger *zap.Logger,
) QueryService {
	return &queryService{
		queryRepo:      queryRepo,
		datasourceSvc:  datasourceSvc,
		adapterFactory: adapterFactory,
		auditor:        auditor,
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

	// Validate and normalize SQL query
	validationResult := sqlvalidator.ValidateAndNormalize(req.SQLQuery)
	if validationResult.Error != nil {
		return nil, validationResult.Error
	}
	req.SQLQuery = validationResult.NormalizedSQL

	// Validate SQL statement type and allows_modification flag
	sqlType, err := ValidateSQLType(req.SQLQuery, req.AllowsModification)
	if err != nil {
		return nil, err
	}

	// Auto-correct: SELECT statements don't need allows_modification flag
	if ShouldAutoCorrectAllowsModification(sqlType, req.AllowsModification) {
		req.AllowsModification = false
	}

	// Fetch datasource to derive dialect from type
	ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Validate that all {{param}} in SQL have corresponding parameter definitions
	// This is required even if no parameters are defined (catches undefined params in SQL)
	if err := s.ValidateParameterizedQuery(req.SQLQuery, req.Parameters); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	// Require output_columns for SELECT queries.
	// Modifying queries (INSERT/UPDATE/DELETE/CALL) may not have output columns
	// unless they use RETURNING clause, so empty is allowed for those.
	if len(req.OutputColumns) == 0 && !req.AllowsModification {
		return nil, fmt.Errorf("output_column_descriptions parameter is required. Provide descriptions for output columns, e.g., {\"total\": \"Total count of records\"}")
	}

	// Ensure Parameters is never nil (database column has NOT NULL constraint)
	params := req.Parameters
	if params == nil {
		params = []models.QueryParameter{}
	}

	// OutputColumns already validated as non-empty above
	outputCols := req.OutputColumns

	// Ensure Tags is never nil
	tags := req.Tags
	if tags == nil {
		tags = []string{}
	}

	// Set status (default to "approved" for backward compatibility)
	status := "approved"
	if req.Status != "" {
		status = req.Status
	}

	// Create query model with dialect derived from datasource type
	query := &models.Query{
		ProjectID:             projectID,
		DatasourceID:          datasourceID,
		NaturalLanguagePrompt: req.NaturalLanguagePrompt,
		SQLQuery:              req.SQLQuery,
		Dialect:               ds.DatasourceType, // Derived from datasource type
		IsEnabled:             req.IsEnabled,
		Parameters:            params,
		OutputColumns:         outputCols,
		Tags:                  tags,
		Status:                status,
		SuggestionContext:     req.SuggestionContext,
		UsageCount:            0,
		AllowsModification:    req.AllowsModification,
	}

	if req.AdditionalContext != "" {
		query.AdditionalContext = &req.AdditionalContext
	}

	if req.Constraints != "" {
		query.Constraints = &req.Constraints
	}

	if req.SuggestedBy != "" {
		query.SuggestedBy = &req.SuggestedBy
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

	// Validate and normalize SQL query if being updated
	if req.SQLQuery != nil {
		validationResult := sqlvalidator.ValidateAndNormalize(*req.SQLQuery)
		if validationResult.Error != nil {
			return nil, validationResult.Error
		}
		normalized := validationResult.NormalizedSQL
		req.SQLQuery = &normalized
	}

	// If SQL is being updated, require new output_columns from test execution.
	// Exception: Modifying queries (INSERT/UPDATE/DELETE/CALL) may not have output columns
	// unless they use RETURNING clause, so empty is allowed for those.
	if req.SQLQuery != nil && *req.SQLQuery != query.SQLQuery {
		// Determine effective AllowsModification: use request value if provided, else existing
		effectiveAllowsModification := query.AllowsModification
		if req.AllowsModification != nil {
			effectiveAllowsModification = *req.AllowsModification
		}

		// Only require output_columns for SELECT queries (non-modifying)
		if !effectiveAllowsModification {
			if req.OutputColumns == nil || len(*req.OutputColumns) == 0 {
				return nil, fmt.Errorf("output_column_descriptions parameter is required when updating SQL. Provide descriptions for output columns, e.g., {\"total\": \"Total count of records\"}")
			}
		}
	}

	// Apply updates (dialect is not updatable - derived from datasource type)
	if req.NaturalLanguagePrompt != nil {
		query.NaturalLanguagePrompt = *req.NaturalLanguagePrompt
	}
	if req.AdditionalContext != nil {
		query.AdditionalContext = req.AdditionalContext
	}
	if req.SQLQuery != nil {
		query.SQLQuery = *req.SQLQuery
	}
	if req.IsEnabled != nil {
		query.IsEnabled = *req.IsEnabled
	}
	if req.OutputColumns != nil {
		query.OutputColumns = *req.OutputColumns
	}
	if req.Constraints != nil {
		query.Constraints = req.Constraints
	}
	if req.AllowsModification != nil {
		query.AllowsModification = *req.AllowsModification
	}
	if req.Parameters != nil {
		query.Parameters = *req.Parameters
	}
	if req.Tags != nil {
		query.Tags = *req.Tags
	}

	// Validate SQL statement type and allows_modification flag
	sqlType, err := ValidateSQLType(query.SQLQuery, query.AllowsModification)
	if err != nil {
		return nil, err
	}

	// Auto-correct: SELECT statements don't need allows_modification flag
	if ShouldAutoCorrectAllowsModification(sqlType, query.AllowsModification) {
		query.AllowsModification = false
	}

	// Validate that all {{param}} in SQL have corresponding parameter definitions
	if err := s.ValidateParameterizedQuery(query.SQLQuery, query.Parameters); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
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

// ListEnabledByTags lists enabled queries filtered by tags (queries matching ANY of the provided tags).
func (s *queryService) ListEnabledByTags(ctx context.Context, projectID, datasourceID uuid.UUID, tags []string) ([]*models.Query, error) {
	queries, err := s.queryRepo.ListEnabledByTags(ctx, projectID, datasourceID, tags)
	if err != nil {
		return nil, fmt.Errorf("failed to list enabled queries by tags: %w", err)
	}
	return queries, nil
}

// HasEnabledQueries efficiently checks if any enabled queries exist.
func (s *queryService) HasEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (bool, error) {
	return s.queryRepo.HasEnabledQueries(ctx, projectID, datasourceID)
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
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user ID not found in context: %w", err)
	}

	// Get query
	query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get query: %w", err)
	}

	// If query has parameters, delegate to ExecuteWithParameters (defaults will apply)
	if len(query.Parameters) > 0 {
		return s.ExecuteWithParameters(ctx, projectID, queryID, map[string]any{}, req)
	}

	// Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, query.DatasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create query executor with identity parameters for connection pooling
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, query.DatasourceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// Execute query
	limit := 0
	if req != nil {
		limit = req.Limit
	}

	result, err := executor.Query(ctx, query.SQLQuery, limit)
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
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user ID not found in context: %w", err)
	}

	// Validate request
	if strings.TrimSpace(req.SQLQuery) == "" {
		return nil, fmt.Errorf("SQL query is required")
	}

	// Validate and normalize SQL query
	validationResult := sqlvalidator.ValidateAndNormalize(req.SQLQuery)
	if validationResult.Error != nil {
		return nil, validationResult.Error
	}
	req.SQLQuery = validationResult.NormalizedSQL

	// Validate parameters if provided
	if len(req.ParameterDefinitions) > 0 {
		if err := s.ValidateParameterizedQuery(req.SQLQuery, req.ParameterDefinitions); err != nil {
			return nil, fmt.Errorf("parameter validation failed: %w", err)
		}
	}

	// Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create query executor with identity parameters for connection pooling
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// Apply default preview limit if not specified
	limit := req.Limit
	if limit <= 0 {
		limit = DefaultPreviewLimit
	}

	// Execute query with or without parameters
	// Check for parameter definitions (not just values) since defaults may apply
	var result *datasource.QueryExecutionResult
	if len(req.ParameterDefinitions) > 0 {
		// Use parameter values or empty map (defaults will apply)
		paramValues := req.ParameterValues
		if paramValues == nil {
			paramValues = map[string]any{}
		}

		// Validate and coerce parameter values
		if err := s.validateRequiredParameters(req.ParameterDefinitions, paramValues); err != nil {
			return nil, err
		}
		coercedParams, err := s.coerceParameterTypes(req.ParameterDefinitions, paramValues)
		if err != nil {
			return nil, err
		}

		// Check for SQL injection
		injectionResults := sqlvalidator.CheckAllParameters(coercedParams)
		if len(injectionResults) > 0 {
			return nil, fmt.Errorf("potential SQL injection detected in parameter '%s'", injectionResults[0].ParamName)
		}

		// Substitute parameters
		preparedSQL, orderedValues, err := sqlvalidator.SubstituteParameters(req.SQLQuery, req.ParameterDefinitions, coercedParams)
		if err != nil {
			return nil, err
		}

		result, err = executor.QueryWithParams(ctx, preparedSQL, orderedValues, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to execute parameterized query: %w", err)
		}
	} else {
		result, err = executor.Query(ctx, req.SQLQuery, limit)
		if err != nil {
			return nil, fmt.Errorf("failed to execute query: %w", err)
		}
	}

	return result, nil
}

// ValidationResult contains the result of SQL validation.
type ValidationResult struct {
	Valid   bool
	Message string
}

// Validate checks if a SQL query is syntactically valid.
// Returns a ValidationResult with valid=true and a custom message if parameters are detected.
func (s *queryService) Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) (*ValidationResult, error) {
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user ID not found in context: %w", err)
	}

	// Validate input
	if strings.TrimSpace(sqlQuery) == "" {
		return nil, fmt.Errorf("SQL query is required")
	}

	// Validate and normalize SQL query
	validationResult := sqlvalidator.ValidateAndNormalize(sqlQuery)
	if validationResult.Error != nil {
		return nil, validationResult.Error
	}
	sqlQuery = validationResult.NormalizedSQL

	// Check for {{param}} placeholders - skip DB validation if present
	params := sqlpkg.ExtractParameters(sqlQuery)
	if len(params) > 0 {
		return &ValidationResult{
			Valid:   true,
			Message: "Parameters detected - full validation on Test Query",
		}, nil
	}

	// Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Create query executor with identity parameters for connection pooling
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, datasourceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// Validate query
	if err := executor.ValidateQuery(ctx, sqlQuery); err != nil {
		return nil, err
	}

	return &ValidationResult{
		Valid:   true,
		Message: "SQL is valid",
	}, nil
}

// ExecuteWithParameters runs a parameterized query with supplied values.
func (s *queryService) ExecuteWithParameters(
	ctx context.Context,
	projectID, queryID uuid.UUID,
	params map[string]any,
	req *ExecuteQueryRequest,
) (*datasource.QueryExecutionResult, error) {
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user ID not found in context: %w", err)
	}

	// 1. Get query
	query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get query: %w", err)
	}

	// 2. Validate required parameters are provided
	if err := s.validateRequiredParameters(query.Parameters, params); err != nil {
		return nil, err
	}

	// 3. Type-check and coerce parameter values
	coercedParams, err := s.coerceParameterTypes(query.Parameters, params)
	if err != nil {
		return nil, err
	}

	// 4. Check for SQL injection attempts
	injectionResults := sqlpkg.CheckAllParameters(coercedParams)
	if len(injectionResults) > 0 {
		// Log to SIEM for all detected injection attempts
		for _, result := range injectionResults {
			s.auditor.LogInjectionAttempt(ctx, projectID, queryID,
				audit.SQLInjectionDetails{
					ParamName:   result.ParamName,
					ParamValue:  fmt.Sprintf("%v", result.ParamValue),
					Fingerprint: result.Fingerprint,
					QueryName:   query.NaturalLanguagePrompt,
				},
				getClientIPFromContext(ctx),
			)
		}
		return nil, fmt.Errorf("potential SQL injection detected in parameter '%s'",
			injectionResults[0].ParamName)
	}

	// 5. Substitute parameters to get prepared SQL
	preparedSQL, orderedValues, err := sqlpkg.SubstituteParameters(
		query.SQLQuery, query.Parameters, coercedParams)
	if err != nil {
		return nil, fmt.Errorf("failed to substitute parameters: %w", err)
	}

	// 6. Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, query.DatasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// 7. Create query executor with identity parameters for connection pooling
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, query.DatasourceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// 8. Execute with parameterized binding
	limit := 0
	if req != nil {
		limit = req.Limit
	}

	result, err := executor.QueryWithParams(ctx, preparedSQL, orderedValues, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// 9. Increment usage count (fire-and-forget, log warning on failure)
	if err := s.queryRepo.IncrementUsageCount(ctx, queryID); err != nil {
		s.logger.Warn("Failed to increment usage count",
			zap.String("query_id", queryID.String()),
			zap.Error(err),
		)
	}

	return result, nil
}

// ExecuteModifyingWithParameters executes a data-modifying query (INSERT/UPDATE/DELETE/CALL).
func (s *queryService) ExecuteModifyingWithParameters(
	ctx context.Context,
	projectID, queryID uuid.UUID,
	params map[string]any,
) (*datasource.ExecuteResult, error) {
	// Extract userID from context (JWT claims)
	userID, err := auth.RequireUserIDFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("user ID not found in context: %w", err)
	}

	// 1. Get query
	query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get query: %w", err)
	}

	// 2. Verify the query allows modification
	if !query.AllowsModification {
		return nil, fmt.Errorf("query is not authorized for data modification")
	}

	// 3. Validate required parameters are provided
	if err := s.validateRequiredParameters(query.Parameters, params); err != nil {
		return nil, err
	}

	// 4. Type-check and coerce parameter values
	coercedParams, err := s.coerceParameterTypes(query.Parameters, params)
	if err != nil {
		return nil, err
	}

	// 5. Check for SQL injection attempts
	injectionResults := sqlpkg.CheckAllParameters(coercedParams)
	if len(injectionResults) > 0 {
		// Log to SIEM for all detected injection attempts
		for _, result := range injectionResults {
			s.auditor.LogInjectionAttempt(ctx, projectID, queryID,
				audit.SQLInjectionDetails{
					ParamName:   result.ParamName,
					ParamValue:  fmt.Sprintf("%v", result.ParamValue),
					Fingerprint: result.Fingerprint,
					QueryName:   query.NaturalLanguagePrompt,
				},
				getClientIPFromContext(ctx),
			)
		}
		return nil, fmt.Errorf("potential SQL injection detected in parameter '%s'",
			injectionResults[0].ParamName)
	}

	// 6. Substitute parameters to get prepared SQL
	preparedSQL, orderedValues, err := sqlpkg.SubstituteParameters(
		query.SQLQuery, query.Parameters, coercedParams)
	if err != nil {
		return nil, fmt.Errorf("failed to substitute parameters: %w", err)
	}

	// 7. Get datasource config
	ds, err := s.datasourceSvc.Get(ctx, projectID, query.DatasourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// 8. Create query executor with identity parameters for connection pooling
	executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config, projectID, query.DatasourceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to create query executor: %w", err)
	}
	defer executor.Close()

	// 9. Execute with parameterized binding (no limit for modifying statements)
	result, err := executor.ExecuteWithParams(ctx, preparedSQL, orderedValues)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}

	// 10. Increment usage count (fire-and-forget, log warning on failure)
	if err := s.queryRepo.IncrementUsageCount(ctx, queryID); err != nil {
		s.logger.Warn("Failed to increment usage count",
			zap.String("query_id", queryID.String()),
			zap.Error(err),
		)
	}

	return result, nil
}

// ValidateParameterizedQuery validates SQL template and parameter definitions.
func (s *queryService) ValidateParameterizedQuery(
	sqlQuery string,
	params []models.QueryParameter,
) error {
	// Check all {{param}} in SQL have definitions
	return sqlpkg.ValidateParameterDefinitions(sqlQuery, params)
}

// validateRequiredParameters checks that all required parameters are provided.
func (s *queryService) validateRequiredParameters(
	paramDefs []models.QueryParameter,
	suppliedParams map[string]any,
) error {
	for _, p := range paramDefs {
		if p.Required {
			value, exists := suppliedParams[p.Name]
			// Check if provided and not nil
			if !exists || value == nil {
				// Check if there's a default value
				if p.Default == nil {
					return fmt.Errorf("required parameter '%s' is missing", p.Name)
				}
			}
		}
	}
	return nil
}

// coerceParameterTypes validates and coerces parameter values to their declared types.
func (s *queryService) coerceParameterTypes(
	paramDefs []models.QueryParameter,
	suppliedParams map[string]any,
) (map[string]any, error) {
	coerced := make(map[string]any)

	// Build lookup for parameter definitions
	defLookup := make(map[string]models.QueryParameter)
	for _, p := range paramDefs {
		defLookup[p.Name] = p
	}

	// Process each supplied parameter
	for name, value := range suppliedParams {
		def, exists := defLookup[name]
		if !exists {
			return nil, fmt.Errorf("unknown parameter '%s'", name)
		}

		// Skip nil values - they'll use defaults during substitution
		if value == nil {
			continue
		}

		// Coerce based on type
		coercedValue, err := s.coerceValue(value, def.Type, name)
		if err != nil {
			return nil, err
		}
		coerced[name] = coercedValue
	}

	return coerced, nil
}

// coerceValue attempts to coerce a value to the target type.
func (s *queryService) coerceValue(value any, targetType string, paramName string) (any, error) {
	switch targetType {
	case "string":
		return s.coerceToString(value)
	case "integer":
		return s.coerceToInteger(value, paramName)
	case "decimal":
		return s.coerceToDecimal(value, paramName)
	case "boolean":
		return s.coerceToBoolean(value, paramName)
	case "date":
		return s.coerceToDate(value, paramName)
	case "timestamp":
		return s.coerceToTimestamp(value, paramName)
	case "uuid":
		return s.coerceToUUID(value, paramName)
	case "string[]":
		return s.coerceToStringArray(value, paramName)
	case "integer[]":
		return s.coerceToIntegerArray(value, paramName)
	default:
		return nil, fmt.Errorf("unsupported parameter type '%s' for parameter '%s'", targetType, paramName)
	}
}

func (s *queryService) coerceToString(value any) (string, error) {
	if str, ok := value.(string); ok {
		return str, nil
	}
	return fmt.Sprintf("%v", value), nil
}

func (s *queryService) coerceToInteger(value any, paramName string) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case float32:
		return int64(v), nil
	case string:
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parameter '%s': cannot convert '%s' to integer: %w", paramName, v, err)
		}
		return i, nil
	default:
		return 0, fmt.Errorf("parameter '%s': cannot convert type %T to integer", paramName, value)
	}
}

func (s *queryService) coerceToDecimal(value any, paramName string) (float64, error) {
	switch v := value.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case string:
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return 0, fmt.Errorf("parameter '%s': cannot convert '%s' to decimal: %w", paramName, v, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("parameter '%s': cannot convert type %T to decimal", paramName, value)
	}
}

func (s *queryService) coerceToBoolean(value any, paramName string) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		b, err := strconv.ParseBool(v)
		if err != nil {
			return false, fmt.Errorf("parameter '%s': cannot convert '%s' to boolean: %w", paramName, v, err)
		}
		return b, nil
	case int, int32, int64:
		// Treat non-zero as true
		return v != 0, nil
	case float32, float64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("parameter '%s': cannot convert type %T to boolean", paramName, value)
	}
}

func (s *queryService) coerceToDate(value any, paramName string) (string, error) {
	str, err := s.coerceToString(value)
	if err != nil {
		return "", err
	}

	// Validate ISO 8601 date format (YYYY-MM-DD)
	_, err = time.Parse("2006-01-02", str)
	if err != nil {
		return "", fmt.Errorf("parameter '%s': invalid date format '%s', expected YYYY-MM-DD: %w", paramName, str, err)
	}
	return str, nil
}

func (s *queryService) coerceToTimestamp(value any, paramName string) (string, error) {
	str, err := s.coerceToString(value)
	if err != nil {
		return "", err
	}

	// Validate ISO 8601 timestamp format
	_, err = time.Parse(time.RFC3339, str)
	if err != nil {
		return "", fmt.Errorf("parameter '%s': invalid timestamp format '%s', expected RFC3339: %w", paramName, str, err)
	}
	return str, nil
}

func (s *queryService) coerceToUUID(value any, paramName string) (string, error) {
	str, err := s.coerceToString(value)
	if err != nil {
		return "", err
	}

	// Validate UUID format
	_, err = uuid.Parse(str)
	if err != nil {
		return "", fmt.Errorf("parameter '%s': invalid UUID format '%s': %w", paramName, str, err)
	}
	return str, nil
}

func (s *queryService) coerceToStringArray(value any, paramName string) ([]string, error) {
	// Handle []interface{} from JSON
	if arr, ok := value.([]interface{}); ok {
		result := make([]string, len(arr))
		for i, v := range arr {
			str, err := s.coerceToString(v)
			if err != nil {
				return nil, fmt.Errorf("parameter '%s': array element %d: %w", paramName, i, err)
			}
			result[i] = str
		}
		return result, nil
	}

	// Handle []string directly
	if arr, ok := value.([]string); ok {
		return arr, nil
	}

	return nil, fmt.Errorf("parameter '%s': cannot convert type %T to string array", paramName, value)
}

func (s *queryService) coerceToIntegerArray(value any, paramName string) ([]int64, error) {
	// Handle []interface{} from JSON
	if arr, ok := value.([]interface{}); ok {
		result := make([]int64, len(arr))
		for i, v := range arr {
			intVal, err := s.coerceToInteger(v, fmt.Sprintf("%s[%d]", paramName, i))
			if err != nil {
				return nil, err
			}
			result[i] = intVal
		}
		return result, nil
	}

	// Handle []int64 directly
	if arr, ok := value.([]int64); ok {
		return arr, nil
	}

	// Handle []int
	if arr, ok := value.([]int); ok {
		result := make([]int64, len(arr))
		for i, v := range arr {
			result[i] = int64(v)
		}
		return result, nil
	}

	return nil, fmt.Errorf("parameter '%s': cannot convert type %T to integer array", paramName, value)
}

// getClientIPFromContext extracts the client IP from the request context.
// Returns empty string if not found.
func getClientIPFromContext(ctx context.Context) string {
	// In a real implementation, this would extract from HTTP context
	// For now, return a placeholder
	return ""
}

// ============================================================================
// Approval Workflow Methods
// ============================================================================

// SuggestUpdate creates a pending update suggestion for an existing query.
// The original query remains active until the suggestion is approved.
func (s *queryService) SuggestUpdate(ctx context.Context, projectID uuid.UUID, req *SuggestUpdateRequest) (*models.Query, error) {
	// Fetch the original query
	original, err := s.queryRepo.GetByID(ctx, projectID, req.QueryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get original query: %w", err)
	}

	// Create a copy of the original query with proposed changes
	suggestion := &models.Query{
		ProjectID:             original.ProjectID,
		DatasourceID:          original.DatasourceID,
		NaturalLanguagePrompt: original.NaturalLanguagePrompt,
		AdditionalContext:     original.AdditionalContext,
		SQLQuery:              original.SQLQuery,
		Dialect:               original.Dialect,
		IsEnabled:             false, // Pending suggestions are not enabled
		Parameters:            original.Parameters,
		OutputColumns:         original.OutputColumns,
		Constraints:           original.Constraints,
		Tags:                  original.Tags,
		Status:                "pending",
		SuggestionContext:     req.SuggestionContext,
		AllowsModification:    original.AllowsModification,
		ParentQueryID:         &req.QueryID, // Link to original query
	}

	// Apply updates from request
	if req.NaturalLanguagePrompt != nil {
		suggestion.NaturalLanguagePrompt = *req.NaturalLanguagePrompt
	}
	if req.AdditionalContext != nil {
		suggestion.AdditionalContext = req.AdditionalContext
	}
	if req.SQLQuery != nil {
		// Validate and normalize SQL
		validationResult := sqlvalidator.ValidateAndNormalize(*req.SQLQuery)
		if validationResult.Error != nil {
			return nil, validationResult.Error
		}
		suggestion.SQLQuery = validationResult.NormalizedSQL
	}
	if req.Parameters != nil {
		suggestion.Parameters = *req.Parameters
	}
	if req.OutputColumns != nil {
		suggestion.OutputColumns = *req.OutputColumns
	}
	if req.Constraints != nil {
		suggestion.Constraints = req.Constraints
	}
	if req.Tags != nil {
		suggestion.Tags = *req.Tags
	}
	if req.AllowsModification != nil {
		suggestion.AllowsModification = *req.AllowsModification
	}

	// Validate SQL statement type and allows_modification flag
	sqlType, err := ValidateSQLType(suggestion.SQLQuery, suggestion.AllowsModification)
	if err != nil {
		return nil, err
	}

	// Auto-correct: SELECT statements don't need allows_modification flag
	if ShouldAutoCorrectAllowsModification(sqlType, suggestion.AllowsModification) {
		suggestion.AllowsModification = false
	}

	// Validate parameters if SQL was updated
	if err := s.ValidateParameterizedQuery(suggestion.SQLQuery, suggestion.Parameters); err != nil {
		return nil, fmt.Errorf("parameter validation failed: %w", err)
	}

	// Set suggested_by to "agent" (this is typically called by MCP tools)
	suggestedBy := "agent"
	suggestion.SuggestedBy = &suggestedBy

	if err := s.queryRepo.Create(ctx, suggestion); err != nil {
		return nil, fmt.Errorf("failed to create update suggestion: %w", err)
	}

	s.logger.Info("Created update suggestion",
		zap.String("id", suggestion.ID.String()),
		zap.String("parent_query_id", req.QueryID.String()),
		zap.String("project_id", projectID.String()),
	)

	return suggestion, nil
}

// DirectCreate creates a new query with status="approved" (no review required).
// This is for admin users who can bypass the approval workflow.
func (s *queryService) DirectCreate(ctx context.Context, projectID, datasourceID uuid.UUID, req *CreateQueryRequest) (*models.Query, error) {
	// Force approved status and admin suggested_by
	req.Status = "approved"
	req.SuggestedBy = "admin"

	// Enable the query by default for direct creation
	req.IsEnabled = true

	return s.Create(ctx, projectID, datasourceID, req)
}

// DirectUpdate updates an existing query directly (no pending record).
// This is for admin users who can bypass the approval workflow.
func (s *queryService) DirectUpdate(ctx context.Context, projectID, queryID uuid.UUID, req *UpdateQueryRequest) (*models.Query, error) {
	// Use the existing Update method - it already handles direct updates
	return s.Update(ctx, projectID, queryID, req)
}

// ApproveQuery approves a pending query suggestion.
// For new queries: sets status="approved" and enables the query.
// For update suggestions: applies changes to original query and soft-deletes pending record.
func (s *queryService) ApproveQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) error {
	// Get the pending query
	pending, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("failed to get query: %w", err)
	}

	// Verify it's pending
	if pending.Status != "pending" {
		return fmt.Errorf("query is not pending approval (status: %s)", pending.Status)
	}

	// Check if this is an update suggestion (has parent_query_id)
	if pending.ParentQueryID != nil {
		// This is an update suggestion - apply changes to the original query
		original, err := s.queryRepo.GetByID(ctx, projectID, *pending.ParentQueryID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				// Original was deleted - reject this suggestion instead
				reason := "Original query was deleted"
				return s.queryRepo.UpdateApprovalStatus(ctx, projectID, queryID, "rejected", reviewerID, &reason)
			}
			return fmt.Errorf("failed to get original query: %w", err)
		}

		// Apply changes from pending to original
		original.NaturalLanguagePrompt = pending.NaturalLanguagePrompt
		original.AdditionalContext = pending.AdditionalContext
		original.SQLQuery = pending.SQLQuery
		original.Parameters = pending.Parameters
		original.OutputColumns = pending.OutputColumns
		original.Constraints = pending.Constraints
		original.Tags = pending.Tags
		original.AllowsModification = pending.AllowsModification

		// Update the original query
		if err := s.queryRepo.Update(ctx, original); err != nil {
			return fmt.Errorf("failed to update original query: %w", err)
		}

		// Soft-delete the pending suggestion
		if err := s.queryRepo.SoftDelete(ctx, projectID, queryID); err != nil {
			return fmt.Errorf("failed to delete pending suggestion: %w", err)
		}

		s.logger.Info("Approved update suggestion and applied to original",
			zap.String("suggestion_id", queryID.String()),
			zap.String("original_id", pending.ParentQueryID.String()),
			zap.String("reviewer", reviewerID),
		)
	} else {
		// This is a new query suggestion - approve and enable it
		if err := s.queryRepo.UpdateApprovalStatus(ctx, projectID, queryID, "approved", reviewerID, nil); err != nil {
			return fmt.Errorf("failed to update approval status: %w", err)
		}

		// Enable the query
		if err := s.queryRepo.UpdateEnabledStatus(ctx, projectID, queryID, true); err != nil {
			return fmt.Errorf("failed to enable query: %w", err)
		}

		s.logger.Info("Approved new query suggestion",
			zap.String("id", queryID.String()),
			zap.String("reviewer", reviewerID),
		)
	}

	return nil
}

// RejectQuery rejects a pending query suggestion with a reason.
func (s *queryService) RejectQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string, reason string) error {
	// Get the pending query
	pending, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("failed to get query: %w", err)
	}

	// Verify it's pending
	if pending.Status != "pending" {
		return fmt.Errorf("query is not pending approval (status: %s)", pending.Status)
	}

	// Update status to rejected
	if err := s.queryRepo.UpdateApprovalStatus(ctx, projectID, queryID, "rejected", reviewerID, &reason); err != nil {
		return fmt.Errorf("failed to update approval status: %w", err)
	}

	s.logger.Info("Rejected query suggestion",
		zap.String("id", queryID.String()),
		zap.String("reviewer", reviewerID),
		zap.String("reason", reason),
	)

	return nil
}

func (s *queryService) MoveToPending(ctx context.Context, projectID, queryID uuid.UUID) error {
	// Get the rejected query
	query, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return apperrors.ErrNotFound
		}
		return fmt.Errorf("failed to get query: %w", err)
	}

	// Verify it's rejected
	if query.Status != "rejected" {
		return fmt.Errorf("query is not rejected (status: %s)", query.Status)
	}

	// Update status to pending and clear review fields
	if err := s.queryRepo.UpdateApprovalStatus(ctx, projectID, queryID, "pending", "", nil); err != nil {
		return fmt.Errorf("failed to update approval status: %w", err)
	}

	s.logger.Info("Moved rejected query back to pending",
		zap.String("id", queryID.String()),
	)

	return nil
}

// ListPending returns all pending query suggestions for a project.
func (s *queryService) ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error) {
	queries, err := s.queryRepo.ListPending(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending queries: %w", err)
	}
	return queries, nil
}

// DeleteWithPendingRejection soft-deletes a query and auto-rejects any pending update suggestions.
// Returns the count of pending suggestions that were auto-rejected.
func (s *queryService) DeleteWithPendingRejection(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) (int, error) {
	// Verify query exists
	_, err := s.queryRepo.GetByID(ctx, projectID, queryID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return 0, apperrors.ErrNotFound
		}
		return 0, fmt.Errorf("failed to get query: %w", err)
	}

	// Get pending update suggestions for this query
	pendingSuggestions, err := s.queryRepo.GetPendingUpdatesForQuery(ctx, projectID, queryID)
	if err != nil {
		return 0, fmt.Errorf("failed to get pending updates: %w", err)
	}

	// Auto-reject all pending suggestions with reason "Original query was deleted"
	reason := "Original query was deleted"
	rejectedCount := 0
	for _, suggestion := range pendingSuggestions {
		if err := s.queryRepo.UpdateApprovalStatus(ctx, projectID, suggestion.ID, "rejected", reviewerID, &reason); err != nil {
			s.logger.Error("Failed to auto-reject pending suggestion",
				zap.String("suggestion_id", suggestion.ID.String()),
				zap.String("query_id", queryID.String()),
				zap.Error(err))
			// Continue with other rejections even if one fails
			continue
		}
		rejectedCount++
		s.logger.Info("Auto-rejected pending suggestion due to query deletion",
			zap.String("suggestion_id", suggestion.ID.String()),
			zap.String("query_id", queryID.String()),
		)
	}

	// Soft-delete the query
	if err := s.queryRepo.SoftDelete(ctx, projectID, queryID); err != nil {
		return rejectedCount, fmt.Errorf("failed to delete query: %w", err)
	}

	s.logger.Info("Deleted query with pending rejection",
		zap.String("id", queryID.String()),
		zap.String("project_id", projectID.String()),
		zap.Int("rejected_count", rejectedCount),
	)

	return rejectedCount, nil
}
