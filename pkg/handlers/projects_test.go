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

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

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

	if resp.Status != "success" {
		t.Errorf("expected status 'success', got %q", resp.Status)
	}

	if resp.PID != projectID.String() {
		t.Errorf("expected pid %q, got %q", projectID.String(), resp.PID)
	}

	if resp.Name != "My Project" {
		t.Errorf("expected name 'My Project', got %q", resp.Name)
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

func TestProjectsHandler_Get_ServiceNotFound(t *testing.T) {
	projectService := &mockProjectService{
		err: apperrors.ErrNotFound,
	}
	handler := NewProjectsHandler(projectService, zap.NewNop())

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())

	claims := &auth.Claims{ProjectID: projectID.String()}
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "not_found" {
		t.Errorf("expected error 'not_found', got %q", resp["error"])
	}
}

func TestProjectsHandler_Get_WithPAPIURL(t *testing.T) {
	projectID := uuid.New()
	projectService := &mockProjectService{
		project: &models.Project{
			ID:   projectID,
			Name: "My Project",
			Parameters: map[string]interface{}{
				"papi_url": "https://papi.example.com",
			},
		},
	}
	handler := NewProjectsHandler(projectService, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())

	claims := &auth.Claims{ProjectID: projectID.String()}
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

	if resp.PAPIURL != "https://papi.example.com" {
		t.Errorf("expected papi_url 'https://papi.example.com', got %q", resp.PAPIURL)
	}
}
