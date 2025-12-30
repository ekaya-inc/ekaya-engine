# PLAN: Multi-Tenant Connection Manager for ekaya-engine

## Justification

**Problem:** Every database operation in ekaya-engine creates a NEW connection pool, uses it once, then immediately closes it. This pattern appears in all three service layers (schema, query, datasource) and causes:

1. **Massive resource waste** - Opening/closing pgxpool instances is expensive (TCP handshakes, auth, connection setup)
2. **Performance degradation** - Each query pays the connection setup cost instead of reusing pooled connections
3. **Scalability issues** - Under load, we create hundreds of pools simultaneously for the same datasource
4. **Connection exhaustion** - Customer databases can hit max_connections limits even with low query volume
5. **No resilience to transient failures** - A single network blip fails the entire operation with no retry

**Current problematic pattern (from `pkg/services/schema.go:94-98`):**
```go
// Create schema discoverer - THIS CREATES A NEW POOL
discoverer, err := s.adapterFactory.NewSchemaDiscoverer(ctx, ds.DatasourceType, ds.Config)
if err != nil {
    return nil, fmt.Errorf("failed to create schema discoverer: %w", err)
}
defer discoverer.Close()  // CLOSES POOL AFTER SINGLE USE
```

**Same pattern in query execution (`pkg/services/query.go:~180`):**
```go
executor, err := s.adapterFactory.NewQueryExecutor(ctx, ds.DatasourceType, ds.Config)
defer executor.Close()  // Another pool created and destroyed
```

**Same pattern in connection testing (`pkg/services/datasource.go:228-232`):**
```go
adapter, err := s.adapterFactory.NewConnectionTester(ctx, dsType, config)
defer adapter.Close()  // Yet another pool lifecycle
```

**Why this happened:** The adapter factory pattern was designed for simplicity without considering connection reuse. Each factory method creates a fresh `pgxpool.Pool` instance tied to the adapter's lifecycle.

**Solution:** Implement a connection manager that pools datasource connections by `(projectID, userID, datasourceID)` tuple, reusing them across requests with TTL-based cleanup. Add a retry package with exponential backoff for resilience to transient failures. This mirrors the proven architecture in ekaya-region's SDAP layer (`pkg/sdap/db/connection_manager.go` and `pkg/sdap/internal/retry/retry.go`).

## Current State Analysis

### Internal Database (ekaya_region)
- **Location:** Single shared PostgreSQL database for metadata
- **Access pattern:** `enginedb.DB.WithTenant(ctx, projectID)` - returns TenantScope
- **Connection management:** Single global `pgxpool.Pool` with RLS for multi-tenancy
- **Status:** ✅ Works well, no changes needed

### Customer Datasources (The Problem)
- **Location:** External PostgreSQL databases (customer data)
- **Access pattern:** Operations via adapter factory → new pool every time
- **Connection management:** ❌ None - create/destroy pools on every operation
- **Key insight:** `pkg/adapters/datasource/factory.go` creates fresh adapters with fresh pools

**File evidence:**
- `pkg/adapters/datasource/postgres/adapter.go:18-29` - Creates new `pgxpool.Pool` on every `NewAdapter()` call
- `pkg/services/schema.go:94` - Calls factory for every schema operation
- `pkg/services/query.go:~180` - Calls factory for every query execution
- `pkg/services/datasource.go:228` - Calls factory for connection tests

### Adapter Factory Flow (Current)
```
Service calls factory.NewSchemaDiscoverer(ctx, dsType, config)
  ↓
Factory calls registry.GetSchemaDiscovererFactory(dsType)
  ↓
Returns postgres.NewSchemaDiscoverer(ctx, config)
  ↓
Creates postgres.Adapter with NEW pgxpool.Pool
  ↓
Returns adapter (implements SchemaDiscoverer)
  ↓
Service uses adapter, then calls Close()
  ↓
Pool.Close() destroys all connections
```

## Architecture Design

### Retry Package

**New file:** `pkg/retry/retry.go`

The retry package wraps operations that can fail transiently (network blips, database timeouts, temporary unavailability) and automatically retries them with exponential backoff.

**Why this matters for Connection Manager:**
- `pgxpool.New()` can fail due to transient network issues
- `pool.Ping()` health checks can timeout during brief network partitions
- Customer databases may be temporarily unreachable (Cloud SQL proxy restarts, etc.)
- Without retry, a single transient failure causes the entire operation to fail

**Transient errors it handles:**
- TCP connection refused (database restarting)
- Connection timeout (network congestion)
- "too many connections" (momentary spike)
- Cloud SQL proxy hiccups
- DNS resolution failures

