package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// QueryResponse matches frontend Query interface.
type QueryResponse struct {
	QueryID               string  `json:"query_id"`
	ProjectID             string  `json:"project_id"`
	DatasourceID          string  `json:"datasource_id"`
	NaturalLanguagePrompt string  `json:"natural_language_prompt"`
	AdditionalContext     *string `json:"additional_context,omitempty"`
	SQLQuery              string  `json:"sql_query"`
	Dialect               string  `json:"dialect"`
	IsEnabled             bool    `json:"is_enabled"`
	UsageCount            int     `json:"usage_count"`
	LastUsedAt            *string `json:"last_used_at,omitempty"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

// ListQueriesResponse wraps array for frontend compatibility.
type ListQueriesResponse struct {
	Queries []QueryResponse `json:"queries"`
}

// CreateQueryRequest for POST body.
type CreateQueryRequest struct {
	NaturalLanguagePrompt string `json:"natural_language_prompt"`
	AdditionalContext     string `json:"additional_context,omitempty"`
	SQLQuery              string `json:"sql_query"`
	Dialect               string `json:"dialect"`
	IsEnabled             bool   `json:"is_enabled"`
}

// UpdateQueryRequest for PUT body (all fields optional).
type UpdateQueryRequest struct {
	NaturalLanguagePrompt *string `json:"natural_language_prompt,omitempty"`
	AdditionalContext     *string `json:"additional_context,omitempty"`
	SQLQuery              *string `json:"sql_query,omitempty"`
	Dialect               *string `json:"dialect,omitempty"`
	IsEnabled             *bool   `json:"is_enabled,omitempty"`
}

// ExecuteQueryRequest for POST execute body.
type ExecuteQueryRequest struct {
	Limit int `json:"limit,omitempty"`
}

// ExecuteQueryResponse for query execution results.
type ExecuteQueryResponse struct {
	Columns  []string         `json:"columns"`
	Rows     []map[string]any `json:"rows"`
	RowCount int              `json:"row_count"`
}

// TestQueryRequest for POST test body.
type TestQueryRequest struct {
	SQLQuery string `json:"sql_query"`
	Limit    int    `json:"limit,omitempty"`
}

// ValidateQueryRequest for POST validate body.
type ValidateQueryRequest struct {
	SQLQuery string `json:"sql_query"`
}

// ValidateQueryResponse for validation results.
type ValidateQueryResponse struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
}

// DeleteQueryResponse for delete result.
type DeleteQueryResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
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
	base := "/api/projects/{pid}/datasources/{did}/queries"

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

	// Filtering endpoints
	mux.HandleFunc("GET "+base+"/enabled",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListEnabled)))
}

// List handles GET /api/projects/{pid}/datasources/{did}/queries
func (h *QueriesHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := h.parseProjectAndDatasource(w, r)
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

// Create handles POST /api/projects/{pid}/datasources/{did}/queries
func (h *QueriesHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := h.parseProjectAndDatasource(w, r)
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

	if req.Dialect == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_dialect", "Dialect is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	serviceReq := &services.CreateQueryRequest{
		NaturalLanguagePrompt: req.NaturalLanguagePrompt,
		AdditionalContext:     req.AdditionalContext,
		SQLQuery:              req.SQLQuery,
		Dialect:               req.Dialect,
		IsEnabled:             req.IsEnabled,
	}

	query, err := h.queryService.Create(r.Context(), projectID, datasourceID, serviceReq)
	if err != nil {
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

// Get handles GET /api/projects/{pid}/datasources/{did}/queries/{qid}
func (h *QueriesHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := h.parseProjectAndDatasource(w, r)
	if !ok {
		return
	}

	queryID, ok := h.parseQueryID(w, r)
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

// Update handles PUT /api/projects/{pid}/datasources/{did}/queries/{qid}
func (h *QueriesHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := h.parseProjectAndDatasource(w, r)
	if !ok {
		return
	}

	queryID, ok := h.parseQueryID(w, r)
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
		Dialect:               req.Dialect,
		IsEnabled:             req.IsEnabled,
	}

	query, err := h.queryService.Update(r.Context(), projectID, queryID, serviceReq)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Query not found"); err != nil {
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

// Delete handles DELETE /api/projects/{pid}/datasources/{did}/queries/{qid}
func (h *QueriesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := h.parseProjectAndDatasource(w, r)
	if !ok {
		return
	}

	queryID, ok := h.parseQueryID(w, r)
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

// Execute handles POST /api/projects/{pid}/datasources/{did}/queries/{qid}/execute
func (h *QueriesHandler) Execute(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := h.parseProjectAndDatasource(w, r)
	if !ok {
		return
	}

	queryID, ok := h.parseQueryID(w, r)
	if !ok {
		return
	}

	var req ExecuteQueryRequest
	// Body is optional for execute
	_ = json.NewDecoder(r.Body).Decode(&req)

	serviceReq := &services.ExecuteQueryRequest{
		Limit: req.Limit,
	}

	result, err := h.queryService.Execute(r.Context(), projectID, queryID, serviceReq)
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

	data := ExecuteQueryResponse{
		Columns:  result.Columns,
		Rows:     result.Rows,
		RowCount: result.RowCount,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Test handles POST /api/projects/{pid}/datasources/{did}/queries/test
func (h *QueriesHandler) Test(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := h.parseProjectAndDatasource(w, r)
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
		SQLQuery: req.SQLQuery,
		Limit:    req.Limit,
	}

	result, err := h.queryService.Test(r.Context(), projectID, datasourceID, serviceReq)
	if err != nil {
		// For test, return error as result (like TestConnection pattern)
		h.logger.Info("Query test failed",
			zap.String("project_id", projectID.String()),
			zap.Error(err))

		data := ExecuteQueryResponse{
			Columns:  []string{},
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

	data := ExecuteQueryResponse{
		Columns:  result.Columns,
		Rows:     result.Rows,
		RowCount: result.RowCount,
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Validate handles POST /api/projects/{pid}/datasources/{did}/queries/validate
func (h *QueriesHandler) Validate(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := h.parseProjectAndDatasource(w, r)
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

	err := h.queryService.Validate(r.Context(), projectID, datasourceID, req.SQLQuery)
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

	data := ValidateQueryResponse{
		Valid:   true,
		Message: "SQL is valid",
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ListEnabled handles GET /api/projects/{pid}/datasources/{did}/queries/enabled
func (h *QueriesHandler) ListEnabled(w http.ResponseWriter, r *http.Request) {
	projectID, datasourceID, ok := h.parseProjectAndDatasource(w, r)
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

// Helper methods

func (h *QueriesHandler) parseProjectAndDatasource(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, uuid.Nil, false
	}

	didStr := r.PathValue("did")
	datasourceID, err := uuid.Parse(didStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_datasource_id", "Invalid datasource ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, uuid.Nil, false
	}

	return projectID, datasourceID, true
}

func (h *QueriesHandler) parseQueryID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	qidStr := r.PathValue("qid")
	queryID, err := uuid.Parse(qidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_query_id", "Invalid query ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return uuid.Nil, false
	}
	return queryID, true
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
