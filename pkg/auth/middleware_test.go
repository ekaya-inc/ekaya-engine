package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// mockAuthService is a mock implementation of AuthService for testing.
type mockAuthService struct {
	claims            *Claims
	token             string
	validateErr       error
	requireProjectErr error
	validateMatchErr  error
}

func (m *mockAuthService) ValidateRequest(r *http.Request) (*Claims, string, error) {
	if m.validateErr != nil {
		return nil, "", m.validateErr
	}
	return m.claims, m.token, nil
}

func (m *mockAuthService) RequireProjectID(claims *Claims) error {
	return m.requireProjectErr
}

func (m *mockAuthService) ValidateProjectIDMatch(claims *Claims, urlProjectID string) error {
	return m.validateMatchErr
}

func TestMiddleware_RequireAuth_Success(t *testing.T) {
	claims := &Claims{ProjectID: "project-123"}
	authService := &mockAuthService{claims: claims, token: "test-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	var handlerCalled bool
	var ctxClaims *Claims
	var ctxToken string

	handler := middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxClaims, _ = GetClaims(r.Context())
		ctxToken, _ = GetToken(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ctxClaims == nil || ctxClaims.ProjectID != "project-123" {
		t.Error("expected claims to be set in context")
	}

	if ctxToken != "test-token" {
		t.Errorf("expected token 'test-token' in context, got %q", ctxToken)
	}
}

func TestMiddleware_RequireAuth_Unauthorized(t *testing.T) {
	authService := &mockAuthService{validateErr: ErrMissingAuthorization}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["error"] != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %q", response["error"])
	}
}

func TestMiddleware_RequireAuth_MissingProjectID(t *testing.T) {
	authService := &mockAuthService{
		claims:            &Claims{},
		token:             "test-token",
		requireProjectErr: ErrMissingProjectID,
	}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := middleware.RequireAuth(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["error"] != "bad_request" {
		t.Errorf("expected error 'bad_request', got %q", response["error"])
	}
}

func TestMiddleware_RequireAuthWithPathValidation_Success(t *testing.T) {
	claims := &Claims{ProjectID: "project-123"}
	authService := &mockAuthService{claims: claims, token: "test-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	var handlerCalled bool
	handler := middleware.RequireAuthWithPathValidation("pid")(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Create request with path value set
	req := httptest.NewRequest(http.MethodGet, "/api/projects/project-123", nil)
	req.SetPathValue("pid", "project-123")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_RequireAuthWithPathValidation_ProjectMismatch(t *testing.T) {
	claims := &Claims{ProjectID: "project-123"}
	authService := &mockAuthService{
		claims:           claims,
		token:            "test-token",
		validateMatchErr: ErrProjectIDMismatch,
	}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := middleware.RequireAuthWithPathValidation("pid")(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/projects/project-456", nil)
	req.SetPathValue("pid", "project-456")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["error"] != "forbidden" {
		t.Errorf("expected error 'forbidden', got %q", response["error"])
	}
}

func TestMiddleware_RequireAuthWithPathValidation_Unauthorized(t *testing.T) {
	authService := &mockAuthService{validateErr: errors.New("invalid token")}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := middleware.RequireAuthWithPathValidation("pid")(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodGet, "/api/projects/project-123", nil)
	req.SetPathValue("pid", "project-123")
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestMiddleware_ContextValues(t *testing.T) {
	// Test that claims and token are properly accessible from context
	claims := &Claims{
		ProjectID: "test-project",
	}
	claims.Subject = "user-123"

	ctx := context.Background()
	ctx = context.WithValue(ctx, ClaimsKey, claims)
	ctx = context.WithValue(ctx, TokenKey, "test-token")

	retrievedClaims, ok := GetClaims(ctx)
	if !ok {
		t.Error("expected GetClaims to return true")
	}
	if retrievedClaims.ProjectID != "test-project" {
		t.Errorf("expected ProjectID 'test-project', got %q", retrievedClaims.ProjectID)
	}

	retrievedToken, ok := GetToken(ctx)
	if !ok {
		t.Error("expected GetToken to return true")
	}
	if retrievedToken != "test-token" {
		t.Errorf("expected token 'test-token', got %q", retrievedToken)
	}
}

func TestMiddleware_ContextValues_NotSet(t *testing.T) {
	ctx := context.Background()

	_, ok := GetClaims(ctx)
	if ok {
		t.Error("expected GetClaims to return false for empty context")
	}

	_, ok = GetToken(ctx)
	if ok {
		t.Error("expected GetToken to return false for empty context")
	}
}

func TestMiddleware_RequireCentralService_Success(t *testing.T) {
	claims := &Claims{}
	claims.Subject = "central" // Central service token
	authService := &mockAuthService{claims: claims, token: "central-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	var handlerCalled bool
	var ctxClaims *Claims
	var ctxToken string

	handler := middleware.RequireCentralService(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxClaims, _ = GetClaims(r.Context())
		ctxToken, _ = GetToken(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/projects/new", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ctxClaims == nil || ctxClaims.Subject != "central" {
		t.Error("expected claims with subject 'central' in context")
	}

	if ctxToken != "central-token" {
		t.Errorf("expected token 'central-token' in context, got %q", ctxToken)
	}
}

func TestMiddleware_RequireCentralService_WithProjectID(t *testing.T) {
	// Central service can optionally include a project ID
	claims := &Claims{ProjectID: "project-123"}
	claims.Subject = "central"
	authService := &mockAuthService{claims: claims, token: "central-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	var handlerCalled bool
	handler := middleware.RequireCentralService(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/projects/new", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestMiddleware_RequireCentralService_Unauthorized(t *testing.T) {
	authService := &mockAuthService{validateErr: ErrMissingAuthorization}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := middleware.RequireCentralService(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/projects/new", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["error"] != "unauthorized" {
		t.Errorf("expected error 'unauthorized', got %q", response["error"])
	}
}

func TestMiddleware_RequireCentralService_UserTokenRejected(t *testing.T) {
	// Regular user token should be rejected
	claims := &Claims{ProjectID: "project-123"}
	claims.Subject = "user-123" // Not "central"
	authService := &mockAuthService{claims: claims, token: "user-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := middleware.RequireCentralService(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for user token")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/projects/new", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}

	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["error"] != "forbidden" {
		t.Errorf("expected error 'forbidden', got %q", response["error"])
	}

	if response["message"] != "Central service authorization required" {
		t.Errorf("expected message 'Central service authorization required', got %q", response["message"])
	}
}

func TestMiddleware_RequireCentralService_EmptySubjectRejected(t *testing.T) {
	// Token with empty subject should be rejected
	claims := &Claims{}
	claims.Subject = ""
	authService := &mockAuthService{claims: claims, token: "bad-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := middleware.RequireCentralService(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for empty subject")
	})

	req := httptest.NewRequest(http.MethodPost, "/api/projects/new", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}
}
