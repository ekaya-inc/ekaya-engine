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

	// Agent API Key operations
	GetAgentAPIKey(ctx context.Context, projectID uuid.UUID) (string, error)
	SetAgentAPIKey(ctx context.Context, projectID uuid.UUID, encryptedKey string) error
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
		SELECT project_id, tool_groups, agent_api_key_encrypted, created_at, updated_at
		FROM engine_mcp_config
		WHERE project_id = $1`

	var config models.MCPConfig
	var toolGroupsJSON []byte
	var agentAPIKeyEncrypted *string
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(
		&config.ProjectID,
		&toolGroupsJSON,
		&agentAPIKeyEncrypted,
		&config.CreatedAt,
		&config.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // Not found, return nil without error
		}
		return nil, fmt.Errorf("failed to get MCP config: %w", err)
	}

	if agentAPIKeyEncrypted != nil {
		config.AgentAPIKeyEncrypted = *agentAPIKeyEncrypted
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

	// Convert empty string to nil for nullable column
	var agentAPIKeyEncrypted *string
	if config.AgentAPIKeyEncrypted != "" {
		agentAPIKeyEncrypted = &config.AgentAPIKeyEncrypted
	}

	query := `
		INSERT INTO engine_mcp_config (project_id, tool_groups, agent_api_key_encrypted, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (project_id)
		DO UPDATE SET tool_groups = $2, agent_api_key_encrypted = $3, updated_at = $4`

	_, err = scope.Conn.Exec(ctx, query, config.ProjectID, toolGroupsJSON, agentAPIKeyEncrypted, now)
	if err != nil {
		return fmt.Errorf("failed to upsert MCP config: %w", err)
	}

	config.UpdatedAt = now
	if config.CreatedAt.IsZero() {
		config.CreatedAt = now
	}

	return nil
}

// GetAgentAPIKey retrieves the encrypted agent API key for a project.
func (r *mcpConfigRepository) GetAgentAPIKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return "", fmt.Errorf("no tenant scope in context")
	}

	query := `SELECT agent_api_key_encrypted FROM engine_mcp_config WHERE project_id = $1`

	var encryptedKey *string
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&encryptedKey)
	if err != nil {
		if err == pgx.ErrNoRows {
			return "", nil // Not found, return empty string
		}
		return "", fmt.Errorf("failed to get agent API key: %w", err)
	}

	if encryptedKey == nil {
		return "", nil
	}
	return *encryptedKey, nil
}

// SetAgentAPIKey updates the encrypted agent API key for a project.
// If no MCP config exists (legacy projects provisioned before MCP config was created during provisioning),
// creates one with correct defaults matching models.DefaultMCPConfig().
func (r *mcpConfigRepository) SetAgentAPIKey(ctx context.Context, projectID uuid.UUID, encryptedKey string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	// Default tool_groups matches models.DefaultMCPConfig():
	// - user: allowOntologyMaintenance=true
	// - developer: addQueryTools=true, addOntologyMaintenance=true
	// - agent_tools: enabled=true
	defaultToolGroups := `{
		"user": {"allowOntologyMaintenance": true},
		"developer": {"addQueryTools": true, "addOntologyMaintenance": true},
		"agent_tools": {"enabled": true}
	}`
	query := `
		INSERT INTO engine_mcp_config (project_id, tool_groups, agent_api_key_encrypted, created_at, updated_at)
		VALUES ($1, $2::jsonb, $3, $4, $4)
		ON CONFLICT (project_id)
		DO UPDATE SET agent_api_key_encrypted = $3, updated_at = $4`

	_, err := scope.Conn.Exec(ctx, query, projectID, defaultToolGroups, encryptedKey, now)
	if err != nil {
		return fmt.Errorf("failed to set agent API key: %w", err)
	}

	return nil
}

// Ensure mcpConfigRepository implements MCPConfigRepository at compile time.
var _ MCPConfigRepository = (*mcpConfigRepository)(nil)
