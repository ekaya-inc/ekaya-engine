package datasource

import (
	"context"
	"fmt"
)

// DatasourceAdapterFactory creates adapters from the registry.
type DatasourceAdapterFactory interface {
	// NewConnectionTester creates a connection tester for the given datasource type.
	NewConnectionTester(ctx context.Context, dsType string, config map[string]any) (ConnectionTester, error)

	// ListTypes returns info for all registered adapter types.
	ListTypes() []DatasourceAdapterInfo
}

type registryFactory struct{}

// NewDatasourceAdapterFactory returns a factory that uses the global registry.
func NewDatasourceAdapterFactory() DatasourceAdapterFactory {
	return &registryFactory{}
}

func (f *registryFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any) (ConnectionTester, error) {
	factory := GetFactory(dsType)
	if factory == nil {
		return nil, fmt.Errorf("unsupported datasource type: %s (not compiled in)", dsType)
	}
	return factory(ctx, config)
}

func (f *registryFactory) ListTypes() []DatasourceAdapterInfo {
	return RegisteredAdapters()
}

// Ensure registryFactory implements DatasourceAdapterFactory at compile time.
var _ DatasourceAdapterFactory = (*registryFactory)(nil)
