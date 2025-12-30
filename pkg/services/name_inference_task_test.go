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

// nameInferenceTestContext holds test dependencies.
type nameInferenceTestContext struct {
	t                 *testing.T
	engineDB          *testhelpers.EngineDB
	projectID         uuid.UUID
	datasourceID      uuid.UUID
	workflowID        uuid.UUID
	ontologyID        uuid.UUID
	usersTableID      uuid.UUID
	ordersTableID     uuid.UUID
	companiesTableID  uuid.UUID
	userIDColID       uuid.UUID
	orderUserIDID     uuid.UUID
	orderCompanyIDID  uuid.UUID
	orderCompanyColID uuid.UUID // column named "company" (not company_id)
	candidateRepo     repositories.RelationshipCandidateRepository
	schemaRepo        repositories.SchemaRepository
}

// setupNameInferenceTest initializes test context.
func setupNameInferenceTest(t *testing.T) *nameInferenceTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &nameInferenceTestContext{
		t:                 t,
		engineDB:          engineDB,
		projectID:         uuid.New(),
		datasourceID:      uuid.New(),
		workflowID:        uuid.New(),
		ontologyID:        uuid.New(),
		usersTableID:      uuid.New(),
		ordersTableID:     uuid.New(),
		companiesTableID:  uuid.New(),
		userIDColID:       uuid.New(),
		orderUserIDID:     uuid.New(),
		orderCompanyIDID:  uuid.New(),
		orderCompanyColID: uuid.New(),
		candidateRepo:     repositories.NewRelationshipCandidateRepository(),
		schemaRepo:        repositories.NewSchemaRepository(),
	}
	tc.ensureTestData()
	return tc
}

// ensureTestData creates required test data.
func (tc *nameInferenceTestContext) ensureTestData() {
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
		VALUES ($1, 'NameInference Test Project', 'active')
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

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_tables (id, project_id, datasource_id, schema_name, table_name, is_selected, row_count)
		VALUES ($1, $2, $3, 'public', 'companies', true, 50)
		ON CONFLICT (id) DO NOTHING
	`, tc.companiesTableID, tc.projectID, tc.datasourceID)
	require.NoError(tc.t, err)

	// Create columns
	// users.id (PK)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'id', 'uuid', false, true, true, true, 1)
		ON CONFLICT (id) DO NOTHING
	`, tc.userIDColID, tc.projectID, tc.usersTableID)
	require.NoError(tc.t, err)

	// companies.id (PK)
	companyPKID := uuid.New()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'id', 'uuid', false, true, true, true, 1)
		ON CONFLICT (id) DO NOTHING
	`, companyPKID, tc.projectID, tc.companiesTableID)
	require.NoError(tc.t, err)

	// orders.user_id (FK - pattern: {table}_id)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'user_id', 'uuid', true, false, false, true, 2)
		ON CONFLICT (id) DO NOTHING
	`, tc.orderUserIDID, tc.projectID, tc.ordersTableID)
	require.NoError(tc.t, err)

	// orders.company_id (FK - pattern: {table}_id, singular form)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'company_id', 'uuid', true, false, false, true, 3)
		ON CONFLICT (id) DO NOTHING
	`, tc.orderCompanyIDID, tc.projectID, tc.ordersTableID)
	require.NoError(tc.t, err)

	// orders.company (FK - pattern: column name matches table name)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_schema_columns (id, project_id, schema_table_id, column_name, data_type, is_nullable, is_primary_key, is_unique, is_selected, ordinal_position)
		VALUES ($1, $2, $3, 'company', 'uuid', true, false, false, true, 4)
		ON CONFLICT (id) DO NOTHING
	`, tc.orderCompanyColID, tc.projectID, tc.ordersTableID)
	require.NoError(tc.t, err)
}

func (tc *nameInferenceTestContext) getTenantCtx() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

