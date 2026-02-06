package mcpauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
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

// mockAgentKeyService is a mock implementation of services.AgentAPIKeyService for testing.
type mockAgentKeyService struct {
	validKeys  map[uuid.UUID]string // map of project ID to valid key
	validateFn func(ctx context.Context, projectID uuid.UUID, providedKey string) (bool, error)
}

func newMockAgentKeyService() *mockAgentKeyService {
	return &mockAgentKeyService{
		validKeys: make(map[uuid.UUID]string),
	}
}

func (m *mockAgentKeyService) GenerateKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	key := "generated-key-" + projectID.String()[:8]
	m.validKeys[projectID] = key
	return key, nil
}

func (m *mockAgentKeyService) GetKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	return m.validKeys[projectID], nil
}

func (m *mockAgentKeyService) RegenerateKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	return m.GenerateKey(ctx, projectID)
}

func (m *mockAgentKeyService) ValidateKey(ctx context.Context, projectID uuid.UUID, providedKey string) (bool, error) {
	if m.validateFn != nil {
		return m.validateFn(ctx, projectID, providedKey)
	}
	storedKey, ok := m.validKeys[projectID]
	if !ok {
		return false, nil
	}
	return storedKey == providedKey, nil
}

// mockTenantScopeProvider is a mock implementation of TenantScopeProvider for testing.
type mockTenantScopeProvider struct {
	err error
}

func (m *mockTenantScopeProvider) WithTenantScope(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
	if m.err != nil {
		return nil, nil, m.err
	}
	// Return the same context with a no-op cleanup function
	return ctx, func() {}, nil
}

