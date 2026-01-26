// Package auth provides context helpers for extracting authentication information
// from request contexts. These helpers simplify access to JWT claims that are
// injected by the auth middleware.
//
// Example usage in a service:
//
//	func (s *Service) DoSomething(ctx context.Context, projectID, datasourceID uuid.UUID) error {
//	    // Extract user ID for connection manager
//	    userID, err := auth.RequireUserIDFromContext(ctx)
//	    if err != nil {
//	        return fmt.Errorf("authentication required: %w", err)
//	    }
//
//	    // Use userID for connection pooling
//	    pool, err := s.connMgr.GetOrCreatePool(ctx, projectID, userID, datasourceID, connStr)
//	    // ...
//	}
package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// GetUserIDFromContext extracts the user ID from JWT claims in the context.
// Returns empty string if not authenticated or claims are missing.
// Use this when you only need the user ID and can handle empty string gracefully.
func GetUserIDFromContext(ctx context.Context) string {
	claims, ok := GetClaims(ctx)
	if !ok || claims == nil {
		return ""
	}
	return claims.Subject
}

// GetProjectIDFromContext extracts the project ID from JWT claims in the context.
// Returns uuid.Nil if not authenticated or claims are missing.
// Use this when you only need the project ID and can handle uuid.Nil gracefully.
func GetProjectIDFromContext(ctx context.Context) uuid.UUID {
	claims, ok := GetClaims(ctx)
	if !ok || claims == nil {
		return uuid.Nil
	}

	if claims.ProjectID == "" {
		return uuid.Nil
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return uuid.Nil
	}

	return projectID
}

// RequireUserIDFromContext extracts the user ID from context and returns an error if not found.
// Use this when user ID is required for the operation.
func RequireUserIDFromContext(ctx context.Context) (string, error) {
	userID := GetUserIDFromContext(ctx)
	if userID == "" {
		return "", fmt.Errorf("user ID not found in context")
	}
	return userID, nil
}

// GetUserUUIDFromContext extracts the user ID from JWT claims and parses it as UUID.
// Returns the parsed UUID and true if successful, otherwise uuid.Nil and false.
// Use this when you need the user ID as a UUID for provenance tracking or database operations.
func GetUserUUIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userIDStr := GetUserIDFromContext(ctx)
	if userIDStr == "" {
		return uuid.Nil, false
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, false
	}

	return userID, true
}

// RequireUserUUIDFromContext extracts the user ID from context as a UUID and returns an error if not found or invalid.
// Use this when user UUID is required for the operation (e.g., provenance tracking).
func RequireUserUUIDFromContext(ctx context.Context) (uuid.UUID, error) {
	userID, ok := GetUserUUIDFromContext(ctx)
	if !ok {
		return uuid.Nil, fmt.Errorf("valid user UUID not found in context")
	}
	return userID, nil
}

// RequireProjectIDFromContext extracts the project ID from context and returns an error if not found.
// Use this when project ID is required for the operation.
func RequireProjectIDFromContext(ctx context.Context) (uuid.UUID, error) {
	projectID := GetProjectIDFromContext(ctx)
	if projectID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("project ID not found in context")
	}
	return projectID, nil
}

// RequireClaimsFromContext extracts both project ID and user ID from context with validation.
// Returns an error if either is missing or invalid.
// This is equivalent to ExtractClaimsFromContext but with a clearer name for required extraction.
func RequireClaimsFromContext(ctx context.Context) (projectID uuid.UUID, userID string, err error) {
	return ExtractClaimsFromContext(ctx)
}
