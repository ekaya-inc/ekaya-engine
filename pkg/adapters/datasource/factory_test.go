package datasource

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

// mockConnectionTester for testing factory
type mockConnectionTester struct {
	projectID    uuid.UUID
	datasourceID uuid.UUID
	userID       string
	connMgr      *ConnectionManager
}

func (m *mockConnectionTester) TestConnection(ctx context.Context) error {
	return nil
}

func (m *mockConnectionTester) Close() error {
	return nil
}

// mockSchemaDiscoverer for testing factory
type mockSchemaDiscoverer struct {
	projectID    uuid.UUID
	datasourceID uuid.UUID
	userID       string
	connMgr      *ConnectionManager
}

func (m *mockSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]TableMetadata, error) {
	return []TableMetadata{}, nil
}

func (m *mockSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]ColumnMetadata, error) {
	return []ColumnMetadata{}, nil
}

func (m *mockSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]ForeignKeyMetadata, error) {
	return []ForeignKeyMetadata{}, nil
}

func (m *mockSchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

func (m *mockSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]ColumnStats, error) {
	return []ColumnStats{}, nil
}

func (m *mockSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*ValueOverlapResult, error) {
	return &ValueOverlapResult{}, nil
}

func (m *mockSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*JoinAnalysis, error) {
	return &JoinAnalysis{}, nil
}

func (m *mockSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return []string{}, nil
}

func (m *mockSchemaDiscoverer) Close() error {
	return nil
}

// mockQueryExecutor for testing factory
type mockQueryExecutor struct {
	projectID    uuid.UUID
	datasourceID uuid.UUID
	userID       string
	connMgr      *ConnectionManager
}

func (m *mockQueryExecutor) ExecuteQuery(ctx context.Context, sqlQuery string, limit int) (*QueryExecutionResult, error) {
	return &QueryExecutionResult{}, nil
}

func (m *mockQueryExecutor) Execute(ctx context.Context, sqlStatement string) (*ExecuteResult, error) {
	return &ExecuteResult{}, nil
}

func (m *mockQueryExecutor) ValidateQuery(ctx context.Context, sqlQuery string) error {
	return nil
}

func (m *mockQueryExecutor) Close() error {
	return nil
}

