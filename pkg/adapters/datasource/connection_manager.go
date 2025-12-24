package datasource

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/logging"
	"github.com/ekaya-inc/ekaya-engine/pkg/retry"
)

// PoolFactory is a function type for creating connection pools
type PoolFactory func(ctx context.Context, connString string, config ConnectionManagerConfig) (PoolConnector, error)

const (
	DefaultConnectionTTLMinutes  = 5
	DefaultCleanupInterval       = 1 * time.Minute
	DefaultMaxConnectionsPerUser = 10
	DefaultPoolMaxConns          = 10
	DefaultPoolMinConns          = 1
)

// ConnectionManagerConfig holds configuration for the connection manager
type ConnectionManagerConfig struct {
	TTLMinutes            int
	MaxConnectionsPerUser int
	PoolMaxConns          int32
	PoolMinConns          int32
}

// ConnectionManager manages connection pools for multi-tenant datasource access
// with TTL-based pooling and automatic cleanup.
type ConnectionManager struct {
	mu                    sync.RWMutex
	connections           map[string]*ManagedConnection // key: "{projectId}:{userId}:{datasourceId}"
	poolFactories         map[string]PoolFactory        // registered pool factories by datasource type
	ttl                   time.Duration
	maxConnectionsPerUser int
	poolMaxConns          int32
	poolMinConns          int32
	stopped               bool
	stopChan              chan struct{}
	logger                *zap.Logger
}

// ManagedConnection represents a pooled connection with access control
type ManagedConnection struct {
	connector PoolConnector
	lastUsed  time.Time
	mu        sync.Mutex // Per-connection mutex to prevent concurrent access issues
}

// NewConnectionManager creates a connection manager with the given configuration.
// Starts a background cleanup goroutine that runs until Close() is called.
func NewConnectionManager(cfg ConnectionManagerConfig, logger *zap.Logger) *ConnectionManager {
	if cfg.TTLMinutes <= 0 {
		cfg.TTLMinutes = DefaultConnectionTTLMinutes
	}
	if cfg.MaxConnectionsPerUser <= 0 {
		cfg.MaxConnectionsPerUser = DefaultMaxConnectionsPerUser
	}
	if cfg.PoolMaxConns <= 0 {
		cfg.PoolMaxConns = DefaultPoolMaxConns
	}
	if cfg.PoolMinConns <= 0 {
		cfg.PoolMinConns = DefaultPoolMinConns
	}

	manager := &ConnectionManager{
		connections:           make(map[string]*ManagedConnection),
		poolFactories:         make(map[string]PoolFactory),
		ttl:                   time.Duration(cfg.TTLMinutes) * time.Minute,
		maxConnectionsPerUser: cfg.MaxConnectionsPerUser,
		poolMaxConns:          cfg.PoolMaxConns,
		poolMinConns:          cfg.PoolMinConns,
		stopChan:              make(chan struct{}),
		logger:                logger,
	}

	// Register default factories
	manager.registerDefaultFactories()

	go manager.cleanupExpiredConnections()
	return manager
}

// RegisterPoolFactory registers a pool factory for a datasource type
func (m *ConnectionManager) RegisterPoolFactory(dsType string, factory PoolFactory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.poolFactories[dsType] = factory
}

// RegisterConnection registers an already-created connection connector.
// This is useful for adapters that need to create connections outside the factory pattern
// (e.g., MSSQL with user_delegation that requires context for tokens).
// Returns the connector if a connection with the same key already exists.
func (m *ConnectionManager) RegisterConnection(
	ctx context.Context,
	projectID uuid.UUID,
	userID string,
	datasourceID uuid.UUID,
	connector PoolConnector,
) (PoolConnector, error) {
	key := fmt.Sprintf("%s:%s:%s", projectID, userID, datasourceID)

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if connection already exists
	if managed, exists := m.connections[key]; exists && managed != nil {
		managed.mu.Lock()
		defer managed.mu.Unlock()

		// Health check existing connection
		healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := retry.Do(healthCtx, retry.DefaultConfig(), func() error {
			return managed.connector.Ping(healthCtx)
		})

		if err == nil {
			// Existing connection is healthy, return it
			managed.lastUsed = time.Now()
			return managed.connector, nil
		}

		// Existing connection is unhealthy, close it and replace
		if managed.connector != nil {
			managed.connector.Close()
		}
	}

	// Check per-user connection limit
	userConnCount := m.countConnectionsForUser(userID)
	if userConnCount >= m.maxConnectionsPerUser {
		if connector != nil {
			connector.Close()
		}
		m.logger.Warn("user reached max connections limit",
			zap.String("userID", userID),
			zap.Int("current", userConnCount),
			zap.Int("max", m.maxConnectionsPerUser),
		)
		return nil, fmt.Errorf("user %s has reached maximum connections limit (%d)", userID, m.maxConnectionsPerUser)
	}

	// Register the new connection
	m.connections[key] = &ManagedConnection{
		connector: connector,
		lastUsed:  time.Now(),
	}

	m.logger.Info("registered connection",
		zap.String("key", key),
		zap.String("projectID", projectID.String()),
		zap.String("userID", userID),
		zap.String("datasourceID", datasourceID.String()),
		zap.Int("userTotalConnections", userConnCount+1),
	)

	return connector, nil
}

