package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AuditPageFilters contains common pagination and time-range filters.
type AuditPageFilters struct {
	Since  *time.Time
	Until  *time.Time
	Limit  int
	Offset int
}

// QueryExecutionFilters contains filters for the query executions tab.
type QueryExecutionFilters struct {
	AuditPageFilters
	UserID      string
	Success     *bool
	IsModifying *bool
	Source      string
	QueryID     *uuid.UUID
}

// OntologyChangeFilters contains filters for the ontology changes tab.
type OntologyChangeFilters struct {
	AuditPageFilters
	UserID     string
	EntityType string
	Action     string
	Source     string
}

// SchemaChangeFilters contains filters for the schema changes tab.
type SchemaChangeFilters struct {
	AuditPageFilters
	ChangeType string
	Status     string
	TableName  string
}

// QueryApprovalFilters contains filters for the query approvals tab.
type QueryApprovalFilters struct {
	AuditPageFilters
	Status     string
	SuggestedBy string
	ReviewedBy  string
}

// AuditSummary contains aggregate counts for the audit dashboard header.
type AuditSummary struct {
	TotalQueryExecutions  int `json:"total_query_executions"`
	FailedQueryCount      int `json:"failed_query_count"`
	DestructiveQueryCount int `json:"destructive_query_count"`
	OntologyChangesCount  int `json:"ontology_changes_count"`
	PendingSchemaChanges  int `json:"pending_schema_changes"`
	PendingQueryApprovals int `json:"pending_query_approvals"`
}

