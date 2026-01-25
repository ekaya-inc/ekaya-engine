package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	sqlvalidator "github.com/ekaya-inc/ekaya-engine/pkg/sql"
)

// QueryResponse matches frontend Query interface.
type QueryResponse struct {
	QueryID               string                  `json:"query_id"`
	ProjectID             string                  `json:"project_id"`
	DatasourceID          string                  `json:"datasource_id"`
	NaturalLanguagePrompt string                  `json:"natural_language_prompt"`
	AdditionalContext     *string                 `json:"additional_context,omitempty"`
	SQLQuery              string                  `json:"sql_query"`
	Dialect               string                  `json:"dialect"`
	IsEnabled             bool                    `json:"is_enabled"`
	AllowsModification    bool                    `json:"allows_modification"`
	Parameters            []models.QueryParameter `json:"parameters,omitempty"`
	OutputColumns         []models.OutputColumn   `json:"output_columns,omitempty"`
	Constraints           *string                 `json:"constraints,omitempty"`
	UsageCount            int                     `json:"usage_count"`
	LastUsedAt            *string                 `json:"last_used_at,omitempty"`
	CreatedAt             string                  `json:"created_at"`
	UpdatedAt             string                  `json:"updated_at"`
}

// ListQueriesResponse wraps array for frontend compatibility.
type ListQueriesResponse struct {
	Queries []QueryResponse `json:"queries"`
}

// ListPendingQueriesResponse wraps pending queries for admin review.
type ListPendingQueriesResponse struct {
	Queries []PendingQueryResponse `json:"queries"`
	Count   int                    `json:"count"`
}

// PendingQueryResponse represents a pending query for admin review.
// Includes additional fields for suggestion tracking.
type PendingQueryResponse struct {
	QueryID               string                  `json:"query_id"`
	ProjectID             string                  `json:"project_id"`
	DatasourceID          string                  `json:"datasource_id"`
	NaturalLanguagePrompt string                  `json:"natural_language_prompt"`
	AdditionalContext     *string                 `json:"additional_context,omitempty"`
	SQLQuery              string                  `json:"sql_query"`
	Dialect               string                  `json:"dialect"`
	Parameters            []models.QueryParameter `json:"parameters,omitempty"`
	OutputColumns         []models.OutputColumn   `json:"output_columns,omitempty"`
	Constraints           *string                 `json:"constraints,omitempty"`
	Tags                  []string                `json:"tags,omitempty"`
	Status                string                  `json:"status"`
	SuggestedBy           *string                 `json:"suggested_by,omitempty"`
	SuggestionContext     map[string]any          `json:"suggestion_context,omitempty"`
	ParentQueryID         *string                 `json:"parent_query_id,omitempty"`
	AllowsModification    bool                    `json:"allows_modification"`
	CreatedAt             string                  `json:"created_at"`
	UpdatedAt             string                  `json:"updated_at"`
}

// CreateQueryRequest for POST body.
// Note: Dialect is derived from datasource type in the service layer.
type CreateQueryRequest struct {
	NaturalLanguagePrompt string                  `json:"natural_language_prompt"`
	AdditionalContext     string                  `json:"additional_context,omitempty"`
	SQLQuery              string                  `json:"sql_query"`
	IsEnabled             bool                    `json:"is_enabled"`
	Parameters            []models.QueryParameter `json:"parameters,omitempty"`
	OutputColumns         []models.OutputColumn   `json:"output_columns,omitempty"`
	Constraints           string                  `json:"constraints,omitempty"`
}

// UpdateQueryRequest for PUT body (all fields optional).
// Note: Dialect cannot be updated - it's derived from datasource type.
type UpdateQueryRequest struct {
	NaturalLanguagePrompt *string                  `json:"natural_language_prompt,omitempty"`
	AdditionalContext     *string                  `json:"additional_context,omitempty"`
	SQLQuery              *string                  `json:"sql_query,omitempty"`
	IsEnabled             *bool                    `json:"is_enabled,omitempty"`
	Parameters            *[]models.QueryParameter `json:"parameters,omitempty"`
	OutputColumns         *[]models.OutputColumn   `json:"output_columns,omitempty"`
	Constraints           *string                  `json:"constraints,omitempty"`
}

