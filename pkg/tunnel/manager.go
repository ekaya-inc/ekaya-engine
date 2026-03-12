package tunnel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// Manager manages tunnel clients across all projects that have the mcp-tunnel
// app activated. It starts tunnels on engine startup and responds to
// app lifecycle events (activate/uninstall).
type Manager struct {
	db              *database.DB
	agentKeyService services.AgentAPIKeyService
	tunnelURL       string
	localBaseURL    string
	clientConfig    ClientConfig
	logger          *zap.Logger

	mu      sync.RWMutex
	clients map[uuid.UUID]*Client
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewManager creates a new tunnel manager.
func NewManager(
	db *database.DB,
	agentKeyService services.AgentAPIKeyService,
	tunnelURL string,
	localBaseURL string,
	clientConfig ClientConfig,
	logger *zap.Logger,
) *Manager {
	return &Manager{
		db:              db,
		agentKeyService: agentKeyService,
		tunnelURL:       tunnelURL,
		localBaseURL:    localBaseURL,
		clientConfig:    clientConfig,
		logger:          logger.Named("tunnel-manager"),
		clients:         make(map[uuid.UUID]*Client),
	}
}

// Start queries for all projects with mcp-tunnel activated and starts a tunnel
// client for each. Non-blocking — tunnel clients run in background goroutines.
func (m *Manager) Start(ctx context.Context) error {
	m.ctx, m.cancel = context.WithCancel(ctx)

	projectIDs, err := m.listActivatedProjects(m.ctx)
	if err != nil {
		m.logger.Error("Failed to list activated tunnel projects", zap.Error(err))
		// Don't fail startup — tunnels will be started via lifecycle hooks
		return nil
	}

	if len(projectIDs) == 0 {
		m.logger.Info("No projects with mcp-tunnel activated")
		return nil
	}

	m.logger.Info("Starting tunnels for activated projects",
		zap.Int("count", len(projectIDs)),
	)

	for _, pid := range projectIDs {
		if err := m.StartTunnel(m.ctx, pid); err != nil {
			m.logger.Error("Failed to start tunnel for project",
				zap.String("project_id", pid.String()),
				zap.Error(err),
			)
		}
	}

	return nil
}

// StartTunnel starts a tunnel client for a specific project. If a tunnel is
// already running for this project, it is stopped and replaced.
func (m *Manager) StartTunnel(ctx context.Context, projectID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop existing client if present
	if existing, ok := m.clients[projectID]; ok {
		m.logger.Info("Replacing existing tunnel",
			zap.String("project_id", projectID.String()),
		)
		existing.Stop()
		delete(m.clients, projectID)
	}

	// Ensure an agent API key exists for authenticating local MCP requests
	apiKey, err := m.ensureAgentAPIKey(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to ensure agent API key: %w", err)
	}

	client := NewClient(
		projectID,
		m.tunnelURL,
		m.localBaseURL,
		apiKey,
		m.clientConfig,
		m.logger,
	)

	m.clients[projectID] = client

	// Start the client in a background goroutine
	go client.Start(m.ctx)

	m.logger.Info("Tunnel started",
		zap.String("project_id", projectID.String()),
		zap.String("tunnel_url", m.tunnelURL),
	)

	return nil
}

// StopTunnel stops the tunnel client for a specific project.
func (m *Manager) StopTunnel(projectID uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	client, ok := m.clients[projectID]
	if !ok {
		return
	}

	client.Stop()
	delete(m.clients, projectID)

	m.logger.Info("Tunnel stopped",
		zap.String("project_id", projectID.String()),
	)
}

// Status returns the tunnel status for a project.
func (m *Manager) Status(projectID uuid.UUID) *StatusInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[projectID]
	if !ok {
		return &StatusInfo{Status: StatusDisconnected}
	}

	return &StatusInfo{
		Status:         client.Status(),
		PublicURL:      client.PublicURL(),
		ConnectedSince: client.ConnectedSince(),
	}
}

// StatusInfo contains the tunnel status details for a project.
type StatusInfo struct {
	Status         TunnelStatus `json:"tunnel_status"`
	PublicURL      string       `json:"public_url,omitempty"`
	ConnectedSince *time.Time   `json:"connected_since,omitempty"`
}

// Shutdown gracefully stops all tunnel clients.
func (m *Manager) Shutdown() {
	m.logger.Info("Shutting down tunnel manager")

	if m.cancel != nil {
		m.cancel()
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for pid, client := range m.clients {
		client.Stop()
		m.logger.Info("Tunnel stopped during shutdown",
			zap.String("project_id", pid.String()),
		)
	}
	m.clients = make(map[uuid.UUID]*Client)
}

// listActivatedProjects queries the database for all projects that have the
// mcp-tunnel app activated. Uses a direct pool query (no tenant scope) because
// this is a cross-tenant operation at startup.
func (m *Manager) listActivatedProjects(ctx context.Context) ([]uuid.UUID, error) {
	query := `
		SELECT project_id
		FROM engine_installed_apps
		WHERE app_id = $1 AND activated_at IS NOT NULL`

	rows, err := m.db.Pool.Query(ctx, query, models.AppIDMCPTunnel)
	if err != nil {
		return nil, fmt.Errorf("failed to query activated tunnel projects: %w", err)
	}
	defer rows.Close()

	var projectIDs []uuid.UUID
	for rows.Next() {
		var pid uuid.UUID
		if err := rows.Scan(&pid); err != nil {
			return nil, fmt.Errorf("failed to scan project ID: %w", err)
		}
		projectIDs = append(projectIDs, pid)
	}

	return projectIDs, rows.Err()
}

// ensureAgentAPIKey gets or creates an agent API key for the project.
// The tunnel client uses this key to authenticate local MCP requests.
func (m *Manager) ensureAgentAPIKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	if m.db == nil {
		return "", fmt.Errorf("database not available")
	}

	// Need a tenant-scoped context to access the agent API key service
	scope, err := m.db.WithTenant(ctx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get tenant scope: %w", err)
	}
	defer scope.Close()

	tenantCtx := database.SetTenantScope(ctx, scope)

	key, err := m.agentKeyService.GetKey(tenantCtx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to get agent API key: %w", err)
	}

	if key != "" {
		return key, nil
	}

	// No key exists — generate one
	key, err = m.agentKeyService.GenerateKey(tenantCtx, projectID)
	if err != nil {
		return "", fmt.Errorf("failed to generate agent API key: %w", err)
	}

	m.logger.Info("Generated agent API key for tunnel",
		zap.String("project_id", projectID.String()),
	)

	return key, nil
}
