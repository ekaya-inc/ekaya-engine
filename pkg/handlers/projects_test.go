package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockProjectService is a mock implementation of ProjectService for testing.
type mockProjectService struct {
	project *models.Project
	err     error
}

func (m *mockProjectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.project != nil {
		return m.project, nil
	}
	return &models.Project{
		ID:   id,
		Name: "Test Project",
	}, nil
}

func TestProjectsHandler_Get_Success(t *testing.T) {
	projectID := uuid.New()
	projectService := &mockProjectService{
		project: &models.Project{
			ID:   projectID,
			Name: "My Project",
		},
	}
	handler := NewProjectsHandler(projectService, zap.NewNop())

	// Create request with claims in context
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())

	claims := &auth.Claims{
		ProjectID: projectID.String(),
	}
	claims.Subject = "user-123"
	claims.Email = "user@example.com"

	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ProjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.UserID != "user-123" {
		t.Errorf("expected user_id 'user-123', got %q", resp.UserID)
	}

	if resp.UserEmail != "user@example.com" {
		t.Errorf("expected user_email 'user@example.com', got %q", resp.UserEmail)
	}

	// Check project in response
	projectMap, ok := resp.Project.(map[string]interface{})
	if !ok {
		t.Fatal("expected project to be a map")
	}

	if projectMap["name"] != "My Project" {
		t.Errorf("expected project name 'My Project', got %v", projectMap["name"])
	}
}

func TestProjectsHandler_Get_InvalidProjectID(t *testing.T) {
	handler := NewProjectsHandler(&mockProjectService{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-a-uuid", nil)
	req.SetPathValue("pid", "not-a-uuid")

	// Still need claims in context
	claims := &auth.Claims{ProjectID: "some-project"}
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_project_id" {
		t.Errorf("expected error 'invalid_project_id', got %q", resp["error"])
	}
}

func TestProjectsHandler_Get_MissingClaims(t *testing.T) {
	handler := NewProjectsHandler(&mockProjectService{}, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	// No claims in context

	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "internal_error" {
		t.Errorf("expected error 'internal_error', got %q", resp["error"])
	}
}

func TestProjectsHandler_Get_ServiceError(t *testing.T) {
	projectService := &mockProjectService{
		err: errors.New("database error"),
	}
	handler := NewProjectsHandler(projectService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "internal_error" {
		t.Errorf("expected error 'internal_error', got %q", resp["error"])
	}
}

func TestProjectsHandler_Get_NoEmail(t *testing.T) {
	// Test that response works when email is not in claims
	projectID := uuid.New()
	handler := NewProjectsHandler(&mockProjectService{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())

	claims := &auth.Claims{
		ProjectID: projectID.String(),
		// No Email set
	}
	claims.Subject = "user-123"

	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ProjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Email should be empty but not cause an error
	if resp.UserEmail != "" {
		t.Errorf("expected empty user_email, got %q", resp.UserEmail)
	}
}

// TestProjectsHandler_IntegrationWithMiddleware tests the full flow with middleware
func TestProjectsHandler_IntegrationWithMiddleware(t *testing.T) {
	// This test verifies the handler works correctly with the auth middleware
	projectID := uuid.New()

	// Create mock auth service that returns valid claims
	mockAuthService := &mockAuthServiceForProjects{
		claims: &auth.Claims{
			ProjectID: projectID.String(),
		},
		token: "test-token",
	}
	mockAuthService.claims.Subject = "user-456"
	mockAuthService.claims.Email = "test@example.com"

	middleware := auth.NewMiddleware(mockAuthService, zap.NewNop())
	handler := NewProjectsHandler(services.NewProjectService(), zap.NewNop())

	// Create a test mux and register the route
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, middleware)

	// Create request with auth header
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String(), nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ProjectResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.UserID != "user-456" {
		t.Errorf("expected user_id 'user-456', got %q", resp.UserID)
	}
}

// mockAuthServiceForProjects is a mock AuthService for integration testing
type mockAuthServiceForProjects struct {
	claims *auth.Claims
	token  string
}

func (m *mockAuthServiceForProjects) ValidateRequest(r *http.Request) (*auth.Claims, string, error) {
	return m.claims, m.token, nil
}

func (m *mockAuthServiceForProjects) RequireProjectID(claims *auth.Claims) error {
	if claims.ProjectID == "" {
		return auth.ErrMissingProjectID
	}
	return nil
}

func (m *mockAuthServiceForProjects) ValidateProjectIDMatch(claims *auth.Claims, urlProjectID string) error {
	if urlProjectID != "" && claims.ProjectID != urlProjectID {
		return auth.ErrProjectIDMismatch
	}
	return nil
}
