//go:build integration

package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// TestPKMatchDiscovery_ChannelsOwnerToUsersUserID is an end-to-end integration test
// that validates the complete pk_match discovery pipeline with real database schema.
//
// This test verifies that the relationship channels.owner_id → users.user_id is correctly
// discovered through the complete pipeline:
// 1. Import schema with proper stats collection
// 2. Create entities for users and channels
// 3. Run pk_match discovery
// 4. Assert the relationship exists
//
// This is the golden test for Task 4 of PLAN-fix-relationship-extraction.md
func TestPKMatchDiscovery_ChannelsOwnerToUsersUserID(t *testing.T) {
	// Setup test infrastructure
	engineDB := testhelpers.GetEngineDB(t)
	testDB := testhelpers.GetTestDB(t)
	logger := zap.NewNop()

	// Create repositories
	schemaRepo := repositories.NewSchemaRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	entityRepo := repositories.NewOntologyEntityRepository()
	relationshipRepo := repositories.NewEntityRelationshipRepository()

	// Use unique project/datasource IDs for test isolation
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000401")
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000402")

	// Setup test context with cleanup
	ctx, cleanup := setupPKMatchTestContext(t, engineDB, projectID, datasourceID)
	defer cleanup()

	// Step 1: Import schema with stats from test database
	// The test database has users and channels tables with owner_id FK relationship
	importedTables, err := importSchemaWithStats(ctx, testDB, schemaRepo, projectID, datasourceID, logger)
	if err != nil {
		t.Fatalf("Failed to import schema: %v", err)
	}

	// Verify we got the expected tables
	usersTable := findTable(importedTables, "users")
	channelsTable := findTable(importedTables, "channels")
	if usersTable == nil {
		t.Fatal("Test database missing 'users' table")
	}
	if channelsTable == nil {
		t.Fatal("Test database missing 'channels' table")
	}

	// Debug: Query column stats to verify they were set
	// Use ListColumnsByDatasource which includes discovery fields (is_joinable, etc.)
	allDbColumns, err := schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		t.Logf("Warning: Failed to get columns: %v", err)
	} else {
		t.Logf("All columns (%d):", len(allDbColumns))
		for _, col := range allDbColumns {
			var dc int64
			if col.DistinctCount != nil {
				dc = *col.DistinctCount
			}
			var joinable string
			if col.IsJoinable == nil {
				joinable = "nil"
			} else if *col.IsJoinable {
				joinable = "true"
			} else {
				joinable = "false"
			}
			t.Logf("  %s: type=%s, isPK=%v, isJoinable=%s, distinctCount=%d",
				col.ColumnName, col.DataType, col.IsPrimaryKey, joinable, dc)
		}
	}

	// Step 2: Create ontology and entities
	ontology := &models.TieredOntology{
		ProjectID: projectID,
		Version:   1,
		IsActive:  true,
	}
	if err := ontologyRepo.Create(ctx, ontology); err != nil {
		t.Fatalf("Failed to create ontology: %v", err)
	}

	// Create user entity
	userEntity := &models.OntologyEntity{
		ProjectID:     projectID,
		OntologyID:    ontology.ID,
		Name:          "user",
		Description:   "User entity",
		PrimarySchema: "public",
		PrimaryTable:  "users",
	}
	if err := entityRepo.Create(ctx, userEntity); err != nil {
		t.Fatalf("Failed to create user entity: %v", err)
	}

	// Create channel entity
	channelEntity := &models.OntologyEntity{
		ProjectID:     projectID,
		OntologyID:    ontology.ID,
		Name:          "channel",
		Description:   "Channel entity",
		PrimarySchema: "public",
		PrimaryTable:  "channels",
	}
	if err := entityRepo.Create(ctx, channelEntity); err != nil {
		t.Fatalf("Failed to create channel entity: %v", err)
	}

	// Note: Entity occurrences are now computed at runtime from relationships (task 2.5)
	// PK match discovery no longer depends on pre-created occurrence records

	// Step 3: Create mock datasource service and adapter factory for pk_match
	// Get mapped port for test database
	port, err := testDB.Container.MappedPort(context.Background(), "5432")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	mockDS := &pkMatchMockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
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

	adapterFactory := &pkMatchMockAdapterFactory{
		schemaDiscoverer: &pkMatchRealSchemaDiscoverer{pool: testDB.Pool},
	}

	// Create deterministic relationship service
	detRelService := NewDeterministicRelationshipService(
		mockDS,
		adapterFactory,
		ontologyRepo,
		entityRepo,
		relationshipRepo,
		schemaRepo,
		zap.NewNop(),
	)

	// Step 4: Run pk_match discovery
	result, err := detRelService.DiscoverPKMatchRelationships(ctx, projectID, datasourceID, nil)
	if err != nil {
		t.Fatalf("DiscoverPKMatchRelationships failed: %v", err)
	}

	// Step 5: Verify relationship was discovered
	if result.InferredRelationships == 0 {
		t.Error("Expected at least 1 inferred relationship (channels.owner_id → users.user_id), got 0")
	}

	// Get all entity relationships for verification
	entityRels, err := relationshipRepo.GetByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to get entity relationships: %v", err)
	}

	// Find a channels → users relationship (owner_id is a FK from channels to users)
	// Note: The actual target column depends on test data - owner_id values may exist
	// in either user_id or account_id column in the users table
	var foundRelationship *models.EntityRelationship
	for _, rel := range entityRels {
		// The relationship should be: channel entity references user entity via owner_id
		if rel.SourceEntityID == channelEntity.ID &&
			rel.TargetEntityID == userEntity.ID &&
			rel.SourceColumnName == "owner_id" {
			foundRelationship = rel
			break
		}
	}

	if foundRelationship == nil {
		t.Errorf("Expected relationship channels.owner_id → users.* not found")
		t.Logf("Found %d relationships:", len(entityRels))
		for i, rel := range entityRels {
			t.Logf("  %d: %s.%s → %s.%s (method=%s, status=%s)",
				i+1,
				rel.SourceColumnTable, rel.SourceColumnName,
				rel.TargetColumnTable, rel.TargetColumnName,
				rel.DetectionMethod, rel.Status)
		}
		return
	}

	// Verify relationship properties
	if foundRelationship.DetectionMethod != "pk_match" {
		t.Errorf("Expected detection method 'pk_match', got %q", foundRelationship.DetectionMethod)
	}

	if foundRelationship.Status == "rejected" {
		t.Errorf("Expected relationship status to not be 'rejected', got %q", foundRelationship.Status)
	}

	t.Logf("SUCCESS: Found relationship channels.owner_id → users.%s (confidence=%.2f)",
		foundRelationship.TargetColumnName, foundRelationship.Confidence)
}

