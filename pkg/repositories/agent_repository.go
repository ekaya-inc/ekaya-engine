package repositories

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AgentRepository provides data access for named AI agents.
type AgentRepository interface {
	Create(ctx context.Context, agent *models.Agent, queryIDs []uuid.UUID) error
	GetByID(ctx context.Context, projectID, agentID uuid.UUID) (*models.Agent, error)
	ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error)
	UpdateAPIKey(ctx context.Context, agentID uuid.UUID, encryptedKey string) error
	Delete(ctx context.Context, projectID, agentID uuid.UUID) error
	SetQueryAccess(ctx context.Context, agentID uuid.UUID, queryIDs []uuid.UUID) error
	GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error)
	GetQueryAccessByAgentIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error)
	FindByAPIKey(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error)
	RecordAccess(ctx context.Context, agentID uuid.UUID) error
}

type agentRepository struct{}

type agentExecer interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// NewAgentRepository creates a new AgentRepository.
func NewAgentRepository() AgentRepository {
	return &agentRepository{}
}

func (r *agentRepository) Create(ctx context.Context, agent *models.Agent, queryIDs []uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best effort cleanup

	if err := r.insertAgent(ctx, tx, agent); err != nil {
		return err
	}

	if len(queryIDs) > 0 {
		if err := r.setQueryAccessTx(ctx, tx, agent.ID, queryIDs); err != nil {
			return err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit agent create: %w", err)
	}

	return nil
}

func (r *agentRepository) insertAgent(ctx context.Context, execer agentExecer, agent *models.Agent) error {
	now := time.Now().UTC()
	if agent.ID == uuid.Nil {
		agent.ID = uuid.New()
	}
	agent.CreatedAt = now
	agent.UpdatedAt = now

	query := `
		INSERT INTO engine_agents (id, project_id, name, api_key_encrypted, created_at, updated_at, last_access_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := execer.Exec(ctx, query,
		agent.ID,
		agent.ProjectID,
		agent.Name,
		agent.APIKeyEncrypted,
		agent.CreatedAt,
		agent.UpdatedAt,
		agent.LastAccessAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return apperrors.ErrConflict
		}
		return fmt.Errorf("failed to create agent: %w", err)
	}

	return nil
}

func (r *agentRepository) GetByID(ctx context.Context, projectID, agentID uuid.UUID) (*models.Agent, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT a.id, a.project_id, a.name, a.api_key_encrypted, a.created_at, a.updated_at, a.last_access_at,
		       COALESCE(audit.total_mcp_calls, 0)
		FROM engine_agents a
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS total_mcp_calls
			FROM engine_mcp_audit_log l
			WHERE l.project_id = a.project_id
			  AND l.user_id = 'agent:' || a.id::text
		) audit ON true
		WHERE a.project_id = $1 AND a.id = $2`

	var agent models.Agent
	err := scope.Conn.QueryRow(ctx, query, projectID, agentID).Scan(
		&agent.ID,
		&agent.ProjectID,
		&agent.Name,
		&agent.APIKeyEncrypted,
		&agent.CreatedAt,
		&agent.UpdatedAt,
		&agent.LastAccessAt,
		&agent.MCPCallCount,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	return &agent, nil
}

func (r *agentRepository) ListByProject(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT a.id, a.project_id, a.name, a.api_key_encrypted, a.created_at, a.updated_at, a.last_access_at,
		       COALESCE(audit.total_mcp_calls, 0)
		FROM engine_agents a
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::bigint AS total_mcp_calls
			FROM engine_mcp_audit_log l
			WHERE l.project_id = a.project_id
			  AND l.user_id = 'agent:' || a.id::text
		) audit ON true
		WHERE a.project_id = $1
		ORDER BY a.created_at ASC, a.name ASC`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}
	defer rows.Close()

	var agents []*models.Agent
	for rows.Next() {
		var agent models.Agent
		if err := rows.Scan(
			&agent.ID,
			&agent.ProjectID,
			&agent.Name,
			&agent.APIKeyEncrypted,
			&agent.CreatedAt,
			&agent.UpdatedAt,
			&agent.LastAccessAt,
			&agent.MCPCallCount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		agents = append(agents, &agent)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agents: %w", err)
	}

	return agents, nil
}

func (r *agentRepository) UpdateAPIKey(ctx context.Context, agentID uuid.UUID, encryptedKey string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_agents
		SET api_key_encrypted = $2, updated_at = $3
		WHERE id = $1`

	tag, err := scope.Conn.Exec(ctx, query, agentID, encryptedKey, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update agent API key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

func (r *agentRepository) Delete(ctx context.Context, projectID, agentID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_agents WHERE project_id = $1 AND id = $2`
	tag, err := scope.Conn.Exec(ctx, query, projectID, agentID)
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

func (r *agentRepository) SetQueryAccess(ctx context.Context, agentID uuid.UUID, queryIDs []uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // best effort cleanup

	if err := r.setQueryAccessTx(ctx, tx, agentID, queryIDs); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit agent query access: %w", err)
	}

	return nil
}

func (r *agentRepository) setQueryAccessTx(ctx context.Context, tx pgx.Tx, agentID uuid.UUID, queryIDs []uuid.UUID) error {
	if _, err := tx.Exec(ctx, `DELETE FROM engine_agent_queries WHERE agent_id = $1`, agentID); err != nil {
		return fmt.Errorf("failed to clear agent query access: %w", err)
	}

	for _, queryID := range queryIDs {
		tag, err := tx.Exec(ctx, `
			INSERT INTO engine_agent_queries (agent_id, query_id)
			SELECT a.id, q.id
			FROM engine_agents a
			JOIN engine_queries q
			  ON q.id = $2
			 AND q.project_id = a.project_id
			WHERE a.id = $1
			  AND q.is_enabled = true
			  AND q.status = 'approved'
			  AND q.deleted_at IS NULL`,
			agentID,
			queryID,
		)
		if err != nil {
			return fmt.Errorf("failed to set agent query access: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return fmt.Errorf("%w: %s", apperrors.ErrNotFound, queryID.String())
		}
	}

	return nil
}

func (r *agentRepository) GetQueryAccess(ctx context.Context, agentID uuid.UUID) ([]uuid.UUID, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	rows, err := scope.Conn.Query(ctx, `
		SELECT query_id
		FROM engine_agent_queries
		WHERE agent_id = $1
		ORDER BY query_id ASC`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent query access: %w", err)
	}
	defer rows.Close()

	var queryIDs []uuid.UUID
	for rows.Next() {
		var queryID uuid.UUID
		if err := rows.Scan(&queryID); err != nil {
			return nil, fmt.Errorf("failed to scan agent query access: %w", err)
		}
		queryIDs = append(queryIDs, queryID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate agent query access: %w", err)
	}

	return queryIDs, nil
}

func (r *agentRepository) GetQueryAccessByAgentIDs(ctx context.Context, agentIDs []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	result := make(map[uuid.UUID][]uuid.UUID, len(agentIDs))
	if len(agentIDs) == 0 {
		return result, nil
	}

	rows, err := scope.Conn.Query(ctx, `
		SELECT agent_id, query_id
		FROM engine_agent_queries
		WHERE agent_id = ANY($1)
		ORDER BY agent_id ASC, query_id ASC`,
		agentIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to batch get agent query access: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var agentID uuid.UUID
		var queryID uuid.UUID
		if err := rows.Scan(&agentID, &queryID); err != nil {
			return nil, fmt.Errorf("failed to scan agent query access: %w", err)
		}
		result[agentID] = append(result[agentID], queryID)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate batched agent query access: %w", err)
	}

	return result, nil
}

func (r *agentRepository) HasQueryAccess(ctx context.Context, agentID, queryID uuid.UUID) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	var exists bool
	err := scope.Conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM engine_agent_queries
			WHERE agent_id = $1 AND query_id = $2
		)`,
		agentID,
		queryID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check agent query access: %w", err)
	}

	return exists, nil
}

func (r *agentRepository) FindByAPIKey(ctx context.Context, projectID uuid.UUID) ([]*models.Agent, error) {
	return r.ListByProject(ctx, projectID)
}

func (r *agentRepository) RecordAccess(ctx context.Context, agentID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_agents
		SET last_access_at = $2
		WHERE id = $1`

	_, err := scope.Conn.Exec(ctx, query, agentID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to record agent access: %w", err)
	}

	return nil
}

var _ AgentRepository = (*agentRepository)(nil)
