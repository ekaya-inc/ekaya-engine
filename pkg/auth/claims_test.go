package auth

import (
	"context"
	"testing"
)

func TestGetClaims_Success(t *testing.T) {
	claims := &Claims{ProjectID: "test-project"}
	claims.Subject = "user-123"

	ctx := context.WithValue(context.Background(), ClaimsKey, claims)

	got, ok := GetClaims(ctx)
	if !ok {
		t.Fatal("expected claims to be found")
	}
	if got.Subject != "user-123" {
		t.Errorf("expected subject 'user-123', got %q", got.Subject)
	}
	if got.ProjectID != "test-project" {
		t.Errorf("expected project ID 'test-project', got %q", got.ProjectID)
	}
}

func TestGetClaims_NotFound(t *testing.T) {
	ctx := context.Background()

	_, ok := GetClaims(ctx)
	if ok {
		t.Error("expected claims to not be found")
	}
}

func TestGetClaims_WrongType(t *testing.T) {
	// Context has wrong type for claims key
	ctx := context.WithValue(context.Background(), ClaimsKey, "not-a-claims-struct")

	_, ok := GetClaims(ctx)
	if ok {
		t.Error("expected claims to not be found when wrong type")
	}
}

func TestGetToken_Success(t *testing.T) {
	ctx := context.WithValue(context.Background(), TokenKey, "test-token-abc123")

	got, ok := GetToken(ctx)
	if !ok {
		t.Fatal("expected token to be found")
	}
	if got != "test-token-abc123" {
		t.Errorf("expected 'test-token-abc123', got %q", got)
	}
}

func TestGetToken_NotFound(t *testing.T) {
	ctx := context.Background()

	_, ok := GetToken(ctx)
	if ok {
		t.Error("expected token to not be found")
	}
}

func TestGetToken_WrongType(t *testing.T) {
	// Context has wrong type for token key
	ctx := context.WithValue(context.Background(), TokenKey, 12345)

	_, ok := GetToken(ctx)
	if ok {
		t.Error("expected token to not be found when wrong type")
	}
}