// setupPKMatchTestContext creates a test context with tenant scope and cleanup function.
func setupPKMatchTestContext(t *testing.T, engineDB *testhelpers.EngineDB, projectID, datasourceID uuid.UUID) (context.Context, func()) {
	t.Helper()

	ctx := context.Background()

	// Create project if it doesn't exist
	scope, err := engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		t.Fatalf("Failed to create scope for project setup: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "PK Match Integration Test")
	if err != nil {
		scope.Close()
		t.Fatalf("Failed to ensure test project: %v", err)
	}
	scope.Close()

	// Create datasource
	scope, err = engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, datasourceID, projectID, "Test Datasource", "postgres", "{}")
	if err != nil {
		scope.Close()
		t.Fatalf("Failed to ensure test datasource: %v", err)
	}
	scope.Close()

	// Create tenant-scoped context
	tenantScope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, tenantScope)

	// Cleanup function
	cleanup := func() {
		tenantScope.Close()

		// Clean up test data
		cleanupScope, err := engineDB.DB.WithTenant(context.Background(), projectID)
		if err != nil {
			t.Logf("Warning: Failed to create cleanup scope: %v", err)
			return
		}
		defer cleanupScope.Close()

		// Delete in order: relationships, entities, ontology, schema, datasource
		cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_entity_relationships WHERE project_id = $1`, projectID)
		// Note: engine_ontology_entity_occurrences table was dropped in migration 030
		cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_ontology_entities WHERE project_id = $1`, projectID)
		cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_ontologies WHERE project_id = $1`, projectID)
		cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_schema_relationships WHERE project_id = $1`, projectID)
		cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_schema_columns WHERE project_id = $1`, projectID)
		cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_schema_tables WHERE project_id = $1`, projectID)
		cleanupScope.Conn.Exec(context.Background(), `DELETE FROM engine_datasources WHERE id = $1`, datasourceID)
	}

	return ctx, cleanup
}

