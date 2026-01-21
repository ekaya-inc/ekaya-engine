//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// pendingChangeTestContext holds all dependencies for pending change repository integration tests.
type pendingChangeTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      PendingChangeRepository
	projectID uuid.UUID
}

// setupPendingChangeTest creates a test context with real database.
func setupPendingChangeTest(t *testing.T) *pendingChangeTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	repo := NewPendingChangeRepository()

	// Use fixed ID for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000010")

	tc := &pendingChangeTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      repo,
		projectID: projectID,
	}

	// Ensure project exists
	tc.ensureTestProject()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *pendingChangeTestContext) createTestContext() (context.Context, func()) {
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
func (tc *pendingChangeTestContext) ensureTestProject() {
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
	`, tc.projectID, "Pending Change Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// cleanup removes all pending changes for this project.
func (tc *pendingChangeTestContext) cleanup() {
	tc.t.Helper()
	ctx, done := tc.createTestContext()
	defer done()

	if err := tc.repo.DeleteByProject(ctx, tc.projectID); err != nil {
		tc.t.Logf("Warning: cleanup failed: %v", err)
	}
}

func TestPendingChangeRepository_CreateAndList(t *testing.T) {
	tc := setupPendingChangeTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, done := tc.createTestContext()
	defer done()

	// Create a pending change
	change := &models.PendingChange{
		ProjectID:       tc.projectID,
		ChangeType:      models.ChangeTypeNewTable,
		ChangeSource:    models.ChangeSourceSchemaRefresh,
		TableName:       "public.test_table",
		SuggestedAction: models.SuggestedActionCreateEntity,
		SuggestedPayload: map[string]any{
			"name":          "TestTable",
			"primary_table": "public.test_table",
		},
		Status: models.ChangeStatusPending,
	}

	err := tc.repo.Create(ctx, change)
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, change.ID)

	// List pending changes
	changes, err := tc.repo.List(ctx, tc.projectID, "pending", 50)
	require.NoError(t, err)
	require.Len(t, changes, 1)

	assert.Equal(t, change.ID, changes[0].ID)
	assert.Equal(t, models.ChangeTypeNewTable, changes[0].ChangeType)
	assert.Equal(t, "public.test_table", changes[0].TableName)
	assert.Equal(t, models.SuggestedActionCreateEntity, changes[0].SuggestedAction)
}

func TestPendingChangeRepository_CreateBatch(t *testing.T) {
	tc := setupPendingChangeTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, done := tc.createTestContext()
	defer done()

	// Create multiple changes
	changes := []*models.PendingChange{
		{
			ProjectID:       tc.projectID,
			ChangeType:      models.ChangeTypeNewTable,
			ChangeSource:    models.ChangeSourceSchemaRefresh,
			TableName:       "public.table1",
			SuggestedAction: models.SuggestedActionCreateEntity,
			Status:          models.ChangeStatusPending,
		},
		{
			ProjectID:    tc.projectID,
			ChangeType:   models.ChangeTypeNewColumn,
			ChangeSource: models.ChangeSourceSchemaRefresh,
			TableName:    "public.table1",
			ColumnName:   "new_col",
			NewValue:     map[string]any{"type": "varchar"},
			Status:       models.ChangeStatusPending,
		},
	}

	err := tc.repo.CreateBatch(ctx, changes)
	require.NoError(t, err)

	// Verify all changes got IDs
	for _, c := range changes {
		assert.NotEqual(t, uuid.Nil, c.ID)
	}

	// List and verify
	listed, err := tc.repo.List(ctx, tc.projectID, "pending", 50)
	require.NoError(t, err)
	assert.Len(t, listed, 2)
}

func TestPendingChangeRepository_ListByType(t *testing.T) {
	tc := setupPendingChangeTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, done := tc.createTestContext()
	defer done()

	// Create changes of different types
	changes := []*models.PendingChange{
		{
			ProjectID:    tc.projectID,
			ChangeType:   models.ChangeTypeNewTable,
			ChangeSource: models.ChangeSourceSchemaRefresh,
			TableName:    "public.new_table",
			Status:       models.ChangeStatusPending,
		},
		{
			ProjectID:    tc.projectID,
			ChangeType:   models.ChangeTypeNewColumn,
			ChangeSource: models.ChangeSourceSchemaRefresh,
			TableName:    "public.existing",
			ColumnName:   "new_col",
			Status:       models.ChangeStatusPending,
		},
		{
			ProjectID:    tc.projectID,
			ChangeType:   models.ChangeTypeNewColumn,
			ChangeSource: models.ChangeSourceSchemaRefresh,
			TableName:    "public.existing",
			ColumnName:   "another_col",
			Status:       models.ChangeStatusPending,
		},
	}

	err := tc.repo.CreateBatch(ctx, changes)
	require.NoError(t, err)

	// Filter by type
	newColumns, err := tc.repo.ListByType(ctx, tc.projectID, models.ChangeTypeNewColumn, 50)
	require.NoError(t, err)
	assert.Len(t, newColumns, 2)

	newTables, err := tc.repo.ListByType(ctx, tc.projectID, models.ChangeTypeNewTable, 50)
	require.NoError(t, err)
	assert.Len(t, newTables, 1)
}

func TestPendingChangeRepository_UpdateStatus(t *testing.T) {
	tc := setupPendingChangeTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, done := tc.createTestContext()
	defer done()

	// Create a change
	change := &models.PendingChange{
		ProjectID:    tc.projectID,
		ChangeType:   models.ChangeTypeNewTable,
		ChangeSource: models.ChangeSourceSchemaRefresh,
		TableName:    "public.test_table",
		Status:       models.ChangeStatusPending,
	}
	err := tc.repo.Create(ctx, change)
	require.NoError(t, err)

	// Update status
	err = tc.repo.UpdateStatus(ctx, change.ID, models.ChangeStatusApproved, "admin")
	require.NoError(t, err)

	// Verify update
	updated, err := tc.repo.GetByID(ctx, change.ID)
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, models.ChangeStatusApproved, updated.Status)
	assert.NotNil(t, updated.ReviewedBy)
	assert.Equal(t, "admin", *updated.ReviewedBy)
	assert.NotNil(t, updated.ReviewedAt)
}

func TestPendingChangeRepository_CountByStatus(t *testing.T) {
	tc := setupPendingChangeTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, done := tc.createTestContext()
	defer done()

	// Create changes with different statuses
	changes := []*models.PendingChange{
		{ProjectID: tc.projectID, ChangeType: models.ChangeTypeNewTable, ChangeSource: models.ChangeSourceSchemaRefresh, TableName: "t1", Status: models.ChangeStatusPending},
		{ProjectID: tc.projectID, ChangeType: models.ChangeTypeNewTable, ChangeSource: models.ChangeSourceSchemaRefresh, TableName: "t2", Status: models.ChangeStatusPending},
		{ProjectID: tc.projectID, ChangeType: models.ChangeTypeNewTable, ChangeSource: models.ChangeSourceSchemaRefresh, TableName: "t3", Status: models.ChangeStatusPending},
	}
	err := tc.repo.CreateBatch(ctx, changes)
	require.NoError(t, err)

	// Manually update one to approved status
	err = tc.repo.UpdateStatus(ctx, changes[2].ID, models.ChangeStatusApproved, "admin")
	require.NoError(t, err)

	// Get counts
	counts, err := tc.repo.CountByStatus(ctx, tc.projectID)
	require.NoError(t, err)
	assert.Equal(t, 2, counts[models.ChangeStatusPending])
	assert.Equal(t, 1, counts[models.ChangeStatusApproved])
}

func TestPendingChangeRepository_DeleteByProject(t *testing.T) {
	tc := setupPendingChangeTest(t)
	tc.cleanup()

	ctx, done := tc.createTestContext()
	defer done()

	// Create changes
	changes := []*models.PendingChange{
		{ProjectID: tc.projectID, ChangeType: models.ChangeTypeNewTable, ChangeSource: models.ChangeSourceSchemaRefresh, TableName: "t1", Status: models.ChangeStatusPending},
		{ProjectID: tc.projectID, ChangeType: models.ChangeTypeNewTable, ChangeSource: models.ChangeSourceSchemaRefresh, TableName: "t2", Status: models.ChangeStatusPending},
	}
	err := tc.repo.CreateBatch(ctx, changes)
	require.NoError(t, err)

	// Verify they exist
	listed, err := tc.repo.List(ctx, tc.projectID, "", 50)
	require.NoError(t, err)
	assert.Len(t, listed, 2)

	// Delete all
	err = tc.repo.DeleteByProject(ctx, tc.projectID)
	require.NoError(t, err)

	// Verify deletion
	listed, err = tc.repo.List(ctx, tc.projectID, "", 50)
	require.NoError(t, err)
	assert.Empty(t, listed)
}

func TestPendingChangeRepository_GetByID(t *testing.T) {
	tc := setupPendingChangeTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, done := tc.createTestContext()
	defer done()

	// Create a change
	change := &models.PendingChange{
		ProjectID:       tc.projectID,
		ChangeType:      models.ChangeTypeNewColumn,
		ChangeSource:    models.ChangeSourceSchemaRefresh,
		TableName:       "public.users",
		ColumnName:      "email",
		NewValue:        map[string]any{"type": "varchar"},
		SuggestedAction: models.SuggestedActionCreateColumnMetadata,
		Status:          models.ChangeStatusPending,
	}
	err := tc.repo.Create(ctx, change)
	require.NoError(t, err)

	// Get by ID
	fetched, err := tc.repo.GetByID(ctx, change.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)

	assert.Equal(t, change.ID, fetched.ID)
	assert.Equal(t, tc.projectID, fetched.ProjectID)
	assert.Equal(t, models.ChangeTypeNewColumn, fetched.ChangeType)
	assert.Equal(t, "public.users", fetched.TableName)
	assert.Equal(t, "email", fetched.ColumnName)
	assert.Equal(t, "varchar", fetched.NewValue["type"])
}

func TestPendingChangeRepository_GetByID_NotFound(t *testing.T) {
	tc := setupPendingChangeTest(t)

	ctx, done := tc.createTestContext()
	defer done()

	// Get non-existent ID
	fetched, err := tc.repo.GetByID(ctx, uuid.New())
	require.NoError(t, err)
	assert.Nil(t, fetched)
}
