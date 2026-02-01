//go:build integration

package services

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// ============================================================================
// Test Infrastructure for LLMRelationshipDiscoveryService Integration Tests
// ============================================================================

// llmDiscoveryTestContext holds all dependencies for LLM relationship discovery integration tests.
type llmDiscoveryTestContext struct {
	t                *testing.T
	engineDB         *testhelpers.EngineDB
	testDB           *testhelpers.TestDB
	schemaRepo       repositories.SchemaRepository
	ontologyRepo     repositories.OntologyRepository
	entityRepo       repositories.OntologyEntityRepository
	relationshipRepo repositories.EntityRelationshipRepository
	projectID        uuid.UUID
	datasourceID     uuid.UUID
	ontologyID       uuid.UUID
	testUserID       uuid.UUID
	logger           *zap.Logger
}

// setupLLMDiscoveryTest creates a test context for LLM relationship discovery integration tests.
func setupLLMDiscoveryTest(t *testing.T) *llmDiscoveryTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	testDB := testhelpers.GetTestDB(t)

	// Use unique IDs for test isolation
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000701")
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000702")
	ontologyID := uuid.MustParse("00000000-0000-0000-0000-000000000703")
	testUserID := uuid.MustParse("00000000-0000-0000-0000-000000000704")

	tc := &llmDiscoveryTestContext{
		t:                t,
		engineDB:         engineDB,
		testDB:           testDB,
		schemaRepo:       repositories.NewSchemaRepository(),
		ontologyRepo:     repositories.NewOntologyRepository(),
		entityRepo:       repositories.NewOntologyEntityRepository(),
		relationshipRepo: repositories.NewEntityRelationshipRepository(),
		projectID:        projectID,
		datasourceID:     datasourceID,
		ontologyID:       ontologyID,
		testUserID:       testUserID,
		logger:           zap.NewNop(),
	}

	// Setup test project, datasource, and ontology in engine database
	tc.setupTestProject()

	return tc
}

// setupTestProject ensures the test project, datasource, and user exist.
func (tc *llmDiscoveryTestContext) setupTestProject() {
	tc.t.Helper()
	ctx := context.Background()

	// Create project (without tenant scope)
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "LLM Relationship Discovery Integration Test")
	if err != nil {
		scope.Close()
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}

	// Create test user for provenance FK constraints
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_users (project_id, user_id, role)
		VALUES ($1, $2, 'admin')
		ON CONFLICT (project_id, user_id) DO NOTHING
	`, tc.projectID, tc.testUserID)
	if err != nil {
		scope.Close()
		tc.t.Fatalf("Failed to ensure test user: %v", err)
	}
	scope.Close()

	// Create datasource (with tenant scope)
	scope, err = tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID, "LLM Test Datasource", "postgres", "{}")
	if err != nil {
		scope.Close()
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
	scope.Close()
}

// createTestContext creates a context with tenant scope and inferred provenance.
func (tc *llmDiscoveryTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)
	// Add inferred provenance for DAG-like operations (entity/relationship creation)
	ctx = models.WithInferredProvenance(ctx, tc.testUserID)

	return ctx, func() {
		scope.Close()
	}
}

// cleanup removes all test data for the test project.
func (tc *llmDiscoveryTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Logf("Warning: Failed to create cleanup scope: %v", err)
		return
	}
	defer scope.Close()

	// Clean in dependency order
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_entity_relationships WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_relationships WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_entities WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_columns WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_tables WHERE project_id = $1`, tc.projectID)
}

// createTestOntology creates an active ontology for the test project.
func (tc *llmDiscoveryTestContext) createTestOntology(ctx context.Context) *models.TieredOntology {
	tc.t.Helper()

	ontology := &models.TieredOntology{
		ID:        tc.ontologyID,
		ProjectID: tc.projectID,
		IsActive:  true,
	}

	if err := tc.ontologyRepo.Create(ctx, ontology); err != nil {
		tc.t.Fatalf("Failed to create test ontology: %v", err)
	}

	return ontology
}

// createTestEntity creates an entity for a table.
func (tc *llmDiscoveryTestContext) createTestEntity(ctx context.Context, name, primaryTable string) *models.OntologyEntity {
	tc.t.Helper()

	entity := &models.OntologyEntity{
		ProjectID:    tc.projectID,
		OntologyID:   tc.ontologyID,
		Name:         name,
		PrimaryTable: primaryTable,
		IsPromoted:   true,
	}

	if err := tc.entityRepo.Create(ctx, entity); err != nil {
		tc.t.Fatalf("Failed to create test entity: %v", err)
	}

	return entity
}

