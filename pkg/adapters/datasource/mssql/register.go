//go:build mssql || all_adapters

package mssql

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

func init() {
	datasource.Register(datasource.DatasourceAdapterRegistration{
		Info: datasource.DatasourceAdapterInfo{
			Type:        "mssql",
			DisplayName: "Microsoft SQL Server",
			Description: "Connect to SQL Server 2019+, Azure SQL Database",
			Icon:        "mssql",
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
			// Pass nil logger - a no-op logger will be used internally
			return NewSchemaDiscoverer(ctx, cfg, connMgr, projectID, datasourceID, userID, nil)
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
