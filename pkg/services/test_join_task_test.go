//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres" // Register postgres adapter
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// testJoinTestContext holds test dependencies.
type testJoinTestContext struct {
	t              *testing.T
	testDB         *testhelpers.TestDB
	engineDB       *testhelpers.EngineDB
	projectID      uuid.UUID
	datasourceID   uuid.UUID
	workflowID     uuid.UUID
	ontologyID     uuid.UUID
	usersTableID   uuid.UUID
	ordersTableID  uuid.UUID
	usersIDColID   uuid.UUID
	orderUserIDID  uuid.UUID
	candidateID    uuid.UUID
	candidateRepo  repositories.RelationshipCandidateRepository
	schemaRepo     repositories.SchemaRepository
	dsSvc          DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
}

// setupTestJoinTest initializes test context with real Postgres test data.
func setupTestJoinTest(t *testing.T) *testJoinTestContext {
	testDB := testhelpers.GetTestDB(t)
	engineDB := testhelpers.GetEngineDB(t)

	projectID := uuid.New()
	datasourceID := uuid.New()

	// Get testDB container config
	host, err := testDB.Container.Host(context.Background())
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(context.Background(), "5432")
	require.NoError(t, err)

	tc := &testJoinTestContext{
		t:             t,
		testDB:        testDB,
		engineDB:      engineDB,
		projectID:     projectID,
		datasourceID:  datasourceID,
		workflowID:    uuid.New(),
		ontologyID:    uuid.New(),
		usersTableID:  uuid.New(),
		ordersTableID: uuid.New(),
		usersIDColID:  uuid.New(),
		orderUserIDID: uuid.New(),
		candidateID:   uuid.New(),
		candidateRepo: repositories.NewRelationshipCandidateRepository(),
		schemaRepo:    repositories.NewSchemaRepository(),
		dsSvc: &mockDatasourceService{
			datasource: &models.Datasource{
				ID:             datasourceID,
				ProjectID:      projectID,
				DatasourceType: "postgres",
				Config: map[string]any{
					"host":     host,
					"port":     port.Int(),
					"user":     "postgres",
					"password": "postgres",
					"database": "test_data",
					"ssl_mode": "disable",
				},
			},
		},
		adapterFactory: datasource.NewDatasourceAdapterFactory(nil), // No connection manager for tests
	}

	tc.ensureEngineData()
	tc.ensureTestData()
	return tc
}