```go
package retry

import (
    "context"
    "time"
)

// Config defines retry behavior
type Config struct {
    MaxRetries   int
    InitialDelay time.Duration
    MaxDelay     time.Duration
    Multiplier   float64
}

// DefaultConfig returns sensible defaults for database operations
func DefaultConfig() *Config {
    return &Config{
        MaxRetries:   3,
        InitialDelay: 100 * time.Millisecond,
        MaxDelay:     5 * time.Second,
        Multiplier:   2.0,
    }
}

// Do executes fn with exponential backoff retry logic
// Returns nil on success, or last error after all retries exhausted
func Do(ctx context.Context, cfg *Config, fn func() error) error {
    if cfg == nil {
        cfg = DefaultConfig()
    }

    var lastErr error
    delay := cfg.InitialDelay

    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        if err := fn(); err == nil {
            return nil
        } else {
            lastErr = err

            if attempt < cfg.MaxRetries {
                select {
                case <-time.After(delay):
                    delay = time.Duration(float64(delay) * cfg.Multiplier)
                    if delay > cfg.MaxDelay {
                        delay = cfg.MaxDelay
                    }
                case <-ctx.Done():
                    return ctx.Err()
                }
            }
        }
    }

    return lastErr
}

// DoWithResult executes fn and returns both result and error
// Useful for functions that return values (like pgxpool.New)
func DoWithResult[T any](ctx context.Context, cfg *Config, fn func() (T, error)) (T, error) {
    if cfg == nil {
        cfg = DefaultConfig()
    }

    var result T
    var lastErr error
    delay := cfg.InitialDelay

    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        r, err := fn()
        if err == nil {
            return r, nil
        }

        lastErr = err
        result = r // Keep last result even on error

        if attempt < cfg.MaxRetries {
            select {
            case <-time.After(delay):
                delay = time.Duration(float64(delay) * cfg.Multiplier)
                if delay > cfg.MaxDelay {
                    delay = cfg.MaxDelay
                }
            case <-ctx.Done():
                return result, ctx.Err()
            }
        }
    }

    return result, lastErr
}

// IsRetryable determines if an error is transient and worth retrying
// This prevents wasting retries on permanent failures (auth errors, bad SQL, etc.)
func IsRetryable(err error) bool {
    if err == nil {
        return false
    }

    errStr := strings.ToLower(err.Error())
    retryablePatterns := []string{
        "connection refused",
        "connection reset",
        "broken pipe",
        "no such host",
        "timeout",
        "temporary failure",
        "too many connections",
        "deadlock",
        "i/o timeout",
        "network is unreachable",
        "connection timed out",
    }

    for _, pattern := range retryablePatterns {
        if strings.Contains(errStr, pattern) {
            return true
        }
    }

    return false
}

// DoIfRetryable only retries if the error is transient
func DoIfRetryable(ctx context.Context, cfg *Config, fn func() error) error {
    if cfg == nil {
        cfg = DefaultConfig()
    }

    var lastErr error
    delay := cfg.InitialDelay

    for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
        if err := fn(); err == nil {
            return nil
        } else {
            lastErr = err

            // Don't retry non-transient errors
            if !IsRetryable(err) {
                return err
            }

            if attempt < cfg.MaxRetries {
                select {
                case <-time.After(delay):
                    delay = time.Duration(float64(delay) * cfg.Multiplier)
                    if delay > cfg.MaxDelay {
                        delay = cfg.MaxDelay
                    }
                case <-ctx.Done():
                    return ctx.Err()
                }
            }
        }
    }

    return lastErr
}
```

**Usage in Connection Manager:**

```go
// Creating pools with retry
func (m *ConnectionManager) createNewPool(ctx context.Context, key, connString string) (*pgxpool.Pool, error) {
    pool, err := retry.DoWithResult(ctx, retry.DefaultConfig(), func() (*pgxpool.Pool, error) {
        return pgxpool.New(ctx, connString)
    })
    if err != nil {
        return nil, fmt.Errorf("failed to create pool for %s after retries: %w", key, err)
    }
    return pool, nil
}

// Health checks with retry
func (m *ConnectionManager) healthCheck(ctx context.Context, pool *pgxpool.Pool) error {
    return retry.Do(ctx, retry.DefaultConfig(), func() error {
        healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
        defer cancel()
        return pool.Ping(healthCtx)
    })
}
```

### Log Sanitizer

**New file:** `pkg/logging/sanitizer.go`

Prevents accidental credential leakage in logs. This is a **security-critical** component that should be used whenever logging connection strings, errors, or queries.

**Why this matters:**
- Connection strings contain passwords
- Error messages may include connection details
- Query logs might contain sensitive data
- JWT tokens in error messages expose credentials

```go
package logging

import (
    "regexp"
    "strings"
)

const (
    // MaxQueryLogLength is the maximum length of a query to log
    MaxQueryLogLength = 100
    // RedactedText is the replacement text for sensitive data
    RedactedText = "[REDACTED]"
)

var (
    // Pattern to match potential passwords in connection strings
    // Matches: password=xxx, pwd=xxx, pass=xxx (until next delimiter)
    passwordPattern = regexp.MustCompile(`(?i)(password|pwd|pass)=[^;&\s]+`)

    // Pattern to match JWT tokens (three base64 segments separated by dots)
    jwtPattern = regexp.MustCompile(`Bearer\s+[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]*`)

    // Pattern to match potential API keys
    apiKeyPattern = regexp.MustCompile(`(?i)(api[_-]?key|apikey|key)=[A-Za-z0-9-_]{20,}`)

    // Pattern to match connection string credentials (user:pass@host format)
    connStringPattern = regexp.MustCompile(`://[^:]+:[^@]+@[^/\s]+`)
)

// SanitizeConnectionString removes sensitive data from connection strings
// Use this before logging any connection string
func SanitizeConnectionString(connStr string) string {
    if connStr == "" {
        return ""
    }

    // Replace password values
    sanitized := passwordPattern.ReplaceAllString(connStr, "${1}="+RedactedText)

    // Replace user:pass@host format
    sanitized = connStringPattern.ReplaceAllString(sanitized, "://"+RedactedText+"@"+RedactedText)

    return sanitized
}

