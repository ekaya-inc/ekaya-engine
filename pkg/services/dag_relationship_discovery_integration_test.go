//go:build ignore

package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// relationshipDiscoveryTestContext holds all dependencies for DAG relationship discovery integration tests.
type relationshipDiscoveryTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	testDB       *testhelpers.TestDB
	schemaRepo   repositories.SchemaRepository
	projectID    uuid.UUID
	datasourceID uuid.UUID
	logger       *zap.Logger
}

// setupRelationshipDiscoveryTest creates a test context for relationship discovery integration tests.
func setupRelationshipDiscoveryTest(t *testing.T) *relationshipDiscoveryTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	testDB := testhelpers.GetTestDB(t)

	// Use unique IDs for test isolation
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000501")
	datasourceID := uuid.MustParse("00000000-0000-0000-0000-000000000502")

	tc := &relationshipDiscoveryTestContext{
		t:            t,
		engineDB:     engineDB,
		testDB:       testDB,
		schemaRepo:   repositories.NewSchemaRepository(),
		projectID:    projectID,
		datasourceID: datasourceID,
		logger:       zap.NewNop(),
	}

	// Setup test project and datasource in engine database
	tc.setupTestProjectAndDatasource()

	return tc
}

// setupTestProjectAndDatasource ensures the test project and datasource exist.
func (tc *relationshipDiscoveryTestContext) setupTestProjectAndDatasource() {
	tc.t.Helper()
	ctx := context.Background()

	// Create project
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Relationship Discovery DAG Integration Test")
	if err != nil {
		scope.Close()
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
	scope.Close()

	// Create datasource
	scope, err = tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID, "DAG Test Datasource", "postgres", "{}")
	if err != nil {
		scope.Close()
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
	scope.Close()
}

// createTestContext creates a context with tenant scope.
func (tc *relationshipDiscoveryTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)

	return ctx, func() {
		scope.Close()
	}
}

