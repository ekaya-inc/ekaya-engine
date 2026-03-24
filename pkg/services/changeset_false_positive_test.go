package services

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// TestComputeChangeSet_NoFalsePositivesAfterRefresh verifies that re-upserting
// the same tables/columns (a no-op schema refresh) does NOT produce any modified
// items in the ChangeSet. This is the core regression test for the false-positive
// schema change detection bug.
func TestComputeChangeSet_NoFalsePositivesAfterRefresh(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create initial tables and columns
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", true)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, true)

	// 2. Simulate a completed ontology extraction by recording builtAt after all inserts
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	// 3. Simulate a schema refresh — re-upsert the exact same tables and columns.
	// This is what happens when a user clicks "Refresh Schema" and nothing changed.
	upsertTable := func(schemaName, tableName string, rowCount *int64) *models.SchemaTable {
		t.Helper()
		tbl := &models.SchemaTable{
			ProjectID:    tc.projectID,
			DatasourceID: tc.dsID,
			SchemaName:   schemaName,
			TableName:    tableName,
			IsSelected:   true,
			RowCount:     rowCount,
		}
		require.NoError(t, tc.repo.UpsertTable(ctx, tbl))
		return tbl
	}

	upsertCol := func(tableID uuid.UUID, name, dataType string, ordinal int) {
		t.Helper()
		c := &models.SchemaColumn{
			ProjectID:       tc.projectID,
			SchemaTableID:   tableID,
			ColumnName:      name,
			DataType:        dataType,
			IsNullable:      true,
			IsPrimaryKey:    name == "id",
			IsSelected:      true,
			OrdinalPosition: ordinal,
		}
		require.NoError(t, tc.repo.UpsertColumn(ctx, c))
	}

	usersRefresh := upsertTable("public", "users", usersTable.RowCount)
	ordersRefresh := upsertTable("public", "orders", ordersTable.RowCount)

	upsertCol(usersRefresh.ID, "id", "uuid", 1)
	upsertCol(usersRefresh.ID, "email", "text", 2)
	upsertCol(ordersRefresh.ID, "id", "uuid", 1)
	upsertCol(ordersRefresh.ID, "user_id", "uuid", 2)

	// 4. Compute ChangeSet — should be empty since nothing ontology-relevant changed
	dagSvc := NewOntologyDAGService(nil, repositories.NewSchemaRepository(), nil, nil, nil, nil, nil, zap.NewNop())
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)

	assert.Empty(t, changeSet.AddedTables, "no new tables should be detected")
	assert.Empty(t, changeSet.ModifiedTables, "no modified tables should be detected")
	assert.Empty(t, changeSet.DeletedTables, "no deleted tables should be detected")
	assert.Empty(t, changeSet.AddedColumns, "no new columns should be detected")
	assert.Empty(t, changeSet.ModifiedColumns, "no modified columns should be detected after no-op refresh")
	assert.Empty(t, changeSet.DeletedColumns, "no deleted columns should be detected")
	assert.True(t, changeSet.IsEmpty(), "ChangeSet should be empty after no-op schema refresh")
}

// TestComputeChangeSet_DetectsRealChange verifies that an actual schema change
// (e.g., data_type change) IS properly detected by ComputeChangeSet.
func TestComputeChangeSet_DetectsRealChange(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create initial table and column
	table := tc.createTestTable(ctx, "public", "products", true)
	tc.createTestColumn(ctx, table.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, table.ID, "price", "integer", 2, true)

	// 2. Simulate completed ontology extraction
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	// 3. Change the price column's data_type (a real schema change)
	priceCol := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   table.ID,
		ColumnName:      "price",
		DataType:        "numeric", // changed from "integer"
		IsNullable:      true,
		IsPrimaryKey:    false,
		IsSelected:      true,
		OrdinalPosition: 2,
	}
	require.NoError(t, tc.repo.UpsertColumn(ctx, priceCol))

	// 4. Compute ChangeSet — should detect the modified column
	dagSvc := NewOntologyDAGService(nil, repositories.NewSchemaRepository(), nil, nil, nil, nil, nil, zap.NewNop())
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)

	assert.False(t, changeSet.IsEmpty(), "ChangeSet should NOT be empty after data_type change")
	assert.Len(t, changeSet.ModifiedColumns, 1, "should detect exactly 1 modified column")
	assert.Equal(t, "price", changeSet.ModifiedColumns[0].ColumnName)
	assert.Equal(t, "numeric", changeSet.ModifiedColumns[0].DataType)
}

