//go:build integration

package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// TestOAuthFlow_EndToEnd_BearerToken verifies the complete OAuth flow:
// 1. Complete OAuth flow returns JWT in response body
// 2. JWT can be used as Bearer token for subsequent API requests
// 3. Bearer token authentication works end-to-end
func TestOAuthFlow_EndToEnd_BearerToken(t *testing.T) {
	// Initialize session store for OAuth handler
	auth.InitSessionStore("test-secret")

	// Test project ID
	testProjectID := uuid.New()
	testProjectIDStr := testProjectID.String()

	// Create a test JWT with the project ID
	testJWT := testhelpers.GenerateTestJWT("test-user", testProjectIDStr, "test@example.com")

	// Setup configuration
	cfg := &config.Config{
		BaseURL:      "http://localhost:3443",
		CookieDomain: "",
	}

	// Setup mock OAuth service that returns our test JWT
	oauthService := &mockOAuthService{
		token: testJWT,
		err:   nil,
	}

	// Setup mock project service
	projectService := &mockProjectService{
		project: &models.Project{
			ID:   testProjectID,
			Name: "Test Project",
		},
		err: nil,
	}

	// Create auth handler for OAuth flow
	authHandler := NewAuthHandler(oauthService, projectService, cfg, zap.NewNop())

	// Step 1: Complete OAuth flow
	t.Log("Step 1: Complete OAuth flow to get JWT")
	completeReq := CompleteOAuthRequest{
		Code:         "valid-auth-code",
		State:        "valid-state",
		CodeVerifier: "valid-verifier",
		AuthURL:      "http://localhost:5002",
		RedirectURI:  "http://localhost:3443/oauth/callback",
	}
	bodyBytes, err := json.Marshal(completeReq)
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	authHandler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d. Response: %s", rec.Code, rec.Body.String())
	}

	// Step 2: Verify JWT is in response body with project_id
	t.Log("Step 2: Verify JWT and project_id in response")
	var oauthResp CompleteOAuthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &oauthResp); err != nil {
		t.Fatalf("Failed to parse OAuth response: %v", err)
	}

	if oauthResp.Token == "" {
		t.Fatal("Expected token in response body, got empty string")
	}

	if oauthResp.Token != testJWT {
		t.Errorf("Expected token %q, got %q", testJWT, oauthResp.Token)
	}

	if oauthResp.ProjectID != testProjectIDStr {
		t.Errorf("Expected project_id %q, got %q", testProjectIDStr, oauthResp.ProjectID)
	}

	t.Logf("✓ OAuth flow returned JWT: %s", oauthResp.Token[:20]+"...")
	t.Logf("✓ OAuth flow returned project_id: %s", oauthResp.ProjectID)

	// Step 3: Use JWT as Bearer token for API call
	t.Log("Step 3: Use JWT as Bearer token for API request")

	// Setup auth service and middleware for API call
	// For integration tests, we use a mock auth service that bypasses JWKS validation
	// but still verifies token extraction and claims handling
	mockAuthService := &mockAuthService{
		claims: &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "test-user",
			},
			ProjectID: testProjectIDStr,
			Email:     "test@example.com",
		},
		token: testJWT,
	}

	authMiddleware := auth.NewMiddleware(mockAuthService, zap.NewNop())

	// Create projects handler
	projectsHandler := NewProjectsHandler(projectService, cfg, zap.NewNop())

	// Create API request with Bearer token
	apiReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+testProjectIDStr, nil)
	apiReq.Header.Set("Authorization", "Bearer "+testJWT)
	apiReq.SetPathValue("pid", testProjectIDStr)
	apiRec := httptest.NewRecorder()

	// Wrap handler with auth middleware using path validation
	wrappedHandler := authMiddleware.RequireAuthWithPathValidation("pid")(projectsHandler.Get)
	wrappedHandler(apiRec, apiReq)

	// Step 4: Verify API call succeeded
	t.Log("Step 4: Verify API call succeeded with Bearer token")
	if apiRec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 for API call with Bearer token, got %d. Response: %s",
			apiRec.Code, apiRec.Body.String())
	}

	var projectResp ProjectResponse
	if err := json.Unmarshal(apiRec.Body.Bytes(), &projectResp); err != nil {
		t.Fatalf("Failed to parse project response: %v", err)
	}

	if projectResp.PID != testProjectIDStr {
		t.Errorf("Expected project ID %q in response, got %q", testProjectIDStr, projectResp.PID)
	}

	t.Log("✓ API request succeeded with Bearer token")
	t.Logf("✓ Got project: %s (%s)", projectResp.Name, projectResp.PID)
}