// cleanup removes all schema data for the test project.
func (tc *relationshipDiscoveryTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Logf("Warning: Failed to create cleanup scope: %v", err)
		return
	}
	defer scope.Close()

	// Clean in order: relationships first, then columns, then tables
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_relationships WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_entity_relationships WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_entities WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_columns WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_tables WHERE project_id = $1`, tc.projectID)
}

// setupTestTablesInTargetDB creates test tables in the target test database for validation.
func (tc *relationshipDiscoveryTestContext) setupTestTablesInTargetDB() error {
	ctx := context.Background()

	// Create test tables that demonstrate:
	// 1. A valid FK relationship (orders.customer_id -> customers.id)
	// 2. A false positive candidate (settings.identity_provider with values {1,2,3} and jobs.id with 1-100)
	setupSQL := `
		-- Drop tables if they exist (for clean test runs)
		DROP TABLE IF EXISTS test_orders CASCADE;
		DROP TABLE IF EXISTS test_customers CASCADE;
		DROP TABLE IF EXISTS test_settings CASCADE;
		DROP TABLE IF EXISTS test_jobs CASCADE;

		-- Valid FK relationship: orders -> customers
		CREATE TABLE test_customers (
			id INT PRIMARY KEY,
			name TEXT NOT NULL,
			email TEXT
		);
		INSERT INTO test_customers (id, name, email)
		SELECT i, 'Customer ' || i, 'customer' || i || '@example.com'
		FROM generate_series(1, 50) i;

		CREATE TABLE test_orders (
			id SERIAL PRIMARY KEY,
			customer_id INT NOT NULL,
			amount DECIMAL(10,2),
			created_at TIMESTAMP DEFAULT NOW()
		);
		-- Create orders referencing customers 1-50
		INSERT INTO test_orders (customer_id, amount)
		SELECT (random() * 49 + 1)::int, (random() * 1000)::decimal(10,2)
		FROM generate_series(1, 200);

		-- False positive candidate: settings.identity_provider vs jobs.id
		-- identity_provider has only 3 values {1, 2, 3} that exist in jobs.id (1-100)
		-- This should be REJECTED by bidirectional validation
		CREATE TABLE test_jobs (
			id INT PRIMARY KEY,
			name TEXT NOT NULL,
			status TEXT DEFAULT 'pending'
		);
		INSERT INTO test_jobs (id, name)
		SELECT i, 'Job ' || i
		FROM generate_series(1, 100) i;

		CREATE TABLE test_settings (
			id SERIAL PRIMARY KEY,
			key TEXT NOT NULL,
			identity_provider INT NOT NULL,
			value TEXT
		);
		-- Insert settings with identity_provider values 1, 2, 3 only
		INSERT INTO test_settings (key, identity_provider, value)
		VALUES
			('auth_method', 1, 'password'),
			('auth_method', 2, 'oauth'),
			('auth_method', 3, 'saml'),
			('backup_auth', 1, 'recovery'),
			('backup_auth', 2, 'backup_code');
	`

	_, err := tc.testDB.Pool.Exec(ctx, setupSQL)
	return err
}

// cleanupTestTablesInTargetDB removes the test tables from the target database.
func (tc *relationshipDiscoveryTestContext) cleanupTestTablesInTargetDB() {
	ctx := context.Background()
	cleanupSQL := `
		DROP TABLE IF EXISTS test_orders CASCADE;
		DROP TABLE IF EXISTS test_customers CASCADE;
		DROP TABLE IF EXISTS test_settings CASCADE;
		DROP TABLE IF EXISTS test_jobs CASCADE;
	`
	_, _ = tc.testDB.Pool.Exec(ctx, cleanupSQL)
}

// importTestSchema imports the test tables from the target database into the engine schema.
func (tc *relationshipDiscoveryTestContext) importTestSchema(ctx context.Context) ([]*models.SchemaTable, error) {
	tables := []struct {
		schemaName string
		tableName  string
	}{
		{"public", "test_customers"},
		{"public", "test_orders"},
		{"public", "test_settings"},
		{"public", "test_jobs"},
	}

	var importedTables []*models.SchemaTable

	for _, t := range tables {
		// Get row count
		var rowCount int64
		err := tc.testDB.Pool.QueryRow(ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s.%s", t.schemaName, t.tableName),
		).Scan(&rowCount)
		if err != nil {
			return nil, fmt.Errorf("get row count for %s.%s: %w", t.schemaName, t.tableName, err)
		}

		table := &models.SchemaTable{
			ProjectID:    tc.projectID,
			DatasourceID: tc.datasourceID,
			SchemaName:   t.schemaName,
			TableName:    t.tableName,
			RowCount:     &rowCount,
			IsSelected:   true,
		}

		if err := tc.schemaRepo.UpsertTable(ctx, table); err != nil {
			return nil, fmt.Errorf("upsert table %s.%s: %w", t.schemaName, t.tableName, err)
		}

		// Import columns
		if err := tc.importTableColumns(ctx, table); err != nil {
			return nil, fmt.Errorf("import columns for %s.%s: %w", t.schemaName, t.tableName, err)
		}

		importedTables = append(importedTables, table)
	}

	return importedTables, nil
}

// importTableColumns imports columns for a table from the target database.
func (tc *relationshipDiscoveryTestContext) importTableColumns(ctx context.Context, table *models.SchemaTable) error {
	rows, err := tc.testDB.Pool.Query(ctx, `
		SELECT column_name, data_type, is_nullable, ordinal_position
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

		// Determine if column is PK
		var isPK bool
		err = tc.testDB.Pool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.table_constraints tc
				JOIN information_schema.key_column_usage kcu
					ON tc.constraint_name = kcu.constraint_name
					AND tc.table_schema = kcu.table_schema
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
			ProjectID:       tc.projectID,
			SchemaTableID:   table.ID,
			ColumnName:      columnName,
			DataType:        dataType,
			IsNullable:      isNullableStr == "YES",
			IsPrimaryKey:    isPK,
			OrdinalPosition: ordinalPosition,
			IsSelected:      true,
		}

		if err := tc.schemaRepo.UpsertColumn(ctx, column); err != nil {
			return err
		}

		// Collect and set column stats for joinability
		if err := tc.collectAndSetColumnStats(ctx, table, column); err != nil {
			tc.t.Logf("Warning: Failed to collect stats for %s.%s: %v", table.TableName, columnName, err)
		}
	}

	return rows.Err()
}

