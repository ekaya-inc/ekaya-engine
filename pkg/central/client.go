// Package central provides a client for communicating with ekaya-central.
package central

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"go.uber.org/zap"
)

// DefaultTimeout is the maximum time to wait for ekaya-central responses.
const DefaultTimeout = 30 * time.Second

// Application name constants for known application types.
const (
	AppMCPServer     = "mcp-server"
	AppAIDataLiaison = "ai-data-liaison"
	AppAIAgents      = "ai-agents"
)

// ApplicationInfo describes an application assigned to a project by ekaya-central.
type ApplicationInfo struct {
	Name    string       `json:"name"`
	Billing *BillingInfo `json:"billing,omitempty"`
}

// BillingInfo contains billing status from ekaya-central.
type BillingInfo struct {
	Status         string `json:"status"`
	FreeSeatsLimit int    `json:"freeSeatsLimit"`
}

// ProjectInfo contains project information from ekaya-central.
type ProjectInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description,omitempty"`
	Applications []ApplicationInfo `json:"applications,omitempty"`
	URLs         ProjectURLs       `json:"urls,omitempty"`
}

// AppActionResponse is the response from app lifecycle endpoints (install, activate, uninstall).
type AppActionResponse struct {
	Status      string `json:"status"`                // e.g. "installed", "activated", "pending_activation"
	RedirectUrl string `json:"redirectUrl,omitempty"` // if present, redirect user here
}

// ProjectURLs contains URLs for navigating to ekaya-central pages.
type ProjectURLs struct {
	ProjectsPage  string `json:"projectsPage,omitempty"`
	ProjectPage   string `json:"projectPage,omitempty"`
	Redirect      string `json:"redirect,omitempty"`
	AuthServerURL string `json:"authServerUrl,omitempty"`
}

// Client provides access to ekaya-central API.
type Client struct {
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a new ekaya-central client.
func NewClient(logger *zap.Logger) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		logger: logger.Named("central"),
	}
}

// ProvisionProject notifies ekaya-central that this project is being provisioned in ekaya-engine.
// This should be called on first access to a project.
// baseURL is the ekaya-central API base URL (from JWT papi claim).
// projectID is the project UUID.
// token is the JWT token for authentication.
func (c *Client) ProvisionProject(ctx context.Context, baseURL, projectID, token string) (*ProjectInfo, error) {
	endpoint, err := buildURL(baseURL, "api", "v1", "projects", projectID, "provision")
	if err != nil {
		return nil, fmt.Errorf("failed to build URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	c.logger.Info("Provisioning project with ekaya-central",
		zap.String("url", endpoint),
		zap.String("project_id", projectID))

	return c.doProjectRequest(req, projectID)
}

// GetProject fetches project information from ekaya-central.
// Use this for background sync, not initial provisioning.
// baseURL is the ekaya-central API base URL (from JWT papi claim).
// projectID is the project UUID.
// token is the JWT token for authentication.
func (c *Client) GetProject(ctx context.Context, baseURL, projectID, token string) (*ProjectInfo, error) {
	endpoint, err := buildURL(baseURL, "api", "v1", "projects", projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to build URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	c.logger.Debug("Fetching project from ekaya-central",
		zap.String("url", endpoint),
		zap.String("project_id", projectID))

	return c.doProjectRequest(req, projectID)
}

// UpdateServerUrl updates the project's serverUrl in ekaya-central.
// This is called when the admin configures HTTPS and wants to sync the new URL
// so that ekaya-central redirect URLs and MCP setup links are correct.
// baseURL is the ekaya-central API base URL (from JWT papi claim).
// projectID is the project UUID.
// serverURL is the new server URL (e.g., "https://data.acme.com").
// token is the JWT token for authentication (must have admin role).
func (c *Client) UpdateServerUrl(ctx context.Context, baseURL, projectID, serverURL, token string) (*ProjectInfo, error) {
	endpoint, err := buildURL(baseURL, "api", "v1", "projects", projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to build URL: %w", err)
	}

	body, err := json.Marshal(map[string]string{"serverUrl": serverURL})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	c.logger.Info("Updating server URL in ekaya-central",
		zap.String("url", endpoint),
		zap.String("project_id", projectID),
		zap.String("server_url", serverURL))

	return c.doProjectRequest(req, projectID)
}

// InstallApp notifies ekaya-central that an application is being installed.
// callbackUrl is the engine URL that central should redirect to if user interaction is needed.
func (c *Client) InstallApp(ctx context.Context, baseURL, projectID, appID, token, callbackUrl string) (*AppActionResponse, error) {
	return c.doAppAction(ctx, baseURL, projectID, appID, "install", token, callbackUrl)
}

// ActivateApp notifies ekaya-central that an application is being activated.
// callbackUrl is the engine URL that central should redirect to if user interaction is needed.
func (c *Client) ActivateApp(ctx context.Context, baseURL, projectID, appID, token, callbackUrl string) (*AppActionResponse, error) {
	return c.doAppAction(ctx, baseURL, projectID, appID, "activate", token, callbackUrl)
}

// UninstallApp notifies ekaya-central that an application is being uninstalled.
// callbackUrl is the engine URL that central should redirect to if user interaction is needed.
func (c *Client) UninstallApp(ctx context.Context, baseURL, projectID, appID, token, callbackUrl string) (*AppActionResponse, error) {
	return c.doAppAction(ctx, baseURL, projectID, appID, "uninstall", token, callbackUrl)
}

// DeleteProject notifies ekaya-central that a project is being deleted.
// Central returns a redirect URL where the user confirms deletion (billing cleanup, etc.).
// callbackUrl is the engine URL that central should redirect to after the user confirms or cancels.
func (c *Client) DeleteProject(ctx context.Context, baseURL, projectID, token, callbackUrl string) (*AppActionResponse, error) {
	endpoint, err := buildURL(baseURL, "api", "v1", "projects", projectID, "delete")
	if err != nil {
		return nil, fmt.Errorf("failed to build URL: %w", err)
	}

	body, err := json.Marshal(map[string]string{"callbackUrl": callbackUrl})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	c.logger.Info("Calling ekaya-central project delete",
		zap.String("url", endpoint),
		zap.String("project_id", projectID))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call ekaya-central: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("ekaya-central project delete failed",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)))
		return nil, fmt.Errorf("ekaya-central returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result AppActionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Info("ekaya-central project delete response",
		zap.String("status", result.Status),
		zap.Bool("has_redirect", result.RedirectUrl != ""))

	return &result, nil
}