// setupTablesInTestDB creates test tables in the target test database.
func (tc *llmDiscoveryTestContext) setupTablesInTestDB() error {
	ctx := context.Background()

	// Create tables with an actual FK constraint
	setupSQL := `
		-- Drop tables if they exist (for clean test runs)
		DROP TABLE IF EXISTS llm_test_orders CASCADE;
		DROP TABLE IF EXISTS llm_test_users CASCADE;

		-- Create users table
		CREATE TABLE llm_test_users (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			email TEXT
		);

		-- Create orders table with actual FK constraint
		CREATE TABLE llm_test_orders (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			user_id UUID NOT NULL REFERENCES llm_test_users(id),
			amount DECIMAL(10,2),
			created_at TIMESTAMP DEFAULT NOW()
		);

		-- Insert test data
		INSERT INTO llm_test_users (id, name, email) VALUES
			('11111111-1111-1111-1111-111111111111', 'Alice', 'alice@example.com'),
			('22222222-2222-2222-2222-222222222222', 'Bob', 'bob@example.com'),
			('33333333-3333-3333-3333-333333333333', 'Charlie', 'charlie@example.com');

		INSERT INTO llm_test_orders (user_id, amount) VALUES
			('11111111-1111-1111-1111-111111111111', 100.00),
			('11111111-1111-1111-1111-111111111111', 200.00),
			('22222222-2222-2222-2222-222222222222', 150.00);
	`

	_, err := tc.testDB.Pool.Exec(ctx, setupSQL)
	return err
}

// cleanupTablesInTestDB removes the test tables from the target database.
func (tc *llmDiscoveryTestContext) cleanupTablesInTestDB() {
	ctx := context.Background()
	cleanupSQL := `
		DROP TABLE IF EXISTS llm_test_orders CASCADE;
		DROP TABLE IF EXISTS llm_test_users CASCADE;
	`
	_, _ = tc.testDB.Pool.Exec(ctx, cleanupSQL)
}

// seedEngineSchema creates schema tables and columns in the engine database.
func (tc *llmDiscoveryTestContext) seedEngineSchema(ctx context.Context) (usersTable, ordersTable *models.SchemaTable, userIDCol, orderIDCol, orderUserIDCol *models.SchemaColumn) {
	tc.t.Helper()

	// Create users table
	usersRowCount := int64(3)
	usersTable = &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		SchemaName:   "public",
		TableName:    "llm_test_users",
		RowCount:     &usersRowCount,
		IsSelected:   true,
	}
	if err := tc.schemaRepo.UpsertTable(ctx, usersTable); err != nil {
		tc.t.Fatalf("Failed to create users table: %v", err)
	}

	// Create orders table
	ordersRowCount := int64(3)
	ordersTable = &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		SchemaName:   "public",
		TableName:    "llm_test_orders",
		RowCount:     &ordersRowCount,
		IsSelected:   true,
	}
	if err := tc.schemaRepo.UpsertTable(ctx, ordersTable); err != nil {
		tc.t.Fatalf("Failed to create orders table: %v", err)
	}

	// Create users.id column (PK)
	userIDCol = &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   usersTable.ID,
		ColumnName:      "id",
		DataType:        "uuid",
		IsNullable:      false,
		IsPrimaryKey:    true,
		OrdinalPosition: 1,
		IsSelected:      true,
	}
	if err := tc.schemaRepo.UpsertColumn(ctx, userIDCol); err != nil {
		tc.t.Fatalf("Failed to create users.id column: %v", err)
	}

	// Create orders.id column (PK)
	orderIDCol = &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   ordersTable.ID,
		ColumnName:      "id",
		DataType:        "uuid",
		IsNullable:      false,
		IsPrimaryKey:    true,
		OrdinalPosition: 1,
		IsSelected:      true,
	}
	if err := tc.schemaRepo.UpsertColumn(ctx, orderIDCol); err != nil {
		tc.t.Fatalf("Failed to create orders.id column: %v", err)
	}

	// Create orders.user_id column (FK)
	orderUserIDCol = &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   ordersTable.ID,
		ColumnName:      "user_id",
		DataType:        "uuid",
		IsNullable:      false,
		IsPrimaryKey:    false,
		OrdinalPosition: 2,
		IsSelected:      true,
	}
	if err := tc.schemaRepo.UpsertColumn(ctx, orderUserIDCol); err != nil {
		tc.t.Fatalf("Failed to create orders.user_id column: %v", err)
	}

	return usersTable, ordersTable, userIDCol, orderIDCol, orderUserIDCol
}

