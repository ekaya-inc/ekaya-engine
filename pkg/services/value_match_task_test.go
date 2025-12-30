// go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// valueMatchTestContext holds test dependencies.
type valueMatchTestContext struct {
	t             *testing.T
	engineDB      *testhelpers.EngineDB
	projectID     uuid.UUID
	datasourceID  uuid.UUID
	workflowID    uuid.UUID
	ontologyID    uuid.UUID
	usersTableID  uuid.UUID
	ordersTableID uuid.UUID
	userIDColID   uuid.UUID
	orderUserIDID uuid.UUID
	stateRepo     repositories.WorkflowStateRepository
	candidateRepo repositories.RelationshipCandidateRepository
	schemaRepo    repositories.SchemaRepository
}

// setupValueMatchTest initializes test context.
func setupValueMatchTest(t *testing.T) *valueMatchTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &valueMatchTestContext{
		t:             t,
		engineDB:      engineDB,
		projectID:     uuid.New(),
		datasourceID:  uuid.New(),
		workflowID:    uuid.New(),
		ontologyID:    uuid.New(),
		usersTableID:  uuid.New(),
		ordersTableID: uuid.New(),
		userIDColID:   uuid.New(),
		orderUserIDID: uuid.New(),
		stateRepo:     repositories.NewWorkflowStateRepository(),
		candidateRepo: repositories.NewRelationshipCandidateRepository(),
		schemaRepo:    repositories.NewSchemaRepository(),
	}
	tc.ensureTestData()
	return tc
}

// ensureTestData creates required test data.
func (tc *valueMatchTestContext) ensureTestData() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope: %v", err)
	}
	defer scope.Close()

	// Create project
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, 'ValueMatch Test Project', 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID)
	require.NoError(tc.t, err)

	// Create datasource
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, 'Test DS', 'postgres', 'encrypted')
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID)
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

	// Create tables
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected, row_count)
		VALUES ($1, $2, $3, 'public', 'users', true, 100)
		ON CONFLICT (id) DO NOTHING
	`, tc.usersTableID, tc.projectID, tc.datasourceID)
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected, row_count)
		VALUES ($1, $2, $3, 'public', 'orders', true, 500)
		ON CONFLICT (id) DO NOTHING
	`, tc.ordersTableID, tc.projectID, tc.datasourceID)
	require.NoError(tc.t, err)

	// Create columns
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'id', 'uuid', false, true, true, true, 1)
		ON CONFLICT (id) DO NOTHING
	`, tc.userIDColID, tc.projectID, tc.usersTableID)
	require.NoError(tc.t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'user_id', 'uuid', true, false, false, true, 2)
		ON CONFLICT (id) DO NOTHING
	`, tc.orderUserIDID, tc.projectID, tc.ordersTableID)
	require.NoError(tc.t, err)
}

func (tc *valueMatchTestContext) getTenantCtx() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

