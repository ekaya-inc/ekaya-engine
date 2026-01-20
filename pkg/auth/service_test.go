package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// mockJWKSClient is a mock implementation of JWKSClientInterface for testing.
type mockJWKSClient struct {
	claims *Claims
	err    error
}

func (m *mockJWKSClient) ValidateToken(tokenString string) (*Claims, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.claims, nil
}

func (m *mockJWKSClient) Close() {}

func TestAuthService_ValidateRequest_Cookie(t *testing.T) {
	expectedClaims := &Claims{
		ProjectID: "project-123",
	}

	service := NewAuthService(&mockJWKSClient{claims: expectedClaims}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: "test-token"})

	claims, token, err := service.ValidateRequest(req)
	if err != nil {
		t.Fatalf("ValidateRequest failed: %v", err)
	}

	if token != "test-token" {
		t.Errorf("expected token 'test-token', got %q", token)
	}

	if claims.ProjectID != "project-123" {
		t.Errorf("expected ProjectID 'project-123', got %q", claims.ProjectID)
	}
}

func TestAuthService_ValidateRequest_AuthHeader(t *testing.T) {
	expectedClaims := &Claims{
		ProjectID: "project-456",
	}

	service := NewAuthService(&mockJWKSClient{claims: expectedClaims}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer my-jwt-token")

	claims, token, err := service.ValidateRequest(req)
	if err != nil {
		t.Fatalf("ValidateRequest failed: %v", err)
	}

	if token != "my-jwt-token" {
		t.Errorf("expected token 'my-jwt-token', got %q", token)
	}

	if claims.ProjectID != "project-456" {
		t.Errorf("expected ProjectID 'project-456', got %q", claims.ProjectID)
	}
}

func TestAuthService_ValidateRequest_AuthorizationHeaderTakesPrecedence(t *testing.T) {
	// When both Authorization header and cookie are present, header should win
	// This enables tab-scoped authentication via Bearer tokens while maintaining
	// backward compatibility with cookie-based auth during transition.
	expectedClaims := &Claims{
		ProjectID: "from-header",
	}

	service := NewAuthService(&mockJWKSClient{claims: expectedClaims}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer header-token")
	req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: "cookie-token"})

	_, token, err := service.ValidateRequest(req)
	if err != nil {
		t.Fatalf("ValidateRequest failed: %v", err)
	}

	if token != "header-token" {
		t.Errorf("expected Authorization header token to take precedence, got %q", token)
	}
}

func TestAuthService_ValidateRequest_FallsBackToCookie(t *testing.T) {
	// When only cookie is present (no Authorization header), cookie should be used
	// This provides backward compatibility during transition to Bearer tokens.
	expectedClaims := &Claims{
		ProjectID: "from-cookie",
	}

	service := NewAuthService(&mockJWKSClient{claims: expectedClaims}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	// No Authorization header
	req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: "cookie-token"})

	claims, token, err := service.ValidateRequest(req)
	if err != nil {
		t.Fatalf("ValidateRequest failed: %v", err)
	}

	if token != "cookie-token" {
		t.Errorf("expected cookie token, got %q", token)
	}

	if claims.ProjectID != "from-cookie" {
		t.Errorf("expected ProjectID 'from-cookie', got %q", claims.ProjectID)
	}
}

func TestAuthService_ValidateRequest_MissingAuth(t *testing.T) {
	// When neither Authorization header nor cookie is present, should return error
	service := NewAuthService(&mockJWKSClient{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	// No Authorization header, no cookie

	_, _, err := service.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for missing authorization")
	}

	if !errors.Is(err, ErrMissingAuthorization) {
		t.Errorf("expected ErrMissingAuthorization, got %v", err)
	}
}

func TestAuthService_ValidateRequest_InvalidAuthFormat(t *testing.T) {
	service := NewAuthService(&mockJWKSClient{}, zap.NewNop())

	tests := []struct {
		name   string
		header string
	}{
		{"no bearer prefix", "just-a-token"},
		{"wrong prefix", "Basic some-token"},
		{"missing token", "Bearer"},
		{"extra parts", "Bearer token extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Authorization", tt.header)

			_, _, err := service.ValidateRequest(req)
			if err == nil {
				t.Fatal("expected error for invalid auth format")
			}

			if !errors.Is(err, ErrInvalidAuthFormat) {
				t.Errorf("expected ErrInvalidAuthFormat, got %v", err)
			}
		})
	}
}

func TestAuthService_ValidateRequest_TokenValidationError(t *testing.T) {
	validationErr := errors.New("token expired")
	service := NewAuthService(&mockJWKSClient{err: validationErr}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")

	_, _, err := service.ValidateRequest(req)
	if err == nil {
		t.Fatal("expected error for token validation failure")
	}

	if !errors.Is(err, validationErr) {
		t.Errorf("expected token validation error, got %v", err)
	}
}

func TestAuthService_RequireProjectID(t *testing.T) {
	service := NewAuthService(&mockJWKSClient{}, zap.NewNop())

	tests := []struct {
		name      string
		projectID string
		wantErr   bool
	}{
		{"with project ID", "project-123", false},
		{"empty project ID", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &Claims{ProjectID: tt.projectID}
			err := service.RequireProjectID(claims)

			if tt.wantErr && err == nil {
				t.Error("expected error for missing project ID")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthService_ValidateProjectIDMatch(t *testing.T) {
	service := NewAuthService(&mockJWKSClient{}, zap.NewNop())

	tests := []struct {
		name        string
		tokenPID    string
		urlPID      string
		wantErr     bool
		expectedErr error
	}{
		{"matching IDs", "project-123", "project-123", false, nil},
		{"mismatched IDs", "project-123", "project-456", true, ErrProjectIDMismatch},
		{"empty URL ID (skipped)", "project-123", "", false, nil},
		{"empty token ID with URL ID", "", "project-456", true, ErrProjectIDMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &Claims{ProjectID: tt.tokenPID}
			err := service.ValidateProjectIDMatch(claims, tt.urlPID)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				} else if tt.expectedErr != nil && !errors.Is(err, tt.expectedErr) {
					t.Errorf("expected %v, got %v", tt.expectedErr, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAuthService_Interface(t *testing.T) {
	// Verify that authService implements AuthService
	service := NewAuthService(&mockJWKSClient{}, zap.NewNop())
	var _ AuthService = service
}
