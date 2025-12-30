package datasource

import (
	"context"
	"sync"

	"github.com/google/uuid"
)

// DatasourceAdapterInfo describes a registered adapter for UI discovery.
type DatasourceAdapterInfo struct {
	Type        string `json:"type"`         // "postgres", "sqlserver", "bigquery"
	DisplayName string `json:"display_name"` // "PostgreSQL", "Microsoft SQL Server"
	Description string `json:"description"`  // "Connect to PostgreSQL 12+"
	Icon        string `json:"icon"`         // Icon identifier for UI
}

// DatasourceAdapterRegistration contains info + factories for creating adapters.
// Factory functions accept connection manager and identity parameters for connection pooling.
type DatasourceAdapterRegistration struct {
	Info                    DatasourceAdapterInfo
	Factory                 func(ctx context.Context, config map[string]any, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (ConnectionTester, error)
	SchemaDiscovererFactory func(ctx context.Context, config map[string]any, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (SchemaDiscoverer, error)
	QueryExecutorFactory    func(ctx context.Context, config map[string]any, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (QueryExecutor, error)
}

var (
	registryMu sync.RWMutex
	registry   = make(map[string]DatasourceAdapterRegistration)
)

// Register is called by each adapter's init() function.
// Thread-safe for concurrent init() calls.
func Register(reg DatasourceAdapterRegistration) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[reg.Info.Type] = reg
}

// RegisteredAdapters returns info for all registered adapters.
// Used by API endpoint to tell UI which datasource types are available.
func RegisteredAdapters() []DatasourceAdapterInfo {
	registryMu.RLock()
	defer registryMu.RUnlock()

	result := make([]DatasourceAdapterInfo, 0, len(registry))
	for _, reg := range registry {
		result = append(result, reg.Info)
	}
	return result
}

// GetFactory returns the factory for a datasource type.
// Returns nil if type is not registered.
func GetFactory(dsType string) func(ctx context.Context, config map[string]any, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (ConnectionTester, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if reg, ok := registry[dsType]; ok {
		return reg.Factory
	}
	return nil
}

// GetSchemaDiscovererFactory returns the schema discoverer factory for a datasource type.
// Returns nil if type is not registered or doesn't support schema discovery.
func GetSchemaDiscovererFactory(dsType string) func(ctx context.Context, config map[string]any, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (SchemaDiscoverer, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if reg, ok := registry[dsType]; ok {
		return reg.SchemaDiscovererFactory
	}
	return nil
}

// GetQueryExecutorFactory returns the query executor factory for a datasource type.
// Returns nil if type is not registered or doesn't support query execution.
func GetQueryExecutorFactory(dsType string) func(ctx context.Context, config map[string]any, connMgr *ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (QueryExecutor, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()

	if reg, ok := registry[dsType]; ok {
		return reg.QueryExecutorFactory
	}
	return nil
}

// IsRegistered checks if an adapter type is available.
func IsRegistered(dsType string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[dsType]
	return ok
}