// collectAndSetColumnStats collects statistics for a column and sets joinability.
func (tc *relationshipDiscoveryTestContext) collectAndSetColumnStats(ctx context.Context, table *models.SchemaTable, column *models.SchemaColumn) error {
	// Query stats
	var distinctCount, nonNullCount int64
	err := tc.testDB.Pool.QueryRow(ctx, fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT %s) as distinct_count,
			COUNT(%s) as non_null_count
		FROM %s.%s
	`, column.ColumnName, column.ColumnName, table.SchemaName, table.TableName)).Scan(&distinctCount, &nonNullCount)
	if err != nil {
		return err
	}

	// Determine joinability
	isJoinable := true
	joinabilityReason := models.JoinabilityCardinalityOK

	if column.IsPrimaryKey {
		joinabilityReason = models.JoinabilityPK
	} else if distinctCount < 20 && table.RowCount != nil && *table.RowCount > 100 {
		isJoinable = false
		joinabilityReason = models.JoinabilityLowCardinality
	}

	return tc.schemaRepo.UpdateColumnJoinability(ctx, column.ID, table.RowCount, &nonNullCount, &distinctCount, &isJoinable, &joinabilityReason)
}

// TestPKMatchDiscovery_Integration runs the PKMatchDiscovery pipeline and verifies results.
// This is an integration test that exercises the full relationship discovery flow.
func TestPKMatchDiscovery_Integration(t *testing.T) {
	tc := setupRelationshipDiscoveryTest(t)
	tc.cleanup()
	defer tc.cleanup()

	// Setup test tables in target database
	if err := tc.setupTestTablesInTargetDB(); err != nil {
		t.Fatalf("Failed to setup test tables: %v", err)
	}
	defer tc.cleanupTestTablesInTargetDB()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Import schema from test database
	tables, err := tc.importTestSchema(ctx)
	if err != nil {
		t.Fatalf("Failed to import test schema: %v", err)
	}

	if len(tables) != 4 {
		t.Fatalf("Expected 4 tables imported, got %d", len(tables))
	}

	// Get mapped port for test database
	port, err := tc.testDB.Container.MappedPort(context.Background(), "5432")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	// Create mocks for the deterministic relationship service
	mockDS := &dagTestMockDatasourceService{
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

	mockProjectService := &dagTestMockProjectService{
		ontologySettings: &OntologySettings{UseLegacyPatternMatching: true},
	}

	adapterFactory := &dagTestMockAdapterFactory{
		schemaDiscoverer: &dagTestRealSchemaDiscoverer{pool: tc.testDB.Pool},
	}

	// Create deterministic relationship service
	detRelService := NewDeterministicRelationshipService(
		mockDS,
		mockProjectService,
		adapterFactory,
		repositories.NewOntologyRepository(),
		repositories.NewOntologyEntityRepository(),
		repositories.NewEntityRelationshipRepository(),
		tc.schemaRepo,
		nil, // columnMetadataRepo not needed for these tests
		tc.logger,
	)

	// Run FK discovery first (collects stats)
	fkResult, err := detRelService.DiscoverFKRelationships(ctx, tc.projectID, tc.datasourceID, nil)
	if err != nil {
		t.Fatalf("DiscoverFKRelationships failed: %v", err)
	}
	t.Logf("FK discovery result: %+v", fkResult)

	// Run PK-match discovery
	pkResult, err := detRelService.DiscoverPKMatchRelationships(ctx, tc.projectID, tc.datasourceID, nil)
	if err != nil {
		t.Fatalf("DiscoverPKMatchRelationships failed: %v", err)
	}
	t.Logf("PK-match discovery result: %+v", pkResult)

	// Get all schema relationships to verify
	schemaRels, err := tc.schemaRepo.ListRelationshipsByDatasource(ctx, tc.projectID, tc.datasourceID)
	if err != nil {
		t.Fatalf("Failed to get schema relationships: %v", err)
	}

	t.Logf("Found %d schema relationships:", len(schemaRels))
	for _, rel := range schemaRels {
		// Look up table/column names for logging
		sourceCol, _ := tc.schemaRepo.GetColumnByID(ctx, tc.projectID, rel.SourceColumnID)
		targetCol, _ := tc.schemaRepo.GetColumnByID(ctx, tc.projectID, rel.TargetColumnID)

		sourceTable, _ := tc.schemaRepo.GetTableByID(ctx, tc.projectID, rel.SourceTableID)
		targetTable, _ := tc.schemaRepo.GetTableByID(ctx, tc.projectID, rel.TargetTableID)

		var sourceName, targetName string
		if sourceTable != nil && sourceCol != nil {
			sourceName = fmt.Sprintf("%s.%s", sourceTable.TableName, sourceCol.ColumnName)
		}
		if targetTable != nil && targetCol != nil {
			targetName = fmt.Sprintf("%s.%s", targetTable.TableName, targetCol.ColumnName)
		}

		method := "unknown"
		if rel.InferenceMethod != nil {
			method = *rel.InferenceMethod
		}
		t.Logf("  %s -> %s (method=%s, confidence=%.2f, cardinality=%s)",
			sourceName, targetName, method, rel.Confidence, rel.Cardinality)
	}

	// Verify: At least one relationship found (orders.customer_id -> customers.id)
	if len(schemaRels) == 0 {
		t.Error("Expected at least one relationship discovered")
	}

	// Verify: Relationships are stored in engine_schema_relationships
	for _, rel := range schemaRels {
		if rel.ProjectID != tc.projectID {
			t.Errorf("Relationship has wrong project_id: expected %s, got %s", tc.projectID, rel.ProjectID)
		}
	}
}

// TestPKMatchDiscovery_NoFalsePositives verifies that the bidirectional validation
// correctly rejects false positive relationships like identity_provider -> jobs.id.
func TestPKMatchDiscovery_NoFalsePositives(t *testing.T) {
	tc := setupRelationshipDiscoveryTest(t)
	tc.cleanup()
	defer tc.cleanup()

	// Setup test tables
	if err := tc.setupTestTablesInTargetDB(); err != nil {
		t.Fatalf("Failed to setup test tables: %v", err)
	}
	defer tc.cleanupTestTablesInTargetDB()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Import schema
	if _, err := tc.importTestSchema(ctx); err != nil {
		t.Fatalf("Failed to import test schema: %v", err)
	}

	// Get mapped port
	port, err := tc.testDB.Container.MappedPort(context.Background(), "5432")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	// Create service
	mockDS := &dagTestMockDatasourceService{
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

	mockProjectService := &dagTestMockProjectService{
		ontologySettings: &OntologySettings{UseLegacyPatternMatching: true},
	}

	adapterFactory := &dagTestMockAdapterFactory{
		schemaDiscoverer: &dagTestRealSchemaDiscoverer{pool: tc.testDB.Pool},
	}

	detRelService := NewDeterministicRelationshipService(
		mockDS,
		mockProjectService,
		adapterFactory,
		repositories.NewOntologyRepository(),
		repositories.NewOntologyEntityRepository(),
		repositories.NewEntityRelationshipRepository(),
		tc.schemaRepo,
		nil, // columnMetadataRepo not needed for these tests
		tc.logger,
	)

	// Run discovery
	if _, err := detRelService.DiscoverFKRelationships(ctx, tc.projectID, tc.datasourceID, nil); err != nil {
		t.Fatalf("DiscoverFKRelationships failed: %v", err)
	}
	if _, err := detRelService.DiscoverPKMatchRelationships(ctx, tc.projectID, tc.datasourceID, nil); err != nil {
		t.Fatalf("DiscoverPKMatchRelationships failed: %v", err)
	}

	// Get relationships
	schemaRels, err := tc.schemaRepo.ListRelationshipsByDatasource(ctx, tc.projectID, tc.datasourceID)
	if err != nil {
		t.Fatalf("Failed to get schema relationships: %v", err)
	}

	// Find the false positive candidate: settings.identity_provider -> jobs.id
	// This should NOT exist in the relationships
	for _, rel := range schemaRels {
		sourceCol, _ := tc.schemaRepo.GetColumnByID(ctx, tc.projectID, rel.SourceColumnID)
		targetCol, _ := tc.schemaRepo.GetColumnByID(ctx, tc.projectID, rel.TargetColumnID)
		sourceTable, _ := tc.schemaRepo.GetTableByID(ctx, tc.projectID, rel.SourceTableID)
		targetTable, _ := tc.schemaRepo.GetTableByID(ctx, tc.projectID, rel.TargetTableID)

		if sourceTable == nil || targetTable == nil || sourceCol == nil || targetCol == nil {
			continue
		}

		// Check for the false positive pattern
		if sourceTable.TableName == "test_settings" && sourceCol.ColumnName == "identity_provider" &&
			targetTable.TableName == "test_jobs" && targetCol.ColumnName == "id" {
			t.Errorf("False positive detected: test_settings.identity_provider -> test_jobs.id should NOT be discovered")
			t.Logf("  This relationship has method=%v, confidence=%.2f",
				rel.InferenceMethod, rel.Confidence)
		}
	}

	t.Log("Bidirectional validation correctly rejected false positives")
}

// TestPKMatchDiscovery_LegitimateRelationships verifies that valid FK relationships
// are discovered with correct inference_method and validation metrics.
func TestPKMatchDiscovery_LegitimateRelationships(t *testing.T) {
	tc := setupRelationshipDiscoveryTest(t)
	tc.cleanup()
	defer tc.cleanup()

	// Setup test tables
	if err := tc.setupTestTablesInTargetDB(); err != nil {
		t.Fatalf("Failed to setup test tables: %v", err)
	}
	defer tc.cleanupTestTablesInTargetDB()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Import schema
	if _, err := tc.importTestSchema(ctx); err != nil {
		t.Fatalf("Failed to import test schema: %v", err)
	}

	// Get mapped port
	port, err := tc.testDB.Container.MappedPort(context.Background(), "5432")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	// Create service
	mockDS := &dagTestMockDatasourceService{
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

	mockProjectService := &dagTestMockProjectService{
		ontologySettings: &OntologySettings{UseLegacyPatternMatching: true},
	}

	adapterFactory := &dagTestMockAdapterFactory{
		schemaDiscoverer: &dagTestRealSchemaDiscoverer{pool: tc.testDB.Pool},
	}

	detRelService := NewDeterministicRelationshipService(
		mockDS,
		mockProjectService,
		adapterFactory,
		repositories.NewOntologyRepository(),
		repositories.NewOntologyEntityRepository(),
		repositories.NewEntityRelationshipRepository(),
		tc.schemaRepo,
		nil, // columnMetadataRepo not needed for these tests
		tc.logger,
	)

	// Run discovery
	if _, err := detRelService.DiscoverFKRelationships(ctx, tc.projectID, tc.datasourceID, nil); err != nil {
		t.Fatalf("DiscoverFKRelationships failed: %v", err)
	}
	if _, err := detRelService.DiscoverPKMatchRelationships(ctx, tc.projectID, tc.datasourceID, nil); err != nil {
		t.Fatalf("DiscoverPKMatchRelationships failed: %v", err)
	}

	// Get relationships
	schemaRels, err := tc.schemaRepo.ListRelationshipsByDatasource(ctx, tc.projectID, tc.datasourceID)
	if err != nil {
		t.Fatalf("Failed to get schema relationships: %v", err)
	}

	// Find the legitimate relationship: orders.customer_id -> customers.id
	var foundLegitimate bool
	for _, rel := range schemaRels {
		sourceCol, _ := tc.schemaRepo.GetColumnByID(ctx, tc.projectID, rel.SourceColumnID)
		targetCol, _ := tc.schemaRepo.GetColumnByID(ctx, tc.projectID, rel.TargetColumnID)
		sourceTable, _ := tc.schemaRepo.GetTableByID(ctx, tc.projectID, rel.SourceTableID)
		targetTable, _ := tc.schemaRepo.GetTableByID(ctx, tc.projectID, rel.TargetTableID)

		if sourceTable == nil || targetTable == nil || sourceCol == nil || targetCol == nil {
			continue
		}

		// Check for the legitimate relationship
		if sourceTable.TableName == "test_orders" && sourceCol.ColumnName == "customer_id" &&
			targetTable.TableName == "test_customers" && targetCol.ColumnName == "id" {
			foundLegitimate = true

			// Verify inference_method is set correctly
			if rel.InferenceMethod == nil {
				t.Error("Expected inference_method to be set for legitimate relationship")
			} else {
				// Should be either pk_match or column_features
				validMethods := map[string]bool{
					models.InferenceMethodPKMatch:        true,
					models.InferenceMethodColumnFeatures: true,
				}
				if !validMethods[*rel.InferenceMethod] {
					t.Errorf("Expected inference_method to be 'pk_match' or 'column_features', got %q", *rel.InferenceMethod)
				}
			}

			// Verify confidence is reasonable
			if rel.Confidence < 0.5 {
				t.Errorf("Expected confidence >= 0.5 for legitimate relationship, got %.2f", rel.Confidence)
			}

			// Verify cardinality is set
			if rel.Cardinality == "" {
				t.Error("Expected cardinality to be set for legitimate relationship")
			}

			// Verify discovery metrics are stored (for pk_match relationships)
			// Note: We need to fetch the relationship by ID to get the full details including metrics
			if rel.InferenceMethod != nil && *rel.InferenceMethod == models.InferenceMethodPKMatch {
				fullRel, err := tc.schemaRepo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
				if err != nil {
					t.Errorf("Failed to get relationship by ID: %v", err)
				} else {
					if fullRel.MatchRate == nil {
						t.Error("Expected match_rate to be set for pk_match relationship")
					}
					if fullRel.SourceDistinct == nil {
						t.Error("Expected source_distinct to be set for pk_match relationship")
					}
					if fullRel.TargetDistinct == nil {
						t.Error("Expected target_distinct to be set for pk_match relationship")
					}
				}
			}

			t.Logf("Found legitimate relationship: %s.%s -> %s.%s",
				sourceTable.TableName, sourceCol.ColumnName,
				targetTable.TableName, targetCol.ColumnName)
			t.Logf("  inference_method=%v, confidence=%.2f, cardinality=%s",
				*rel.InferenceMethod, rel.Confidence, rel.Cardinality)
		}
	}

	if !foundLegitimate {
		t.Error("Expected to find legitimate relationship: test_orders.customer_id -> test_customers.id")
		t.Logf("Found %d relationships total", len(schemaRels))
	}
}

// TestPKMatchDiscovery_SchemaRelationshipsNotEntityRelationships verifies that
// relationships are stored in engine_schema_relationships, not engine_entity_relationships.
func TestPKMatchDiscovery_SchemaRelationshipsNotEntityRelationships(t *testing.T) {
	tc := setupRelationshipDiscoveryTest(t)
	tc.cleanup()
	defer tc.cleanup()

	// Setup test tables
	if err := tc.setupTestTablesInTargetDB(); err != nil {
		t.Fatalf("Failed to setup test tables: %v", err)
	}
	defer tc.cleanupTestTablesInTargetDB()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Import schema
	if _, err := tc.importTestSchema(ctx); err != nil {
		t.Fatalf("Failed to import test schema: %v", err)
	}

	// Get mapped port
	port, err := tc.testDB.Container.MappedPort(context.Background(), "5432")
	if err != nil {
		t.Fatalf("Failed to get mapped port: %v", err)
	}

	// Create service
	mockDS := &dagTestMockDatasourceService{
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

	mockProjectService := &dagTestMockProjectService{
		ontologySettings: &OntologySettings{UseLegacyPatternMatching: true},
	}

	adapterFactory := &dagTestMockAdapterFactory{
		schemaDiscoverer: &dagTestRealSchemaDiscoverer{pool: tc.testDB.Pool},
	}

	entityRelRepo := repositories.NewEntityRelationshipRepository()

	detRelService := NewDeterministicRelationshipService(
		mockDS,
		mockProjectService,
		adapterFactory,
		repositories.NewOntologyRepository(),
		repositories.NewOntologyEntityRepository(),
		entityRelRepo,
		tc.schemaRepo,
		nil, // columnMetadataRepo not needed for these tests
		tc.logger,
	)

	// Run discovery
	if _, err := detRelService.DiscoverFKRelationships(ctx, tc.projectID, tc.datasourceID, nil); err != nil {
		t.Fatalf("DiscoverFKRelationships failed: %v", err)
	}
	if _, err := detRelService.DiscoverPKMatchRelationships(ctx, tc.projectID, tc.datasourceID, nil); err != nil {
		t.Fatalf("DiscoverPKMatchRelationships failed: %v", err)
	}

	// Verify relationships are in engine_schema_relationships
	schemaRels, err := tc.schemaRepo.ListRelationshipsByDatasource(ctx, tc.projectID, tc.datasourceID)
	if err != nil {
		t.Fatalf("Failed to get schema relationships: %v", err)
	}

	if len(schemaRels) == 0 {
		t.Error("Expected relationships to be stored in engine_schema_relationships")
	} else {
		t.Logf("Found %d relationships in engine_schema_relationships", len(schemaRels))
	}

	// Verify entity relationships were NOT created by these discovery steps
	// (Entity relationships are created later in the DAG by relationship enrichment)
	entityRels, err := entityRelRepo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Failed to get entity relationships: %v", err)
	}

	// Entity relationships should be empty since we haven't run entity discovery/enrichment
	if len(entityRels) > 0 {
		t.Errorf("Expected 0 entity relationships (those come from later DAG steps), got %d", len(entityRels))
		for _, rel := range entityRels {
			t.Logf("  Unexpected entity relationship: %s -> %s", rel.SourceColumnName, rel.TargetColumnName)
		}
	} else {
		t.Log("Correctly found 0 entity relationships (schema relationships are separate)")
	}
}

// ============================================================================
// Mock implementations
// ============================================================================

type dagTestMockDatasourceService struct {
	datasource *models.Datasource
}

func (m *dagTestMockDatasourceService) Get(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *dagTestMockDatasourceService) GetByID(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.Datasource, error) {
	return m.datasource, nil
}

func (m *dagTestMockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.DatasourceWithStatus, error) {
	return nil, nil
}

func (m *dagTestMockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType, provider string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *dagTestMockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType, provider string, config map[string]any) error {
	return nil
}

func (m *dagTestMockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *dagTestMockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any, datasourceID uuid.UUID) error {
	return nil
}

func (m *dagTestMockDatasourceService) SetDefault(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (m *dagTestMockDatasourceService) Delete(ctx context.Context, datasourceID uuid.UUID) error {
	return nil
}

type dagTestMockProjectService struct {
	ontologySettings *OntologySettings
}

func (m *dagTestMockProjectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, nil
}

func (m *dagTestMockProjectService) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, nil
}

func (m *dagTestMockProjectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *dagTestMockProjectService) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *dagTestMockProjectService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *dagTestMockProjectService) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (m *dagTestMockProjectService) SetDefaultDatasourceID(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (m *dagTestMockProjectService) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
}

func (m *dagTestMockProjectService) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", nil
}

func (m *dagTestMockProjectService) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return nil
}

func (m *dagTestMockProjectService) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error) {
	return nil, nil
}

func (m *dagTestMockProjectService) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error {
	return nil
}

func (m *dagTestMockProjectService) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error) {
	if m.ontologySettings != nil {
		return m.ontologySettings, nil
	}
	return &OntologySettings{UseLegacyPatternMatching: true}, nil
}

func (m *dagTestMockProjectService) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error {
	return nil
}

type dagTestMockAdapterFactory struct {
	schemaDiscoverer datasource.SchemaDiscoverer
}

func (m *dagTestMockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *dagTestMockAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return m.schemaDiscoverer, nil
}

func (m *dagTestMockAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, nil
}

func (m *dagTestMockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

// dagTestRealSchemaDiscoverer wraps a real database connection for join analysis.
type dagTestRealSchemaDiscoverer struct {
	pool *pgxpool.Pool
}

func (r *dagTestRealSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}

func (r *dagTestRealSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}

func (r *dagTestRealSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}

func (r *dagTestRealSchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

func (r *dagTestRealSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if len(columnNames) == 0 {
		return nil, nil
	}

	result := make([]datasource.ColumnStats, len(columnNames))
	for i, colName := range columnNames {
		var rowCount, distinctCount, nonNullCount int64

		query := fmt.Sprintf(`
			SELECT
				(SELECT COUNT(*) FROM %s.%s) as row_count,
				COUNT(DISTINCT %s) as distinct_count,
				COUNT(%s) as non_null_count
			FROM %s.%s
		`, schemaName, tableName, colName, colName, schemaName, tableName)

		err := r.pool.QueryRow(ctx, query).Scan(&rowCount, &distinctCount, &nonNullCount)
		if err != nil {
			result[i] = datasource.ColumnStats{ColumnName: colName}
			continue
		}

		result[i] = datasource.ColumnStats{
			ColumnName:    colName,
			RowCount:      rowCount,
			DistinctCount: distinctCount,
			NonNullCount:  nonNullCount,
		}
	}

	return result, nil
}

func (r *dagTestRealSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}

func (r *dagTestRealSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	// Full join analysis query with bidirectional validation
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

func (r *dagTestRealSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (r *dagTestRealSchemaDiscoverer) GetEnumValueDistribution(ctx context.Context, schemaName, tableName, columnName string, completionTimestampCol string, limit int) (*datasource.EnumDistributionResult, error) {
	return nil, nil
}

func (r *dagTestRealSchemaDiscoverer) Close() error {
	return nil
}
