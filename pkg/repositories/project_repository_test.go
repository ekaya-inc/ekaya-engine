//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// projectTestContext holds test dependencies for project repository tests.
type projectTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      ProjectRepository
	userRepo  UserRepository
	projectID uuid.UUID
}

// setupProjectTest initializes the test context with shared testcontainer.
func setupProjectTest(t *testing.T) *projectTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	return &projectTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      NewProjectRepository(),
		userRepo:  NewUserRepository(),
		projectID: uuid.MustParse("00000000-0000-0000-0000-000000000020"),
	}
}

// cleanup removes test data from engine_projects.
func (tc *projectTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Clean up users first (FK constraint)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_users WHERE project_id = $1", tc.projectID)
	// Then clean up project
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_projects WHERE id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *projectTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestProject creates a project for testing.
func (tc *projectTestContext) createTestProject(ctx context.Context, name string) *models.Project {
	tc.t.Helper()
	project := &models.Project{
		ID:   tc.projectID,
		Name: name,
		Parameters: map[string]interface{}{
			"tier":   "standard",
			"region": "us-central1",
		},
	}
	err := tc.repo.Create(ctx, project)
	if err != nil {
		tc.t.Fatalf("failed to create test project: %v", err)
	}
	return project
}

// createTestUser creates a user associated with the project for testing.
func (tc *projectTestContext) createTestUser(ctx context.Context, userID uuid.UUID, role string) *models.User {
	tc.t.Helper()
	user := &models.User{
		ProjectID: tc.projectID,
		UserID:    userID,
		Role:      role,
	}
	err := tc.userRepo.Add(ctx, user)
	if err != nil {
		tc.t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

// TestProjectRepository_Create_Success tests creating a new project.
func TestProjectRepository_Create_Success(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	project := &models.Project{
		ID:   tc.projectID,
		Name: "Test Project",
		Parameters: map[string]interface{}{
			"tier":    "premium",
			"feature": "enabled",
		},
	}

	err := tc.repo.Create(ctx, project)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify project was created
	retrieved, err := tc.repo.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Name != "Test Project" {
		t.Errorf("expected name 'Test Project', got %q", retrieved.Name)
	}
	if retrieved.Status != "active" {
		t.Errorf("expected default status 'active', got %q", retrieved.Status)
	}
	if retrieved.Parameters["tier"] != "premium" {
		t.Errorf("expected parameter tier=premium, got %v", retrieved.Parameters["tier"])
	}
	if retrieved.Parameters["feature"] != "enabled" {
		t.Errorf("expected parameter feature=enabled, got %v", retrieved.Parameters["feature"])
	}
	if retrieved.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

// TestProjectRepository_Create_GeneratesUUID tests that Create generates UUID when not provided.
func TestProjectRepository_Create_GeneratesUUID(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	project := &models.Project{
		// ID not set - should be generated
		Name:       "Auto UUID Project",
		Parameters: map[string]interface{}{},
	}

	err := tc.repo.Create(ctx, project)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if project.ID == uuid.Nil {
		t.Error("expected ID to be generated, got nil UUID")
	}

	// Clean up the auto-generated project
	ctx2, cleanup2 := tc.createTestContext()
	defer cleanup2()
	_ = tc.repo.Delete(ctx2, project.ID)
}

// TestProjectRepository_Create_Idempotent tests that Create is idempotent (upsert).
func TestProjectRepository_Create_Idempotent(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// First create
	project := &models.Project{
		ID:   tc.projectID,
		Name: "Original Name",
		Parameters: map[string]interface{}{
			"version": float64(1),
		},
	}
	err := tc.repo.Create(ctx, project)
	if err != nil {
		t.Fatalf("first Create failed: %v", err)
	}

	// Second create with same ID (should update)
	project2 := &models.Project{
		ID:   tc.projectID,
		Name: "Updated Name",
		Parameters: map[string]interface{}{
			"version": float64(2),
		},
		Status: "inactive",
	}
	err = tc.repo.Create(ctx, project2)
	if err != nil {
		t.Fatalf("second Create failed: %v", err)
	}

	// Verify update happened
	retrieved, err := tc.repo.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %q", retrieved.Name)
	}
	if retrieved.Status != "inactive" {
		t.Errorf("expected status 'inactive', got %q", retrieved.Status)
	}
	if retrieved.Parameters["version"] != float64(2) {
		t.Errorf("expected parameter version=2, got %v", retrieved.Parameters["version"])
	}
}

// TestProjectRepository_Get_Success tests retrieving an existing project.
func TestProjectRepository_Get_Success(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a project first
	created := tc.createTestProject(ctx, "Get Test Project")

	// Retrieve it
	retrieved, err := tc.repo.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID %v, got %v", created.ID, retrieved.ID)
	}
	if retrieved.Name != "Get Test Project" {
		t.Errorf("expected name 'Get Test Project', got %q", retrieved.Name)
	}
	// Verify parameters unmarshaled correctly
	if retrieved.Parameters["tier"] != "standard" {
		t.Errorf("expected tier=standard, got %v", retrieved.Parameters["tier"])
	}
	if retrieved.Parameters["region"] != "us-central1" {
		t.Errorf("expected region=us-central1, got %v", retrieved.Parameters["region"])
	}
}

// TestProjectRepository_Get_NotFound tests Get returns ErrNotFound for missing project.
func TestProjectRepository_Get_NotFound(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.MustParse("00000000-0000-0000-0000-999999999999")
	_, err := tc.repo.Get(ctx, nonExistentID)
	if err != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestProjectRepository_Delete_Success tests deleting an existing project.
func TestProjectRepository_Delete_Success(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a project first
	tc.createTestProject(ctx, "Delete Test Project")

	// Delete it
	err := tc.repo.Delete(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, err = tc.repo.Get(ctx, tc.projectID)
	if err != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// TestProjectRepository_Delete_NotFound tests Delete returns ErrNotFound for missing project.
func TestProjectRepository_Delete_NotFound(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.MustParse("00000000-0000-0000-0000-999999999998")
	err := tc.repo.Delete(ctx, nonExistentID)
	if err != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// TestProjectRepository_Delete_CascadesToUsers tests that deleting a project cascades to users.
func TestProjectRepository_Delete_CascadesToUsers(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create project
	tc.createTestProject(ctx, "Cascade Test Project")

	// Add users to the project
	user1ID := uuid.MustParse("00000000-0000-0000-0000-000000000021")
	user2ID := uuid.MustParse("00000000-0000-0000-0000-000000000022")
	tc.createTestUser(ctx, user1ID, models.RoleAdmin)
	tc.createTestUser(ctx, user2ID, models.RoleUser)

	// Verify users exist
	users, err := tc.userRepo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("expected 2 users before delete, got %d", len(users))
	}

	// Delete the project
	err = tc.repo.Delete(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify users are also deleted (cascade)
	users, err = tc.userRepo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject after delete failed: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users after cascade delete, got %d", len(users))
	}
}

// TestProjectRepository_Update_UpdatesName tests that Update correctly updates the project name.
func TestProjectRepository_Update_UpdatesName(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a project with initial name
	project := tc.createTestProject(ctx, "Original Name")

	// Update the name
	project.Name = "Updated Name"
	err := tc.repo.Update(ctx, project)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Retrieve and verify name was updated
	retrieved, err := tc.repo.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got %q", retrieved.Name)
	}
}

// TestProjectRepository_Update_UpdatesNameAndParameters tests that Update correctly updates both name and parameters.
func TestProjectRepository_Update_UpdatesNameAndParameters(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a project
	project := tc.createTestProject(ctx, "Original Name")

	// Update both name and parameters
	project.Name = "New Name"
	project.Parameters["new_key"] = "new_value"
	err := tc.repo.Update(ctx, project)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Retrieve and verify both were updated
	retrieved, err := tc.repo.Get(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved.Name != "New Name" {
		t.Errorf("expected name 'New Name', got %q", retrieved.Name)
	}
	if retrieved.Parameters["new_key"] != "new_value" {
		t.Errorf("expected parameter new_key=new_value, got %v", retrieved.Parameters["new_key"])
	}
}

// TestProjectRepository_NoTenantScope tests that operations fail without tenant scope.
func TestProjectRepository_NoTenantScope(t *testing.T) {
	tc := setupProjectTest(t)
	tc.cleanup()

	// Use context without tenant scope
	ctx := context.Background()

	project := &models.Project{
		ID:         tc.projectID,
		Name:       "No Scope Project",
		Parameters: map[string]interface{}{},
	}

	// Create should fail
	err := tc.repo.Create(ctx, project)
	if err == nil {
		t.Error("expected error for Create without tenant scope")
	}

	// Get should fail
	_, err = tc.repo.Get(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for Get without tenant scope")
	}

	// Delete should fail
	err = tc.repo.Delete(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for Delete without tenant scope")
	}
}