// SanitizeError sanitizes error messages that might contain sensitive data
// Use this before logging any error from database operations
func SanitizeError(err error) string {
    if err == nil {
        return ""
    }

    errStr := err.Error()

    // Remove potential passwords
    sanitized := passwordPattern.ReplaceAllString(errStr, "${1}="+RedactedText)

    // Remove JWT tokens
    sanitized = jwtPattern.ReplaceAllString(sanitized, "Bearer "+RedactedText)

    // Remove API keys
    sanitized = apiKeyPattern.ReplaceAllString(sanitized, "${1}="+RedactedText)

    // Remove connection string details
    sanitized = connStringPattern.ReplaceAllString(sanitized, "://"+RedactedText+"@"+RedactedText)

    return sanitized
}

// SanitizeQuery truncates and sanitizes a SQL query for logging
// Prevents logging very long queries and removes sensitive patterns
func SanitizeQuery(query string) string {
    if query == "" {
        return ""
    }

    // Truncate if too long
    sanitized := query
    if len(sanitized) > MaxQueryLogLength {
        sanitized = sanitized[:MaxQueryLogLength] + "..."
    }

    // Remove potential sensitive data patterns
    sanitized = passwordPattern.ReplaceAllString(sanitized, "${1}="+RedactedText)
    sanitized = apiKeyPattern.ReplaceAllString(sanitized, "${1}="+RedactedText)

    return sanitized
}

// TruncateString truncates a string to maxLen and adds ellipsis if needed
func TruncateString(s string, maxLen int) string {
    if len(s) <= maxLen {
        return s
    }
    return s[:maxLen] + "..."
}
```

**Usage in Connection Manager:**

```go
// When logging errors from pool creation
pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
if err != nil {
    m.logger.Error("failed to create pool",
        zap.String("key", key),
        zap.String("error", logging.SanitizeError(err)),  // Sanitized!
    )
    return nil, fmt.Errorf("failed to create pool: %w", err)
}

// When logging connection issues
m.logger.Warn("connection unhealthy",
    zap.String("key", key),
    zap.String("connString", logging.SanitizeConnectionString(connString)),  // Sanitized!
)
```

### Connection Manager Interface

**New file:** `pkg/adapters/datasource/connection_manager.go`

```go
package datasource

import (
    "context"
    "fmt"
    "sync"
    "time"
    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"
    "go.uber.org/zap"
)

const (
    DefaultConnectionTTLMinutes    = 5
    DefaultCleanupInterval         = 1 * time.Minute
    DefaultMaxConnectionsPerUser   = 10
    DefaultPoolMaxConns            = 10
    DefaultPoolMinConns            = 1
)

// ConnectionManagerConfig holds configuration for the connection manager
type ConnectionManagerConfig struct {
    TTLMinutes           int
    MaxConnectionsPerUser int
    PoolMaxConns         int32
    PoolMinConns         int32
}

// ConnectionManager manages connection pools for multi-tenant datasource access
type ConnectionManager struct {
    mu                    sync.RWMutex
    connections           map[string]*ManagedConnection  // key: "{projectId}:{userId}:{datasourceId}"
    ttl                   time.Duration
    maxConnectionsPerUser int
    poolMaxConns          int32
    poolMinConns          int32
    stopped               bool
    stopChan              chan struct{}
    logger                *zap.Logger
}

// ManagedConnection represents a pooled connection
type ManagedConnection struct {
    pool      *pgxpool.Pool
    lastUsed  time.Time
    mu        sync.Mutex  // Per-connection mutex
}

// NewConnectionManager creates a connection manager
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
        ttl:                   time.Duration(cfg.TTLMinutes) * time.Minute,
        maxConnectionsPerUser: cfg.MaxConnectionsPerUser,
        poolMaxConns:          cfg.PoolMaxConns,
        poolMinConns:          cfg.PoolMinConns,
        stopChan:              make(chan struct{}),
        logger:                logger,
    }

    go manager.cleanupExpiredConnections()
    return manager
}

// countConnectionsForUser counts active connections for a specific user
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

// GetOrCreatePool gets or creates a connection pool for the given datasource
// Key: "{projectID}:{userID}:{datasourceID}"
func (m *ConnectionManager) GetOrCreatePool(
    ctx context.Context,
    projectID uuid.UUID,
    userID string,
    datasourceID uuid.UUID,
    connString string,
) (*pgxpool.Pool, error) {
    key := fmt.Sprintf("%s:%s:%s", projectID, userID, datasourceID)

    // Try existing connection with read lock
    m.mu.RLock()
    managed, exists := m.connections[key]
    m.mu.RUnlock()

    if exists {
        managed.mu.Lock()
        defer managed.mu.Unlock()

        // Health check with 5s timeout
        healthCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
        defer cancel()

        if err := managed.pool.Ping(healthCtx); err != nil {
            // Unhealthy - remove and recreate
            managed.mu.Unlock()
            m.removeConnection(key)
            return m.createNewPool(ctx, key, projectID, userID, datasourceID, connString)
        }

        managed.lastUsed = time.Now()
        return managed.pool, nil
    }

    // Create new pool
    return m.createNewPool(ctx, key, projectID, userID, datasourceID, connString)
}

