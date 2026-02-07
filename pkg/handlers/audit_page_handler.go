package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// AuditPageHandler handles audit page HTTP requests.
type AuditPageHandler struct {
	auditPageService services.AuditPageService
	logger           *zap.Logger
}

// NewAuditPageHandler creates a new audit page handler.
func NewAuditPageHandler(auditPageService services.AuditPageService, logger *zap.Logger) *AuditPageHandler {
	return &AuditPageHandler{
		auditPageService: auditPageService,
		logger:           logger,
	}
}

// RegisterRoutes registers the audit page handler's routes on the given mux.
func (h *AuditPageHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/audit"

	mux.HandleFunc("GET "+base+"/query-executions",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListQueryExecutions)))
	mux.HandleFunc("GET "+base+"/ontology-changes",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListOntologyChanges)))
	mux.HandleFunc("GET "+base+"/schema-changes",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListSchemaChanges)))
	mux.HandleFunc("GET "+base+"/query-approvals",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListQueryApprovals)))
	mux.HandleFunc("GET "+base+"/summary",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetSummary)))
	mux.HandleFunc("GET "+base+"/mcp-events",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListMCPEvents)))
}

// PaginatedResponse wraps paginated results with metadata.
type PaginatedResponse struct {
	Items  any `json:"items"`
	Total  int `json:"total"`
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// ListQueryExecutions handles GET /api/projects/{pid}/audit/query-executions
func (h *AuditPageHandler) ListQueryExecutions(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	filters := models.QueryExecutionFilters{
		AuditPageFilters: parsePageFilters(r),
		UserID:           r.URL.Query().Get("user_id"),
		Source:           r.URL.Query().Get("source"),
	}

	if v := r.URL.Query().Get("success"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			filters.Success = &b
		}
	}
	if v := r.URL.Query().Get("is_modifying"); v != "" {
		b, err := strconv.ParseBool(v)
		if err == nil {
			filters.IsModifying = &b
		}
	}
	if v := r.URL.Query().Get("query_id"); v != "" {
		qid, err := uuid.Parse(v)
		if err == nil {
			filters.QueryID = &qid
		}
	}

	results, total, err := h.auditPageService.ListQueryExecutions(r.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list query executions", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_query_executions_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if results == nil {
		results = make([]*models.QueryExecutionRow, 0)
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: PaginatedResponse{
			Items:  results,
			Total:  total,
			Limit:  filters.Limit,
			Offset: filters.Offset,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ListOntologyChanges handles GET /api/projects/{pid}/audit/ontology-changes
func (h *AuditPageHandler) ListOntologyChanges(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	filters := models.OntologyChangeFilters{
		AuditPageFilters: parsePageFilters(r),
		UserID:           r.URL.Query().Get("user_id"),
		EntityType:       r.URL.Query().Get("entity_type"),
		Action:           r.URL.Query().Get("action"),
		Source:           r.URL.Query().Get("source"),
	}

	results, total, err := h.auditPageService.ListOntologyChanges(r.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list ontology changes", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_ontology_changes_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if results == nil {
		results = make([]*models.AuditLogEntry, 0)
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: PaginatedResponse{
			Items:  results,
			Total:  total,
			Limit:  filters.Limit,
			Offset: filters.Offset,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ListSchemaChanges handles GET /api/projects/{pid}/audit/schema-changes
func (h *AuditPageHandler) ListSchemaChanges(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	filters := models.SchemaChangeFilters{
		AuditPageFilters: parsePageFilters(r),
		ChangeType:       r.URL.Query().Get("change_type"),
		Status:           r.URL.Query().Get("status"),
		TableName:        r.URL.Query().Get("table_name"),
	}

	results, total, err := h.auditPageService.ListSchemaChanges(r.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list schema changes", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_schema_changes_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if results == nil {
		results = make([]*models.PendingChange, 0)
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: PaginatedResponse{
			Items:  results,
			Total:  total,
			Limit:  filters.Limit,
			Offset: filters.Offset,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ListQueryApprovals handles GET /api/projects/{pid}/audit/query-approvals
func (h *AuditPageHandler) ListQueryApprovals(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	filters := models.QueryApprovalFilters{
		AuditPageFilters: parsePageFilters(r),
		Status:           r.URL.Query().Get("status"),
		SuggestedBy:      r.URL.Query().Get("suggested_by"),
		ReviewedBy:       r.URL.Query().Get("reviewed_by"),
	}

	results, total, err := h.auditPageService.ListQueryApprovals(r.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list query approvals", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_query_approvals_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if results == nil {
		results = make([]*models.Query, 0)
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: PaginatedResponse{
			Items:  results,
			Total:  total,
			Limit:  filters.Limit,
			Offset: filters.Offset,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetSummary handles GET /api/projects/{pid}/audit/summary
func (h *AuditPageHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	summary, err := h.auditPageService.GetSummary(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get audit summary", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_audit_summary_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: summary}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// ListMCPEvents handles GET /api/projects/{pid}/audit/mcp-events
func (h *AuditPageHandler) ListMCPEvents(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	filters := models.MCPAuditEventFilters{
		AuditPageFilters: parsePageFilters(r),
		UserID:           r.URL.Query().Get("user_id"),
		EventType:        r.URL.Query().Get("event_type"),
		ToolName:         r.URL.Query().Get("tool_name"),
		SecurityLevel:    r.URL.Query().Get("security_level"),
	}

	results, total, err := h.auditPageService.ListMCPEvents(r.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list MCP events", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_mcp_events_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if results == nil {
		results = make([]*models.MCPAuditEvent, 0)
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: PaginatedResponse{
			Items:  results,
			Total:  total,
			Limit:  filters.Limit,
			Offset: filters.Offset,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// parsePageFilters extracts common pagination and time-range filters from query params.
func parsePageFilters(r *http.Request) models.AuditPageFilters {
	filters := models.AuditPageFilters{
		Limit:  50,
		Offset: 0,
	}

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filters.Limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			filters.Offset = n
		}
	}
	if v := r.URL.Query().Get("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filters.Since = &t
		}
	}
	if v := r.URL.Query().Get("until"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filters.Until = &t
		}
	}

	return filters
}
