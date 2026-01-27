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

// auditTestContext holds test dependencies for audit repository tests.
type auditTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       AuditRepository
	projectID  uuid.UUID
	testUserID uuid.UUID
}

// setupAuditTest initializes the test context with shared testcontainer.
func setupAuditTest(t *testing.T) *auditTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &auditTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewAuditRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000070"),
		testUserID: uuid.MustParse("00000000-0000-0000-0000-000000000071"),
	}
	tc.ensureTestProject()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *auditTestContext) ensureTestProject() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Audit Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Create test user for audit log entries
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_users (project_id, user_id, role)
		VALUES ($1, $2, 'admin')
		ON CONFLICT (project_id, user_id) DO NOTHING
	`, tc.projectID, tc.testUserID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test user: %v", err)
	}
}

// cleanup removes test audit log entries.
func (tc *auditTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_audit_log WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *auditTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

func TestAuditRepository_Create(t *testing.T) {
	tc := setupAuditTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entityID := uuid.New()
	entry := &models.AuditLogEntry{
		ProjectID:  tc.projectID,
		EntityType: models.AuditEntityTypeEntity,
		EntityID:   entityID,
		Action:     models.AuditActionCreate,
		Source:     models.ProvenanceManual,
		UserID:     &tc.testUserID,
	}

	err := tc.repo.Create(ctx, entry)
	require.NoError(t, err)

	// Verify the entry was created with an ID
	assert.NotEqual(t, uuid.Nil, entry.ID)
	assert.False(t, entry.CreatedAt.IsZero())
}

func TestAuditRepository_CreateWithChangedFields(t *testing.T) {
	tc := setupAuditTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entityID := uuid.New()
	changes := map[string]models.FieldChange{
		"description": {
			Old: "old description",
			New: "new description",
		},
		"name": {
			Old: "old name",
			New: "new name",
		},
	}

	entry := &models.AuditLogEntry{
		ProjectID:     tc.projectID,
		EntityType:    models.AuditEntityTypeEntity,
		EntityID:      entityID,
		Action:        models.AuditActionUpdate,
		Source:        models.ProvenanceMCP,
		UserID:        &tc.testUserID,
		ChangedFields: changes,
	}

	err := tc.repo.Create(ctx, entry)
	require.NoError(t, err)

	// Retrieve and verify
	entries, err := tc.repo.GetByEntity(ctx, models.AuditEntityTypeEntity, entityID)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	retrieved := entries[0]
	assert.Equal(t, entry.ID, retrieved.ID)
	assert.Equal(t, models.AuditActionUpdate, retrieved.Action)
	assert.Len(t, retrieved.ChangedFields, 2)
	assert.Equal(t, "old description", retrieved.ChangedFields["description"].Old)
	assert.Equal(t, "new description", retrieved.ChangedFields["description"].New)
}

func TestAuditRepository_GetByProject(t *testing.T) {
	tc := setupAuditTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create multiple entries
	for i := 0; i < 5; i++ {
		entry := &models.AuditLogEntry{
			ProjectID:  tc.projectID,
			EntityType: models.AuditEntityTypeEntity,
			EntityID:   uuid.New(),
			Action:     models.AuditActionCreate,
			Source:     models.ProvenanceInferred,
			UserID:     &tc.testUserID,
		}
		err := tc.repo.Create(ctx, entry)
		require.NoError(t, err)
	}

	// Get entries with limit
	entries, err := tc.repo.GetByProject(ctx, tc.projectID, 3)
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// Verify ordering (newest first)
	for i := 0; i < len(entries)-1; i++ {
		assert.True(t, entries[i].CreatedAt.After(entries[i+1].CreatedAt) ||
			entries[i].CreatedAt.Equal(entries[i+1].CreatedAt))
	}
}

func TestAuditRepository_GetByEntity(t *testing.T) {
	tc := setupAuditTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	entityID := uuid.New()

	// Create multiple entries for the same entity
	actions := []string{models.AuditActionCreate, models.AuditActionUpdate, models.AuditActionUpdate}
	for _, action := range actions {
		entry := &models.AuditLogEntry{
			ProjectID:  tc.projectID,
			EntityType: models.AuditEntityTypeRelationship,
			EntityID:   entityID,
			Action:     action,
			Source:     models.ProvenanceManual,
			UserID:     &tc.testUserID,
		}
		err := tc.repo.Create(ctx, entry)
		require.NoError(t, err)
	}

	// Create entry for different entity
	entry := &models.AuditLogEntry{
		ProjectID:  tc.projectID,
		EntityType: models.AuditEntityTypeRelationship,
		EntityID:   uuid.New(),
		Action:     models.AuditActionCreate,
		Source:     models.ProvenanceManual,
		UserID:     &tc.testUserID,
	}
	err := tc.repo.Create(ctx, entry)
	require.NoError(t, err)

	// Get entries for specific entity
	entries, err := tc.repo.GetByEntity(ctx, models.AuditEntityTypeRelationship, entityID)
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// All entries should be for the same entity
	for _, e := range entries {
		assert.Equal(t, entityID, e.EntityID)
		assert.Equal(t, models.AuditEntityTypeRelationship, e.EntityType)
	}
}

func TestAuditRepository_GetByUser(t *testing.T) {
	tc := setupAuditTest(t)
	t.Cleanup(tc.cleanup)

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	anotherUserID := uuid.New()

	// Create entries for our test user
	for i := 0; i < 3; i++ {
		entry := &models.AuditLogEntry{
			ProjectID:  tc.projectID,
			EntityType: models.AuditEntityTypeGlossaryTerm,
			EntityID:   uuid.New(),
			Action:     models.AuditActionCreate,
			Source:     models.ProvenanceManual,
			UserID:     &tc.testUserID,
		}
		err := tc.repo.Create(ctx, entry)
		require.NoError(t, err)
	}

	// Create entries for another user
	for i := 0; i < 2; i++ {
		entry := &models.AuditLogEntry{
			ProjectID:  tc.projectID,
			EntityType: models.AuditEntityTypeGlossaryTerm,
			EntityID:   uuid.New(),
			Action:     models.AuditActionCreate,
			Source:     models.ProvenanceMCP,
			UserID:     &anotherUserID,
		}
		err := tc.repo.Create(ctx, entry)
		require.NoError(t, err)
	}

	// Get entries for our test user
	entries, err := tc.repo.GetByUser(ctx, tc.projectID, tc.testUserID, 10)
	require.NoError(t, err)
	assert.Len(t, entries, 3)

	// All entries should be for our test user
	for _, e := range entries {
		assert.Equal(t, tc.testUserID, *e.UserID)
	}
}
