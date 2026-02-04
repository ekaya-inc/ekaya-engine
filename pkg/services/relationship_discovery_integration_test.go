//go:build ignore

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

	service := NewSchemaService(repo, nil, nil, nil, nil, nil, logger)

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
	distinctCount := int64(950)
	isJoinable := true
	reason := models.JoinabilityUniqueValues

	err := tc.repo.UpdateColumnJoinability(ctx, column.ID, &rowCount, &nonNullCount, &distinctCount, &isJoinable, &reason)
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

	// Remove relationship (soft-delete)
	err = tc.service.RemoveRelationship(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("RemoveRelationship failed: %v", err)
	}

	// Verify relationship is soft-deleted and no longer visible
	_, err = tc.repo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
	if err == nil {
		t.Error("expected relationship to be not found after soft-delete")
	}

	// Verify the relationship still exists in the database but is soft-deleted
	response, err = tc.service.GetRelationshipsResponse(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetRelationshipsResponse failed: %v", err)
	}
	if len(response.Relationships) != 0 {
		t.Errorf("expected 0 visible relationships after removal, got %d", len(response.Relationships))
	}
}

// ============================================================================
// Review Candidate Integration Tests
// ============================================================================

// createTestColumnWithStats creates a test column with stats for cardinality checks.
func (tc *discoveryTestContext) createTestColumnWithStats(ctx context.Context, tableID uuid.UUID, columnName, dataType string, ordinal int, isPrimaryKey bool, distinctCount int64) *models.SchemaColumn {
	tc.t.Helper()

	column := tc.createTestColumn(ctx, tableID, columnName, dataType, ordinal, isPrimaryKey)

	// Update distinct count for cardinality checks
	// UpdateColumnStats signature: (ctx, columnID, distinctCount, nullCount, minLength, maxLength)
	if err := tc.repo.UpdateColumnStats(ctx, column.ID, &distinctCount, nil, nil, nil); err != nil {
		tc.t.Fatalf("Failed to update column stats: %v", err)
	}

	// Re-fetch column to get updated stats
	updated, err := tc.repo.GetColumnByID(ctx, tc.projectID, column.ID)
	if err != nil {
		tc.t.Fatalf("Failed to get updated column: %v", err)
	}

	return updated
}

func TestSchemaRepository_GetNonPKColumnsByExactType_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create table with various columns
	usersTable := tc.createTestTable(ctx, "public", "users", 100)
	tc.createTestColumn(ctx, usersTable.ID, "id", "bigint", 1, true)             // PK bigint - should be excluded
	tc.createTestColumn(ctx, usersTable.ID, "department_id", "bigint", 2, false) // Non-PK bigint - should be included
	tc.createTestColumn(ctx, usersTable.ID, "region_id", "integer", 3, false)    // Non-PK integer - different type
	tc.createTestColumn(ctx, usersTable.ID, "external_id", "bigint", 4, false)   // Non-PK bigint - should be included

	ordersTable := tc.createTestTable(ctx, "public", "orders", 500)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "bigint", 1, true)          // PK bigint - should be excluded
	tc.createTestColumn(ctx, ordersTable.ID, "order_type", "bigint", 2, false) // Non-PK bigint - should be included

	// Get non-PK columns by exact type "bigint"
	columns, err := tc.repo.GetNonPKColumnsByExactType(ctx, tc.projectID, tc.dsID, "bigint")
	if err != nil {
		t.Fatalf("GetNonPKColumnsByExactType failed: %v", err)
	}

	// Should find 3: department_id, external_id, order_type (all non-PK bigint)
	if len(columns) != 3 {
		t.Errorf("expected 3 non-PK bigint columns, got %d", len(columns))
		for _, col := range columns {
			t.Logf("  found: %s.%s (is_pk=%v, type=%s)", col.SchemaTableID, col.ColumnName, col.IsPrimaryKey, col.DataType)
		}
	}

	// Verify none are primary keys
	for _, col := range columns {
		if col.IsPrimaryKey {
			t.Errorf("column %s should not be a primary key", col.ColumnName)
		}
	}
}

