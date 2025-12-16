//go:build integration

package repositories

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// userTestContext holds test dependencies for user repository tests.
type userTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      UserRepository
	projectID uuid.UUID
}

// setupUserTest initializes the test context with shared testcontainer.
func setupUserTest(t *testing.T) *userTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &userTestContext{
		t:         t,
		engineDB:  engineDB,
		repo:      NewUserRepository(),
		projectID: uuid.MustParse("00000000-0000-0000-0000-000000000030"),
	}
	// Ensure project exists for FK constraint
	tc.ensureTestProject()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *userTestContext) ensureTestProject() {
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
	`, tc.projectID, "User Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}
}

// cleanup removes test users from engine_users.
func (tc *userTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Clean up users for this project
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_users WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *userTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestUser adds a user directly for testing.
func (tc *userTestContext) createTestUser(ctx context.Context, userID uuid.UUID, role string) *models.User {
	tc.t.Helper()
	user := &models.User{
		ProjectID: tc.projectID,
		UserID:    userID,
		Role:      role,
	}
	err := tc.repo.Add(ctx, user)
	if err != nil {
		tc.t.Fatalf("failed to create test user: %v", err)
	}
	return user
}

// TestUserRepository_Add_Create tests adding a new user.
func TestUserRepository_Add_Create(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	userID := uuid.MustParse("00000000-0000-0000-0000-000000000031")
	user := &models.User{
		ProjectID: tc.projectID,
		UserID:    userID,
		Role:      models.RoleAdmin,
	}

	err := tc.repo.Add(ctx, user)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Verify user was created
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, userID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.ProjectID != tc.projectID {
		t.Errorf("expected ProjectID %v, got %v", tc.projectID, retrieved.ProjectID)
	}
	if retrieved.UserID != userID {
		t.Errorf("expected UserID %v, got %v", userID, retrieved.UserID)
	}
	if retrieved.Role != models.RoleAdmin {
		t.Errorf("expected role admin, got %q", retrieved.Role)
	}
	if retrieved.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

// TestUserRepository_Add_UpsertChangesRole tests that Add upserts existing users.
func TestUserRepository_Add_UpsertChangesRole(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	userID := uuid.MustParse("00000000-0000-0000-0000-000000000032")

	// First add with admin role
	user1 := &models.User{
		ProjectID: tc.projectID,
		UserID:    userID,
		Role:      models.RoleAdmin,
	}
	err := tc.repo.Add(ctx, user1)
	if err != nil {
		t.Fatalf("first Add failed: %v", err)
	}

	// Second add with user role (should update)
	user2 := &models.User{
		ProjectID: tc.projectID,
		UserID:    userID,
		Role:      models.RoleUser,
	}
	err = tc.repo.Add(ctx, user2)
	if err != nil {
		t.Fatalf("second Add failed: %v", err)
	}

	// Verify role was changed
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, userID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Role != models.RoleUser {
		t.Errorf("expected role to change to user, got %q", retrieved.Role)
	}
}

// TestUserRepository_Remove_Success tests removing a user.
func TestUserRepository_Remove_Success(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	userID := uuid.MustParse("00000000-0000-0000-0000-000000000033")
	tc.createTestUser(ctx, userID, models.RoleData)

	// Remove user
	err := tc.repo.Remove(ctx, tc.projectID, userID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify user is gone
	_, err = tc.repo.GetByID(ctx, tc.projectID, userID)
	if err == nil {
		t.Error("expected error for removed user")
	}
}

// TestUserRepository_Remove_NotFound tests Remove returns error for missing user.
func TestUserRepository_Remove_NotFound(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.MustParse("00000000-0000-0000-0000-999999999991")
	err := tc.repo.Remove(ctx, tc.projectID, nonExistentID)
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

// TestUserRepository_Update_Success tests updating a user's role.
func TestUserRepository_Update_Success(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	userID := uuid.MustParse("00000000-0000-0000-0000-000000000034")
	tc.createTestUser(ctx, userID, models.RoleUser)

	// Update to admin role
	err := tc.repo.Update(ctx, tc.projectID, userID, models.RoleAdmin)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify role changed
	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, userID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Role != models.RoleAdmin {
		t.Errorf("expected role admin, got %q", retrieved.Role)
	}
}

// TestUserRepository_Update_NotFound tests Update returns error for missing user.
func TestUserRepository_Update_NotFound(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.MustParse("00000000-0000-0000-0000-999999999992")
	err := tc.repo.Update(ctx, tc.projectID, nonExistentID, models.RoleAdmin)
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

// TestUserRepository_GetByProject_Success tests listing users for a project.
func TestUserRepository_GetByProject_Success(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Add multiple users
	user1ID := uuid.MustParse("00000000-0000-0000-0000-000000000035")
	user2ID := uuid.MustParse("00000000-0000-0000-0000-000000000036")
	user3ID := uuid.MustParse("00000000-0000-0000-0000-000000000037")

	tc.createTestUser(ctx, user1ID, models.RoleAdmin)
	tc.createTestUser(ctx, user2ID, models.RoleData)
	tc.createTestUser(ctx, user3ID, models.RoleUser)

	// Get all users
	users, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	if len(users) != 3 {
		t.Errorf("expected 3 users, got %d", len(users))
	}

	// Verify users are ordered by created_at
	for i := 0; i < len(users)-1; i++ {
		if users[i].CreatedAt.After(users[i+1].CreatedAt) {
			t.Errorf("users not ordered by created_at: %v > %v", users[i].CreatedAt, users[i+1].CreatedAt)
		}
	}
}

// TestUserRepository_GetByProject_Empty tests GetByProject returns empty slice for no users.
func TestUserRepository_GetByProject_Empty(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	users, err := tc.repo.GetByProject(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	if users == nil {
		// nil is ok for empty result
		users = []*models.User{}
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

// TestUserRepository_GetByID_Success tests retrieving a specific user.
func TestUserRepository_GetByID_Success(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	userID := uuid.MustParse("00000000-0000-0000-0000-000000000038")
	tc.createTestUser(ctx, userID, models.RoleData)

	retrieved, err := tc.repo.GetByID(ctx, tc.projectID, userID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if retrieved.UserID != userID {
		t.Errorf("expected UserID %v, got %v", userID, retrieved.UserID)
	}
	if retrieved.Role != models.RoleData {
		t.Errorf("expected role data, got %q", retrieved.Role)
	}
}

// TestUserRepository_GetByID_NotFound tests GetByID returns error for missing user.
func TestUserRepository_GetByID_NotFound(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.MustParse("00000000-0000-0000-0000-999999999993")
	_, err := tc.repo.GetByID(ctx, tc.projectID, nonExistentID)
	if err == nil {
		t.Error("expected error for non-existent user")
	}
}

// TestUserRepository_CountAdmins_Zero tests CountAdmins returns 0 when no admins.
func TestUserRepository_CountAdmins_Zero(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Add users with non-admin roles
	user1ID := uuid.MustParse("00000000-0000-0000-0000-000000000039")
	user2ID := uuid.MustParse("00000000-0000-0000-0000-00000000003a")
	tc.createTestUser(ctx, user1ID, models.RoleUser)
	tc.createTestUser(ctx, user2ID, models.RoleData)

	count, err := tc.repo.CountAdmins(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("CountAdmins failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 admins, got %d", count)
	}
}

// TestUserRepository_CountAdmins_Multiple tests CountAdmins counts only admins.
func TestUserRepository_CountAdmins_Multiple(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Add mix of users
	admin1ID := uuid.MustParse("00000000-0000-0000-0000-00000000003b")
	admin2ID := uuid.MustParse("00000000-0000-0000-0000-00000000003c")
	userID := uuid.MustParse("00000000-0000-0000-0000-00000000003d")
	dataID := uuid.MustParse("00000000-0000-0000-0000-00000000003e")

	tc.createTestUser(ctx, admin1ID, models.RoleAdmin)
	tc.createTestUser(ctx, admin2ID, models.RoleAdmin)
	tc.createTestUser(ctx, userID, models.RoleUser)
	tc.createTestUser(ctx, dataID, models.RoleData)

	count, err := tc.repo.CountAdmins(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("CountAdmins failed: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 admins, got %d", count)
	}
}

// TestUserRepository_NoTenantScope tests that operations fail without tenant scope.
func TestUserRepository_NoTenantScope(t *testing.T) {
	tc := setupUserTest(t)
	tc.cleanup()

	// Use context without tenant scope
	ctx := context.Background()
	userID := uuid.MustParse("00000000-0000-0000-0000-00000000003f")

	// Add should fail
	user := &models.User{
		ProjectID: tc.projectID,
		UserID:    userID,
		Role:      models.RoleUser,
	}
	err := tc.repo.Add(ctx, user)
	if err == nil {
		t.Error("expected error for Add without tenant scope")
	}

	// Remove should fail
	err = tc.repo.Remove(ctx, tc.projectID, userID)
	if err == nil {
		t.Error("expected error for Remove without tenant scope")
	}

	// Update should fail
	err = tc.repo.Update(ctx, tc.projectID, userID, models.RoleAdmin)
	if err == nil {
		t.Error("expected error for Update without tenant scope")
	}

	// GetByProject should fail
	_, err = tc.repo.GetByProject(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for GetByProject without tenant scope")
	}

	// GetByID should fail
	_, err = tc.repo.GetByID(ctx, tc.projectID, userID)
	if err == nil {
		t.Error("expected error for GetByID without tenant scope")
	}

	// CountAdmins should fail
	_, err = tc.repo.CountAdmins(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for CountAdmins without tenant scope")
	}
}
