package datasource

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// DatasourceAdapterFactory creates adapters from the registry.
type DatasourceAdapterFactory interface {
	// NewConnectionTester creates a connection tester for the given datasource type.
	NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (ConnectionTester, error)

	// NewSchemaDiscoverer creates a schema discoverer for the given datasource type.
	NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (SchemaDiscoverer, error)

	// NewQueryExecutor creates a query executor for the given datasource type.
	NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (QueryExecutor, error)

	// ListTypes returns info for all registered adapter types.
	ListTypes() []DatasourceAdapterInfo
}

type registryFactory struct {
	connMgr *ConnectionManager
}

// NewDatasourceAdapterFactory returns a factory that uses the global registry.
func NewDatasourceAdapterFactory(connMgr *ConnectionManager) DatasourceAdapterFactory {
	return &registryFactory{
		connMgr: connMgr,
	}
}

func (f *registryFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (ConnectionTester, error) {
	factory := GetFactory(dsType)
	if factory == nil {
		return nil, fmt.Errorf("unsupported datasource type: %s (not compiled in)", dsType)
	}
	return factory(ctx, config, f.connMgr, projectID, datasourceID, userID)
}

func (f *registryFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (SchemaDiscoverer, error) {
	factory := GetSchemaDiscovererFactory(dsType)
	if factory == nil {
		return nil, fmt.Errorf("schema discovery not supported for type: %s", dsType)
	}
	return factory(ctx, config, f.connMgr, projectID, datasourceID, userID)
}

func (f *registryFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (QueryExecutor, error) {
	factory := GetQueryExecutorFactory(dsType)
	if factory == nil {
		return nil, fmt.Errorf("query execution not supported for type: %s", dsType)
	}
	return factory(ctx, config, f.connMgr, projectID, datasourceID, userID)
}

func (f *registryFactory) ListTypes() []DatasourceAdapterInfo {
	return RegisteredAdapters()
}

// Ensure registryFactory implements DatasourceAdapterFactory at compile time.
var _ DatasourceAdapterFactory = (*registryFactory)(nil)