func TestNameInferenceTask_Execute(t *testing.T) {
	tc := setupNameInferenceTest(t)
	ctx, cleanup := tc.getTenantCtx()
	defer cleanup()

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

	task := NewNameInferenceTask(
		tc.candidateRepo,
		tc.schemaRepo,
		getTenantCtx,
		tc.projectID,
		tc.workflowID,
		tc.datasourceID,
		logger,
	)

	// Execute task
	err := task.Execute(ctx, nil)
	require.NoError(t, err)

	// Verify candidates were created
	candidates, err := tc.candidateRepo.GetByWorkflow(ctx, tc.workflowID)
	require.NoError(t, err)

	// Should have 3 candidates:
	// 1. orders.user_id → users.id (user_id pattern, exact match)
	// 2. orders.company_id → companies.id (company_id pattern, pluralization)
	// 3. orders.company → companies.id (column name match, pluralization)
	require.Len(t, candidates, 3, "Expected 3 candidates")

	// Verify each candidate
	candidateMap := make(map[uuid.UUID]*models.RelationshipCandidate)
	for _, c := range candidates {
		candidateMap[c.SourceColumnID] = c
	}

	// Check orders.user_id → users.id
	userIDCandidate, ok := candidateMap[tc.orderUserIDID]
	require.True(t, ok, "Expected candidate for orders.user_id")
	require.Equal(t, tc.userIDColID, userIDCandidate.TargetColumnID)
	require.Equal(t, models.DetectionMethodNameInference, userIDCandidate.DetectionMethod)
	require.InDelta(t, ConfidenceTableIDPattern, userIDCandidate.Confidence, 0.01)
	require.NotNil(t, userIDCandidate.NameSimilarity)
	require.InDelta(t, ConfidenceTableIDPattern, *userIDCandidate.NameSimilarity, 0.01)

	// Check orders.company_id → companies.id
	companyIDCandidate, ok := candidateMap[tc.orderCompanyIDID]
	require.True(t, ok, "Expected candidate for orders.company_id")
	require.Equal(t, models.DetectionMethodNameInference, companyIDCandidate.DetectionMethod)
	require.InDelta(t, ConfidenceTableIDPattern, companyIDCandidate.Confidence, 0.01)

	// Check orders.company → companies.id
	companyCandidate, ok := candidateMap[tc.orderCompanyColID]
	require.True(t, ok, "Expected candidate for orders.company")
	require.Equal(t, models.DetectionMethodNameInference, companyCandidate.DetectionMethod)
	require.InDelta(t, ConfidenceColumnNameMatch, companyCandidate.Confidence, 0.01)
}

func TestNameInferenceTask_NoDuplicates(t *testing.T) {
	tc := setupNameInferenceTest(t)
	ctx, cleanup := tc.getTenantCtx()
	defer cleanup()

	// Create an existing candidate
	existingCandidate := &models.RelationshipCandidate{
		WorkflowID:      tc.workflowID,
		DatasourceID:    tc.datasourceID,
		SourceColumnID:  tc.orderUserIDID,
		TargetColumnID:  tc.userIDColID,
		DetectionMethod: models.DetectionMethodValueMatch,
		Confidence:      0.9,
		Status:          models.RelCandidateStatusPending,
		IsRequired:      false,
	}
	err := tc.candidateRepo.Create(ctx, existingCandidate)
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

	task := NewNameInferenceTask(
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

	// Verify candidates
	candidates, err := tc.candidateRepo.GetByWorkflow(ctx, tc.workflowID)
	require.NoError(t, err)

	// Should have 3 candidates total (1 existing + 2 new)
	// The orders.user_id → users.id should NOT be duplicated
	require.Len(t, candidates, 3, "Expected 3 candidates (no duplicates)")

	// Verify the existing candidate is still there
	var foundExisting bool
	for _, c := range candidates {
		if c.SourceColumnID == tc.orderUserIDID && c.TargetColumnID == tc.userIDColID {
			foundExisting = true
			// Should still be the value_match method (not overwritten)
			require.Equal(t, models.DetectionMethodValueMatch, c.DetectionMethod)
			require.InDelta(t, 0.9, c.Confidence, 0.01)
		}
	}
	require.True(t, foundExisting, "Existing candidate should be preserved")
}

func TestSingularize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"users", "user"},
		{"companies", "company"},
		{"orders", "order"},
		{"classes", "class"},
		{"boxes", "box"},
		{"dishes", "dish"},
		{"matches", "match"},
		{"wolves", "wolf"},
		{"knives", "knife"},
		{"people", "person"},
		{"children", "child"},
		{"men", "man"},
		{"women", "woman"},
		{"categories", "category"},
		{"status", "status"},   // ends in 's' but shouldn't change
		{"address", "address"}, // ends in 'ss', shouldn't change
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := singularize(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestPluralize(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"user", "users"},
		{"company", "companies"},
		{"order", "orders"},
		{"class", "classes"},
		{"box", "boxes"},
		{"dish", "dishes"},
		{"match", "matches"},
		{"wolf", "wolves"},
		{"knife", "knives"},
		{"person", "people"},
		{"child", "children"},
		{"man", "men"},
		{"woman", "women"},
		{"category", "categories"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := pluralize(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestNormalizeTableName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Users", "users"},
		{"  orders  ", "orders"},
		{"COMPANIES", "companies"},
		{"Mixed_Case", "mixed_case"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeTableName(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIsVowel(t *testing.T) {
	vowels := []rune{'a', 'e', 'i', 'o', 'u', 'A', 'E', 'I', 'O', 'U'}
	consonants := []rune{'b', 'c', 'd', 'f', 'g', 'h', 'j', 'k', 'l', 'm', 'n', 'p', 'q', 'r', 's', 't', 'v', 'w', 'x', 'y', 'z'}

	for _, v := range vowels {
		require.True(t, isVowel(v), "Expected %c to be a vowel", v)
	}

	for _, c := range consonants {
		require.False(t, isVowel(c), "Expected %c to be a consonant", c)
	}
}