// TestOAuthFlow_BearerToken_ProjectIDMismatch verifies that Bearer token
// with wrong project ID is rejected with 403 Forbidden.
func TestOAuthFlow_BearerToken_ProjectIDMismatch(t *testing.T) {
	auth.InitSessionStore("test-secret")

	// Create JWT with project-1
	project1ID := uuid.New()
	testJWT := testhelpers.GenerateTestJWT("test-user", project1ID.String(), "test@example.com")

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	// Create mock auth service with project-1 in claims
	mockAuthService := &mockAuthService{
		claims: &auth.Claims{
			RegisteredClaims: jwt.RegisteredClaims{
				Subject: "test-user",
			},
			ProjectID: project1ID.String(),
			Email:     "test@example.com",
		},
		token: testJWT,
	}

	authMiddleware := auth.NewMiddleware(mockAuthService, zap.NewNop())

	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, cfg, zap.NewNop())

	// Try to access project-2 with JWT for project-1
	project2ID := uuid.New()
	apiReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+project2ID.String(), nil)
	apiReq.Header.Set("Authorization", "Bearer "+testJWT)
	apiReq.SetPathValue("pid", project2ID.String())
	apiRec := httptest.NewRecorder()

	wrappedHandler := authMiddleware.RequireAuthWithPathValidation("pid")(projectsHandler.Get)
	wrappedHandler(apiRec, apiReq)

	// Should get 403 Forbidden due to project ID mismatch
	if apiRec.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 for project ID mismatch, got %d. Response: %s",
			apiRec.Code, apiRec.Body.String())
	}

	var errResp map[string]string
	if err := json.Unmarshal(apiRec.Body.Bytes(), &errResp); err == nil {
		if errResp["error"] != "forbidden" {
			t.Errorf("Expected error 'forbidden', got %q", errResp["error"])
		}
		t.Logf("✓ Got expected error: %s - %s", errResp["error"], errResp["message"])
	}
}

// TestOAuthFlow_BearerToken_MissingToken verifies that requests without
// Bearer token are rejected with 401 Unauthorized.
func TestOAuthFlow_BearerToken_MissingToken(t *testing.T) {
	auth.InitSessionStore("test-secret")

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	// Create a mock auth service that validates real requests
	// When there's no Authorization header, ValidateRequest in the real mockAuthService
	// will return nil claims, and RequireProjectID will fail
	mockAuthSvc := &mockAuthServiceReturningError{}

	authMiddleware := auth.NewMiddleware(mockAuthSvc, zap.NewNop())

	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, cfg, zap.NewNop())

	// Make request without Authorization header
	testProjectID := uuid.New()
	apiReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+testProjectID.String(), nil)
	// No Authorization header
	apiReq.SetPathValue("pid", testProjectID.String())
	apiRec := httptest.NewRecorder()

	wrappedHandler := authMiddleware.RequireAuthWithPathValidation("pid")(projectsHandler.Get)
	wrappedHandler(apiRec, apiReq)

	// Should get 401 Unauthorized due to missing token
	if apiRec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status 401 for missing token, got %d. Response: %s",
			apiRec.Code, apiRec.Body.String())
	}

	var errResp map[string]string
	if err := json.Unmarshal(apiRec.Body.Bytes(), &errResp); err == nil {
		if errResp["error"] != "unauthorized" {
			t.Errorf("Expected error 'unauthorized', got %q", errResp["error"])
		}
		t.Logf("✓ Got expected error: %s - %s", errResp["error"], errResp["message"])
	}
}

// mockAuthServiceReturningError is a mock that returns error from ValidateRequest.
type mockAuthServiceReturningError struct{}

func (m *mockAuthServiceReturningError) ValidateRequest(r *http.Request) (*auth.Claims, string, error) {
	return nil, "", auth.ErrMissingAuthorization
}

func (m *mockAuthServiceReturningError) RequireProjectID(claims *auth.Claims) error {
	if claims == nil || claims.ProjectID == "" {
		return auth.ErrMissingProjectID
	}
	return nil
}

func (m *mockAuthServiceReturningError) ValidateProjectIDMatch(claims *auth.Claims, urlProjectID string) error {
	if urlProjectID != "" && claims != nil && claims.ProjectID != urlProjectID {
		return auth.ErrProjectIDMismatch
	}
	return nil
}