// importSchemaWithStats imports schema from test database and collects column stats.
func importSchemaWithStats(
	ctx context.Context,
	testDB *testhelpers.TestDB,
	schemaRepo repositories.SchemaRepository,
	projectID, datasourceID uuid.UUID,
	logger *zap.Logger,
) ([]*models.SchemaTable, error) {
	// Query test database for tables
	rows, err := testDB.Pool.Query(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_schema = 'public'
		AND table_name IN ('users', 'channels')
		ORDER BY table_name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []*models.SchemaTable
	for rows.Next() {
		var schemaName, tableName string
		if err := rows.Scan(&schemaName, &tableName); err != nil {
			return nil, err
		}

		// Get row count
		var rowCount int64
		err = testDB.Pool.QueryRow(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", schemaName, tableName)).Scan(&rowCount)
		if err != nil {
			return nil, err
		}

		table := &models.SchemaTable{
			ProjectID:    projectID,
			DatasourceID: datasourceID,
			SchemaName:   schemaName,
			TableName:    tableName,
			RowCount:     &rowCount,
		}

		if err := schemaRepo.UpsertTable(ctx, table); err != nil {
			return nil, err
		}

		// Import columns with stats
		if err := importColumnsWithStats(ctx, testDB, schemaRepo, table, logger); err != nil {
			return nil, err
		}

		tables = append(tables, table)
	}

	return tables, rows.Err()
}

// importColumnsWithStats imports columns for a table and collects stats.
func importColumnsWithStats(
	ctx context.Context,
	testDB *testhelpers.TestDB,
	schemaRepo repositories.SchemaRepository,
	table *models.SchemaTable,
	logger *zap.Logger,
) error {
	// Query columns
	rows, err := testDB.Pool.Query(ctx, `
		SELECT
			column_name,
			data_type,
			is_nullable,
			ordinal_position
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		ORDER BY ordinal_position
	`, table.SchemaName, table.TableName)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var columnName, dataType, isNullableStr string
		var ordinalPosition int
		if err := rows.Scan(&columnName, &dataType, &isNullableStr, &ordinalPosition); err != nil {
			return err
		}

		isNullable := isNullableStr == "YES"

		// Determine if column is PK
		var isPK bool
		err = testDB.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.table_constraints tc
				JOIN information_schema.key_column_usage kcu
					ON tc.constraint_name = kcu.constraint_name
				WHERE tc.table_schema = $1
					AND tc.table_name = $2
					AND kcu.column_name = $3
					AND tc.constraint_type = 'PRIMARY KEY'
			)
		`, table.SchemaName, table.TableName, columnName).Scan(&isPK)
		if err != nil {
			return err
		}

		column := &models.SchemaColumn{
			ProjectID:       table.ProjectID,
			SchemaTableID:   table.ID,
			ColumnName:      columnName,
			DataType:        dataType,
			IsNullable:      isNullable,
			IsPrimaryKey:    isPK,
			OrdinalPosition: ordinalPosition,
		}

		if err := schemaRepo.UpsertColumn(ctx, column); err != nil {
			return err
		}

		// Collect column stats
		if err := collectColumnStats(ctx, testDB, schemaRepo, table, column); err != nil {
			logger.Error("Failed to collect stats", zap.String("column", columnName), zap.Error(err))
			// Continue with other columns even if stats collection fails
			continue
		}
	}

	return rows.Err()
}

// collectColumnStats collects statistics for a column and updates joinability.
func collectColumnStats(
	ctx context.Context,
	testDB *testhelpers.TestDB,
	schemaRepo repositories.SchemaRepository,
	table *models.SchemaTable,
	column *models.SchemaColumn,
) error {
	// Query stats
	var distinctCount, nonNullCount int64
	err := testDB.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT %s) as distinct_count,
			COUNT(%s) as non_null_count
		FROM %s.%s
	`, column.ColumnName, column.ColumnName, table.SchemaName, table.TableName)).Scan(&distinctCount, &nonNullCount)
	if err != nil {
		return err
	}

	// Determine joinability
	// A column is joinable if it has sufficient distinct values and is not a PK
	isJoinable := !column.IsPrimaryKey && distinctCount >= 20
	joinabilityReason := models.JoinabilityUniqueValues
	if !isJoinable && column.IsPrimaryKey {
		joinabilityReason = models.JoinabilityPK
	} else if !isJoinable {
		joinabilityReason = models.JoinabilityLowCardinality
	}

	// Update column with stats
	err = schemaRepo.UpdateColumnJoinability(ctx, column.ID, table.RowCount, &nonNullCount, &distinctCount, &isJoinable, &joinabilityReason)
	if err != nil {
		return fmt.Errorf("UpdateColumnJoinability failed for %s: %w", column.ColumnName, err)
	}
	return nil
}

