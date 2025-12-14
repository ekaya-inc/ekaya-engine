// Package testhelpers provides utilities for testing ekaya-engine components.
package testhelpers

import (
	"encoding/base64"
	"fmt"
)

// GenerateTestJWT creates a test JWT token for use when verification is disabled.
// The token has a valid structure but no signature (alg: none).
// This is useful for testing auth flows without needing real JWKS validation.
// The token includes aud: "engine" which is required for validation.
func GenerateTestJWT(sub, projectID, email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))

	payload := fmt.Sprintf(`{"sub":"%s","aud":"engine"`, sub)
	if projectID != "" {
		payload += fmt.Sprintf(`,"pid":"%s"`, projectID)
	}
	if email != "" {
		payload += fmt.Sprintf(`,"email":"%s"`, email)
	}
	payload += "}"

	encodedPayload := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return fmt.Sprintf("%s.%s.", header, encodedPayload)
}

// GenerateTestJWTWithBearer returns token with "Bearer " prefix for Authorization header.
func GenerateTestJWTWithBearer(sub, projectID, email string) string {
	return "Bearer " + GenerateTestJWT(sub, projectID, email)
}