// QueryExecutionRow represents a row from the query executions audit view.
type QueryExecutionRow struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	QueryID         uuid.UUID  `json:"query_id"`
	SQL             string     `json:"sql"`
	ExecutedAt      time.Time  `json:"executed_at"`
	RowCount        int        `json:"row_count"`
	ExecutionTimeMs int        `json:"execution_time_ms"`
	UserID          *string    `json:"user_id,omitempty"`
	Source          string     `json:"source"`
	IsModifying     bool       `json:"is_modifying"`
	Success         bool       `json:"success"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
	QueryName       *string    `json:"query_name,omitempty"` // Joined from engine_queries
}

// AuditPageRepository provides data access for the audit page.
type AuditPageRepository interface {
	ListQueryExecutions(ctx context.Context, projectID uuid.UUID, filters QueryExecutionFilters) ([]*QueryExecutionRow, int, error)
	ListOntologyChanges(ctx context.Context, projectID uuid.UUID, filters OntologyChangeFilters) ([]*models.AuditLogEntry, int, error)
	ListSchemaChanges(ctx context.Context, projectID uuid.UUID, filters SchemaChangeFilters) ([]*models.PendingChange, int, error)
	ListQueryApprovals(ctx context.Context, projectID uuid.UUID, filters QueryApprovalFilters) ([]*models.Query, int, error)
	GetSummary(ctx context.Context, projectID uuid.UUID) (*AuditSummary, error)
}

type auditPageRepository struct{}

func NewAuditPageRepository() AuditPageRepository {
	return &auditPageRepository{}
}

var _ AuditPageRepository = (*auditPageRepository)(nil)

func (r *auditPageRepository) ListQueryExecutions(ctx context.Context, projectID uuid.UUID, filters QueryExecutionFilters) ([]*QueryExecutionRow, int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, 0, fmt.Errorf("no tenant scope in context")
	}

	limit, offset := normalizePageParams(filters.Limit, filters.Offset)

	// Build WHERE conditions
	conditions := []string{"e.project_id = $1"}
	args := []any{projectID}
	argIdx := 2

	if filters.UserID != "" {
		conditions = append(conditions, fmt.Sprintf("e.user_id = $%d", argIdx))
		args = append(args, filters.UserID)
		argIdx++
	}
	if filters.Since != nil {
		conditions = append(conditions, fmt.Sprintf("e.executed_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}
	if filters.Until != nil {
		conditions = append(conditions, fmt.Sprintf("e.executed_at <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}
	if filters.Success != nil {
		conditions = append(conditions, fmt.Sprintf("e.success = $%d", argIdx))
		args = append(args, *filters.Success)
		argIdx++
	}
	if filters.IsModifying != nil {
		conditions = append(conditions, fmt.Sprintf("e.is_modifying = $%d", argIdx))
		args = append(args, *filters.IsModifying)
		argIdx++
	}
	if filters.Source != "" {
		conditions = append(conditions, fmt.Sprintf("e.source = $%d", argIdx))
		args = append(args, filters.Source)
		argIdx++
	}
	if filters.QueryID != nil {
		conditions = append(conditions, fmt.Sprintf("e.query_id = $%d", argIdx))
		args = append(args, *filters.QueryID)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count query
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_query_executions e WHERE %s`, where)
	var total int
	if err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count query executions: %w", err)
	}

	// Data query with join to engine_queries for query name
	dataQuery := fmt.Sprintf(`
		SELECT e.id, e.project_id, e.query_id, e.sql, e.executed_at, e.row_count,
		       e.execution_time_ms, e.user_id, e.source, e.is_modifying, e.success,
		       e.error_message, q.natural_language_prompt
		FROM engine_query_executions e
		LEFT JOIN engine_queries q ON q.id = e.query_id AND q.deleted_at IS NULL
		WHERE %s
		ORDER BY e.executed_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := scope.Conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list query executions: %w", err)
	}
	defer rows.Close()

	var results []*QueryExecutionRow
	for rows.Next() {
		row := &QueryExecutionRow{}
		if err := rows.Scan(
			&row.ID, &row.ProjectID, &row.QueryID, &row.SQL, &row.ExecutedAt,
			&row.RowCount, &row.ExecutionTimeMs, &row.UserID, &row.Source,
			&row.IsModifying, &row.Success, &row.ErrorMessage, &row.QueryName,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan query execution: %w", err)
		}
		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating query executions: %w", err)
	}

	return results, total, nil
}

func (r *auditPageRepository) ListOntologyChanges(ctx context.Context, projectID uuid.UUID, filters OntologyChangeFilters) ([]*models.AuditLogEntry, int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, 0, fmt.Errorf("no tenant scope in context")
	}

	limit, offset := normalizePageParams(filters.Limit, filters.Offset)

	conditions := []string{"project_id = $1"}
	args := []any{projectID}
	argIdx := 2

	if filters.UserID != "" {
		conditions = append(conditions, fmt.Sprintf("user_id = $%d", argIdx))
		uid, err := uuid.Parse(filters.UserID)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid user_id: %w", err)
		}
		args = append(args, uid)
		argIdx++
	}
	if filters.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}
	if filters.Until != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}
	if filters.EntityType != "" {
		conditions = append(conditions, fmt.Sprintf("entity_type = $%d", argIdx))
		args = append(args, filters.EntityType)
		argIdx++
	}
	if filters.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, filters.Action)
		argIdx++
	}
	if filters.Source != "" {
		conditions = append(conditions, fmt.Sprintf("source = $%d", argIdx))
		args = append(args, filters.Source)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_audit_log WHERE %s`, where)
	var total int
	if err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count ontology changes: %w", err)
	}

	// Data
	dataQuery := fmt.Sprintf(`
		SELECT id, project_id, entity_type, entity_id, action, source, user_id, changed_fields, created_at
		FROM engine_audit_log
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := scope.Conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list ontology changes: %w", err)
	}
	defer rows.Close()

	var entries []*models.AuditLogEntry
	for rows.Next() {
		entry, err := scanAuditLogEntry(rows)
		if err != nil {
			return nil, 0, err
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating ontology changes: %w", err)
	}

	return entries, total, nil
}

func (r *auditPageRepository) ListSchemaChanges(ctx context.Context, projectID uuid.UUID, filters SchemaChangeFilters) ([]*models.PendingChange, int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, 0, fmt.Errorf("no tenant scope in context")
	}

	limit, offset := normalizePageParams(filters.Limit, filters.Offset)

	conditions := []string{"project_id = $1"}
	args := []any{projectID}
	argIdx := 2

	if filters.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}
	if filters.Until != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}
	if filters.ChangeType != "" {
		conditions = append(conditions, fmt.Sprintf("change_type = $%d", argIdx))
		args = append(args, filters.ChangeType)
		argIdx++
	}
	if filters.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.TableName != "" {
		conditions = append(conditions, fmt.Sprintf("table_name ILIKE $%d", argIdx))
		args = append(args, "%"+filters.TableName+"%")
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_ontology_pending_changes WHERE %s`, where)
	var total int
	if err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count schema changes: %w", err)
	}

	// Data
	dataQuery := fmt.Sprintf(`
		SELECT id, project_id, change_type, change_source, table_name, column_name,
		       old_value, new_value, suggested_action, suggested_payload,
		       status, reviewed_by, reviewed_at, created_at
		FROM engine_ontology_pending_changes
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := scope.Conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list schema changes: %w", err)
	}
	defer rows.Close()

	changes, err := scanPendingChanges(rows)
	if err != nil {
		return nil, 0, err
	}
	return changes, total, nil
}

func (r *auditPageRepository) ListQueryApprovals(ctx context.Context, projectID uuid.UUID, filters QueryApprovalFilters) ([]*models.Query, int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, 0, fmt.Errorf("no tenant scope in context")
	}

	limit, offset := normalizePageParams(filters.Limit, filters.Offset)

	// Filter to queries with suggestion workflow activity
	conditions := []string{"project_id = $1", "suggested_by IS NOT NULL", "deleted_at IS NULL"}
	args := []any{projectID}
	argIdx := 2

	if filters.Since != nil {
		conditions = append(conditions, fmt.Sprintf("created_at >= $%d", argIdx))
		args = append(args, *filters.Since)
		argIdx++
	}
	if filters.Until != nil {
		conditions = append(conditions, fmt.Sprintf("created_at <= $%d", argIdx))
		args = append(args, *filters.Until)
		argIdx++
	}
	if filters.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.SuggestedBy != "" {
		conditions = append(conditions, fmt.Sprintf("suggested_by = $%d", argIdx))
		args = append(args, filters.SuggestedBy)
		argIdx++
	}
	if filters.ReviewedBy != "" {
		conditions = append(conditions, fmt.Sprintf("reviewed_by = $%d", argIdx))
		args = append(args, filters.ReviewedBy)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_queries WHERE %s`, where)
	var total int
	if err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count query approvals: %w", err)
	}

	// Data
	dataQuery := fmt.Sprintf(`SELECT `+querySelectColumns+`
		FROM engine_queries
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := scope.Conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list query approvals: %w", err)
	}
	defer rows.Close()

	queries := make([]*models.Query, 0)
	for rows.Next() {
		q, err := scanQuery(rows)
		if err != nil {
			return nil, 0, err
		}
		queries = append(queries, q)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating query approvals: %w", err)
	}

	return queries, total, nil
}

func (r *auditPageRepository) GetSummary(ctx context.Context, projectID uuid.UUID) (*AuditSummary, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	summary := &AuditSummary{}
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	// Total query executions (last 30d)
	err := scope.Conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM engine_query_executions WHERE project_id = $1 AND executed_at >= $2`,
		projectID, thirtyDaysAgo).Scan(&summary.TotalQueryExecutions)
	if err != nil {
		return nil, fmt.Errorf("failed to count total query executions: %w", err)
	}

	// Failed queries (last 30d)
	err = scope.Conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM engine_query_executions WHERE project_id = $1 AND executed_at >= $2 AND success = false`,
		projectID, thirtyDaysAgo).Scan(&summary.FailedQueryCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count failed queries: %w", err)
	}

	// Destructive queries (last 30d)
	err = scope.Conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM engine_query_executions WHERE project_id = $1 AND executed_at >= $2 AND is_modifying = true`,
		projectID, thirtyDaysAgo).Scan(&summary.DestructiveQueryCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count destructive queries: %w", err)
	}

	// Ontology changes (last 30d)
	err = scope.Conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM engine_audit_log WHERE project_id = $1 AND created_at >= $2`,
		projectID, thirtyDaysAgo).Scan(&summary.OntologyChangesCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count ontology changes: %w", err)
	}

	// Pending schema changes
	err = scope.Conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM engine_ontology_pending_changes WHERE project_id = $1 AND status = 'pending'`,
		projectID).Scan(&summary.PendingSchemaChanges)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending schema changes: %w", err)
	}

	// Pending query approvals
	err = scope.Conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM engine_queries WHERE project_id = $1 AND status = 'pending' AND deleted_at IS NULL`,
		projectID).Scan(&summary.PendingQueryApprovals)
	if err != nil {
		return nil, fmt.Errorf("failed to count pending query approvals: %w", err)
	}

	return summary, nil
}

// normalizePageParams ensures limit and offset are within reasonable bounds.
func normalizePageParams(limit, offset int) (int, int) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