func TestMiddleware_RequireAuth_Success(t *testing.T) {
	claims := &auth.Claims{ProjectID: "project-123"}
	authService := &mockAuthService{claims: claims, token: "test-token"}
	middleware := NewMiddleware(authService, nil, nil, zap.NewNop())

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
	middleware := NewMiddleware(authService, nil, nil, zap.NewNop())

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
	middleware := NewMiddleware(authService, nil, nil, zap.NewNop())

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
	middleware := NewMiddleware(authService, nil, nil, zap.NewNop())

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
	middleware := NewMiddleware(authService, nil, nil, zap.NewNop())

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
			middleware := NewMiddleware(tc.authService, nil, nil, zap.NewNop())

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

// --- Agent API Key Authentication Tests ---

func TestMiddleware_RequireAuth_AgentAPIKey_AuthorizationHeader(t *testing.T) {
	projectID := uuid.New()
	apiKey := "test-api-key-12345"

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = apiKey
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	var handlerCalled bool
	var ctxClaims *auth.Claims

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxClaims, _ = auth.GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:"+apiKey)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ctxClaims == nil {
		t.Fatal("expected claims to be set in context")
	}

	if ctxClaims.ProjectID != projectID.String() {
		t.Errorf("expected project ID %q, got %q", projectID.String(), ctxClaims.ProjectID)
	}

	if ctxClaims.Subject != "agent" {
		t.Errorf("expected Subject 'agent', got %q", ctxClaims.Subject)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_XAPIKeyHeader(t *testing.T) {
	projectID := uuid.New()
	apiKey := "test-api-key-67890"

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = apiKey
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	var handlerCalled bool
	var ctxClaims *auth.Claims

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxClaims, _ = auth.GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("X-API-Key", apiKey)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ctxClaims == nil {
		t.Fatal("expected claims to be set in context")
	}

	if ctxClaims.Subject != "agent" {
		t.Errorf("expected Subject 'agent', got %q", ctxClaims.Subject)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_InvalidKey(t *testing.T) {
	projectID := uuid.New()

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = "correct-key"
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:wrong-key")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, `error="invalid_token"`) {
		t.Errorf("expected error=invalid_token in WWW-Authenticate, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_NoKeyConfigured(t *testing.T) {
	projectID := uuid.New()

	agentKeyService := newMockAgentKeyService()
	// No key configured for this project
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:some-key")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_InvalidProjectID(t *testing.T) {
	agentKeyService := newMockAgentKeyService()
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/not-a-uuid", nil)
	req.SetPathValue("pid", "not-a-uuid")
	req.Header.Set("Authorization", "api-key:some-key")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, `error="invalid_request"`) {
		t.Errorf("expected error=invalid_request in WWW-Authenticate, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_ServiceError(t *testing.T) {
	projectID := uuid.New()

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validateFn = func(ctx context.Context, pid uuid.UUID, key string) (bool, error) {
		return false, errors.New("database error")
	}
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:some-key")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, `error="server_error"`) {
		t.Errorf("expected error=server_error in WWW-Authenticate, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_NoService(t *testing.T) {
	// Test when agentKeyService is nil but API key is provided
	middleware := NewMiddleware(nil, nil, nil, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:some-key")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, `error="server_error"`) {
		t.Errorf("expected error=server_error in WWW-Authenticate, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_NoTenantProvider(t *testing.T) {
	// Test when tenantProvider is nil but API key is provided
	projectID := uuid.New()
	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = "test-key"

	middleware := NewMiddleware(nil, agentKeyService, nil, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:test-key")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, `error="server_error"`) {
		t.Errorf("expected error=server_error in WWW-Authenticate, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_TenantScopeError(t *testing.T) {
	// Test when tenant scope creation fails
	projectID := uuid.New()
	apiKey := "test-api-key"

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = apiKey
	tenantProvider := &mockTenantScopeProvider{err: errors.New("database connection failed")}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:"+apiKey)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	wwwAuth := rec.Header().Get("WWW-Authenticate")
	if !strings.Contains(wwwAuth, `error="server_error"`) {
		t.Errorf("expected error=server_error in WWW-Authenticate, got %q", wwwAuth)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_ExtractProjectFromPath(t *testing.T) {
	// Test when path value is not set but URL path contains project ID
	projectID := uuid.New()
	apiKey := "test-api-key"

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = apiKey
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	var handlerCalled bool
	var ctxClaims *auth.Claims

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxClaims, _ = auth.GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	// Request without SetPathValue - should fall back to extractProjectIDFromPath
	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	// Note: Not setting req.SetPathValue("pid", ...)
	req.Header.Set("Authorization", "api-key:"+apiKey)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ctxClaims == nil {
		t.Fatal("expected claims to be set in context")
	}

	if ctxClaims.ProjectID != projectID.String() {
		t.Errorf("expected project ID %q, got %q", projectID.String(), ctxClaims.ProjectID)
	}
}

func TestMiddleware_RequireAuth_AgentAPIKey_BearerToken(t *testing.T) {
	projectID := uuid.New()
	apiKey := "8b9bc7dce2de106351ba7f7712dfbfb16d010a2478a094a8c328cdafd21f468b"

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = apiKey
	tenantProvider := &mockTenantScopeProvider{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop())

	var handlerCalled bool
	var ctxClaims *auth.Claims

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		ctxClaims, _ = auth.GetClaims(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "Bearer "+apiKey)
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("expected handler to be called")
	}

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if ctxClaims == nil {
		t.Fatal("expected claims in context")
	}

	if ctxClaims.ProjectID != projectID.String() {
		t.Errorf("expected project ID %q, got %q", projectID.String(), ctxClaims.ProjectID)
	}

	if ctxClaims.Subject != "agent" {
		t.Errorf("expected subject 'agent', got %q", ctxClaims.Subject)
	}
}

func TestIsJWT(t *testing.T) {
	testCases := []struct {
		name     string
		token    string
		expected bool
	}{
		{
			name:     "valid JWT format",
			token:    "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.signature",
			expected: true,
		},
		{
			name:     "API key (hex string)",
			token:    "8b9bc7dce2de106351ba7f7712dfbfb16d010a2478a094a8c328cdafd21f468b",
			expected: false,
		},
		{
			name:     "two segments",
			token:    "header.payload",
			expected: false,
		},
		{
			name:     "four segments",
			token:    "a.b.c.d",
			expected: false,
		},
		{
			name:     "empty string",
			token:    "",
			expected: false,
		},
		{
			name:     "no dots",
			token:    "simpletokenwithnodots",
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isJWT(tc.token)
			if result != tc.expected {
				t.Errorf("isJWT(%q) = %v, expected %v", tc.token, result, tc.expected)
			}
		})
	}
}

// mockAuditLogger records auth failure events for test verification.
type mockAuditLogger struct {
	events []authFailureEvent
}

type authFailureEvent struct {
	ProjectID uuid.UUID
	UserID    string
	Reason    string
	ClientIP  string
}

func (m *mockAuditLogger) RecordAuthFailure(projectID uuid.UUID, userID, reason, clientIP string) {
	m.events = append(m.events, authFailureEvent{
		ProjectID: projectID,
		UserID:    userID,
		Reason:    reason,
		ClientIP:  clientIP,
	})
}

func TestMiddleware_RequireAuth_AgentAPIKey_InvalidKey_AuditsFailure(t *testing.T) {
	projectID := uuid.New()

	agentKeyService := newMockAgentKeyService()
	agentKeyService.validKeys[projectID] = "correct-key"
	tenantProvider := &mockTenantScopeProvider{}
	auditLog := &mockAuditLogger{}

	middleware := NewMiddleware(nil, agentKeyService, tenantProvider, zap.NewNop(),
		WithAuditLogger(auditLog),
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Authorization", "api-key:wrong-key")
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	if len(auditLog.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditLog.events))
	}

	event := auditLog.events[0]
	if event.ProjectID != projectID {
		t.Errorf("expected project ID %s, got %s", projectID, event.ProjectID)
	}
	if event.UserID != "agent" {
		t.Errorf("expected user ID 'agent', got %q", event.UserID)
	}
	if event.Reason != "Invalid API key" {
		t.Errorf("expected reason 'Invalid API key', got %q", event.Reason)
	}
}

func TestMiddleware_RequireAuth_JWT_InvalidToken_AuditsFailure(t *testing.T) {
	authService := &mockAuthService{validateErr: auth.ErrMissingAuthorization}
	auditLog := &mockAuditLogger{}

	middleware := NewMiddleware(authService, nil, nil, zap.NewNop(),
		WithAuditLogger(auditLog),
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}

	if len(auditLog.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditLog.events))
	}

	event := auditLog.events[0]
	if event.ProjectID != projectID {
		t.Errorf("expected project ID %s, got %s", projectID, event.ProjectID)
	}
	if event.Reason != "Invalid or expired token" {
		t.Errorf("expected reason 'Invalid or expired token', got %q", event.Reason)
	}
}

func TestMiddleware_RequireAuth_ProjectMismatch_AuditsFailure(t *testing.T) {
	claims := &auth.Claims{ProjectID: "project-123"}
	claims.Subject = "user-456"
	authService := &mockAuthService{
		claims:           claims,
		token:            "test-token",
		validateMatchErr: auth.ErrProjectIDMismatch,
	}
	auditLog := &mockAuditLogger{}

	middleware := NewMiddleware(authService, nil, nil, zap.NewNop(),
		WithAuditLogger(auditLog),
	)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	pid := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/mcp/"+pid.String(), nil)
	req.SetPathValue("pid", pid.String())
	rec := httptest.NewRecorder()

	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rec.Code)
	}

	if len(auditLog.events) != 1 {
		t.Fatalf("expected 1 audit event, got %d", len(auditLog.events))
	}

	event := auditLog.events[0]
	if event.ProjectID != pid {
		t.Errorf("expected project ID %s, got %s", pid, event.ProjectID)
	}
	if event.UserID != "user-456" {
		t.Errorf("expected user ID 'user-456', got %q", event.UserID)
	}
	if event.Reason != "Project ID mismatch" {
		t.Errorf("expected reason 'Project ID mismatch', got %q", event.Reason)
	}
}

func TestMiddleware_RequireAuth_NoAuditLogger_NoPanic(t *testing.T) {
	// Verify that auth failures don't panic when no audit logger is configured
	authService := &mockAuthService{validateErr: auth.ErrMissingAuthorization}
	middleware := NewMiddleware(authService, nil, nil, zap.NewNop())

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	})

	wrappedHandler := middleware.RequireAuth("pid")(handler)

	projectID := uuid.New()
	req := httptest.NewRequest(http.MethodPost, "/mcp/"+projectID.String(), nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	// Should not panic
	wrappedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestExtractProjectIDFromPath(t *testing.T) {
	testCases := []struct {
		name        string
		path        string
		expectedID  string
		expectError bool
	}{
		{
			name:       "valid path with project ID",
			path:       "/mcp/550e8400-e29b-41d4-a716-446655440000",
			expectedID: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:       "valid path with trailing slash",
			path:       "/mcp/550e8400-e29b-41d4-a716-446655440000/",
			expectedID: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:       "valid path with additional segments",
			path:       "/mcp/550e8400-e29b-41d4-a716-446655440000/tools/list",
			expectedID: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:        "invalid - missing mcp prefix",
			path:        "/api/550e8400-e29b-41d4-a716-446655440000",
			expectError: true,
		},
		{
			name:        "invalid - no project ID",
			path:        "/mcp/",
			expectError: true,
		},
		{
			name:        "invalid - just mcp",
			path:        "/mcp",
			expectError: true,
		},
		{
			name:        "invalid - empty path",
			path:        "",
			expectError: true,
		},
		{
			name:        "invalid - not a UUID",
			path:        "/mcp/not-a-uuid",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			projectID, err := extractProjectIDFromPath(tc.path)

			if tc.expectError {
				if err == nil {
					t.Errorf("expected error for path %q, got nil", tc.path)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error for path %q: %v", tc.path, err)
				return
			}

			if projectID.String() != tc.expectedID {
				t.Errorf("expected ID %q, got %q", tc.expectedID, projectID.String())
			}
		})
	}
}
