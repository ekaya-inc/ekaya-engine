//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// discoveryTestContext holds all dependencies for relationship discovery integration tests.
type discoveryTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	service   SchemaService
	repo      repositories.SchemaRepository
	projectID uuid.UUID
	dsID      uuid.UUID
}

// setupDiscoveryTest creates a test context with real database.
func setupDiscoveryTest(t *testing.T) *discoveryTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	repo := repositories.NewSchemaRepository()
	logger := zap.NewNop()

	service := NewSchemaService(repo, nil, nil, logger)

	// Use fixed IDs for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000201")
	dsID := uuid.MustParse("00000000-0000-0000-0000-000000000202")

	tc := &discoveryTestContext{
		t:         t,
		engineDB:  engineDB,
		service:   service,
		repo:      repo,
		projectID: projectID,
		dsID:      dsID,
	}

	tc.ensureTestProject()
	tc.ensureTestDatasource()

	return tc
}

// createTestContext creates a context with tenant scope.
func (tc *discoveryTestContext) createTestContext() (context.Context, func()) {
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

// ensureTestProject creates the test project if it doesn't exist.
func (tc *discoveryTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Relationship Discovery Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates the test datasource if it doesn't exist.
func (tc *discoveryTestContext) ensureTestDatasource() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.dsID, tc.projectID, "Discovery Test Datasource", "postgres", "{}")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all schema data for the test datasource.
func (tc *discoveryTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_relationships WHERE project_id = $1`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup relationships: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_columns WHERE project_id = $1`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup columns: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_tables WHERE project_id = $1`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup tables: %v", err)
	}
}

// createTestTable creates a test table and returns it.
func (tc *discoveryTestContext) createTestTable(ctx context.Context, schemaName, tableName string, rowCount int64) *models.SchemaTable {
	tc.t.Helper()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   schemaName,
		TableName:    tableName,
		RowCount:     &rowCount,
	}

	if err := tc.repo.UpsertTable(ctx, table); err != nil {
		tc.t.Fatalf("Failed to create test table: %v", err)
	}

	return table
}

// createTestColumn creates a test column and returns it.
func (tc *discoveryTestContext) createTestColumn(ctx context.Context, tableID uuid.UUID, columnName, dataType string, ordinal int, isPrimaryKey bool) *models.SchemaColumn {
	tc.t.Helper()

	column := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   tableID,
		ColumnName:      columnName,
		DataType:        dataType,
		IsNullable:      true,
		IsPrimaryKey:    isPrimaryKey,
		OrdinalPosition: ordinal,
	}

	if err := tc.repo.UpsertColumn(ctx, column); err != nil {
		tc.t.Fatalf("Failed to create test column: %v", err)
	}

	return column
}

// ============================================================================
// GetRelationshipsResponse Integration Tests
// ============================================================================

func TestSchemaService_GetRelationshipsResponse_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables
	usersTable := tc.createTestTable(ctx, "public", "users", 100)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "varchar", 2, false)

	ordersTable := tc.createTestTable(ctx, "public", "orders", 500)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	orderUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	// Create empty table
	tc.createTestTable(ctx, "public", "audit_logs", 0)

	// Create relationship
	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    ordersTable.ID,
		SourceColumnID:   orderUserIDCol.ID,
		TargetTableID:    usersTable.ID,
		TargetColumnID:   usersIDCol.ID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
	}
	if err := tc.repo.UpsertRelationship(ctx, rel); err != nil {
		t.Fatalf("Failed to create relationship: %v", err)
	}

	// Get relationships response
	response, err := tc.service.GetRelationshipsResponse(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetRelationshipsResponse failed: %v", err)
	}

	// Verify response structure
	if response.TotalCount != 1 {
		t.Errorf("expected TotalCount 1, got %d", response.TotalCount)
	}

	if len(response.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(response.Relationships))
	}

	// Verify relationship details include column types
	relDetail := response.Relationships[0]
	if relDetail.SourceColumnType != "uuid" {
		t.Errorf("expected source column type 'uuid', got %q", relDetail.SourceColumnType)
	}
	if relDetail.TargetColumnType != "uuid" {
		t.Errorf("expected target column type 'uuid', got %q", relDetail.TargetColumnType)
	}
	if relDetail.SourceTableName != "orders" {
		t.Errorf("expected source table 'orders', got %q", relDetail.SourceTableName)
	}
	if relDetail.TargetTableName != "users" {
		t.Errorf("expected target table 'users', got %q", relDetail.TargetTableName)
	}

	// Verify empty tables list
	if len(response.EmptyTables) != 1 {
		t.Errorf("expected 1 empty table, got %d", len(response.EmptyTables))
	}
	if len(response.EmptyTables) > 0 && response.EmptyTables[0] != "audit_logs" {
		t.Errorf("expected empty table 'audit_logs', got %q", response.EmptyTables[0])
	}
}

func TestSchemaService_GetRelationshipsResponse_OrphanTables_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables with data
	usersTable := tc.createTestTable(ctx, "public", "users", 100)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", 500)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	orderUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	// Orphan table (has data but no relationships)
	orphanTable := tc.createTestTable(ctx, "public", "settings", 10)
	tc.createTestColumn(ctx, orphanTable.ID, "key", "varchar", 1, true)

	// Create relationship between users and orders
	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    ordersTable.ID,
		SourceColumnID:   orderUserIDCol.ID,
		TargetTableID:    usersTable.ID,
		TargetColumnID:   usersIDCol.ID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
	}
	if err := tc.repo.UpsertRelationship(ctx, rel); err != nil {
		t.Fatalf("Failed to create relationship: %v", err)
	}

	// Get relationships response
	response, err := tc.service.GetRelationshipsResponse(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetRelationshipsResponse failed: %v", err)
	}

	// Verify orphan tables (settings has data but no relationships)
	if len(response.OrphanTables) != 1 {
		t.Errorf("expected 1 orphan table, got %d", len(response.OrphanTables))
	}
	if len(response.OrphanTables) > 0 && response.OrphanTables[0] != "settings" {
		t.Errorf("expected orphan table 'settings', got %q", response.OrphanTables[0])
	}
}

// ============================================================================
// GetRelationshipCandidates Integration Tests
// ============================================================================

func TestSchemaService_GetRelationshipCandidates_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables
	usersTable := tc.createTestTable(ctx, "public", "users", 100)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", 500)
	orderUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	// Create a verified relationship
	approvedTrue := true
	rel1 := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    ordersTable.ID,
		SourceColumnID:   orderUserIDCol.ID,
		TargetTableID:    usersTable.ID,
		TargetColumnID:   usersIDCol.ID,
		RelationshipType: models.RelationshipTypeInferred,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       0.95,
		IsApproved:       &approvedTrue,
	}
	if err := tc.repo.UpsertRelationship(ctx, rel1); err != nil {
		t.Fatalf("Failed to create verified relationship: %v", err)
	}

	// Create a rejected candidate
	rejectionReason := models.RejectionLowMatchRate
	rel2 := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    ordersTable.ID,
		SourceColumnID:   orderUserIDCol.ID,
		TargetTableID:    usersTable.ID,
		TargetColumnID:   usersIDCol.ID,
		RelationshipType: models.RelationshipTypeInferred,
		Cardinality:      models.CardinalityUnknown,
		Confidence:       0.3,
		RejectionReason:  &rejectionReason,
	}
	// Use different column to avoid duplicate
	ordersIDCol := tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	rel2.SourceColumnID = ordersIDCol.ID
	if err := tc.repo.UpsertRelationship(ctx, rel2); err != nil {
		t.Fatalf("Failed to create rejected relationship: %v", err)
	}

	// Get candidates
	response, err := tc.service.GetRelationshipCandidates(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetRelationshipCandidates failed: %v", err)
	}

	// Verify summary
	if response.Summary.Total != 2 {
		t.Errorf("expected total 2, got %d", response.Summary.Total)
	}
	if response.Summary.Verified != 1 {
		t.Errorf("expected verified 1, got %d", response.Summary.Verified)
	}
	if response.Summary.Rejected != 1 {
		t.Errorf("expected rejected 1, got %d", response.Summary.Rejected)
	}
}

// ============================================================================
// Repository Discovery Methods Integration Tests
// ============================================================================

func TestSchemaRepository_UpdateColumnJoinability_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create table and column
	table := tc.createTestTable(ctx, "public", "products", 1000)
	column := tc.createTestColumn(ctx, table.ID, "sku", "varchar", 1, false)

	// Update joinability
	rowCount := int64(1000)
	nonNullCount := int64(980)
	isJoinable := true
	reason := models.JoinabilityUniqueValues

	err := tc.repo.UpdateColumnJoinability(ctx, column.ID, &rowCount, &nonNullCount, &isJoinable, &reason)
	if err != nil {
		t.Fatalf("UpdateColumnJoinability failed: %v", err)
	}

	// Verify via GetJoinableColumns
	joinable, err := tc.repo.GetJoinableColumns(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetJoinableColumns failed: %v", err)
	}

	if len(joinable) != 1 {
		t.Errorf("expected 1 joinable column, got %d", len(joinable))
	}

	if len(joinable) > 0 {
		col := joinable[0]
		if col.ColumnName != "sku" {
			t.Errorf("expected column 'sku', got %q", col.ColumnName)
		}
		if col.IsJoinable == nil || !*col.IsJoinable {
			t.Error("expected IsJoinable to be true")
		}
		if col.JoinabilityReason == nil || *col.JoinabilityReason != reason {
			t.Errorf("expected JoinabilityReason %q, got %v", reason, col.JoinabilityReason)
		}
	}
}

func TestSchemaRepository_UpsertRelationshipWithMetrics_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables and columns
	usersTable := tc.createTestTable(ctx, "public", "users", 100)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", 500)
	orderUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	// Create relationship with metrics
	inferenceMethod := models.InferenceMethodValueOverlap
	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    ordersTable.ID,
		SourceColumnID:   orderUserIDCol.ID,
		TargetTableID:    usersTable.ID,
		TargetColumnID:   usersIDCol.ID,
		RelationshipType: models.RelationshipTypeInferred,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       0.85,
		InferenceMethod:  &inferenceMethod,
		IsValidated:      true,
	}

	metrics := &models.DiscoveryMetrics{
		MatchRate:      0.85,
		SourceDistinct: 500,
		TargetDistinct: 100,
		MatchedCount:   425,
	}

	err := tc.repo.UpsertRelationshipWithMetrics(ctx, rel, metrics)
	if err != nil {
		t.Fatalf("UpsertRelationshipWithMetrics failed: %v", err)
	}

	// Verify relationship was created
	retrieved, err := tc.repo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("GetRelationshipByID failed: %v", err)
	}

	if retrieved.Confidence != 0.85 {
		t.Errorf("expected Confidence 0.85, got %f", retrieved.Confidence)
	}
	if retrieved.MatchRate == nil || *retrieved.MatchRate != 0.85 {
		t.Errorf("expected MatchRate 0.85, got %v", retrieved.MatchRate)
	}
	if retrieved.SourceDistinct == nil || *retrieved.SourceDistinct != 500 {
		t.Errorf("expected SourceDistinct 500, got %v", retrieved.SourceDistinct)
	}
}

func TestSchemaRepository_GetPrimaryKeyColumns_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables with PK columns
	usersTable := tc.createTestTable(ctx, "public", "users", 100)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "varchar", 2, false)

	ordersTable := tc.createTestTable(ctx, "public", "orders", 500)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	// Get PK columns
	pkColumns, err := tc.repo.GetPrimaryKeyColumns(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetPrimaryKeyColumns failed: %v", err)
	}

	if len(pkColumns) != 2 {
		t.Errorf("expected 2 PK columns, got %d", len(pkColumns))
	}

	// Verify all are PKs
	for _, col := range pkColumns {
		if !col.IsPrimaryKey {
			t.Errorf("expected column %s to be PK", col.ColumnName)
		}
	}
}

// ============================================================================
// Manual Relationship CRUD Integration Tests
// ============================================================================

func TestSchemaService_ManualRelationship_CRUD_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables
	usersTable := tc.createTestTable(ctx, "public", "users", 100)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", 500)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	// Create manual relationship (API accepts just table names now)
	req := &models.AddRelationshipRequest{
		SourceTableName:  "orders",
		SourceColumnName: "user_id",
		TargetTableName:  "users",
		TargetColumnName: "id",
	}

	rel, err := tc.service.AddManualRelationship(ctx, tc.projectID, tc.dsID, req)
	if err != nil {
		t.Fatalf("AddManualRelationship failed: %v", err)
	}

	if rel.RelationshipType != models.RelationshipTypeManual {
		t.Errorf("expected type 'manual', got %q", rel.RelationshipType)
	}
	if rel.IsApproved == nil || !*rel.IsApproved {
		t.Error("expected IsApproved to be true for manual relationship")
	}

	// Verify via GetRelationshipsResponse
	response, err := tc.service.GetRelationshipsResponse(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetRelationshipsResponse failed: %v", err)
	}

	if len(response.Relationships) != 1 {
		t.Fatalf("expected 1 relationship, got %d", len(response.Relationships))
	}

	// Remove relationship
	err = tc.service.RemoveRelationship(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("RemoveRelationship failed: %v", err)
	}

	// Verify relationship is marked as not approved (still exists but disapproved)
	retrieved, err := tc.repo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("GetRelationshipByID failed: %v", err)
	}

	if retrieved.IsApproved == nil || *retrieved.IsApproved {
		t.Error("expected IsApproved to be false after removal")
	}
}