// createNewPool creates a new connection pool (caller must not hold locks)
func (m *ConnectionManager) createNewPool(
    ctx context.Context,
    key string,
    projectID uuid.UUID,
    userID string,
    datasourceID uuid.UUID,
    connString string,
) (*pgxpool.Pool, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    // Double-check after acquiring write lock
    if managed, exists := m.connections[key]; exists && managed != nil {
        managed.mu.Lock()
        defer managed.mu.Unlock()
        managed.lastUsed = time.Now()
        return managed.pool, nil
    }

    // Check per-user connection limit before creating new pool
    userConnCount := m.countConnectionsForUser(userID)
    if userConnCount >= m.maxConnectionsPerUser {
        m.logger.Warn("user reached max connections limit",
            zap.String("userID", userID),
            zap.Int("current", userConnCount),
            zap.Int("max", m.maxConnectionsPerUser),
        )
        return nil, fmt.Errorf("user %s has reached maximum connections limit (%d)", userID, m.maxConnectionsPerUser)
    }

    // Parse and configure the pool with proper settings
    poolConfig, err := pgxpool.ParseConfig(connString)
    if err != nil {
        return nil, fmt.Errorf("failed to parse connection string: %w", err)
    }
    poolConfig.MaxConns = m.poolMaxConns
    poolConfig.MinConns = m.poolMinConns
    poolConfig.MaxConnIdleTime = m.ttl

    pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
    if err != nil {
        return nil, fmt.Errorf("failed to create pool for %s: %w", key, err)
    }

    m.connections[key] = &ManagedConnection{
        pool:     pool,
        lastUsed: time.Now(),
    }

    m.logger.Info("created new connection pool",
        zap.String("key", key),
        zap.String("projectID", projectID.String()),
        zap.String("userID", userID),
        zap.Int("userTotalConnections", userConnCount+1),
    )

    return pool, nil
}

// removeConnection removes a connection from the pool
func (m *ConnectionManager) removeConnection(key string) {
    m.mu.Lock()
    defer m.mu.Unlock()

    if managed, exists := m.connections[key]; exists && managed != nil {
        if managed.pool != nil {
            managed.pool.Close()
        }
        delete(m.connections, key)
    }
}

// cleanupExpiredConnections runs periodically to remove expired connections
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