// registerDefaultFactories registers built-in pool factories
func (m *ConnectionManager) registerDefaultFactories() {
	// Register PostgreSQL factory
	m.RegisterPoolFactory("postgres", CreatePostgresPool)
	// MSSQL factory registration is handled differently - adapters create connections
	// directly and use RegisterConnection due to auth complexity
}

// countConnectionsForUser counts active connections for a specific user.
// Caller must hold m.mu lock.
func (m *ConnectionManager) countConnectionsForUser(userID string) int {
	count := 0
	for key := range m.connections {
		// Key format: "{projectId}:{userId}:{datasourceId}"
		parts := strings.Split(key, ":")
		if len(parts) >= 2 && parts[1] == userID {
			count++
		}
	}
	return count
}

// GetOrCreateConnection gets or creates a connection pool for the given datasource type.
// Key format: "{projectID}:{userID}:{datasourceID}"
// Returns error if user has reached connection limit or pool creation fails.
func (m *ConnectionManager) GetOrCreateConnection(
	ctx context.Context,
	dsType string,
	projectID uuid.UUID,
	userID string,
	datasourceID uuid.UUID,
	connString string,
) (PoolConnector, error) {
	key := fmt.Sprintf("%s:%s:%s", projectID, userID, datasourceID)

	// Try existing connection with read lock (fast path)
	m.mu.RLock()
	managed, exists := m.connections[key]
	m.mu.RUnlock()

	if exists {
		managed.mu.Lock()

		// Health check with retry and timeout
		healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		err := retry.Do(healthCtx, retry.DefaultConfig(), func() error {
			return managed.connector.Ping(healthCtx)
		})

		if err != nil {
			// Unhealthy - log sanitized error, remove, and recreate
			m.logger.Warn("connection unhealthy, recreating",
				zap.String("key", key),
				zap.String("dsType", dsType),
				zap.String("error", logging.SanitizeError(err)),
			)
			managed.mu.Unlock() // Unlock before calling removeConnection
			m.removeConnection(key)
			return m.createNewConnection(ctx, dsType, key, projectID, userID, datasourceID, connString)
		}

		// Update last used time and return connector
		managed.lastUsed = time.Now()
		managed.mu.Unlock()
		return managed.connector, nil
	}

	// Create new connection
	return m.createNewConnection(ctx, dsType, key, projectID, userID, datasourceID, connString)
}

// GetOrCreatePool is a convenience method for PostgreSQL that maintains backwards compatibility.
// Deprecated: Use GetOrCreateConnection instead.
func (m *ConnectionManager) GetOrCreatePool(
	ctx context.Context,
	projectID uuid.UUID,
	userID string,
	datasourceID uuid.UUID,
	connString string,
) (PoolConnector, error) {
	connector, err := m.GetOrCreateConnection(ctx, "postgres", projectID, userID, datasourceID, connString)
	if err != nil {
		return nil, err
	}
	return connector, nil
}

// createNewConnection creates a new connection pool with retry logic.
// Caller must NOT hold any locks (this method acquires write lock).
func (m *ConnectionManager) createNewConnection(
	ctx context.Context,
	dsType string,
	key string,
	projectID uuid.UUID,
	userID string,
	datasourceID uuid.UUID,
	connString string,
) (PoolConnector, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have created it)
	if managed, exists := m.connections[key]; exists && managed != nil {
		managed.mu.Lock()
		defer managed.mu.Unlock()
		managed.lastUsed = time.Now()
		return managed.connector, nil
	}

	// Check per-user connection limit before creating new connection
	userConnCount := m.countConnectionsForUser(userID)
	if userConnCount >= m.maxConnectionsPerUser {
		m.logger.Warn("user reached max connections limit",
			zap.String("userID", userID),
			zap.Int("current", userConnCount),
			zap.Int("max", m.maxConnectionsPerUser),
		)
		return nil, fmt.Errorf("user %s has reached maximum connections limit (%d)", userID, m.maxConnectionsPerUser)
	}

	// Get factory for datasource type
	factory, exists := m.poolFactories[dsType]
	if !exists {
		return nil, fmt.Errorf("no pool factory registered for datasource type: %s", dsType)
	}

	// Build config for factory
	config := ConnectionManagerConfig{
		TTLMinutes:            int(m.ttl.Minutes()),
		MaxConnectionsPerUser: m.maxConnectionsPerUser,
		PoolMaxConns:          m.poolMaxConns,
		PoolMinConns:          m.poolMinConns,
	}

	// Create connection with retry logic for transient failures
	connector, err := retry.DoWithResult(ctx, retry.DefaultConfig(), func() (PoolConnector, error) {
		return factory(ctx, connString, config)
	})
	if err != nil {
		m.logger.Error("failed to create connection after retries",
			zap.String("key", key),
			zap.String("dsType", dsType),
			zap.String("error", logging.SanitizeError(err)),
		)
		return nil, fmt.Errorf("failed to create connection for %s (type: %s) after retries: %w", key, dsType, err)
	}

	// Store the managed connection
	m.connections[key] = &ManagedConnection{
		connector: connector,
		lastUsed:  time.Now(),
	}

	m.logger.Info("created new connection",
		zap.String("key", key),
		zap.String("dsType", dsType),
		zap.String("projectID", projectID.String()),
		zap.String("userID", userID),
		zap.String("datasourceID", datasourceID.String()),
		zap.Int("userTotalConnections", userConnCount+1),
	)

	return connector, nil
}