// ensureEngineData creates required engine metadata.
func (tc *testJoinTestContext) ensureEngineData() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Create project
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, 'TestJoin Test Project', 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID)
	require.NoError(tc.t, err)

	// Create datasource pointing to test_data database
	connStr := tc.testDB.ConnStr
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, 'Test DS', 'postgres', $3)
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID, connStr)
	require.NoError(tc.t, err)

	// Create ontology
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID)
	require.NoError(tc.t, err)

	// Create workflow
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_workflows (id, project_id, ontology_id, state, phase, datasource_id)
		VALUES ($1, $2, $3, 'running', 'relationships', $4)
		ON CONFLICT (id) DO NOTHING
	`, tc.workflowID, tc.projectID, tc.ontologyID, tc.datasourceID)
	require.NoError(tc.t, err)

	// Create schema tables
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected, row_count)
		VALUES ($1, $2, $3, 'public', 'users', true, 3)
		ON CONFLICT (id) DO NOTHING
	`, tc.usersTableID, tc.projectID, tc.datasourceID)
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected, row_count)
		VALUES ($1, $2, $3, 'public', 'orders', true, 5)
		ON CONFLICT (id) DO NOTHING
	`, tc.ordersTableID, tc.projectID, tc.datasourceID)
	require.NoError(tc.t, err)

	// Create columns
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'id', 'integer', false, true, true, true, 1)
		ON CONFLICT (id) DO NOTHING
	`, tc.usersIDColID, tc.projectID, tc.usersTableID)
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'user_id', 'integer', true, false, false, true, 2)
		ON CONFLICT (id) DO NOTHING
	`, tc.orderUserIDID, tc.projectID, tc.ordersTableID)
	require.NoError(tc.t, err)
}

// ensureTestData creates real data in the test_data database.
func (tc *testJoinTestContext) ensureTestData() {
	tc.t.Helper()
	ctx := context.Background()

	// Drop existing tables if they exist to start clean
	_, err := tc.testDB.Pool.Exec(ctx, `DROP TABLE IF EXISTS orders, users CASCADE`)
	require.NoError(tc.t, err)

	// Create users table
	_, err = tc.testDB.Pool.Exec(ctx, `
		CREATE TABLE users (
			id INTEGER PRIMARY KEY
		)
	`)
	require.NoError(tc.t, err)

	// Create orders table
	_, err = tc.testDB.Pool.Exec(ctx, `
		CREATE TABLE orders (
			id INTEGER PRIMARY KEY,
			user_id INTEGER
		)
	`)
	require.NoError(tc.t, err)

	// Insert test data: 3 users
	_, err = tc.testDB.Pool.Exec(ctx, `
		INSERT INTO users (id) VALUES
		(1),
		(2),
		(3)
	`)
	require.NoError(tc.t, err)

	// Insert test data: 5 orders
	// - 2 orders for user 1 (N:1 pattern)
	// - 2 orders for user 2
	// - 1 orphan order (user_id = 99, no matching user)
	_, err = tc.testDB.Pool.Exec(ctx, `
		INSERT INTO orders (id, user_id) VALUES
		(1, 1),
		(2, 1),
		(3, 2),
		(4, 2),
		(5, 99)
	`)
	require.NoError(tc.t, err)

	tc.t.Cleanup(func() {
		_, _ = tc.testDB.Pool.Exec(context.Background(), `DROP TABLE IF EXISTS orders, users CASCADE`)
	})
}

func (tc *testJoinTestContext) getTenantCtx() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	require.NoError(tc.t, err)
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

func TestTestJoinTask_Execute(t *testing.T) {
	tc := setupTestJoinTest(t)
	ctx, cleanup := tc.getTenantCtx()
	defer cleanup()

	// Create candidate
	candidate := &models.RelationshipCandidate{
		ID:              tc.candidateID,
		WorkflowID:      tc.workflowID,
		DatasourceID:    tc.datasourceID,
		SourceColumnID:  tc.orderUserIDID, // orders.user_id
		TargetColumnID:  tc.usersIDColID,  // users.id
		DetectionMethod: models.DetectionMethodValueMatch,
		Confidence:      0.8,
		Status:          models.RelCandidateStatusPending,
	}
	err := tc.candidateRepo.Create(ctx, candidate)
	require.NoError(t, err)

	// Create task
	logger := zap.NewNop()
	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		scope, err := tc.engineDB.DB.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		return tenantCtx, func() { scope.Close() }, nil
	}

	task := NewTestJoinTask(
		tc.candidateRepo,
		tc.schemaRepo,
		tc.dsSvc,
		tc.adapterFactory,
		getTenantCtx,
		tc.projectID,
		tc.workflowID,
		tc.datasourceID,
		tc.candidateID,
		logger,
	)

	// Execute task
	err = task.Execute(ctx, nil)
	require.NoError(t, err)

	// Verify candidate was updated with metrics
	updated, err := tc.candidateRepo.GetByID(ctx, tc.candidateID)
	require.NoError(t, err)

	// Check cardinality
	require.NotNil(t, updated.Cardinality)
	require.Equal(t, "N:1", *updated.Cardinality, "Expected N:1 cardinality (many orders to one user)")

	// Check row counts
	require.NotNil(t, updated.SourceRowCount)
	require.Equal(t, int64(5), *updated.SourceRowCount, "Expected 5 orders")

	require.NotNil(t, updated.TargetRowCount)
	require.Equal(t, int64(3), *updated.TargetRowCount, "Expected 3 users")

	// Check matched rows (4 out of 5 orders have valid user_id)
	require.NotNil(t, updated.MatchedRows)
	require.Equal(t, int64(4), *updated.MatchedRows, "Expected 4 matched orders")

	// Check orphan rows (1 order with user_id = 99)
	require.NotNil(t, updated.OrphanRows)
	require.Equal(t, int64(1), *updated.OrphanRows, "Expected 1 orphan order")

	// Check orphan rate (1/5 = 20%)
	require.NotNil(t, updated.OrphanRate)
	require.InDelta(t, 0.20, *updated.OrphanRate, 0.01, "Expected 20% orphan rate")

	// Check match rate (4/5 = 80%)
	require.NotNil(t, updated.JoinMatchRate)
	require.InDelta(t, 0.80, *updated.JoinMatchRate, 0.01, "Expected 80% match rate")

	// Check target coverage (2 out of 3 users are referenced)
	require.NotNil(t, updated.TargetCoverage)
	require.InDelta(t, 0.67, *updated.TargetCoverage, 0.01, "Expected 67% target coverage (2/3 users)")
}

func TestTestJoinTask_DetermineCardinality(t *testing.T) {
	task := &TestJoinTask{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name           string
		joinResult     *datasource.JoinAnalysis
		sourceRowCount int64
		targetRowCount int64
		expected       string
		description    string
	}{
		{
			name: "N:1 typical FK (many orders to one user)",
			joinResult: &datasource.JoinAnalysis{
				JoinCount:     4, // 4 join rows (orders with valid user)
				SourceMatched: 4, // 4 distinct source values (order rows)
				TargetMatched: 2, // 2 distinct target values (users 1 and 2)
				OrphanCount:   1, // 1 orphan order
			},
			sourceRowCount: 5,
			targetRowCount: 3,
			expected:       "N:1",
			description:    "Multiple orders reference same user",
		},
		{
			name: "1:1 split entity",
			joinResult: &datasource.JoinAnalysis{
				JoinCount:     3, // 3 join rows
				SourceMatched: 3, // 3 distinct source values
				TargetMatched: 3, // 3 distinct target values
				OrphanCount:   0,
			},
			sourceRowCount: 3,
			targetRowCount: 3,
			expected:       "1:1",
			description:    "Each source matches exactly one target",
		},
		{
			name: "1:N inverse direction",
			joinResult: &datasource.JoinAnalysis{
				JoinCount:     6, // 6 join rows
				SourceMatched: 2, // 2 distinct source values
				TargetMatched: 6, // 6 distinct target values
				OrphanCount:   0,
			},
			sourceRowCount: 2,
			targetRowCount: 6,
			expected:       "1:N",
			description:    "One source matches many targets",
		},
		{
			name: "N:M junction table",
			joinResult: &datasource.JoinAnalysis{
				JoinCount:     10, // 10 join rows
				SourceMatched: 5,  // 5 distinct source values
				TargetMatched: 8,  // 8 distinct target values
				OrphanCount:   0,
			},
			sourceRowCount: 5,
			targetRowCount: 8,
			expected:       "N:M",
			description:    "Many-to-many relationship",
		},
		{
			name: "No matches defaults to N:1",
			joinResult: &datasource.JoinAnalysis{
				JoinCount:     0,
				SourceMatched: 0,
				TargetMatched: 0,
				OrphanCount:   5,
			},
			sourceRowCount: 5,
			targetRowCount: 3,
			expected:       "N:1",
			description:    "Default to typical FK pattern when no matches",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := task.determineCardinality(tt.joinResult, tt.sourceRowCount, tt.targetRowCount)
			require.Equal(t, tt.expected, result, tt.description)
		})
	}
}

func TestTestJoinTask_CalculateMetrics(t *testing.T) {
	task := &TestJoinTask{
		logger: zap.NewNop(),
	}

	ctx := context.Background()

	// Test case: 5 orders, 3 users, 4 matches, 1 orphan
	joinResult := &datasource.JoinAnalysis{
		JoinCount:     4,
		SourceMatched: 4,
		TargetMatched: 2,
		OrphanCount:   1,
	}

	sourceRowCount := int64(5)
	targetRowCount := int64(3)
	sourceTable := &models.SchemaTable{
		RowCount: &sourceRowCount,
	}
	targetTable := &models.SchemaTable{
		RowCount: &targetRowCount,
	}

	metrics, err := task.calculateMetrics(ctx, joinResult, sourceTable, targetTable)
	require.NoError(t, err)

	require.Equal(t, "N:1", metrics.Cardinality)
	require.Equal(t, int64(5), metrics.SourceRowCount)
	require.Equal(t, int64(3), metrics.TargetRowCount)
	require.Equal(t, int64(4), metrics.MatchedRows)
	require.Equal(t, int64(1), metrics.OrphanRows)
	require.InDelta(t, 0.8, metrics.JoinMatchRate, 0.01)   // 4/5 = 80%
	require.InDelta(t, 0.2, metrics.OrphanRate, 0.01)      // 1/5 = 20%
	require.InDelta(t, 0.67, metrics.TargetCoverage, 0.01) // 2/3 = 67%
}

func TestFormatJoinTaskDescription(t *testing.T) {
	tests := []struct {
		sourceTable  string
		sourceColumn string
		targetTable  string
		targetColumn string
		expected     string
	}{
		{
			sourceTable:  "orders",
			sourceColumn: "user_id",
			targetTable:  "users",
			targetColumn: "id",
			expected:     "Test join: orders.user_id → users.id",
		},
		{
			sourceTable:  "public.orders",
			sourceColumn: "user_id",
			targetTable:  "public.users",
			targetColumn: "id",
			expected:     "Test join: orders.user_id → users.id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := FormatJoinTaskDescription(tt.sourceTable, tt.sourceColumn, tt.targetTable, tt.targetColumn)
			require.Equal(t, tt.expected, result)
		})
	}
}
