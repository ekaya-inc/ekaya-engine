package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// RefreshAzureTokenResponse represents the response from the token refresh endpoint.
type RefreshAzureTokenResponse struct {
	AccessToken         string `json:"access_token"`
	TokenType           string `json:"token_type"`
	ExpiresIn           int    `json:"expires_in"`
	AzureTokenRefreshed bool   `json:"azure_token_refreshed"`
}

// RefreshAzureToken automatically refreshes an expired Azure token by calling the refresh endpoint.
// This mirrors how JWT expiration is handled automatically by the jwt-go library.
// Returns the new Azure access token and expiry, or an error if refresh fails.
func RefreshAzureToken(ctx context.Context) (string, int64, error) {
	// Get current JWT token from context
	currentToken, hasToken := GetToken(ctx)
	if !hasToken || currentToken == "" {
		return "", 0, fmt.Errorf("no JWT token in context for refresh")
	}

	// Get claims to extract PAPI (auth server URL)
	claims, hasClaims := GetClaims(ctx)
	if !hasClaims || claims == nil {
		return "", 0, fmt.Errorf("no claims in context for refresh")
	}

	if claims.PAPI == "" {
		return "", 0, fmt.Errorf("no auth server URL (PAPI) in claims")
	}

	// Build refresh endpoint URL
	// The endpoint automatically refreshes Azure token if expired
	refreshURL := fmt.Sprintf("%s/project/token/refresh", claims.PAPI)

	// Create HTTP request (no body needed - endpoint auto-refreshes Azure token)
	req, err := http.NewRequestWithContext(ctx, "POST", refreshURL, nil)
	if err != nil {
		return "", 0, fmt.Errorf("create refresh request: %w", err)
	}

	// Set Authorization header with current JWT
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", currentToken))
	req.Header.Set("Content-Type", "application/json")

	// Make the request
	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody := new(bytes.Buffer)
		errorBody.ReadFrom(resp.Body)

		// Try to parse error response for better error message
		var errorResp struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal(errorBody.Bytes(), &errorResp); err == nil {
			if errorResp.Message != "" {
				return "", 0, fmt.Errorf("Azure token refresh failed: %s", errorResp.Message)
			}
			if errorResp.Error != "" {
				return "", 0, fmt.Errorf("Azure token refresh failed: %s", errorResp.Error)
			}
		}

		return "", 0, fmt.Errorf("refresh endpoint returned status %d: %s", resp.StatusCode, errorBody.String())
	}

	// Parse response
	var refreshResp RefreshAzureTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&refreshResp); err != nil {
		return "", 0, fmt.Errorf("decode refresh response: %w", err)
	}

	if refreshResp.AccessToken == "" {
		return "", 0, fmt.Errorf("refresh response missing access_token")
	}

	// Check if Azure token was refreshed
	if !refreshResp.AzureTokenRefreshed {
		// Azure token was not refreshed - this might mean:
		// 1. The Azure token refresh failed on the auth server
		// 2. The user needs to re-authenticate
		// 3. The refresh endpoint doesn't support Azure token refresh
		// We still try to extract it from the JWT in case it was included
	}

	// Parse the new JWT to extract azure_at and azure_exp
	// We use unverified parsing since we trust the auth server response
	// This mirrors the pattern used in jwks.go parseUnverifiedToken
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, _, err := parser.ParseUnverified(refreshResp.AccessToken, &Claims{})
	if err != nil {
		return "", 0, fmt.Errorf("parse refreshed JWT: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return "", 0, fmt.Errorf("invalid claims type in refreshed JWT")
	}

	// Extract Azure token and expiry from new JWT claims
	azureExp := claims.AzureTokenExpiry

	if claims.AzureTokenRefID == "" {
		// Check if the original JWT had a token reference
		originalClaims, hasOriginalClaims := GetClaims(ctx)
		hadOriginalToken := hasOriginalClaims && originalClaims != nil && originalClaims.AzureTokenRefID != ""

		// Provide a more helpful error message
		if !refreshResp.AzureTokenRefreshed {
			if hadOriginalToken {
				return "", 0, fmt.Errorf("refreshed JWT does not contain Azure token reference: Azure token refresh was not performed by auth server (azure_token_refreshed=false). The Azure token may have expired and could not be refreshed. User may need to re-authenticate")
			}
			return "", 0, fmt.Errorf("refreshed JWT does not contain Azure token reference: Azure token refresh was not performed by auth server. User may need to re-authenticate")
		}
		if hadOriginalToken {
			return "", 0, fmt.Errorf("refreshed JWT does not contain Azure token reference: Azure token reference was present in original JWT but missing in refreshed JWT")
		}
		return "", 0, fmt.Errorf("refreshed JWT does not contain Azure token reference")
	}

	// Fetch token using reference ID
	tokenClient := GetTokenClient()
	azureToken, err := tokenClient.FetchTokenByReference(ctx, claims.AzureTokenRefID, claims.PAPI, refreshResp.AccessToken)
	if err != nil {
		return "", 0, fmt.Errorf("fetch token by reference from refreshed JWT: %w", err)
	}

	return azureToken, azureExp, nil
}