// ExecuteQueryRequest for POST execute body.
type ExecuteQueryRequest struct {
	Limit      int            `json:"limit,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

// ColumnInfo describes a result column with type information.
type ColumnInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ExecuteQueryResponse for query execution results.
type ExecuteQueryResponse struct {
	Columns  []ColumnInfo     `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

// TestQueryRequest for POST test body.
type TestQueryRequest struct {
	SQLQuery             string                  `json:"sql_query"`
	Limit                int                     `json:"limit,omitempty"`
	ParameterDefinitions []models.QueryParameter `json:"parameter_definitions,omitempty"`
	ParameterValues      map[string]any          `json:"parameter_values,omitempty"`
}

// ValidateQueryRequest for POST validate body.
type ValidateQueryRequest struct {
	SQLQuery string `json:"sql_query"`
}

// ValidateQueryResponse for validation results.
type ValidateQueryResponse struct {
	Valid    bool     `json:"valid"`
	Message  string   `json:"message,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ValidateParametersRequest for POST validate-parameters body.
type ValidateParametersRequest struct {
	SQLQuery   string                  `json:"sql_query"`
	Parameters []models.QueryParameter `json:"parameters"`
}

// ValidateParametersResponse for parameter validation results.
type ValidateParametersResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// DeleteQueryResponse for delete result.
type DeleteQueryResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ApproveQueryResponse for query approval result.
type ApproveQueryResponse struct {
	Success bool           `json:"success"`
	Message string         `json:"message"`
	Query   *QueryResponse `json:"query,omitempty"`
}

// RejectQueryRequest for POST reject body.
type RejectQueryRequest struct {
	Reason string `json:"reason"`
}

// RejectQueryResponse for query rejection result.
type RejectQueryResponse struct {
	Success bool                  `json:"success"`
	Message string                `json:"message"`
	Query   *PendingQueryResponse `json:"query,omitempty"`
}

// QueriesHandler handles query-related HTTP requests.
type QueriesHandler struct {
	queryService services.QueryService
	logger       *zap.Logger
}

// NewQueriesHandler creates a new queries handler.
func NewQueriesHandler(queryService services.QueryService, logger *zap.Logger) *QueriesHandler {
	return &QueriesHandler{
		queryService: queryService,
		logger:       logger,
	}
}

// RegisterRoutes registers the queries handler's routes on the given mux.
func (h *QueriesHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// All query routes are scoped to project and datasource
	base := "/api/projects/{pid}/datasources/{dsid}/queries"

	// CRUD endpoints
	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
	mux.HandleFunc("POST "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Create)))
	mux.HandleFunc("GET "+base+"/{qid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("PUT "+base+"/{qid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Update)))
	mux.HandleFunc("DELETE "+base+"/{qid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Delete)))

	// Execution endpoints
	mux.HandleFunc("POST "+base+"/{qid}/execute",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Execute)))
	mux.HandleFunc("POST "+base+"/test",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Test)))
	mux.HandleFunc("POST "+base+"/validate",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Validate)))
	mux.HandleFunc("POST "+base+"/validate-parameters",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ValidateParameters)))

	// Filtering endpoints
	mux.HandleFunc("GET "+base+"/enabled",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListEnabled)))

	// Project-level query endpoints (not scoped to datasource)
	projectBase := "/api/projects/{pid}/queries"

	// Admin review endpoints for pending query suggestions
	mux.HandleFunc("GET "+projectBase+"/pending",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListPending)))
	mux.HandleFunc("POST "+projectBase+"/{qid}/approve",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Approve)))
	mux.HandleFunc("POST "+projectBase+"/{qid}/reject",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Reject)))
}

// List handles GET /api/projects/{pid}/datasources/{dsid}/queries
func (h *QueriesHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	queries, err := h.queryService.List(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to list queries",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to list queries"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := ListQueriesResponse{
		Queries: make([]QueryResponse, len(queries)),
	}
	for i, q := range queries {
		data.Queries[i] = h.toQueryResponse(q)
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Create handles POST /api/projects/{pid}/datasources/{dsid}/queries
func (h *QueriesHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	var req CreateQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.NaturalLanguagePrompt == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_prompt", "Natural language prompt is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.SQLQuery == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_sql", "SQL query is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	serviceReq := &services.CreateQueryRequest{
		NaturalLanguagePrompt: req.NaturalLanguagePrompt,
		AdditionalContext:     req.AdditionalContext,
		SQLQuery:              req.SQLQuery,
		IsEnabled:             req.IsEnabled,
		Parameters:            req.Parameters,
		OutputColumns:         req.OutputColumns,
		Constraints:           req.Constraints,
	}

	query, err := h.queryService.Create(r.Context(), projectID, datasourceID, serviceReq)
	if err != nil {
		// Check for validation errors (bad request, not server error)
		if errors.Is(err, sqlvalidator.ErrMultipleStatements) {
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_sql", err.Error()); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to create query",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "create_failed", "Failed to create query"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toQueryResponse(query)}
	if err := WriteJSON(w, http.StatusCreated, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Get handles GET /api/projects/{pid}/datasources/{dsid}/queries/{qid}
func (h *QueriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	queryID, ok := ParseQueryID(w, r, h.logger)
	if !ok {
		return
	}

	query, err := h.queryService.Get(r.Context(), projectID, queryID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to get query",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get query"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toQueryResponse(query)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Update handles PUT /api/projects/{pid}/datasources/{dsid}/queries/{qid}
func (h *QueriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	queryID, ok := ParseQueryID(w, r, h.logger)
	if !ok {
		return
	}

	var req UpdateQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	serviceReq := &services.UpdateQueryRequest{
		NaturalLanguagePrompt: req.NaturalLanguagePrompt,
		AdditionalContext:     req.AdditionalContext,
		SQLQuery:              req.SQLQuery,
		IsEnabled:             req.IsEnabled,
		Parameters:            req.Parameters,
		OutputColumns:         req.OutputColumns,
		Constraints:           req.Constraints,
	}

	query, err := h.queryService.Update(r.Context(), projectID, queryID, serviceReq)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		// Check for validation errors (bad request, not server error)
		if errors.Is(err, sqlvalidator.ErrMultipleStatements) {
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_sql", err.Error()); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to update query",
			zap.String("query_id", queryID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "update_failed", "Failed to update query"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: h.toQueryResponse(query)}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/datasources/{dsid}/queries/{qid}
func (h *QueriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	queryID, ok := ParseQueryID(w, r, h.logger)
	if !ok {
		return
	}

	if err := h.queryService.Delete(r.Context(), projectID, queryID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to delete query",
			zap.String("query_id", queryID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_failed", "Failed to delete query"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := DeleteQueryResponse{
		Success: true,
		Message: "Query deleted successfully",
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Execute handles POST /api/projects/{pid}/datasources/{dsid}/queries/{qid}/execute
func (h *QueriesHandler) Execute(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	queryID, ok := ParseQueryID(w, r, h.logger)
	if !ok {
		return
	}

	var req ExecuteQueryRequest
	// Body is optional for execute
	_ = json.NewDecoder(r.Body).Decode(&req)

	serviceReq := &services.ExecuteQueryRequest{
		Limit: req.Limit,
	}

	var result *datasource.QueryExecutionResult
	var err error

	// Use ExecuteWithParameters if parameters are provided
	if len(req.Parameters) > 0 {
		result, err = h.queryService.ExecuteWithParameters(r.Context(), projectID, queryID, req.Parameters, serviceReq)
	} else {
		result, err = h.queryService.Execute(r.Context(), projectID, queryID, serviceReq)
	}
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to execute query",
			zap.String("query_id", queryID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "execution_failed", "Failed to execute query"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Convert datasource.ColumnInfo to handler's ColumnInfo
	columns := make([]ColumnInfo, len(result.Columns))
	for i, col := range result.Columns {
		columns[i] = ColumnInfo{Name: col.Name, Type: col.Type}
	}

	data := ExecuteQueryResponse{
		Columns:  columns,
		Rows:     result.Rows,
		RowCount: result.RowCount,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Test handles POST /api/projects/{pid}/datasources/{dsid}/queries/test
func (h *QueriesHandler) Test(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	var req TestQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.SQLQuery == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_sql", "SQL query is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	serviceReq := &services.TestQueryRequest{
		SQLQuery:             req.SQLQuery,
		Limit:                req.Limit,
		ParameterDefinitions: req.ParameterDefinitions,
		ParameterValues:      req.ParameterValues,
	}

	result, err := h.queryService.Test(r.Context(), projectID, datasourceID, serviceReq)
	if err != nil {
		// Check for validation errors (bad request, not execution failure)
		if errors.Is(err, sqlvalidator.ErrMultipleStatements) {
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_sql", err.Error()); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}

		// For execution failures, return error as result (like TestConnection pattern)
		h.logger.Info("Query test failed",
			zap.String("project_id", projectID.String()),
			zap.Error(err))

		data := ExecuteQueryResponse{
			Columns:  []ColumnInfo{},
			Rows:     []map[string]any{},
			RowCount: 0,
		}
		response := ApiResponse{
			Success: false,
			Data:    data,
			Error:   err.Error(),
		}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	// Convert datasource.ColumnInfo to handler's ColumnInfo
	columns := make([]ColumnInfo, len(result.Columns))
	for i, col := range result.Columns {
		columns[i] = ColumnInfo{Name: col.Name, Type: col.Type}
	}

	data := ExecuteQueryResponse{
		Columns:  columns,
		Rows:     result.Rows,
		RowCount: result.RowCount,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Validate handles POST /api/projects/{pid}/datasources/{dsid}/queries/validate
func (h *QueriesHandler) Validate(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	var req ValidateQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.SQLQuery == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_sql", "SQL query is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	result, err := h.queryService.Validate(r.Context(), projectID, datasourceID, req.SQLQuery)
	if err != nil {
		// Validation failure returns success=false with message
		data := ValidateQueryResponse{
			Valid:   false,
			Message: err.Error(),
		}
		response := ApiResponse{Success: true, Data: data}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	// Check for parameters inside string literals (common mistake)
	var warnings []string
	paramsInStrings := sqlvalidator.FindParametersInStringLiterals(req.SQLQuery)
	if len(paramsInStrings) > 0 {
		for _, param := range paramsInStrings {
			warnings = append(warnings, "Parameter {{"+param+"}} is inside a string literal and won't be substituted correctly. Use concatenation instead: 'text ' || {{"+param+"}}")
		}
	}

	data := ValidateQueryResponse{
		Valid:    result.Valid,
		Message:  result.Message,
		Warnings: warnings,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ValidateParameters handles POST /api/projects/{pid}/datasources/{dsid}/queries/validate-parameters
func (h *QueriesHandler) ValidateParameters(w http.ResponseWriter, r *http.Request) {
	_, _, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	var req ValidateParametersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.SQLQuery == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_sql", "SQL query is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	err := h.queryService.ValidateParameterizedQuery(req.SQLQuery, req.Parameters)
	if err != nil {
		// Validation failure returns success=false with message
		data := ValidateParametersResponse{
			Valid:   false,
			Message: err.Error(),
		}
		response := ApiResponse{Success: true, Data: data}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	data := ValidateParametersResponse{
		Valid:   true,
		Message: "Parameters are valid",
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ListEnabled handles GET /api/projects/{pid}/datasources/{dsid}/queries/enabled
func (h *QueriesHandler) ListEnabled(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := ParseProjectAndDatasourceIDs(w, r, h.logger)
	if !ok {
		return
	}

	queries, err := h.queryService.ListEnabled(r.Context(), projectID, datasourceID)
	if err != nil {
		h.logger.Error("Failed to list enabled queries",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to list queries"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := ListQueriesResponse{
		Queries: make([]QueryResponse, len(queries)),
	}
	for i, q := range queries {
		data.Queries[i] = h.toQueryResponse(q)
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ListPending handles GET /api/projects/{pid}/queries/pending
// Returns all pending query suggestions for admin review.
func (h *QueriesHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	queries, err := h.queryService.ListPending(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list pending queries",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to list pending queries"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := ListPendingQueriesResponse{
		Queries: make([]PendingQueryResponse, len(queries)),
		Count:   len(queries),
	}
	for i, q := range queries {
		data.Queries[i] = h.toPendingQueryResponse(q)
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Approve handles POST /api/projects/{pid}/queries/{qid}/approve
// Approves a pending query suggestion.
func (h *QueriesHandler) Approve(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	queryID, ok := ParseQueryID(w, r, h.logger)
	if !ok {
		return
	}

	// Get reviewer ID from auth context
	reviewerID := auth.GetUserIDFromContext(r.Context())
	if reviewerID == "" {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "User ID not found in context"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Approve the query
	if err := h.queryService.ApproveQuery(r.Context(), projectID, queryID, reviewerID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to approve query",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "approve_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Fetch the approved query to return in response
	query, err := h.queryService.Get(r.Context(), projectID, queryID)
	if err != nil {
		// Query was approved but we couldn't fetch it - this can happen for update suggestions
		// where the pending record is soft-deleted after approval
		h.logger.Info("Query approved (update suggestion applied to original)",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()))
		data := ApproveQueryResponse{
			Success: true,
			Message: "Query approved and changes applied to original",
		}
		response := ApiResponse{Success: true, Data: data}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	queryResp := h.toQueryResponse(query)
	data := ApproveQueryResponse{
		Success: true,
		Message: "Query approved and enabled",
		Query:   &queryResp,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Reject handles POST /api/projects/{pid}/queries/{qid}/reject
// Rejects a pending query suggestion with a reason.
func (h *QueriesHandler) Reject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	queryID, ok := ParseQueryID(w, r, h.logger)
	if !ok {
		return
	}

	var req RejectQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Reason == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_reason", "Rejection reason is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Get reviewer ID from auth context
	reviewerID := auth.GetUserIDFromContext(r.Context())
	if reviewerID == "" {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "User ID not found in context"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// First get the query to return in response (before rejecting)
	query, err := h.queryService.Get(r.Context(), projectID, queryID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to get query",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get query"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Reject the query
	if err := h.queryService.RejectQuery(r.Context(), projectID, queryID, reviewerID, req.Reason); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to reject query",
			zap.String("project_id", projectID.String()),
			zap.String("query_id", queryID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "reject_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Update the query status in response to reflect rejection
	query.Status = "rejected"
	query.RejectionReason = &req.Reason

	queryResp := h.toPendingQueryResponse(query)
	data := RejectQueryResponse{
		Success: true,
		Message: "Query rejected",
		Query:   &queryResp,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

func (h *QueriesHandler) toQueryResponse(q *models.Query) QueryResponse {
	resp := QueryResponse{
		QueryID:               q.ID.String(),
		ProjectID:             q.ProjectID.String(),
		DatasourceID:          q.DatasourceID.String(),
		NaturalLanguagePrompt: q.NaturalLanguagePrompt,
		AdditionalContext:     q.AdditionalContext,
		SQLQuery:              q.SQLQuery,
		Dialect:               q.Dialect,
		IsEnabled:             q.IsEnabled,
		AllowsModification:    q.AllowsModification,
		Parameters:            q.Parameters,
		OutputColumns:         q.OutputColumns,
		Constraints:           q.Constraints,
		UsageCount:            q.UsageCount,
		CreatedAt:             q.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:             q.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if q.LastUsedAt != nil {
		lastUsed := q.LastUsedAt.Format("2006-01-02T15:04:05Z07:00")
		resp.LastUsedAt = &lastUsed
	}

	return resp
}

func (h *QueriesHandler) toPendingQueryResponse(q *models.Query) PendingQueryResponse {
	resp := PendingQueryResponse{
		QueryID:               q.ID.String(),
		ProjectID:             q.ProjectID.String(),
		DatasourceID:          q.DatasourceID.String(),
		NaturalLanguagePrompt: q.NaturalLanguagePrompt,
		AdditionalContext:     q.AdditionalContext,
		SQLQuery:              q.SQLQuery,
		Dialect:               q.Dialect,
		Parameters:            q.Parameters,
		OutputColumns:         q.OutputColumns,
		Constraints:           q.Constraints,
		Tags:                  q.Tags,
		Status:                q.Status,
		SuggestedBy:           q.SuggestedBy,
		SuggestionContext:     q.SuggestionContext,
		AllowsModification:    q.AllowsModification,
		CreatedAt:             q.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:             q.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if q.ParentQueryID != nil {
		parentID := q.ParentQueryID.String()
		resp.ParentQueryID = &parentID
	}

	return resp
}