// createDBDeclaredFK creates a schema relationship representing a DB-declared FK constraint.
func (tc *llmDiscoveryTestContext) createDBDeclaredFK(ctx context.Context, sourceTableID, sourceColID, targetTableID, targetColID uuid.UUID) *models.SchemaRelationship {
	tc.t.Helper()

	inferenceMethod := models.InferenceMethodForeignKey
	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    sourceTableID,
		SourceColumnID:   sourceColID,
		TargetTableID:    targetTableID,
		TargetColumnID:   targetColID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
		InferenceMethod:  &inferenceMethod,
		IsValidated:      true,
	}

	if err := tc.schemaRepo.UpsertRelationship(ctx, rel); err != nil {
		tc.t.Fatalf("Failed to create DB-declared FK relationship: %v", err)
	}

	return rel
}

// ============================================================================
// Mock LLM Service for Integration Tests
// ============================================================================

// mockLLMServiceForIntegration tracks LLM calls during integration tests.
// It records which relationship candidates were sent for validation and
// returns configurable responses.
type mockLLMServiceForIntegration struct {
	mu        sync.Mutex
	calls     []string // Track "source.col->target.col" keys for validated candidates
	responses map[string]*RelationshipValidationResult
}

// newMockLLMServiceForIntegration creates a mock LLM service with optional pre-configured responses.
func newMockLLMServiceForIntegration() *mockLLMServiceForIntegration {
	return &mockLLMServiceForIntegration{
		calls:     []string{},
		responses: make(map[string]*RelationshipValidationResult),
	}
}

// SetResponse configures the response for a specific candidate key.
func (m *mockLLMServiceForIntegration) SetResponse(sourceTable, sourceCol, targetTable, targetCol string, result *RelationshipValidationResult) {
	key := fmt.Sprintf("%s.%s->%s.%s", sourceTable, sourceCol, targetTable, targetCol)
	m.responses[key] = result
}

// GetCalls returns the list of candidate keys that were validated.
func (m *mockLLMServiceForIntegration) GetCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string{}, m.calls...)
}

// mockRelationshipValidator wraps the mock LLM service as a RelationshipValidator.
type mockRelationshipValidator struct {
	mock   *mockLLMServiceForIntegration
	logger *zap.Logger
}

func (v *mockRelationshipValidator) ValidateCandidate(ctx context.Context, projectID uuid.UUID, candidate *RelationshipCandidate) (*RelationshipValidationResult, error) {
	key := fmt.Sprintf("%s.%s->%s.%s", candidate.SourceTable, candidate.SourceColumn, candidate.TargetTable, candidate.TargetColumn)

	v.mock.mu.Lock()
	v.mock.calls = append(v.mock.calls, key)
	response, ok := v.mock.responses[key]
	v.mock.mu.Unlock()

	if !ok {
		// Default response: reject with low confidence
		return &RelationshipValidationResult{
			IsValidFK:   false,
			Confidence:  0.3,
			Cardinality: "N:1",
			Reasoning:   "Mock: no configured response",
		}, nil
	}

	return response, nil
}

func (v *mockRelationshipValidator) ValidateCandidates(ctx context.Context, projectID uuid.UUID, candidates []*RelationshipCandidate, progressCallback dag.ProgressCallback) ([]*ValidatedRelationship, error) {
	results := make([]*ValidatedRelationship, 0, len(candidates))

	for i, candidate := range candidates {
		result, err := v.ValidateCandidate(ctx, projectID, candidate)
		if err != nil {
			continue
		}

		results = append(results, &ValidatedRelationship{
			Candidate: candidate,
			Result:    result,
			Validated: true,
		})

		if progressCallback != nil {
			progressCallback(i+1, len(candidates), fmt.Sprintf("Validated %d/%d candidates", i+1, len(candidates)))
		}
	}

	return results, nil
}