// doAppAction executes an app lifecycle action (install, activate, uninstall) against ekaya-central.
func (c *Client) doAppAction(ctx context.Context, baseURL, projectID, appID, action, token, callbackUrl string) (*AppActionResponse, error) {
	endpoint, err := buildURL(baseURL, "api", "v1", "projects", projectID, "apps", appID, action)
	if err != nil {
		return nil, fmt.Errorf("failed to build URL: %w", err)
	}

	body, err := json.Marshal(map[string]string{"callbackUrl": callbackUrl})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	c.logger.Info("Calling ekaya-central app action",
		zap.String("action", action),
		zap.String("url", endpoint),
		zap.String("project_id", projectID),
		zap.String("app_id", appID))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call ekaya-central: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("ekaya-central app action failed",
			zap.String("action", action),
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(respBody)))
		return nil, fmt.Errorf("ekaya-central returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result AppActionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Info("ekaya-central app action completed",
		zap.String("action", action),
		zap.String("status", result.Status),
		zap.Bool("has_redirect", result.RedirectUrl != ""))

	return &result, nil
}

// doProjectRequest executes an HTTP request and parses the project response.
func (c *Client) doProjectRequest(req *http.Request, projectID string) (*ProjectInfo, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call ekaya-central: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.logger.Error("ekaya-central returned error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return nil, fmt.Errorf("ekaya-central returned status %d: %s", resp.StatusCode, string(body))
	}

	// Response format: { "project": { "id": "...", "name": "...", "urls": { ... } } }
	var response struct {
		Project ProjectInfo `json:"project"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	c.logger.Debug("Got project from ekaya-central",
		zap.String("project_id", response.Project.ID),
		zap.String("name", response.Project.Name),
		zap.String("projects_page_url", response.Project.URLs.ProjectsPage),
		zap.String("project_page_url", response.Project.URLs.ProjectPage))

	return &response.Project, nil
}

// buildURL constructs a URL by parsing the base and joining path segments.
func buildURL(baseURL string, pathSegments ...string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}

	// Join all path segments
	segments := append([]string{u.Path}, pathSegments...)
	u.Path = path.Join(segments...)

	return u.String(), nil
}
