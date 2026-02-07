package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// MCPAuditRepository provides data access for the MCP audit log.
type MCPAuditRepository interface {
	Create(ctx context.Context, event *models.MCPAuditEvent) error
	List(ctx context.Context, projectID uuid.UUID, filters models.MCPAuditEventFilters) ([]*models.MCPAuditEvent, int, error)
}

type mcpAuditRepository struct{}

func NewMCPAuditRepository() MCPAuditRepository {
	return &mcpAuditRepository{}
}

var _ MCPAuditRepository = (*mcpAuditRepository)(nil)

func (r *mcpAuditRepository) Create(ctx context.Context, event *models.MCPAuditEvent) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	if event.ID == uuid.Nil {
		event.ID = uuid.New()
	}

	requestParamsJSON, err := marshalJSONB(event.RequestParams)
	if err != nil {
		return fmt.Errorf("failed to marshal request_params: %w", err)
	}
	resultSummaryJSON, err := marshalJSONB(event.ResultSummary)
	if err != nil {
		return fmt.Errorf("failed to marshal result_summary: %w", err)
	}
	clientInfoJSON, err := marshalJSONB(event.ClientInfo)
	if err != nil {
		return fmt.Errorf("failed to marshal client_info: %w", err)
	}

	query := `
		INSERT INTO engine_mcp_audit_log (
			id, project_id, user_id, user_email, session_id,
			event_type, tool_name,
			request_params, natural_language, sql_query,
			was_successful, error_message, result_summary,
			duration_ms, security_level, security_flags,
			client_info
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`

	_, err = scope.Conn.Exec(ctx, query,
		event.ID,
		event.ProjectID,
		event.UserID,
		event.UserEmail,
		event.SessionID,
		event.EventType,
		event.ToolName,
		requestParamsJSON,
		event.NaturalLanguage,
		event.SQLQuery,
		event.WasSuccessful,
		event.ErrorMessage,
		resultSummaryJSON,
		event.DurationMs,
		event.SecurityLevel,
		event.SecurityFlags,
		clientInfoJSON,
	)
	if err != nil {
		return fmt.Errorf("failed to create MCP audit event: %w", err)
	}

	return nil
}

func (r *mcpAuditRepository) List(ctx context.Context, projectID uuid.UUID, filters models.MCPAuditEventFilters) ([]*models.MCPAuditEvent, int, error) {
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
		args = append(args, filters.UserID)
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
	if filters.EventType != "" {
		conditions = append(conditions, fmt.Sprintf("event_type = $%d", argIdx))
		args = append(args, filters.EventType)
		argIdx++
	}
	if filters.ToolName != "" {
		conditions = append(conditions, fmt.Sprintf("tool_name = $%d", argIdx))
		args = append(args, filters.ToolName)
		argIdx++
	}
	if filters.SecurityLevel != "" {
		conditions = append(conditions, fmt.Sprintf("security_level = $%d", argIdx))
		args = append(args, filters.SecurityLevel)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	// Count
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM engine_mcp_audit_log WHERE %s`, where)
	var total int
	if err := scope.Conn.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count MCP audit events: %w", err)
	}

	// Data
	dataQuery := fmt.Sprintf(`
		SELECT id, project_id, user_id, user_email, session_id,
		       event_type, tool_name,
		       request_params, natural_language, sql_query,
		       was_successful, error_message, result_summary,
		       duration_ms, security_level, security_flags,
		       client_info, created_at
		FROM engine_mcp_audit_log
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, where, argIdx, argIdx+1)

	args = append(args, limit, offset)

	rows, err := scope.Conn.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list MCP audit events: %w", err)
	}
	defer rows.Close()

	var events []*models.MCPAuditEvent
	for rows.Next() {
		event, err := scanMCPAuditEvent(rows)
		if err != nil {
			return nil, 0, err
		}
		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating MCP audit events: %w", err)
	}

	return events, total, nil
}

func scanMCPAuditEvent(row pgx.Row) (*models.MCPAuditEvent, error) {
	var event models.MCPAuditEvent
	var requestParamsJSON, resultSummaryJSON, clientInfoJSON []byte

	err := row.Scan(
		&event.ID,
		&event.ProjectID,
		&event.UserID,
		&event.UserEmail,
		&event.SessionID,
		&event.EventType,
		&event.ToolName,
		&requestParamsJSON,
		&event.NaturalLanguage,
		&event.SQLQuery,
		&event.WasSuccessful,
		&event.ErrorMessage,
		&resultSummaryJSON,
		&event.DurationMs,
		&event.SecurityLevel,
		&event.SecurityFlags,
		&clientInfoJSON,
		&event.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan MCP audit event: %w", err)
	}

	unmarshalJSONB(requestParamsJSON, &event.RequestParams)
	unmarshalJSONB(resultSummaryJSON, &event.ResultSummary)
	unmarshalJSONB(clientInfoJSON, &event.ClientInfo)

	return &event, nil
}

// marshalJSONB marshals a map to JSON bytes, returning nil for empty/nil maps.
func marshalJSONB(m map[string]any) ([]byte, error) {
	if len(m) == 0 {
		return nil, nil
	}
	return json.Marshal(m)
}

// unmarshalJSONB unmarshals JSON bytes into a map, silently ignoring nil/empty input.
func unmarshalJSONB(data []byte, target *map[string]any) {
	if len(data) > 0 && string(data) != "null" {
		_ = json.Unmarshal(data, target)
	}
}