// removeConnection removes a connection from the pool and closes it.
// Caller must NOT hold m.mu lock (this method acquires write lock).
func (m *ConnectionManager) removeConnection(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if managed, exists := m.connections[key]; exists && managed != nil {
		if managed.connector != nil {
			managed.connector.Close()
		}
		delete(m.connections, key)
		m.logger.Debug("removed connection",
			zap.String("key", key),
		)
	}
}

// cleanupExpiredConnections runs periodically to remove expired connections.
// Runs in a background goroutine until stopChan is closed.
func (m *ConnectionManager) cleanupExpiredConnections() {
	ticker := time.NewTicker(DefaultCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.performCleanup()
		case <-m.stopChan:
			return
		}
	}
}

// performCleanup removes connections that haven't been used within TTL.
// Uses lock ordering: manager lock → connection lock to prevent deadlocks.
func (m *ConnectionManager) performCleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return
	}

	now := time.Now()
	expiredKeys := []string{}

	for key, managed := range m.connections {
		if managed != nil {
			managed.mu.Lock()
			expired := now.Sub(managed.lastUsed) > m.ttl
			idleTime := now.Sub(managed.lastUsed)
			managed.mu.Unlock()

			if expired {
				expiredKeys = append(expiredKeys, key)
				m.logger.Debug("marking connection for cleanup",
					zap.String("key", key),
					zap.Duration("idleTime", idleTime),
					zap.Duration("ttl", m.ttl),
				)
			}
		}
	}

	// Close and remove expired connections
	for _, key := range expiredKeys {
		if managed, exists := m.connections[key]; exists && managed != nil {
			if managed.connector != nil {
				managed.connector.Close()
			}
			delete(m.connections, key)
		}
	}

	if len(expiredKeys) > 0 {
		m.logger.Info("cleaned up expired connections",
			zap.Int("count", len(expiredKeys)),
			zap.Int("remaining", len(m.connections)),
		)
	}
}

// Close closes all connections in the manager and stops the cleanup goroutine.
// This method is idempotent and safe to call multiple times.
func (m *ConnectionManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return nil
	}

	m.stopped = true
	close(m.stopChan)

	// Close all managed connections
	for _, managed := range m.connections {
		if managed != nil && managed.connector != nil {
			managed.connector.Close()
		}
	}

	m.connections = make(map[string]*ManagedConnection)
	m.logger.Info("connection manager closed")
	return nil
}

// GetStats returns statistics about the connection manager.
// Safe to call concurrently.
func (m *ConnectionManager) GetStats() ConnectionStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	stats := ConnectionStats{
		TotalConnections:      len(m.connections),
		MaxConnectionsPerUser: m.maxConnectionsPerUser,
		TTLMinutes:            int(m.ttl.Minutes()),
		ConnectionsByProject:  make(map[string]int),
		ConnectionsByUser:     make(map[string]int),
		OldestIdleSeconds:     0,
	}

	for key, managed := range m.connections {
		// Parse from key "{projectId}:{userId}:{datasourceId}"
		parts := strings.Split(key, ":")
		if len(parts) >= 1 {
			stats.ConnectionsByProject[parts[0]]++
		}
		if len(parts) >= 2 {
			stats.ConnectionsByUser[parts[1]]++
		}

		// Track oldest idle connection
		if managed != nil {
			managed.mu.Lock()
			idleSeconds := int(now.Sub(managed.lastUsed).Seconds())
			managed.mu.Unlock()
			if idleSeconds > stats.OldestIdleSeconds {
				stats.OldestIdleSeconds = idleSeconds
			}
		}
	}

	return stats
}

// ConnectionStats contains statistics about the connection manager state.
type ConnectionStats struct {
	TotalConnections      int            `json:"total_connections"`
	MaxConnectionsPerUser int            `json:"max_connections_per_user"`
	TTLMinutes            int            `json:"ttl_minutes"`
	ConnectionsByProject  map[string]int `json:"connections_by_project"`
	ConnectionsByUser     map[string]int `json:"connections_by_user"`
	OldestIdleSeconds     int            `json:"oldest_idle_seconds"`
}
