//go:build postgres || all_adapters

package postgres

import (
	"context"

	"github.com/google/uuid"

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
		Factory: func(ctx context.Context, config map[string]any, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
			cfg, err := FromMap(config)
			if err != nil {
				return nil, err
			}
			return NewAdapter(ctx, cfg, connMgr, projectID, datasourceID, userID)
		},
		SchemaDiscovererFactory: func(ctx context.Context, config map[string]any, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
			cfg, err := FromMap(config)
			if err != nil {
				return nil, err
			}
			return NewSchemaDiscoverer(ctx, cfg, connMgr, projectID, datasourceID, userID)
		},
		QueryExecutorFactory: func(ctx context.Context, config map[string]any, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
			cfg, err := FromMap(config)
			if err != nil {
				return nil, err
			}
			return NewQueryExecutor(ctx, cfg, connMgr, projectID, datasourceID, userID)
		},
	})
}
