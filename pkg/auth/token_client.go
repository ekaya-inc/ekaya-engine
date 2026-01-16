package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"
)

// DefaultTokenTimeout is the maximum time to wait for token reference responses from ekaya-central.
const DefaultTokenTimeout = 30 * time.Second

// TokenClient handles fetching Azure tokens by reference from ekaya-central
type TokenClient struct {
	httpClient *http.Client
}

// NewTokenClient creates a new token client with default timeout
func NewTokenClient() *TokenClient {
	return &TokenClient{
		httpClient: &http.Client{
			Timeout: DefaultTokenTimeout,
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
	endpoint, err := buildTokenURL(papiURL, refID)
	if err != nil {
		return "", fmt.Errorf("failed to build URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+jwtToken)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call ekaya-central: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("token reference not found or expired")
	}

	if resp.StatusCode == http.StatusForbidden {
		return "", fmt.Errorf("token reference does not belong to user")
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token fetch failed: status %d: %s", resp.StatusCode, string(body))
	}

	var result TokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Token == "" {
		return "", fmt.Errorf("empty token in response")
	}

	return result.Token, nil
}

// buildTokenURL constructs a URL by parsing the base and joining the tokens path with refID
func buildTokenURL(baseURL string, refID string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Join path segments: base path + "tokens" + refID
	segments := append([]string{u.Path}, "tokens", refID)
	u.Path = path.Join(segments...)

	return u.String(), nil
}
