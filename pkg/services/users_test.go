package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockUserRepository is a configurable mock for testing UserService.
type mockUserRepository struct {
	user       *models.User
	users      []*models.User
	adminCount int
	addErr     error
	removeErr  error
	updateErr  error
	getErr     error
	listErr    error
	countErr   error

	// Capture inputs for verification
	capturedUser    *models.User
	capturedRole    string
	capturedUserID  uuid.UUID
	capturedProject uuid.UUID
}

func (m *mockUserRepository) Add(ctx context.Context, user *models.User) error {
	m.capturedUser = user
	return m.addErr
}

func (m *mockUserRepository) Remove(ctx context.Context, projectID, userID uuid.UUID) error {
	m.capturedProject = projectID
	m.capturedUserID = userID
	return m.removeErr
}

func (m *mockUserRepository) Update(ctx context.Context, projectID, userID uuid.UUID, newRole string) error {
	m.capturedProject = projectID
	m.capturedUserID = userID
	m.capturedRole = newRole
	return m.updateErr
}

func (m *mockUserRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.User, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.users, nil
}

func (m *mockUserRepository) GetByID(ctx context.Context, projectID, userID uuid.UUID) (*models.User, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.user, nil
}

func (m *mockUserRepository) CountAdmins(ctx context.Context, projectID uuid.UUID) (int, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return m.adminCount, nil
}

func (m *mockUserRepository) RemoveWithOwnerCheck(ctx context.Context, projectID, userID uuid.UUID) error {
	m.capturedProject = projectID
	m.capturedUserID = userID
	return m.removeErr
}

func (m *mockUserRepository) UpdateRoleWithOwnerCheck(ctx context.Context, projectID, userID uuid.UUID, newRole string) error {
	m.capturedProject = projectID
	m.capturedUserID = userID
	m.capturedRole = newRole
	return m.updateErr
}

func newTestUserService(repo *mockUserRepository) UserService {
	return NewUserService(repo, zap.NewNop())
}

func TestUserService_Add_Success(t *testing.T) {
	repo := &mockUserRepository{}
	service := newTestUserService(repo)

	projectID := uuid.New()
	userID := uuid.New()

	err := service.Add(context.Background(), projectID, userID, models.RoleAdmin)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	if repo.capturedUser == nil {
		t.Fatal("expected user to be captured")
	}
	if repo.capturedUser.ProjectID != projectID {
		t.Errorf("expected project ID %v, got %v", projectID, repo.capturedUser.ProjectID)
	}
	if repo.capturedUser.UserID != userID {
		t.Errorf("expected user ID %v, got %v", userID, repo.capturedUser.UserID)
	}
	if repo.capturedUser.Role != models.RoleAdmin {
		t.Errorf("expected role admin, got %q", repo.capturedUser.Role)
	}
}

func TestUserService_Add_InvalidRole(t *testing.T) {
	repo := &mockUserRepository{}
	service := newTestUserService(repo)

	err := service.Add(context.Background(), uuid.New(), uuid.New(), "invalid-role")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if repo.capturedUser != nil {
		t.Error("should not have called repository for invalid role")
	}
}

func TestUserService_Add_RepoError(t *testing.T) {
	repo := &mockUserRepository{
		addErr: errors.New("database error"),
	}
	service := newTestUserService(repo)

	err := service.Add(context.Background(), uuid.New(), uuid.New(), models.RoleUser)
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestUserService_Remove_Success(t *testing.T) {
	repo := &mockUserRepository{}
	service := newTestUserService(repo)

	projectID := uuid.New()
	userID := uuid.New()

	err := service.Remove(context.Background(), projectID, userID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if repo.capturedProject != projectID {
		t.Errorf("expected project ID %v, got %v", projectID, repo.capturedProject)
	}
	if repo.capturedUserID != userID {
		t.Errorf("expected user ID %v, got %v", userID, repo.capturedUserID)
	}
}

func TestUserService_Remove_LastAdmin(t *testing.T) {
	repo := &mockUserRepository{
		removeErr: apperrors.ErrLastAdmin,
	}
	service := newTestUserService(repo)

	err := service.Remove(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error when removing last admin")
	}
	if !errors.Is(err, apperrors.ErrLastAdmin) {
		t.Errorf("expected ErrLastAdmin, got: %v", err)
	}
}

func TestUserService_Remove_RepoError(t *testing.T) {
	repo := &mockUserRepository{
		removeErr: errors.New("database error"),
	}
	service := newTestUserService(repo)

	err := service.Remove(context.Background(), uuid.New(), uuid.New())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestUserService_Update_Success(t *testing.T) {
	repo := &mockUserRepository{}
	service := newTestUserService(repo)

	projectID := uuid.New()
	userID := uuid.New()

	err := service.Update(context.Background(), projectID, userID, models.RoleData)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if repo.capturedProject != projectID {
		t.Errorf("expected project ID %v, got %v", projectID, repo.capturedProject)
	}
	if repo.capturedUserID != userID {
		t.Errorf("expected user ID %v, got %v", userID, repo.capturedUserID)
	}
	if repo.capturedRole != models.RoleData {
		t.Errorf("expected role data, got %q", repo.capturedRole)
	}
}

func TestUserService_Update_InvalidRole(t *testing.T) {
	repo := &mockUserRepository{}
	service := newTestUserService(repo)

	err := service.Update(context.Background(), uuid.New(), uuid.New(), "invalid-role")
	if err == nil {
		t.Fatal("expected error for invalid role")
	}
	if repo.capturedRole != "" {
		t.Error("should not have called repository for invalid role")
	}
}

func TestUserService_Update_LastAdminDemotion(t *testing.T) {
	repo := &mockUserRepository{
		updateErr: apperrors.ErrLastAdmin,
	}
	service := newTestUserService(repo)

	err := service.Update(context.Background(), uuid.New(), uuid.New(), models.RoleUser)
	if err == nil {
		t.Fatal("expected error when demoting last admin")
	}
	if !errors.Is(err, apperrors.ErrLastAdmin) {
		t.Errorf("expected ErrLastAdmin, got: %v", err)
	}
}

func TestUserService_Update_RepoError(t *testing.T) {
	repo := &mockUserRepository{
		updateErr: errors.New("database error"),
	}
	service := newTestUserService(repo)

	err := service.Update(context.Background(), uuid.New(), uuid.New(), models.RoleAdmin)
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestUserService_GetByProject_Success(t *testing.T) {
	expectedUsers := []*models.User{
		{UserID: uuid.New(), Role: models.RoleAdmin},
		{UserID: uuid.New(), Role: models.RoleUser},
	}
	repo := &mockUserRepository{
		users: expectedUsers,
	}
	service := newTestUserService(repo)

	users, err := service.GetByProject(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}
}

func TestUserService_GetByProject_Empty(t *testing.T) {
	repo := &mockUserRepository{
		users: []*models.User{},
	}
	service := newTestUserService(repo)

	users, err := service.GetByProject(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("GetByProject failed: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestUserService_GetByProject_RepoError(t *testing.T) {
	repo := &mockUserRepository{
		listErr: errors.New("database error"),
	}
	service := newTestUserService(repo)

	_, err := service.GetByProject(context.Background(), uuid.New())
	if err == nil {
		t.Fatal("expected error from repo")
	}
}

func TestUserService_Interface(t *testing.T) {
	repo := &mockUserRepository{}
	service := newTestUserService(repo)
	var _ UserService = service
}