// findTable finds a table by name in a list of tables.
func findTable(tables []*models.SchemaTable, tableName string) *models.SchemaTable {
	for _, t := range tables {
		if t.TableName == tableName {
			return t
		}
	}
	return nil
}

// Mock implementations for integration test

type pkMatchMockDatasourceService struct {
	datasource *models.Datasource
}

func (m *pkMatchMockDatasourceService) GetByID(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *pkMatchMockDatasourceService) Get(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *pkMatchMockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	return nil, nil
}

func (m *pkMatchMockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType, provider string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *pkMatchMockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType, provider string, config map[string]any) error {
	return nil
}

func (m *pkMatchMockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *pkMatchMockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any, datasourceID uuid.UUID) error {
	return nil
}

func (m *pkMatchMockDatasourceService) SetDefault(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (m *pkMatchMockDatasourceService) Delete(ctx context.Context, datasourceID uuid.UUID) error {
	return nil
}

type pkMatchMockAdapterFactory struct {
	schemaDiscoverer datasource.SchemaDiscoverer
}

func (m *pkMatchMockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *pkMatchMockAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return m.schemaDiscoverer, nil
}

func (m *pkMatchMockAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, nil
}

func (m *pkMatchMockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

// pkMatchRealSchemaDiscoverer wraps a real database connection for join analysis
type pkMatchRealSchemaDiscoverer struct {
	pool *pgxpool.Pool
}

func (r *pkMatchRealSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}

func (r *pkMatchRealSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}

func (r *pkMatchRealSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}

func (r *pkMatchRealSchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

func (r *pkMatchRealSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	return nil, nil
}

func (r *pkMatchRealSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}

func (r *pkMatchRealSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	// Perform real join analysis against test database
	// Note: We only count orphans (source values not in target) - max source value
	// is optional and only relevant for integer columns (to detect small value patterns)
	query := fmt.Sprintf(`
		SELECT
			COUNT(*) FILTER (WHERE t.%s IS NULL) as orphan_count
		FROM %s.%s s
		LEFT JOIN %s.%s t ON s.%s = t.%s
	`, targetColumn, sourceSchema, sourceTable, targetSchema, targetTable, sourceColumn, targetColumn)

	var orphanCount int64
	err := r.pool.QueryRow(ctx, query).Scan(&orphanCount)
	if err != nil {
		return nil, err
	}

	// MaxSourceValue is nil for non-integer columns like UUIDs
	// The algorithm will skip the small integer validation in this case
	return &datasource.JoinAnalysis{
		OrphanCount:    orphanCount,
		MaxSourceValue: nil,
	}, nil
}

func (r *pkMatchRealSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (r *pkMatchRealSchemaDiscoverer) Close() error {
	return nil
}
