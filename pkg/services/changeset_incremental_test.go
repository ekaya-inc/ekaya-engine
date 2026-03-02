package services

import (
	"context"
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

// ============================================================================
// Helper functions for incremental ontology tests
// ============================================================================

// cleanupOntologyMetadata removes all ontology metadata for the test project.
func cleanupOntologyMetadata(t *testing.T, tc *schemaServiceTestContext) {
	t.Helper()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		t.Fatal("Failed to get tenant scope for ontology cleanup")
	}

	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_column_metadata WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_table_metadata WHERE project_id = $1`, tc.projectID)
}

// createTestColumnMetadata creates column metadata via the extraction pipeline (source='inferred', no last_edit_source).
func createTestColumnMetadata(t *testing.T, tc *schemaServiceTestContext, ctx context.Context, schemaColumnID uuid.UUID, description string) {
	t.Helper()

	repo := repositories.NewColumnMetadataRepository()
	meta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: schemaColumnID,
		Description:    &description,
	}
	require.NoError(t, repo.UpsertFromExtraction(ctx, meta))
}

// createTestColumnMetadataWithEdit creates column metadata that has been edited by an MCP client.
// This sets last_edit_source so ComputeChangeSet will mark it for skipping.
func createTestColumnMetadataWithEdit(t *testing.T, tc *schemaServiceTestContext, ctx context.Context, schemaColumnID uuid.UUID, description string) {
	t.Helper()

	repo := repositories.NewColumnMetadataRepository()

	// First create via extraction (source='inferred')
	meta := &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: schemaColumnID,
		Description:    &description,
	}
	require.NoError(t, repo.UpsertFromExtraction(ctx, meta))

	// Then update via Upsert (source='mcp') to set last_edit_source
	meta.Source = "mcp"
	editedDesc := description + " (edited)"
	meta.Description = &editedDesc
	require.NoError(t, repo.Upsert(ctx, meta))
}

// createTestTableMetadata creates table metadata via the extraction pipeline.
func createTestTableMetadata(t *testing.T, tc *schemaServiceTestContext, ctx context.Context, schemaTableID uuid.UUID, description string) {
	t.Helper()

	repo := repositories.NewTableMetadataRepository()
	meta := &models.TableMetadata{
		ProjectID:     tc.projectID,
		SchemaTableID: schemaTableID,
		Description:   &description,
	}
	require.NoError(t, repo.UpsertFromExtraction(ctx, meta))
}

// softDeleteColumn sets deleted_at on a column to simulate schema refresh removing it.
func softDeleteColumn(t *testing.T, ctx context.Context, columnID uuid.UUID) {
	t.Helper()

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		t.Fatal("Failed to get tenant scope for soft delete")
	}

	_, err := scope.Conn.Exec(ctx, `UPDATE engine_schema_columns SET deleted_at = NOW() WHERE id = $1`, columnID)
	require.NoError(t, err)
}

// softDeleteTable sets deleted_at on a table to simulate schema refresh removing it.
func softDeleteTable(t *testing.T, ctx context.Context, tableID uuid.UUID) {
	t.Helper()

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		t.Fatal("Failed to get tenant scope for soft delete")
	}

	_, err := scope.Conn.Exec(ctx, `UPDATE engine_schema_tables SET deleted_at = NOW() WHERE id = $1`, tableID)
	require.NoError(t, err)
}

// newDAGService creates an OntologyDAGService with the minimum dependencies needed for changeset tests.
func newDAGService(questionRepo repositories.OntologyQuestionRepository) *ontologyDAGService {
	return NewOntologyDAGService(nil, repositories.NewSchemaRepository(), questionRepo, nil, nil, nil, nil, zap.NewNop())
}

// ============================================================================
// ComputeChangeSet: Added Table Detection (#24)
// ============================================================================

// TestComputeChangeSet_DetectsAddedTable verifies that a table created after the last
// extraction is correctly detected as an AddedTable in the ChangeSet.
func TestComputeChangeSet_DetectsAddedTable(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create initial table (exists before builtAt)
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, true)

	// 2. Record builtAt (simulates last completed extraction)
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	// 3. Add a new table after builtAt (simulates schema refresh finding a new table)
	productsTable := tc.createTestTable(ctx, "public", "products", true)
	tc.createTestColumn(ctx, productsTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, productsTable.ID, "name", "text", 2, true)

	// 4. Compute ChangeSet
	dagSvc := newDAGService(nil)
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)

	// Assert: new table and its columns detected
	assert.False(t, changeSet.IsEmpty(), "ChangeSet should not be empty after adding a table")
	assert.Len(t, changeSet.AddedTables, 1, "should detect exactly 1 added table")
	assert.Equal(t, "products", changeSet.AddedTables[0].TableName)
	assert.Len(t, changeSet.AddedColumns, 2, "should detect 2 added columns (id, name)")

	// Assert: existing table is not affected
	assert.Empty(t, changeSet.ModifiedTables, "existing table should not appear as modified")
	assert.True(t, changeSet.AffectedTableIDs[productsTable.ID], "new table should be in affected set")
	assert.False(t, changeSet.AffectedTableIDs[usersTable.ID], "existing table should not be in affected set")
}

// ============================================================================
// CleanupDeletedItems: Column Deletion (#26)
// ============================================================================

// TestCleanupDeletedItems_DeletedColumn verifies that deleting a column removes its
// ontology metadata and soft-deletes any relationships referencing it.
func TestCleanupDeletedItems_DeletedColumn(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()
	cleanupOntologyMetadata(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create tables and columns
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", true)
	ordersIDCol := tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	ordersUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, true)

	// 2. Create column metadata for both orders columns
	createTestColumnMetadata(t, tc, ctx, ordersIDCol.ID, "Order identifier")
	createTestColumnMetadata(t, tc, ctx, ordersUserIDCol.ID, "FK to users table")

	// 3. Create relationship: orders.user_id → users.id
	tc.createTestRelationship(ctx, ordersTable.ID, ordersUserIDCol.ID, usersTable.ID, usersIDCol.ID)

	// 4. Record builtAt, then soft-delete user_id column
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	softDeleteColumn(t, ctx, ordersUserIDCol.ID)

	// 5. Compute ChangeSet and verify column detected as deleted
	dagSvc := newDAGService(nil)
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)
	assert.Len(t, changeSet.DeletedColumns, 1, "should detect 1 deleted column")
	assert.Equal(t, "user_id", changeSet.DeletedColumns[0].ColumnName)

	// 6. Run cleanup
	err = dagSvc.CleanupDeletedItems(ctx, tc.projectID, changeSet)
	require.NoError(t, err)

	// 7. Verify column metadata for deleted column is gone
	colMetaRepo := repositories.NewColumnMetadataRepository()
	deletedMeta, _ := colMetaRepo.GetBySchemaColumnID(ctx, ordersUserIDCol.ID)
	assert.Nil(t, deletedMeta, "column metadata for deleted column should be removed")

	// 8. Verify column metadata for surviving column is preserved
	preservedMeta, err := colMetaRepo.GetBySchemaColumnID(ctx, ordersIDCol.ID)
	require.NoError(t, err)
	assert.NotNil(t, preservedMeta, "column metadata for surviving column should be preserved")

	// 9. Verify relationship is soft-deleted (GetRelationshipByColumns filters deleted_at IS NULL)
	schemaRepo := repositories.NewSchemaRepository()
	rel, err := schemaRepo.GetRelationshipByColumns(ctx, ordersUserIDCol.ID, usersIDCol.ID)
	assert.NoError(t, err)
	assert.Nil(t, rel, "relationship involving deleted column should be soft-deleted")
}

// ============================================================================
// CleanupDeletedItems: Table Deletion (#27)
// ============================================================================

// TestCleanupDeletedItems_DeletedTable verifies that deleting a table removes its
// table metadata, all its columns' metadata, and soft-deletes relationships.
func TestCleanupDeletedItems_DeletedTable(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()
	cleanupOntologyMetadata(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create two tables with columns
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	logsTable := tc.createTestTable(ctx, "public", "logs", true)
	logsIDCol := tc.createTestColumn(ctx, logsTable.ID, "id", "uuid", 1, true)
	logsUserIDCol := tc.createTestColumn(ctx, logsTable.ID, "user_id", "uuid", 2, true)

	// 2. Create ontology metadata for both tables
	createTestTableMetadata(t, tc, ctx, usersTable.ID, "Core user accounts")
	createTestTableMetadata(t, tc, ctx, logsTable.ID, "Audit logs")
	createTestColumnMetadata(t, tc, ctx, logsIDCol.ID, "Log entry ID")
	createTestColumnMetadata(t, tc, ctx, logsUserIDCol.ID, "FK to users")

	// 3. Create relationship: logs.user_id → users.id
	tc.createTestRelationship(ctx, logsTable.ID, logsUserIDCol.ID, usersTable.ID, usersIDCol.ID)

	// 4. Record builtAt, then soft-delete the logs table
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	softDeleteTable(t, ctx, logsTable.ID)

	// 5. Compute ChangeSet and verify table detected as deleted
	dagSvc := newDAGService(repositories.NewOntologyQuestionRepository())
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)
	assert.Len(t, changeSet.DeletedTables, 1, "should detect 1 deleted table")
	assert.Equal(t, "logs", changeSet.DeletedTables[0].TableName)

	// 6. Run cleanup
	err = dagSvc.CleanupDeletedItems(ctx, tc.projectID, changeSet)
	require.NoError(t, err)

	// 7. Verify table metadata for deleted table is gone
	tblMetaRepo := repositories.NewTableMetadataRepository()
	deletedTblMeta, _ := tblMetaRepo.GetBySchemaTableID(ctx, logsTable.ID)
	assert.Nil(t, deletedTblMeta, "table metadata for deleted table should be removed")

	// 8. Verify column metadata for deleted table's columns is gone
	colMetaRepo := repositories.NewColumnMetadataRepository()
	deletedColMeta1, _ := colMetaRepo.GetBySchemaColumnID(ctx, logsIDCol.ID)
	assert.Nil(t, deletedColMeta1, "column metadata for deleted table's column should be removed")
	deletedColMeta2, _ := colMetaRepo.GetBySchemaColumnID(ctx, logsUserIDCol.ID)
	assert.Nil(t, deletedColMeta2, "column metadata for deleted table's FK column should be removed")

	// 9. Verify relationship is soft-deleted
	schemaRepo := repositories.NewSchemaRepository()
	rel, err := schemaRepo.GetRelationshipByColumns(ctx, logsUserIDCol.ID, usersIDCol.ID)
	assert.NoError(t, err)
	assert.Nil(t, rel, "relationship involving deleted table should be soft-deleted")

	// 10. Verify surviving table's metadata is preserved
	survivingTblMeta, err := tblMetaRepo.GetBySchemaTableID(ctx, usersTable.ID)
	require.NoError(t, err)
	assert.NotNil(t, survivingTblMeta, "table metadata for surviving table should be preserved")
}

// ============================================================================
// ComputeChangeSet: User-Edited Column Skipping (#28)
// ============================================================================

// TestComputeChangeSet_UserEditedColumnsSkipped verifies that columns with
// last_edit_source set (edited by MCP or manual) are included in UserEditedIDs
// and ShouldSkipColumn returns true for them.
func TestComputeChangeSet_UserEditedColumnsSkipped(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()
	cleanupOntologyMetadata(t, tc)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// 1. Create table with columns
	table := tc.createTestTable(ctx, "public", "users", true)
	idCol := tc.createTestColumn(ctx, table.ID, "id", "uuid", 1, true)
	emailCol := tc.createTestColumn(ctx, table.ID, "email", "text", 2, true)
	statusCol := tc.createTestColumn(ctx, table.ID, "status", "text", 3, true)

	// 2. Create column metadata: email is user-edited (MCP), status is extraction-only
	createTestColumnMetadataWithEdit(t, tc, ctx, emailCol.ID, "User email address")
	createTestColumnMetadata(t, tc, ctx, statusCol.ID, "Account status")

	// 3. Record builtAt, then modify status column to trigger a change
	time.Sleep(10 * time.Millisecond)
	builtAt := time.Now()
	time.Sleep(10 * time.Millisecond)

	// Modify status column (data_type change triggers updated_at bump)
	modifiedStatus := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   table.ID,
		ColumnName:      "status",
		DataType:        "varchar", // changed from "text"
		IsNullable:      true,
		IsSelected:      true,
		OrdinalPosition: 3,
	}
	require.NoError(t, tc.repo.UpsertColumn(ctx, modifiedStatus))

	// 4. Compute ChangeSet
	dagSvc := newDAGService(nil)
	changeSet, err := dagSvc.ComputeChangeSet(ctx, tc.projectID, builtAt)
	require.NoError(t, err)

	// Assert: status column detected as modified
	assert.Len(t, changeSet.ModifiedColumns, 1, "should detect 1 modified column")
	assert.Equal(t, "status", changeSet.ModifiedColumns[0].ColumnName)

	// Assert: user-edited email column is in UserEditedIDs
	assert.True(t, changeSet.UserEditedIDs[emailCol.ID],
		"user-edited email column should be in UserEditedIDs")

	// Assert: non-edited columns are NOT in UserEditedIDs
	assert.False(t, changeSet.UserEditedIDs[idCol.ID],
		"id column (no metadata) should not be in UserEditedIDs")
	assert.False(t, changeSet.UserEditedIDs[statusCol.ID],
		"status column (extraction-only metadata) should not be in UserEditedIDs")

	// Assert: ShouldSkipColumn returns correct values
	assert.True(t, changeSet.ShouldSkipColumn(emailCol.ID),
		"ShouldSkipColumn should return true for user-edited column")
	assert.False(t, changeSet.ShouldSkipColumn(statusCol.ID),
		"ShouldSkipColumn should return false for non-edited column")
	assert.False(t, changeSet.ShouldSkipColumn(idCol.ID),
		"ShouldSkipColumn should return false for column without metadata")
}
