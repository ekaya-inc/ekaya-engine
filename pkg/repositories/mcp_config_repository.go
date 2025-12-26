package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// MCPConfigRepository defines the interface for MCP configuration data access.
type MCPConfigRepository interface {
	// Get retrieves the MCP config for a project. Returns nil if not found.
	Get(ctx context.Context, projectID uuid.UUID) (*models.MCPConfig, error)

	// Upsert creates or updates the MCP config for a project.
	Upsert(ctx context.Context, config *models.MCPConfig) error
}

// mcpConfigRepository implements MCPConfigRepository using PostgreSQL.
type mcpConfigRepository struct{}

// NewMCPConfigRepository creates a new MCP config repository.
func NewMCPConfigRepository() MCPConfigRepository {
	return &mcpConfigRepository{}
}

// Get retrieves the MCP config for a project.
func (r *mcpConfigRepository) Get(ctx context.Context, projectID uuid.UUID) (*models.MCPConfig, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT project_id, tool_groups, created_at, updated_at
		FROM engine_mcp_config
		WHERE project_id = $1`

	var config models.MCPConfig
	var toolGroupsJSON []byte
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(
		&config.ProjectID,
		&toolGroupsJSON,
		&config.CreatedAt,
		&config.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found, return nil without error
		}
		return nil, fmt.Errorf("failed to get MCP config: %w", err)
	}

	if err := json.Unmarshal(toolGroupsJSON, &config.ToolGroups); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool_groups: %w", err)
	}

	return &config, nil
}

// Upsert creates or updates the MCP config for a project.
func (r *mcpConfigRepository) Upsert(ctx context.Context, config *models.MCPConfig) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	toolGroupsJSON, err := json.Marshal(config.ToolGroups)
	if err != nil {
		return fmt.Errorf("failed to marshal tool_groups: %w", err)
	}

	now := time.Now()
	query := `
		INSERT INTO engine_mcp_config (project_id, tool_groups, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (project_id)
		DO UPDATE SET tool_groups = $2, updated_at = $3`

	_, err = scope.Conn.Exec(ctx, query, config.ProjectID, toolGroupsJSON, now)
	if err != nil {
		return fmt.Errorf("failed to upsert MCP config: %w", err)
	}

	config.UpdatedAt = now
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}

	return nil
}

// Ensure mcpConfigRepository implements MCPConfigRepository at compile time.
var _ MCPConfigRepository = (*mcpConfigRepository)(nil)