func TestIsHighCardinality_Unit(t *testing.T) {
	// Unit test for isHighCardinality helper
	svc := &relationshipDiscoveryService{}

	tests := []struct {
		name          string
		distinctCount int64
		rowCount      int64
		want          bool
	}{
		{
			name:          "too few distinct values",
			distinctCount: 50,
			rowCount:      1000,
			want:          false, // Below ReviewMinDistinctCount (100)
		},
		{
			name:          "low cardinality ratio",
			distinctCount: 200,
			rowCount:      10000,
			want:          false, // 2% < 10% threshold
		},
		{
			name:          "high cardinality - good join key",
			distinctCount: 500,
			rowCount:      1000,
			want:          true, // 50% > 10% threshold, > 100 distinct
		},
		{
			name:          "very high cardinality",
			distinctCount: 900,
			rowCount:      1000,
			want:          true, // 90% > 10% threshold
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				DistinctCount: &tt.distinctCount,
			}
			table := &models.SchemaTable{
				RowCount: &tt.rowCount,
			}

			got := svc.isHighCardinality(col, table)
			if got != tt.want {
				t.Errorf("isHighCardinality() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRelationshipTypeReview_Constant(t *testing.T) {
	// Verify the constant exists and is valid
	if models.RelationshipTypeReview != "review" {
		t.Errorf("expected RelationshipTypeReview = 'review', got %q", models.RelationshipTypeReview)
	}

	if !models.IsValidRelationshipType(models.RelationshipTypeReview) {
		t.Error("RelationshipTypeReview should be a valid relationship type")
	}
}

func TestSchemaService_ReviewRelationship_StoredCorrectly_Integration(t *testing.T) {
	tc := setupDiscoveryTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create orphan table (PK holder - will be the TARGET in relationship)
	orphanTable := tc.createTestTable(ctx, "public", "orphan_table", 1000)
	orphanIDCol := tc.createTestColumn(ctx, orphanTable.ID, "id", "bigint", 1, true)

	// Create FK table (FK holder - will be the SOURCE in relationship)
	fkTable := tc.createTestTable(ctx, "public", "fk_table", 500)
	tc.createTestColumn(ctx, fkTable.ID, "id", "bigint", 1, true)
	tc.createTestColumnWithStats(ctx, fkTable.ID, "orphan_ref_id", "bigint", 2, false, 300)

	// Create a review relationship following FK convention: source=FK holder, target=PK holder
	// This simulates what findReviewCandidates creates: fk_table.orphan_ref_id -> orphan_table.id
	inferenceMethod := models.InferenceMethodValueOverlap
	fkCol, _ := tc.repo.GetColumnByName(ctx, fkTable.ID, "orphan_ref_id")

	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    fkTable.ID,     // FK holder (source)
		SourceColumnID:   fkCol.ID,       // FK column
		TargetTableID:    orphanTable.ID, // PK holder (target)
		TargetColumnID:   orphanIDCol.ID, // PK column
		RelationshipType: models.RelationshipTypeReview,
		Cardinality:      models.CardinalityUnknown,
		Confidence:       1.0,
		InferenceMethod:  &inferenceMethod,
		IsValidated:      false,
		IsApproved:       nil, // Pending review
	}

	metrics := &models.DiscoveryMetrics{
		MatchRate:      1.0,
		SourceDistinct: 300,
		TargetDistinct: 1000,
		MatchedCount:   300,
	}

	err := tc.repo.UpsertRelationshipWithMetrics(ctx, rel, metrics)
	if err != nil {
		t.Fatalf("Failed to create review relationship: %v", err)
	}

	// Retrieve and verify
	retrieved, err := tc.repo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("GetRelationshipByID failed: %v", err)
	}

	if retrieved.RelationshipType != models.RelationshipTypeReview {
		t.Errorf("expected type 'review', got %q", retrieved.RelationshipType)
	}
	if retrieved.IsApproved != nil {
		t.Errorf("expected IsApproved to be nil (pending), got %v", *retrieved.IsApproved)
	}
	if retrieved.IsValidated != false {
		t.Error("expected IsValidated to be false")
	}

	// Verify correct direction: FK table is source, orphan table is target
	if retrieved.SourceTableID != fkTable.ID {
		t.Errorf("expected SourceTableID to be FK table %s, got %s", fkTable.ID, retrieved.SourceTableID)
	}
	if retrieved.TargetTableID != orphanTable.ID {
		t.Errorf("expected TargetTableID to be orphan table %s, got %s", orphanTable.ID, retrieved.TargetTableID)
	}

	// Verify it appears in relationships response
	response, err := tc.service.GetRelationshipsResponse(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetRelationshipsResponse failed: %v", err)
	}

	found := false
	for _, rd := range response.Relationships {
		if rd.RelationshipType == models.RelationshipTypeReview {
			found = true
			// Verify direction in response: source should be FK table
			if rd.SourceTableName != "fk_table" {
				t.Errorf("expected source table 'fk_table', got %q", rd.SourceTableName)
			}
			if rd.TargetTableName != "orphan_table" {
				t.Errorf("expected target table 'orphan_table', got %q", rd.TargetTableName)
			}
			break
		}
	}
	if !found {
		t.Error("review relationship should appear in relationships response")
	}
}