// performCleanup removes connections that haven't been used within TTL
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

    for _, key := range expiredKeys {
        if managed, exists := m.connections[key]; exists && managed != nil {
            if managed.pool != nil {
                managed.pool.Close()
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

// Close closes all connections in the manager
func (m *ConnectionManager) Close() error {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.stopped = true
    close(m.stopChan)

    for _, managed := range m.connections {
        if managed != nil && managed.pool != nil {
            managed.pool.Close()
        }
    }

    m.connections = make(map[string]*ManagedConnection)
    return nil
}

// GetStats returns statistics about the connection manager
func (m *ConnectionManager) GetStats() ConnectionStats {
    m.mu.RLock()
    defer m.mu.RUnlock()

    now := time.Now()
    stats := ConnectionStats{
        TotalConnections:     len(m.connections),
        MaxConnectionsPerUser: m.maxConnectionsPerUser,
        TTLMinutes:           int(m.ttl.Minutes()),
        ConnectionsByProject: make(map[string]int),
        ConnectionsByUser:    make(map[string]int),
        OldestIdleSeconds:    0,
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

type ConnectionStats struct {
    TotalConnections      int            `json:"total_connections"`
    MaxConnectionsPerUser int            `json:"max_connections_per_user"`
    TTLMinutes            int            `json:"ttl_minutes"`
    ConnectionsByProject  map[string]int `json:"connections_by_project"`
    ConnectionsByUser     map[string]int `json:"connections_by_user"`
    OldestIdleSeconds     int            `json:"oldest_idle_seconds"`
}
```

### Factory Integration

**Modify:** `pkg/adapters/datasource/factory.go`

Add connection manager to factory:

```go
type registryFactory struct {
    connMgr *ConnectionManager  // NEW
}

func NewDatasourceAdapterFactory(connMgr *ConnectionManager) DatasourceAdapterFactory {
    return &registryFactory{
        connMgr: connMgr,
    }
}

// Pass connMgr down to adapter creation
func (f *registryFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any) (SchemaDiscoverer, error) {
    factory := GetSchemaDiscovererFactory(dsType)
    if factory == nil {
        return nil, fmt.Errorf("schema discovery not supported for type: %s", dsType)
    }
    return factory(ctx, config, f.connMgr)  // NEW PARAMETER
}
```

**Key insight:** Factory must pass connection manager to adapter constructors so they can request pooled connections instead of creating new pools.

### Adapter Integration

**Modify:** `pkg/adapters/datasource/postgres/adapter.go`

```go
import (
    "net/url"
)

type Adapter struct {
    pool       *pgxpool.Pool
    connMgr    *ConnectionManager
    projectID  uuid.UUID
    userID     string
    datasourceID uuid.UUID
    ownedPool  bool  // true if we created the pool (for TestConnection case)
}

// buildConnectionString builds a PostgreSQL URL with proper escaping
// IMPORTANT: All user-provided fields must be URL-escaped to handle special characters
// in passwords (e.g., @, /, #, ?) that would otherwise break URL parsing
func buildConnectionString(cfg *Config) string {
    sslMode := cfg.SSLMode
    if sslMode == "" {
        sslMode = "require"
    }
    return fmt.Sprintf(
        "postgresql://%s:%s@%s:%d/%s?sslmode=%s",
        url.QueryEscape(cfg.User),
        url.QueryEscape(cfg.Password),
        cfg.Host,
        cfg.Port,
        url.QueryEscape(cfg.Database),
        sslMode,
    )
}

// NewAdapter creates a PostgreSQL adapter using the connection manager
func NewAdapter(ctx context.Context, cfg *Config, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (*Adapter, error) {
    connStr := buildConnectionString(cfg)

    if connMgr == nil {
        // Fallback for direct instantiation (tests, TestConnection)
        pool, err := pgxpool.New(ctx, connStr)
        if err != nil {
            return nil, fmt.Errorf("connect to postgres: %w", err)
        }

        return &Adapter{
            pool:      pool,
            ownedPool: true,
        }, nil
    }

    // Use connection manager for reusable pool
    pool, err := connMgr.GetOrCreatePool(ctx, projectID, userID, datasourceID, connStr)
    if err != nil {
        return nil, fmt.Errorf("failed to get pooled connection: %w", err)
    }

    return &Adapter{
        pool:         pool,
        connMgr:      connMgr,
        projectID:    projectID,
        userID:       userID,
        datasourceID: datasourceID,
        ownedPool:    false,
    }, nil
}

// Close releases the adapter (but NOT the pool if managed)
func (a *Adapter) Close() error {
    if a.ownedPool && a.pool != nil {
        a.pool.Close()
    }
    // If using connection manager, don't close the pool - it's managed by TTL
    return nil
}
```

**Critical insight:** Adapter no longer owns the pool lifecycle when using connection manager. The `Close()` method becomes a no-op for managed connections.

### Service Layer Changes

Services need to pass `(projectID, userID, datasourceID)` to factory methods.

**Modify:** `pkg/services/schema.go`

```go
type schemaService struct {
    schemaRepo     repositories.SchemaRepository
    datasourceSvc  DatasourceService
    adapterFactory datasource.DatasourceAdapterFactory
    logger         *zap.Logger
}

// RefreshDatasourceSchema - extract context values
func (s *schemaService) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RefreshResult, error) {
    // Extract userID from context (JWT claims)
    userID := auth.GetUserIDFromContext(ctx)  // NEW
    if userID == "" {
        return nil, fmt.Errorf("user ID not found in context")
    }

    ds, err := s.datasourceSvc.Get(ctx, projectID, datasourceID)
    if err != nil {
        return nil, fmt.Errorf("failed to get datasource: %w", err)
    }

    // Pass projectID, userID, datasourceID to factory
    discoverer, err := s.adapterFactory.NewSchemaDiscoverer(
        ctx,
        ds.DatasourceType,
        ds.Config,
        projectID,      // NEW
        userID,         // NEW
        datasourceID,   // NEW
    )
    if err != nil {
        return nil, fmt.Errorf("failed to create schema discoverer: %w", err)
    }
    defer discoverer.Close()  // Now a no-op for managed connections

    // ... rest of implementation unchanged
}
```

**Same pattern for:**
- `pkg/services/query.go` - All query execution methods
- `pkg/services/datasource.go:TestConnection` - Connection testing (uses temporary pool, not managed)

### Configuration

**Add to:** `config/config.go`

```go
// DatasourceConfig contains settings for datasource connection management
type DatasourceConfig struct {
    ConnectionTTLMinutes  int   `yaml:"connection_ttl_minutes" env:"DATASOURCE_CONNECTION_TTL_MINUTES" env-default:"5"`
    MaxConnectionsPerUser int   `yaml:"max_connections_per_user" env:"DATASOURCE_MAX_CONNECTIONS_PER_USER" env-default:"10"`
    PoolMaxConns          int32 `yaml:"pool_max_conns" env:"DATASOURCE_POOL_MAX_CONNS" env-default:"10"`
    PoolMinConns          int32 `yaml:"pool_min_conns" env:"DATASOURCE_POOL_MIN_CONNS" env-default:"1"`
}
```

**Add to Config struct:**
```go
type Config struct {
    // ... existing fields ...
    Datasource DatasourceConfig `yaml:"datasource"`
}
```

**Example config.yaml:**
```yaml
datasource:
  connection_ttl_minutes: 5      # How long idle connections are kept
  max_connections_per_user: 10   # Limit per user to prevent exhaustion
  pool_max_conns: 10             # Max connections per datasource pool
  pool_min_conns: 1              # Min connections per datasource pool
```

### Main Wiring

**Modify:** `main.go:142-150`

```go
// Create connection manager with config-driven settings
connManagerCfg := datasource.ConnectionManagerConfig{
    TTLMinutes:           cfg.Datasource.ConnectionTTLMinutes,
    MaxConnectionsPerUser: cfg.Datasource.MaxConnectionsPerUser,
    PoolMaxConns:         cfg.Datasource.PoolMaxConns,
    PoolMinConns:         cfg.Datasource.PoolMinConns,
}
connManager := datasource.NewConnectionManager(connManagerCfg, logger)
defer connManager.Close()

// Create adapter factory with connection manager
adapterFactory := datasource.NewDatasourceAdapterFactory(connManager)

// Services unchanged (factory now handles connection reuse)
projectService := services.NewProjectService(db, projectRepo, userRepo, redisClient, cfg.BaseURL, logger)
userService := services.NewUserService(userRepo, logger)
datasourceService := services.NewDatasourceService(datasourceRepo, credentialEncryptor, adapterFactory, projectService, logger)
schemaService := services.NewSchemaService(schemaRepo, datasourceService, adapterFactory, logger)
discoveryService := services.NewRelationshipDiscoveryService(schemaRepo, datasourceService, adapterFactory, logger)
queryService := services.NewQueryService(queryRepo, datasourceService, adapterFactory, logger)
```

## Implementation Steps

### Step 1: Create Retry Package
- [x] Create `pkg/retry/retry.go`
- [x] Implement `Config` struct with `MaxRetries`, `InitialDelay`, `MaxDelay`, `Multiplier`
- [x] Implement `DefaultConfig()` returning sensible defaults (3 retries, 100ms initial, 5s max, 2x multiplier)
- [x] Implement `Do(ctx, cfg, fn)` for operations returning only error
- [x] Implement `DoWithResult[T](ctx, cfg, fn)` for operations returning (T, error)
- [x] Implement `IsRetryable(err)` to detect transient errors
- [x] Implement `DoIfRetryable(ctx, cfg, fn)` that skips retries for permanent errors
- [x] Respect context cancellation during wait periods
- [x] Create `pkg/retry/retry_test.go` with unit tests

### Step 2: Create Log Sanitizer
- [x] Create `pkg/logging/sanitizer.go`
- [x] Implement `SanitizeConnectionString()` to redact passwords from connection strings
- [x] Implement `SanitizeError()` to redact passwords, JWT tokens, API keys from error messages
- [x] Implement `SanitizeQuery()` to truncate and sanitize SQL queries
- [x] Implement `TruncateString()` helper
- [x] Use compiled regex patterns for performance
- [x] Create `pkg/logging/sanitizer_test.go` with unit tests

### Step 3: Create Connection Manager
- [ ] Create `pkg/adapters/datasource/connection_manager.go`
- [ ] Implement `ConnectionManager` struct with TTL-based pooling
- [ ] Implement background cleanup goroutine
- [ ] Add `GetOrCreatePool()`, `Close()`, `GetStats()` methods
- [ ] Use `retry.DoWithResult()` for pool creation
- [ ] Use `retry.Do()` for health checks
- [ ] Follow lock ordering rules from ekaya-region reference (manager lock → connection lock)

### Step 4: Add User Context Extraction
- [ ] Create `pkg/auth/context.go` (if not exists)
- [ ] Implement `GetUserIDFromContext(ctx)` to extract user ID from JWT claims
- [ ] Add context helpers for extracting `projectID` and `userID` from incoming requests

### Step 5: Update Factory Interface
- [ ] Modify `pkg/adapters/datasource/factory.go`
- [ ] Add `connMgr *ConnectionManager` field to `registryFactory`
- [ ] Update `NewDatasourceAdapterFactory()` to accept connection manager
- [ ] Update all factory methods to pass `projectID`, `userID`, `datasourceID` to adapters

### Step 6: Update Registry Interface
- [ ] Modify `pkg/adapters/datasource/registry.go`
- [ ] Update factory function signatures to accept `(ctx, config, connMgr, projectID, userID, datasourceID)`
- [ ] Update global registry maps to use new signatures

### Step 7: Update PostgreSQL Adapter
- [ ] Modify `pkg/adapters/datasource/postgres/adapter.go`
- [ ] Add `connMgr`, `projectID`, `userID`, `datasourceID`, `ownedPool` fields
- [ ] Update `NewAdapter()` to use connection manager when available
- [ ] Modify `Close()` to skip closing managed pools
- [ ] Update `pkg/adapters/datasource/postgres/schema.go` (NewSchemaDiscoverer)
- [ ] Update `pkg/adapters/datasource/postgres/query.go` (NewQueryExecutor) if exists

### Step 8: Update Service Layer
- [ ] Modify `pkg/services/schema.go`
  - Extract `userID` from context in all methods calling factory
  - Pass `projectID`, `userID`, `datasourceID` to factory methods
- [ ] Modify `pkg/services/query.go`
  - Same context extraction pattern
  - Update `Execute()` and `Test()` methods
- [ ] Modify `pkg/services/datasource.go`
  - `TestConnection()` continues using unmanaged pools (no reuse needed for one-off tests)

### Step 9: Add Configuration
- [ ] Modify `config/config.go`
- [ ] Add `DatasourceConfig` struct with connection manager settings
- [ ] Add `Datasource DatasourceConfig` field to main `Config` struct
- [ ] Set sensible defaults (TTL: 5min, MaxPerUser: 10, PoolMax: 10, PoolMin: 1)
- [ ] Update config.yaml.example with new datasource section

### Step 10: Wire in Main
- [ ] Modify `main.go`
- [ ] Create `ConnectionManagerConfig` from `cfg.Datasource`
- [ ] Create `ConnectionManager` before adapter factory with config and logger
- [ ] Pass connection manager to factory constructor
- [ ] Defer `connManager.Close()` on shutdown

### Step 11: Testing
- [ ] Create `pkg/retry/retry_test.go`
  - Test exponential backoff timing
  - Test context cancellation during retry
  - Test max retries exhaustion
  - Test successful retry after transient failure
  - Test `IsRetryable()` with various error patterns (connection refused, timeout, etc.)
  - Test `DoIfRetryable()` skips retries for non-transient errors
- [ ] Create `pkg/logging/sanitizer_test.go`
  - Test password redaction in connection strings (`password=secret` → `password=[REDACTED]`)
  - Test JWT token redaction (`Bearer eyJ...` → `Bearer [REDACTED]`)
  - Test API key redaction
  - Test connection string format redaction (`://user:pass@host` → `://[REDACTED]@[REDACTED]`)
  - Test query truncation at MaxQueryLogLength
- [ ] Create `pkg/adapters/datasource/connection_manager_test.go`
  - Test pool reuse for same `(projectID, userID, datasourceID)` tuple
  - Test TTL expiration and cleanup
  - Test health check recovery from bad connections
  - Test concurrent access (race detector)
  - Test retry behavior on pool creation failure
  - Test per-user connection limit enforcement (returns error when exceeded)
- [ ] Create `pkg/adapters/datasource/postgres/adapter_security_test.go`
  - Test SQL injection prevention with parameterized queries
  - Test password URL escaping handles special characters: `@`, `/`, `#`, `?`, `;`, spaces
  - Test connection string building with malicious inputs
  - Test that passwords with SQL injection attempts are safely escaped
- [ ] Update existing integration tests to pass context with user ID
- [ ] Verify connection reuse in integration tests (inspect pool stats)

### Step 12: Observability
- [ ] Add metrics endpoint exposing connection manager stats
- [ ] Log connection pool creation/reuse/cleanup events at INFO level
- [ ] Add health check for connection manager in `/health` endpoint
- [ ] Expose connection stats in `/health` response

### Step 13: Health Endpoint Integration

**Modify:** `pkg/handlers/health.go`

```go
type HealthResponse struct {
    Status      string                      `json:"status"`
    Database    string                      `json:"database"`
    Redis       string                      `json:"redis,omitempty"`
    Connections *datasource.ConnectionStats `json:"connections,omitempty"`
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
    response := HealthResponse{
        Status:   "ok",
        Database: "ok",
    }

    // Check internal database
    if err := h.db.Ping(r.Context()); err != nil {
        response.Status = "degraded"
        response.Database = "error"
    }

    // Check Redis if configured
    if h.redis != nil {
        if err := h.redis.Ping(r.Context()).Err(); err != nil {
            response.Status = "degraded"
            response.Redis = "error"
        } else {
            response.Redis = "ok"
        }
    }

    // Include connection manager stats
    if h.connManager != nil {
        stats := h.connManager.GetStats()
        response.Connections = &stats
    }

    // Return 503 if degraded, 200 otherwise
    statusCode := http.StatusOK
    if response.Status == "degraded" {
        statusCode = http.StatusServiceUnavailable
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    json.NewEncoder(w).Encode(response)
}
```

**Example `/health` response:**
```json
{
  "status": "ok",
  "database": "ok",
  "redis": "ok",
  "connections": {
    "total_connections": 5,
    "max_connections_per_user": 10,
    "ttl_minutes": 5,
    "connections_by_project": {
      "proj-uuid-1": 3,
      "proj-uuid-2": 2
    },
    "connections_by_user": {
      "user-1": 2,
      "user-2": 3
    },
    "oldest_idle_seconds": 45
  }
}
```

## Integration Points

### Context Propagation
All service methods must extract `userID` from context:
```go
userID := auth.GetUserIDFromContext(ctx)
if userID == "" {
    return nil, fmt.Errorf("user ID not found in context")
}
```

### Factory Method Signatures (Updated)
```go
// Before
NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any) (SchemaDiscoverer, error)

// After
NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (SchemaDiscoverer, error)
```

### Adapter Constructor Signatures (Updated)
```go
// Before
NewAdapter(ctx context.Context, cfg *Config) (*Adapter, error)

// After
NewAdapter(ctx context.Context, cfg *Config, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (*Adapter, error)
```

## Testing Strategy

### Unit Tests
- Connection manager lifecycle (create, reuse, expire, cleanup)
- Thread safety with race detector
- Health check behavior on failed connections
- Stats reporting accuracy

### Integration Tests
- Verify same `(projectID, userID, datasourceID)` reuses pool
- Verify different users/projects get separate pools
- Verify TTL expiration works in real time
- Verify query execution still works (no behavioral regressions)

### Load Tests
- Execute 1000 queries across 10 datasources
- Verify only 10 pools created (not 1000)
- Measure latency improvement vs baseline
- Monitor connection count at database level

## Migration Notes

### Backward Compatibility
- TestConnection continues using unmanaged pools (backward compatible)
- Existing tests work unchanged if they don't provide connection manager
- Factory falls back to creating unmanaged pools when `connMgr == nil`

### Rollout Strategy
1. Deploy connection manager with 1 minute TTL (conservative)
2. Monitor pool reuse rates and connection counts
3. Gradually increase TTL to 5 minutes if stable
4. Monitor customer database `max_connections` usage

## Key Differences from ekaya-region

### Connection Key Format
- **ekaya-region:** `{projectID}:{userID}` (ADBC connections to metadata DB)
- **ekaya-engine:** `{projectID}:{userID}:{datasourceID}` (pgxpool to customer datasources)

**Rationale:** The key includes all three components because:
1. **`datasourceID`** - Each customer database has different connection endpoints and credentials
2. **`userID`** - Datasources may be configured with user-level credentials rather than shared project credentials. When users have their own database credentials, each user needs a separate connection pool to the same datasource.
3. **`projectID`** - Provides tenant isolation and enables per-project connection tracking

### Database Type
- **ekaya-region:** ADBC (adbc.Database + adbc.Connection)
- **ekaya-engine:** pgxpool (pgxpool.Pool)

**Rationale:** ekaya-engine uses pgx directly for customer PostgreSQL databases, not ADBC abstraction layer.

### Health Check Pattern
- **ekaya-region:** Execute `SELECT 1` via ADBC statement
- **ekaya-engine:** Use `pool.Ping(ctx)` (native pgx method)

**Rationale:** pgxpool provides built-in health check, no need for custom query execution.

## Graceful Degradation

When capacity limits are reached, the system should fail clearly rather than unpredictably:

### Per-User Limit Reached
**Error:** `"user {userID} has reached maximum connections limit ({max})"`

**Client behavior:**
- Return HTTP 429 (Too Many Requests) with Retry-After header
- Log warning with user ID and current count
- Do NOT queue requests (prevents memory exhaustion)

**Handler integration:**
```go
func handleDatasourceError(err error, w http.ResponseWriter) {
    if strings.Contains(err.Error(), "maximum connections limit") {
        w.Header().Set("Retry-After", "30")
        http.Error(w, err.Error(), http.StatusTooManyRequests)
        return
    }
    // ... other error handling
}
```

### Pool Creation Failure (After Retries)
**Error:** `"failed to create pool for {key} after retries: {underlying error}"`

**Client behavior:**
- Return HTTP 503 (Service Unavailable)
- Include underlying error in response for debugging
- Log error with full context (projectID, datasourceID, userID)

### Alerting Thresholds
Consider alerting when:
- Any user reaches 80% of their connection limit
- Total connections exceed 80% of expected capacity
- Connection creation failures exceed threshold (e.g., 5/minute)

## Files to Create

- `pkg/retry/retry.go` - Retry with exponential backoff, IsRetryable helper
- `pkg/retry/retry_test.go` - Retry package unit tests
- `pkg/logging/sanitizer.go` - Log sanitization for credentials, tokens, connection strings
- `pkg/logging/sanitizer_test.go` - Sanitizer unit tests
- `pkg/adapters/datasource/connection_manager.go` - Connection manager implementation
- `pkg/adapters/datasource/connection_manager_test.go` - Connection manager unit tests
- `pkg/adapters/datasource/postgres/adapter_security_test.go` - Security tests for SQL injection, URL escaping
- `pkg/auth/context.go` - User ID extraction from context (if not exists)

## Files to Modify

- `config/config.go` - Add DatasourceConfig struct with connection manager settings
- `pkg/adapters/datasource/factory.go` - Add connection manager integration
- `pkg/adapters/datasource/registry.go` - Update factory function signatures
- `pkg/adapters/datasource/postgres/adapter.go` - Use connection manager, add URL escaping
- `pkg/adapters/datasource/postgres/schema.go` - Update constructor
- `pkg/services/schema.go` - Extract context, pass IDs to factory
- `pkg/services/query.go` - Extract context, pass IDs to factory
- `pkg/services/datasource.go` - TestConnection uses unmanaged pool
- `pkg/handlers/health.go` - Add connection stats to health response
- `main.go` - Wire connection manager into factory with config

## Success Criteria

- [ ] Retry package handles transient failures with exponential backoff
- [ ] Retry respects context cancellation
- [ ] `IsRetryable()` correctly identifies transient vs permanent errors
- [ ] `DoIfRetryable()` skips retries for permanent errors (auth failures, bad SQL)
- [ ] Log sanitizer redacts passwords from connection strings
- [ ] Log sanitizer redacts JWT tokens from error messages
- [ ] Log sanitizer redacts API keys and credentials
- [ ] Log sanitizer truncates long queries
- [ ] Connection pool reuse verified (same tuple → same pool)
- [ ] TTL expiration works (idle pools cleaned up after configured TTL)
- [ ] Health checks recover from bad connections (with retry)
- [ ] Pool creation survives transient network failures (with retry)
- [ ] No connection leaks (all pools closed on shutdown)
- [ ] Race detector passes on all tests
- [ ] Integration tests pass with no behavioral changes
- [ ] Load test shows pool count << request count
- [ ] Customer database `max_connections` stays low under load
- [ ] Per-user connection limits enforced (returns 429 when exceeded)
- [ ] Connection manager stats exposed in `/health` endpoint
- [ ] All settings configurable via config.yaml and environment variables
- [ ] URL escaping handles special characters in passwords (tested with @, /, #, ?, ;, spaces)
- [ ] SQL injection prevention verified with security tests
- [ ] Structured logging with zap throughout (no log.Printf)
- [ ] All logged errors use `logging.SanitizeError()` to prevent credential leaks
