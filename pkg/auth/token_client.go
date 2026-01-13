package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// TokenClient handles fetching Azure tokens by reference from ekaya-central
type TokenClient struct {
	httpClient *http.Client
}

// NewTokenClient creates a new token client
func NewTokenClient() *TokenClient {
	return &TokenClient{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// TokenResponse represents the response from the token reference endpoint
type TokenResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
}

// FetchTokenByReference fetches Azure token from ekaya-central using reference ID
func (c *TokenClient) FetchTokenByReference(ctx context.Context, refID string, papiURL string, jwtToken string) (string, error) {
	url := fmt.Sprintf("%s/tokens/%s", papiURL, refID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", jwtToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("token reference not found or expired")
	}

	if resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("token reference does not belong to user")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token fetch failed: status %d", resp.StatusCode)
	}

	var result TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	if result.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}

	return result.Token, nil
}