// mockCandidateCollector returns empty candidates (for tests focused on DB-declared FK preservation).
type mockCandidateCollector struct{}

func (m *mockCandidateCollector) CollectCandidates(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback dag.ProgressCallback) ([]*RelationshipCandidate, error) {
	// Return empty list - no inference candidates for DB FK preservation tests
	if progressCallback != nil {
		progressCallback(1, 1, "No candidates to collect")
	}
	return []*RelationshipCandidate{}, nil
}

// ============================================================================
// Mock Adapter Factory for Integration Tests
// ============================================================================

type llmTestMockAdapterFactory struct {
	schemaDiscoverer datasource.SchemaDiscoverer
}

func (m *llmTestMockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *llmTestMockAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return m.schemaDiscoverer, nil
}

func (m *llmTestMockAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, nil
}

func (m *llmTestMockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

// llmTestSchemaDiscoverer wraps a real database connection for join analysis.
type llmTestSchemaDiscoverer struct {
	pool *pgxpool.Pool
}

func (r *llmTestSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}

func (r *llmTestSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}

func (r *llmTestSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}

func (r *llmTestSchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

func (r *llmTestSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	return nil, nil
}

func (r *llmTestSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}

func (r *llmTestSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	// Full join analysis query
	query := fmt.Sprintf(`
		WITH source_values AS (
			SELECT DISTINCT %s::text as val FROM %s.%s WHERE %s IS NOT NULL
		),
		target_values AS (
			SELECT DISTINCT %s::text as val FROM %s.%s WHERE %s IS NOT NULL
		),
		matched AS (
			SELECT s.val FROM source_values s INNER JOIN target_values t ON s.val = t.val
		),
		orphans AS (
			SELECT s.val FROM source_values s LEFT JOIN target_values t ON s.val = t.val WHERE t.val IS NULL
		),
		reverse_orphans AS (
			SELECT t.val FROM target_values t LEFT JOIN source_values s ON t.val = s.val WHERE s.val IS NULL
		),
		join_counts AS (
			SELECT COUNT(*) as join_count
			FROM %s.%s s
			INNER JOIN %s.%s t ON s.%s::text = t.%s::text
		)
		SELECT
			(SELECT COUNT(*) FROM matched) as source_matched,
			(SELECT COUNT(*) FROM orphans) as orphan_count,
			(SELECT COUNT(*) FROM matched) as target_matched,
			(SELECT COUNT(*) FROM reverse_orphans) as reverse_orphan_count,
			(SELECT join_count FROM join_counts) as join_count
	`, sourceColumn, sourceSchema, sourceTable, sourceColumn,
		targetColumn, targetSchema, targetTable, targetColumn,
		sourceSchema, sourceTable, targetSchema, targetTable, sourceColumn, targetColumn)

	var sourceMatched, orphanCount, targetMatched, reverseOrphanCount, joinCount int64
	err := r.pool.QueryRow(ctx, query).Scan(&sourceMatched, &orphanCount, &targetMatched, &reverseOrphanCount, &joinCount)
	if err != nil {
		return nil, err
	}

	return &datasource.JoinAnalysis{
		JoinCount:          joinCount,
		OrphanCount:        orphanCount,
		SourceMatched:      sourceMatched,
		TargetMatched:      targetMatched,
		ReverseOrphanCount: reverseOrphanCount,
	}, nil
}

func (r *llmTestSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (r *llmTestSchemaDiscoverer) GetEnumValueDistribution(ctx context.Context, schemaName, tableName, columnName string, completionTimestampCol string, limit int) (*datasource.EnumDistributionResult, error) {
	return nil, nil
}

func (r *llmTestSchemaDiscoverer) Close() error {
	return nil
}

// ============================================================================
// Mock Datasource Service
// ============================================================================

type llmTestMockDatasourceService struct {
	datasource *models.Datasource
}

func (m *llmTestMockDatasourceService) Get(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *llmTestMockDatasourceService) GetByID(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *llmTestMockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.DatasourceWithStatus, error) {
	return nil, nil
}

func (m *llmTestMockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType, provider string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *llmTestMockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType, provider string, config map[string]any) error {
	return nil
}

func (m *llmTestMockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *llmTestMockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any, datasourceID uuid.UUID) error {
	return nil
}

func (m *llmTestMockDatasourceService) SetDefault(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (m *llmTestMockDatasourceService) Delete(ctx context.Context, datasourceID uuid.UUID) error {
	return nil
}

// ============================================================================
// Integration Tests
// ============================================================================

// TestRelationshipDiscoveryService_DBDeclaredFKPreserved verifies that
// DB-declared FK relationships are preserved without making LLM calls.
func TestRelationshipDiscoveryService_DBDeclaredFKPreserved(t *testing.T) {
	tc := setupLLMDiscoveryTest(t)
	tc.cleanup()
	defer tc.cleanup()

	// Setup test tables in target database
	if err := tc.setupTablesInTestDB(); err != nil {
		t.Fatalf("Failed to setup test tables: %v", err)
	}
	defer tc.cleanupTablesInTestDB()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Seed engine schema
	usersTable, ordersTable, userIDCol, _, orderUserIDCol := tc.seedEngineSchema(ctx)

	// Create DB-declared FK relationship in engine_schema_relationships
	// This simulates what happens when the schema discovery imports FK constraints from the DB
	tc.createDBDeclaredFK(ctx, ordersTable.ID, orderUserIDCol.ID, usersTable.ID, userIDCol.ID)

	// Create ontology and entities
	tc.createTestOntology(ctx)
	tc.createTestEntity(ctx, "User", "llm_test_users")
	tc.createTestEntity(ctx, "Order", "llm_test_orders")

	// Get mapped port for test database
	port, err := tc.testDB.Container.MappedPort(context.Background(), "5432")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	// Create mock LLM service to track calls
	mockLLM := newMockLLMServiceForIntegration()
	mockValidator := &mockRelationshipValidator{mock: mockLLM, logger: tc.logger}

	// Create mock datasource service
	mockDS := &llmTestMockDatasourceService{
		datasource: &models.Datasource{
			ID:             tc.datasourceID,
			DatasourceType: "postgres",
			Config: map[string]any{
				"host":     "localhost",
				"port":     port.Int(),
				"database": "test_data",
				"user":     "ekaya",
				"password": "test_password",
			},
		},
	}

	// Create adapter factory
	adapterFactory := &llmTestMockAdapterFactory{
		schemaDiscoverer: &llmTestSchemaDiscoverer{pool: tc.testDB.Pool},
	}

	// Create mock candidate collector that returns no candidates
	// (since we're testing DB FK preservation, not inference)
	mockCollector := &mockCandidateCollector{}

	// Create the service
	svc := NewLLMRelationshipDiscoveryService(
		mockCollector,
		mockValidator,
		mockDS,
		adapterFactory,
		tc.ontologyRepo,
		tc.entityRepo,
		tc.relationshipRepo,
		tc.schemaRepo,
		tc.logger,
	)

	// Run relationship discovery
	result, err := svc.DiscoverRelationships(ctx, tc.projectID, tc.datasourceID, nil)
	require.NoError(t, err, "DiscoverRelationships should not return error")
	require.NotNil(t, result, "Result should not be nil")

	// Assert: DB-declared FK was preserved
	assert.Equal(t, 1, result.PreservedDBFKs, "Should have preserved 1 DB-declared FK")

	// Assert: No LLM calls were made for the DB-declared FK
	llmCalls := mockLLM.GetCalls()
	assert.Empty(t, llmCalls, "LLM should NOT have been called for DB-declared FK relationships")

	// Assert: EntityRelationship was created in the database
	entityRels, err := tc.relationshipRepo.GetByOntology(ctx, tc.ontologyID)
	require.NoError(t, err, "GetByOntology should not return error")
	require.Len(t, entityRels, 1, "Should have created 1 entity relationship")

	// Verify the relationship details
	rel := entityRels[0]
	assert.Equal(t, "llm_test_orders", rel.SourceColumnTable, "Source table should be llm_test_orders")
	assert.Equal(t, "user_id", rel.SourceColumnName, "Source column should be user_id")
	assert.Equal(t, "llm_test_users", rel.TargetColumnTable, "Target table should be llm_test_users")
	assert.Equal(t, "id", rel.TargetColumnName, "Target column should be id")
	assert.Equal(t, models.DetectionMethodForeignKey, rel.DetectionMethod, "Detection method should be foreign_key")
	assert.Equal(t, float64(1.0), rel.Confidence, "Confidence should be 1.0 for DB-declared FK")

	t.Log("DB-declared FK preserved without LLM call - test passed")
}