func TestValueMatchTask_Execute(t *testing.T) {
	tc := setupValueMatchTest(t)
	ctx, cleanup := tc.getTenantCtx()
	defer cleanup()

	// Create workflow states with scan data
	userIDState := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeColumn,
		EntityKey:  "users.id",
		Status:     models.WorkflowEntityStatusScanned,
		StateData: &models.WorkflowStateData{
			Gathered: map[string]any{
				"row_count":      float64(100),
				"distinct_count": float64(100),
				"null_percent":   float64(0),
				"sample_values":  []any{"uuid-1", "uuid-2", "uuid-3", "uuid-4", "uuid-5"},
			},
		},
	}
	err := tc.stateRepo.Create(ctx, userIDState)
	require.NoError(t, err)

	orderUserIDState := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeColumn,
		EntityKey:  "orders.user_id",
		Status:     models.WorkflowEntityStatusScanned,
		StateData: &models.WorkflowStateData{
			Gathered: map[string]any{
				"row_count":      float64(500),
				"distinct_count": float64(80),
				"null_percent":   float64(5.0),
				"sample_values":  []any{"uuid-1", "uuid-2", "uuid-3", "uuid-99", "uuid-100"}, // 3/5 = 60% match
			},
		},
	}
	err = tc.stateRepo.Create(ctx, orderUserIDState)
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

	task := NewValueMatchTask(
		tc.stateRepo,
		tc.candidateRepo,
		tc.schemaRepo,
		getTenantCtx,
		tc.projectID,
		tc.workflowID,
		tc.datasourceID,
		logger,
	)

	// Execute task
	err = task.Execute(ctx, nil)
	require.NoError(t, err)

	// Verify candidates were created
	candidates, err := tc.candidateRepo.GetByWorkflow(ctx, tc.workflowID)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "Expected 1 candidate")

	candidate := candidates[0]
	require.Equal(t, tc.orderUserIDID, candidate.SourceColumnID)
	require.Equal(t, tc.userIDColID, candidate.TargetColumnID)
	require.Equal(t, models.DetectionMethodValueMatch, candidate.DetectionMethod)
	require.NotNil(t, candidate.ValueMatchRate)
	require.InDelta(t, 0.6, *candidate.ValueMatchRate, 0.01)
}

func TestValueMatchTask_FilterJoinable(t *testing.T) {
	task := &ValueMatchTask{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name     string
		column   *columnScanInfo
		expected bool
	}{
		{
			name: "UUID column with high cardinality - joinable",
			column: &columnScanInfo{
				dataType:      "uuid",
				isPK:          false,
				distinctCount: 100,
			},
			expected: true,
		},
		{
			name: "PK with low cardinality - joinable (is PK)",
			column: &columnScanInfo{
				dataType:      "integer",
				isPK:          true,
				distinctCount: 2,
			},
			expected: true,
		},
		{
			name: "Status column with low cardinality - not joinable",
			column: &columnScanInfo{
				dataType:      "text",
				isPK:          false,
				distinctCount: 2,
			},
			expected: false,
		},
		{
			name: "Timestamp column - not joinable (excluded type)",
			column: &columnScanInfo{
				dataType:      "timestamp",
				isPK:          false,
				distinctCount: 1000,
			},
			expected: false,
		},
		{
			name: "Boolean column - not joinable (excluded type)",
			column: &columnScanInfo{
				dataType:      "boolean",
				isPK:          false,
				distinctCount: 2,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := []*columnScanInfo{tt.column}
			result := task.filterJoinable(columns)

			if tt.expected {
				require.Len(t, result, 1)
			} else {
				require.Len(t, result, 0)
			}
		})
	}
}

func TestValueMatchTask_ComputeMatchRate(t *testing.T) {
	task := &ValueMatchTask{}

	tests := []struct {
		name     string
		source   []string
		target   []string
		expected float64
	}{
		{
			name:     "Perfect match",
			source:   []string{"a", "b", "c"},
			target:   []string{"a", "b", "c", "d", "e"},
			expected: 1.0,
		},
		{
			name:     "Partial match - 60%",
			source:   []string{"a", "b", "c", "d", "e"},
			target:   []string{"a", "b", "c"},
			expected: 0.6,
		},
		{
			name:     "No match",
			source:   []string{"a", "b", "c"},
			target:   []string{"x", "y", "z"},
			expected: 0.0,
		},
		{
			name:     "Empty source",
			source:   []string{},
			target:   []string{"a", "b", "c"},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := task.computeMatchRate(tt.source, tt.target)
			require.InDelta(t, tt.expected, result, 0.01)
		})
	}
}

func TestValueMatchTask_IsExcludedType(t *testing.T) {
	task := &ValueMatchTask{}

	tests := []struct {
		dataType string
		excluded bool
	}{
		{"uuid", false},
		{"text", false},
		{"integer", false},
		{"timestamp", true},
		{"timestamptz", true},
		{"date", true},
		{"boolean", true},
		{"json", true},
		{"jsonb", true},
	}

	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			result := task.isExcludedType(tt.dataType)
			require.Equal(t, tt.excluded, result)
		})
	}
}