func TestFactoryPassesConnectionManager(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := ConnectionManagerConfig{
		TTLMinutes:            1,
		MaxConnectionsPerUser: 5,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	connMgr := NewConnectionManager(cfg, logger)
	defer connMgr.Close()

	factory := NewDatasourceAdapterFactory(connMgr)

	// Verify factory is not nil
	require.NotNil(t, factory)

	// Verify factory is of correct type
	regFactory, ok := factory.(*registryFactory)
	require.True(t, ok, "factory should be of type *registryFactory")

	// Verify connection manager was set
	assert.Equal(t, connMgr, regFactory.connMgr, "connection manager should be set in factory")
}

func TestFactoryPassesIdentityParameters(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := ConnectionManagerConfig{
		TTLMinutes:            1,
		MaxConnectionsPerUser: 5,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	connMgr := NewConnectionManager(cfg, logger)
	defer connMgr.Close()

	// Register a mock adapter
	projectID := uuid.New()
	datasourceID := uuid.New()
	userID := "test-user"

	var capturedProjectID uuid.UUID
	var capturedDatasourceID uuid.UUID
	var capturedUserID string
	var capturedConnMgr *ConnectionManager

	mockType := "test-mock-adapter"
	Register(DatasourceAdapterRegistration{
		Info: DatasourceAdapterInfo{
			Type:        mockType,
			DisplayName: "Test Mock",
			Description: "Test adapter",
		},
		Factory: func(ctx context.Context, config map[string]any, cm *ConnectionManager, pID, dsID uuid.UUID, uID string) (ConnectionTester, error) {
			capturedProjectID = pID
			capturedDatasourceID = dsID
			capturedUserID = uID
			capturedConnMgr = cm
			return &mockConnectionTester{
				projectID:    pID,
				datasourceID: dsID,
				userID:       uID,
				connMgr:      cm,
			}, nil
		},
		SchemaDiscovererFactory: func(ctx context.Context, config map[string]any, cm *ConnectionManager, pID, dsID uuid.UUID, uID string) (SchemaDiscoverer, error) {
			capturedProjectID = pID
			capturedDatasourceID = dsID
			capturedUserID = uID
			capturedConnMgr = cm
			return &mockSchemaDiscoverer{
				projectID:    pID,
				datasourceID: dsID,
				userID:       uID,
				connMgr:      cm,
			}, nil
		},
		QueryExecutorFactory: func(ctx context.Context, config map[string]any, cm *ConnectionManager, pID, dsID uuid.UUID, uID string) (QueryExecutor, error) {
			capturedProjectID = pID
			capturedDatasourceID = dsID
			capturedUserID = uID
			capturedConnMgr = cm
			return &mockQueryExecutor{
				projectID:    pID,
				datasourceID: dsID,
				userID:       uID,
				connMgr:      cm,
			}, nil
		},
	})

	factory := NewDatasourceAdapterFactory(connMgr)
	ctx := context.Background()
	config := map[string]any{}

	t.Run("NewConnectionTester passes parameters", func(t *testing.T) {
		tester, err := factory.NewConnectionTester(ctx, mockType, config, projectID, datasourceID, userID)
		require.NoError(t, err)
		require.NotNil(t, tester)
		defer tester.Close()

		assert.Equal(t, projectID, capturedProjectID, "projectID should be passed to adapter")
		assert.Equal(t, datasourceID, capturedDatasourceID, "datasourceID should be passed to adapter")
		assert.Equal(t, userID, capturedUserID, "userID should be passed to adapter")
		assert.Equal(t, connMgr, capturedConnMgr, "connection manager should be passed to adapter")
	})

	t.Run("NewSchemaDiscoverer passes parameters", func(t *testing.T) {
		discoverer, err := factory.NewSchemaDiscoverer(ctx, mockType, config, projectID, datasourceID, userID)
		require.NoError(t, err)
		require.NotNil(t, discoverer)
		defer discoverer.Close()

		assert.Equal(t, projectID, capturedProjectID, "projectID should be passed to adapter")
		assert.Equal(t, datasourceID, capturedDatasourceID, "datasourceID should be passed to adapter")
		assert.Equal(t, userID, capturedUserID, "userID should be passed to adapter")
		assert.Equal(t, connMgr, capturedConnMgr, "connection manager should be passed to adapter")
	})

	t.Run("NewQueryExecutor passes parameters", func(t *testing.T) {
		executor, err := factory.NewQueryExecutor(ctx, mockType, config, projectID, datasourceID, userID)
		require.NoError(t, err)
		require.NotNil(t, executor)
		defer executor.Close()

		assert.Equal(t, projectID, capturedProjectID, "projectID should be passed to adapter")
		assert.Equal(t, datasourceID, capturedDatasourceID, "datasourceID should be passed to adapter")
		assert.Equal(t, userID, capturedUserID, "userID should be passed to adapter")
		assert.Equal(t, connMgr, capturedConnMgr, "connection manager should be passed to adapter")
	})
}

func TestFactoryErrorHandling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := ConnectionManagerConfig{
		TTLMinutes:            1,
		MaxConnectionsPerUser: 5,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	connMgr := NewConnectionManager(cfg, logger)
	defer connMgr.Close()

	factory := NewDatasourceAdapterFactory(connMgr)
	ctx := context.Background()
	config := map[string]any{}

	projectID := uuid.New()
	datasourceID := uuid.New()
	userID := "test-user"

	t.Run("NewConnectionTester returns error for unsupported type", func(t *testing.T) {
		tester, err := factory.NewConnectionTester(ctx, "unsupported-type", config, projectID, datasourceID, userID)
		assert.Error(t, err)
		assert.Nil(t, tester)
		assert.Contains(t, err.Error(), "unsupported datasource type")
	})

	t.Run("NewSchemaDiscoverer returns error for unsupported type", func(t *testing.T) {
		discoverer, err := factory.NewSchemaDiscoverer(ctx, "unsupported-type", config, projectID, datasourceID, userID)
		assert.Error(t, err)
		assert.Nil(t, discoverer)
		assert.Contains(t, err.Error(), "not supported")
	})

	t.Run("NewQueryExecutor returns error for unsupported type", func(t *testing.T) {
		executor, err := factory.NewQueryExecutor(ctx, "unsupported-type", config, projectID, datasourceID, userID)
		assert.Error(t, err)
		assert.Nil(t, executor)
		assert.Contains(t, err.Error(), "not supported")
	})
}

func TestFactoryListTypes(t *testing.T) {
	logger := zaptest.NewLogger(t)
	cfg := ConnectionManagerConfig{
		TTLMinutes:            1,
		MaxConnectionsPerUser: 5,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	connMgr := NewConnectionManager(cfg, logger)
	defer connMgr.Close()

	factory := NewDatasourceAdapterFactory(connMgr)

	types := factory.ListTypes()
	assert.NotNil(t, types)
	// Note: The actual registered types depend on what's compiled in
	// For unit tests, we just verify the method works
}

func TestFactoryNilConnectionManager(t *testing.T) {
	factory := NewDatasourceAdapterFactory(nil)
	require.NotNil(t, factory)

	regFactory, ok := factory.(*registryFactory)
	require.True(t, ok)
	assert.Nil(t, regFactory.connMgr, "connection manager can be nil for testing scenarios")
}
