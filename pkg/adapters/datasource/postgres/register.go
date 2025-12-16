//go:build postgres || all_adapters

package postgres

import (
	"context"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

func init() {
	datasource.Register(datasource.DatasourceAdapterRegistration{
		Info: datasource.DatasourceAdapterInfo{
			Type:        "postgres",
			DisplayName: "PostgreSQL",
			Description: "Connect to PostgreSQL 12+, Aurora PostgreSQL, Supabase",
			Icon:        "postgres",
		},
		Factory: func(ctx context.Context, config map[string]any) (datasource.ConnectionTester, error) {
			cfg, err := FromMap(config)
			if err != nil {
				return nil, err
			}
			return NewAdapter(ctx, cfg)
		},
		SchemaDiscovererFactory: func(ctx context.Context, config map[string]any) (datasource.SchemaDiscoverer, error) {
			cfg, err := FromMap(config)
			if err != nil {
				return nil, err
			}
			return NewSchemaDiscoverer(ctx, cfg)
		},
	})
}
