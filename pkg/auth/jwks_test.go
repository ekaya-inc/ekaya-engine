package auth

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// createTestToken creates a JWT token for testing (unsigned, for dev mode).
func createTestToken(claims *Claims) string {
	// Create header
	header := map[string]string{
		"alg": "none",
		"typ": "JWT",
	}
	headerJSON, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)

	// Create claims
	claimsJSON, _ := json.Marshal(claims)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Return unsigned token (header.claims.)
	return headerB64 + "." + claimsB64 + "."
}

func TestNewJWKSClient_DevMode(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
		JWKSEndpoints:      nil,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestJWKSClient_ValidateToken_DevMode(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
		JWKSEndpoints:      nil,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	testClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-123",
			Issuer:    "https://auth.example.com",
			Audience:  jwt.ClaimStrings{"engine"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		ProjectID: "550e8400-e29b-41d4-a716-446655440000",
		Email:     "user@example.com",
		Roles:     []string{"admin", "user"},
	}

	token := createTestToken(testClaims)

	claims, err := client.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	if claims.Subject != "user-123" {
		t.Errorf("expected Subject 'user-123', got %q", claims.Subject)
	}
	if claims.ProjectID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("expected ProjectID '550e8400-e29b-41d4-a716-446655440000', got %q", claims.ProjectID)
	}
	if claims.Email != "user@example.com" {
		t.Errorf("expected Email 'user@example.com', got %q", claims.Email)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "admin" {
		t.Errorf("expected Roles ['admin', 'user'], got %v", claims.Roles)
	}
}

func TestJWKSClient_ValidateToken_InvalidFormat(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
		JWKSEndpoints:      nil,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	_, err = client.ValidateToken("not-a-valid-token")
	if err == nil {
		t.Error("expected error for invalid token format")
	}
}

func TestJWKSClient_ValidateToken_EmptyToken(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
		JWKSEndpoints:      nil,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	_, err = client.ValidateToken("")
	if err == nil {
		t.Error("expected error for empty token")
	}
}

func TestJWKSClient_ValidateToken_MalformedBase64(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
		JWKSEndpoints:      nil,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	// Token with invalid base64 in claims section
	_, err = client.ValidateToken("eyJhbGciOiJub25lIn0.!!!invalid!!!.")
	if err == nil {
		t.Error("expected error for malformed base64")
	}
}

func TestJWKSClient_Interface(t *testing.T) {
	// Verify that JWKSClient implements JWKSClientInterface
	config := &JWKSConfig{
		EnableVerification: false,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}

	var _ JWKSClientInterface = client
}

func TestJWKSClient_ParsesAllClaimFields(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	testClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-456",
			Issuer:    "https://auth.ekaya.ai",
			Audience:  jwt.ClaimStrings{"engine", "other-service"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		ProjectID:     "project-uuid-here",
		Email:         "test@ekaya.ai",
		ProjectRegion: "us-central1",
		Roles:         []string{"owner"},
		PAPI:          "https://api.ekaya.ai",
	}

	token := createTestToken(testClaims)
	claims, err := client.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}

	// Verify all custom claims are parsed
	if claims.ProjectID != "project-uuid-here" {
		t.Errorf("ProjectID mismatch: got %q", claims.ProjectID)
	}
	if claims.Email != "test@ekaya.ai" {
		t.Errorf("Email mismatch: got %q", claims.Email)
	}
	if claims.ProjectRegion != "us-central1" {
		t.Errorf("ProjectRegion mismatch: got %q", claims.ProjectRegion)
	}
	if claims.PAPI != "https://api.ekaya.ai" {
		t.Errorf("PAPI mismatch: got %q", claims.PAPI)
	}
	if len(claims.Roles) != 1 || claims.Roles[0] != "owner" {
		t.Errorf("Roles mismatch: got %v", claims.Roles)
	}
}

func TestJWKSClient_ValidateToken_InvalidAudience(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	// Token with wrong audience (not "engine")
	testClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  "user-123",
			Issuer:   "https://auth.example.com",
			Audience: jwt.ClaimStrings{"other-service"},
		},
		ProjectID: "project-123",
	}

	token := createTestToken(testClaims)
	_, err = client.ValidateToken(token)
	if err == nil {
		t.Error("expected error for invalid audience")
	}
	if err != ErrInvalidAudience {
		t.Errorf("expected ErrInvalidAudience, got: %v", err)
	}
}

func TestJWKSClient_ValidateToken_MissingAudience(t *testing.T) {
	config := &JWKSConfig{
		EnableVerification: false,
	}

	client, err := NewJWKSClient(config)
	if err != nil {
		t.Fatalf("NewJWKSClient failed: %v", err)
	}
	defer client.Close()

	// Token with no audience
	testClaims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "user-123",
			Issuer:  "https://auth.example.com",
		},
		ProjectID: "project-123",
	}

	token := createTestToken(testClaims)
	_, err = client.ValidateToken(token)
	if err == nil {
		t.Error("expected error for missing audience")
	}
	if err != ErrInvalidAudience {
		t.Errorf("expected ErrInvalidAudience, got: %v", err)
	}
}

func TestNewJWKSClient_InvalidEndpoint(t *testing.T) {
	// Note: keyfunc.NewDefaultCtx may succeed even with invalid URLs because
	// it uses background refresh. This test verifies the behavior when
	// the JWKS URL is completely malformed (not just unreachable).
	config := &JWKSConfig{
		EnableVerification: true,
		JWKSEndpoints: map[string]string{
			"https://invalid.example.com": "not-a-valid-url",
		},
	}

	_, err := NewJWKSClient(config)
	// The keyfunc library may or may not error on invalid URLs depending
	// on how it handles background refresh. We accept either outcome.
	// The important thing is it doesn't panic.
	if err != nil {
		if !strings.Contains(err.Error(), "failed to create JWKS client") {
			t.Errorf("expected 'failed to create JWKS client' in error, got: %v", err)
		}
	}
}
