// Package auth provides JWT-based authentication for ekaya-engine.
// It validates tokens issued by ekaya-central using JWKS endpoints.
package auth

import (
	"context"
	"fmt"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// ClaimsKey is the context key for storing JWT claims.
	ClaimsKey contextKey = "claims"
	// TokenKey is the context key for storing the raw JWT token string.
	TokenKey contextKey = "token"
	// AzureAccessTokenKey is the context key for storing Azure AD access token.
	AzureAccessTokenKey contextKey = "azure_access_token"
)

// Claims represents the JWT claims structure from ekaya-central.
// It embeds RegisteredClaims for standard JWT fields (sub, iss, exp, etc.)
// and adds custom claims for project context.
type Claims struct {
	jwt.RegisteredClaims
	ProjectID     string   `json:"pid,omitempty"`   // Project UUID
	Email         string   `json:"email,omitempty"` // User email address
	ProjectRegion string   `json:"preg,omitempty"`  // Region where project is hosted
	Roles         []string `json:"roles,omitempty"` // User roles within the project
	PAPI          string   `json:"papi,omitempty"`  // ekaya-central API base URL

	// Azure AD tokens for SQL Server authentication (optional, only present if user requested Azure SQL scope)
	Scope             string `json:"scp,omitempty"`       // OAuth scope (e.g., "project:access https://database.windows.net/.default")
	AzureAccessToken  string `json:"azure_at,omitempty"`  // Azure AD access token for SQL Server
	AzureRefreshToken string `json:"azure_rt,omitempty"`  // Azure AD refresh token for token renewal
	AzureTokenExpiry  int64  `json:"azure_exp,omitempty"` // Azure token expiration timestamp (Unix epoch)
}

// GetClaims retrieves JWT claims from the request context.
// Returns nil and false if claims are not present.
func GetClaims(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(ClaimsKey).(*Claims)
	return claims, ok
}

// GetToken retrieves the raw JWT token string from the request context.
// Returns empty string and false if token is not present.
func GetToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(TokenKey).(string)
	return token, ok
}

// GetAzureAccessToken retrieves the Azure AD access token from the request context.
// Returns empty string and false if token is not present.
func GetAzureAccessToken(ctx context.Context) (string, bool) {
	token, ok := ctx.Value(AzureAccessTokenKey).(string)
	return token, ok
}

// ExtractClaimsFromContext extracts project ID and user ID from JWT claims in context.
// Returns error if not authenticated or claims are invalid.
func ExtractClaimsFromContext(ctx context.Context) (uuid.UUID, string, error) {
	claims, ok := GetClaims(ctx)
	if !ok || claims == nil {
		return uuid.Nil, "", fmt.Errorf("authentication required: no claims in context")
	}

	if claims.ProjectID == "" {
		return uuid.Nil, "", fmt.Errorf("missing project ID in JWT claims")
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("invalid project ID format: %w", err)
	}

	userID := claims.Subject
	if userID == "" {
		return uuid.Nil, "", fmt.Errorf("missing user ID in JWT claims")
	}

	return projectID, userID, nil
}
