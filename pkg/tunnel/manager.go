package tunnel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Manager manages tunnel clients across all projects that have the mcp-tunnel
// app activated. It starts tunnels on engine startup and responds to
// app lifecycle events (activate/uninstall).
type Manager struct {
	db           *database.DB
	localBaseURL string
	clientConfig ClientConfig
	logger       *zap.Logger

	mu            sync.RWMutex
	clients       map[uuid.UUID]*Client
	ctx           context.Context
	cancel        context.CancelFunc
	connectedHook func(context.Context, uuid.UUID)
}

// NewManager creates a new tunnel manager.
func NewManager(
	db *database.DB,
	localBaseURL string,
	clientConfig ClientConfig,
	logger *zap.Logger,
) *Manager {
	return &Manager{
		db:           db,
		localBaseURL: localBaseURL,
		clientConfig: clientConfig,
		logger:       logger.Named("tunnel-manager"),
		clients:      make(map[uuid.UUID]*Client),
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

	settings, err := m.getTunnelSettings(ctx, projectID)
	if err != nil {
		return fmt.Errorf("failed to load tunnel settings: %w", err)
	}
	if settings == nil {
		return fmt.Errorf("mcp-tunnel is not installed")
	}
	if settings.RegistrationURL == "" {
		return fmt.Errorf("mcp-tunnel missing registration_url")
	}

	client := NewClient(
		projectID,
		settings.RegistrationURL,
		m.localBaseURL,
		"",
		m.clientConfig,
		m.logger,
	)
	if m.connectedHook != nil {
		client.SetConnectedHook(m.connectedHook)
	}

	m.clients[projectID] = client

	// Start the client in a background goroutine
	startCtx := m.ctx
	if startCtx == nil {
		startCtx = ctx
	}
	go client.Start(startCtx)

	m.logger.Info("Tunnel started",
		zap.String("project_id", projectID.String()),
		zap.String("tunnel_url", settings.RegistrationURL),
	)

	return nil
}

// SetConnectedHook registers a callback that fires after a tunnel client
// completes its first successful relay registration for a connection.
func (m *Manager) SetConnectedHook(hook func(context.Context, uuid.UUID)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectedHook = hook
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
func (m *Manager) Status(ctx context.Context, projectID uuid.UUID) *StatusInfo {
	m.mu.RLock()
	client, ok := m.clients[projectID]
	m.mu.RUnlock()

	status := &StatusInfo{Status: StatusDisconnected}
	if ok {
		status = &StatusInfo{
			Status:         client.Status(),
			PublicURL:      client.PublicURL(),
			ConnectedSince: client.ConnectedSince(),
		}
	}

	settings, err := m.getTunnelSettings(ctx, projectID)
	if err != nil {
		m.logger.Warn("Failed to load tunnel status settings",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
		return status
	}

	if settings != nil && settings.Endpoint != "" {
		status.PublicURL = settings.Endpoint
	}

	return status
}

// StatusInfo contains the tunnel status details for a project.
type StatusInfo struct {
	Status         TunnelStatus `json:"tunnel_status"`
	PublicURL      string       `json:"public_url,omitempty"`
	ConnectedSince *time.Time   `json:"connected_since,omitempty"`
}

type tunnelSettings struct {
	RegistrationURL string
	Endpoint        string
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

func (m *Manager) getTunnelSettings(ctx context.Context, projectID uuid.UUID) (*tunnelSettings, error) {
	if m.db == nil {
		return nil, fmt.Errorf("database not available")
	}

	query := `
		SELECT COALESCE(settings->>$3, ''), COALESCE(settings->>$4, '')
		FROM engine_installed_apps
		WHERE project_id = $1 AND app_id = $2`

	settings := &tunnelSettings{}
	err := m.db.Pool.QueryRow(
		ctx,
		query,
		projectID,
		models.AppIDMCPTunnel,
		models.MCPTunnelSettingRegistrationURL,
		models.MCPTunnelSettingEndpoint,
	).Scan(&settings.RegistrationURL, &settings.Endpoint)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to query tunnel settings: %w", err)
	}

	return settings, nil
}