// TestComputeChangeSet_DetectsDeselectedTable verifies that deselecting a table
// in the Schema Selection screen is detected as a deletion by ComputeChangeSet.
func TestComputeChangeSet_DetectsDeselectedTable(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create selected tables
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", true)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)

	// 2. Simulate completed ontology extraction
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	// 3. Deselect the orders table (user action from Schema Selection screen)
	require.NoError(t, tc.repo.UpdateTableSelection(ctx, tc.projectID, ordersTable.ID, false))

	// 4. Compute ChangeSet — should detect the deselected table as deleted
	dagSvc := NewOntologyDAGService(nil, repositories.NewSchemaRepository(), nil, nil, nil, nil, nil, zap.NewNop())
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)

	assert.False(t, changeSet.IsEmpty(), "ChangeSet should NOT be empty after deselecting a table")
	assert.Len(t, changeSet.DeletedTables, 1, "deselected table should appear in DeletedTables")
	assert.Equal(t, "orders", changeSet.DeletedTables[0].TableName)
}

// TestComputeChangeSet_DetectsDeselectedColumn verifies that deselecting a column
// in the Schema Selection screen is detected as a deletion by ComputeChangeSet.
func TestComputeChangeSet_DetectsDeselectedColumn(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create table with selected columns
	table := tc.createTestTable(ctx, "public", "users", true)
	tc.createTestColumn(ctx, table.ID, "id", "uuid", 1, true)
	emailCol := tc.createTestColumn(ctx, table.ID, "email", "text", 2, true)

	// 2. Simulate completed ontology extraction
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	// 3. Deselect the email column (user action from Schema Selection screen)
	require.NoError(t, tc.repo.UpdateColumnSelection(ctx, tc.projectID, emailCol.ID, false))

	// 4. Compute ChangeSet — should detect the deselected column as deleted
	dagSvc := NewOntologyDAGService(nil, repositories.NewSchemaRepository(), nil, nil, nil, nil, nil, zap.NewNop())
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)

	assert.False(t, changeSet.IsEmpty(), "ChangeSet should NOT be empty after deselecting a column")
	assert.Len(t, changeSet.DeletedColumns, 1, "deselected column should appear in DeletedColumns")
	assert.Equal(t, "email", changeSet.DeletedColumns[0].ColumnName)
}

// TestGetOntologyStatus_NoFalsePositivesImmediatelyAfterImport verifies that an imported
// ontology does not immediately report its own selected schema rows as modified.
func TestGetOntologyStatus_NoFalsePositivesImmediatelyAfterImport(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	usersTable := tc.createTestTable(ctx, "public", "users", true)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, true)

	time.Sleep(10 * time.Millisecond)
	importedAt := time.Now().UTC()
	require.NotZero(t, importedAt.Nanosecond(), "test requires sub-second precision")

	scope, ok := database.GetTenantScope(ctx)
	require.True(t, ok, "tenant scope should be present")

	_, err := scope.Conn.Exec(ctx, `
		UPDATE engine_schema_tables
		SET updated_at = $2
		WHERE project_id = $1
	`, tc.projectID, importedAt)
	require.NoError(t, err)

	_, err = scope.Conn.Exec(ctx, `
		UPDATE engine_schema_columns
		SET updated_at = $2
		WHERE project_id = $1
	`, tc.projectID, importedAt)
	require.NoError(t, err)

	require.NoError(t, storeOntologyCompletionState(
		ctx,
		scope.Conn,
		tc.projectID,
		tc.dsID,
		models.OntologyCompletionProvenanceImported,
		importedAt,
	))

	dagSvc := NewOntologyDAGService(nil, repositories.NewSchemaRepository(), nil, nil, nil, nil, nil, zap.NewNop())
	status, err := dagSvc.GetOntologyStatus(ctx, tc.projectID, tc.dsID)
	require.NoError(t, err)

	require.True(t, status.HasOntology)
	require.Equal(t, models.OntologyCompletionProvenanceImported, status.CompletionProvenance)
	require.NotNil(t, status.LastBuiltAt)
	require.True(t, status.LastBuiltAt.Equal(importedAt), "stored completion time should preserve import precision")
	assert.False(t, status.SchemaChangedSinceBuild, "import should not report its own schema selection updates as changes")
	require.NotNil(t, status.ChangeSummary)
	assert.Equal(t, 0, status.ChangeSummary.TablesModified)
	assert.Equal(t, 0, status.ChangeSummary.ColumnsModified)
}
