package mcpauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// mockAuthService is a mock implementation of auth.AuthService for testing.
type mockAuthService struct {
	claims            *auth.Claims
	token             string
	validateErr       error
	requireProjectErr error
	validateMatchErr  error
}

func (m *mockAuthService) ValidateRequest(r *http.Request) (*auth.Claims, string, error) {
	if m.validateErr != nil {
		return nil, "", m.validateErr
	}
	return m.claims, m.token, nil
}

func (m *mockAuthService) RequireProjectID(claims *auth.Claims) error {
	return m.requireProjectErr
}

func (m *mockAuthService) ValidateProjectIDMatch(claims *auth.Claims, urlProjectID string) error {
	return m.validateMatchErr
}

func TestMiddleware_RequireAuth_Success(t *testing.T) {
	claims := &auth.Claims{ProjectID: "project-123"}
	authService := &mockAuthService{claims: claims, token: "test-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	var handlerCalled bool
	var ctxClaims *auth.Claims
	var ctxToken string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxClaims, _ = auth.GetClaims(r.Context())
		ctxToken, _ = auth.GetToken(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/project-123", nil)
	req.SetPathValue("pid", "project-123")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

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

func TestMiddleware_RequireAuth_InvalidToken(t *testing.T) {
	authService := &mockAuthService{validateErr: auth.ErrMissingAuthorization}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/project-123", nil)
	req.SetPathValue("pid", "project-123")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	// Check WWW-Authenticate header per RFC 6750
	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if wwwAuth == "" {
		t.Error("expected WWW-Authenticate header")
	}
	if !strings.Contains(wwwAuth, "Bearer") {
		t.Errorf("expected Bearer in WWW-Authenticate, got %q", wwwAuth)
	}
	if !strings.Contains(wwwAuth, `error="invalid_token"`) {
		t.Errorf("expected error=invalid_token in WWW-Authenticate, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_MissingProjectID(t *testing.T) {
	authService := &mockAuthService{
		claims:            &auth.Claims{},
		token:             "test-token",
		requireProjectErr: auth.ErrMissingProjectID,
	}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/project-123", nil)
	req.SetPathValue("pid", "project-123")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, "invalid_token") {
		t.Errorf("expected invalid_token error, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_MissingURLProjectID(t *testing.T) {
	claims := &auth.Claims{ProjectID: "project-123"}
	authService := &mockAuthService{claims: claims, token: "test-token"}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	// Request without path value set
	req := httptest.NewRequest(http.MethodPost, "/mcp/", nil)
	// Note: Not setting req.SetPathValue("pid", ...)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, "invalid_request") {
		t.Errorf("expected invalid_request error, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_ProjectMismatch(t *testing.T) {
	claims := &auth.Claims{ProjectID: "project-123"}
	authService := &mockAuthService{
		claims:           claims,
		token:            "test-token",
		validateMatchErr: auth.ErrProjectIDMismatch,
	}
	middleware := NewMiddleware(authService, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/project-456", nil)
	req.SetPathValue("pid", "project-456") // Different from claims.ProjectID
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, "insufficient_scope") {
		t.Errorf("expected insufficient_scope error, got %q", wwwAuth)
	}
}

func TestMiddleware_WWWAuthenticateFormat(t *testing.T) {
	// Test that WWW-Authenticate header follows RFC 6750 Section 3 format
	testCases := []struct {
		name           string
		authService    *mockAuthService
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "invalid_token on auth failure",
			authService:    &mockAuthService{validateErr: auth.ErrInvalidAuthFormat},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "invalid_token",
		},
		{
			name: "insufficient_scope on project mismatch",
			authService: &mockAuthService{
				claims:           &auth.Claims{ProjectID: "p1"},
				token:            "t",
				validateMatchErr: auth.ErrProjectIDMismatch,
			},
			expectedStatus: http.StatusForbidden,
			expectedError:  "insufficient_scope",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			middleware := NewMiddleware(tc.authService, zap.NewNop())

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("handler should not be called")
			})

			wrappedHandler := middleware.RequireAuth("pid")(handler)

			req := httptest.NewRequest(http.MethodPost, "/mcp/project-123", nil)
			req.SetPathValue("pid", "project-123")
			rec := httptest.NewRecorder()

			wrappedHandler.ServeHTTP(rec, req)

			if rec.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d", tc.expectedStatus, rec.Code)
			}

			wwwAuth := rec.Header().Get("WWW-Authenticate")

			// RFC 6750 format: Bearer error="...", error_description="..."
			if !strings.HasPrefix(wwwAuth, "Bearer ") {
				t.Errorf("expected Bearer scheme, got %q", wwwAuth)
			}

			if !strings.Contains(wwwAuth, `error="`+tc.expectedError+`"`) {
				t.Errorf("expected error=%q, got %q", tc.expectedError, wwwAuth)
			}

			if !strings.Contains(wwwAuth, `error_description="`) {
				t.Errorf("expected error_description in %q", wwwAuth)
			}
		})
	}
}
