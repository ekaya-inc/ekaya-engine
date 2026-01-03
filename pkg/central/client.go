// Package central provides a client for communicating with ekaya-central.
package central

import (
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

// ProjectInfo contains project information from ekaya-central.
type ProjectInfo struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	URLs        ProjectURLs `json:"urls,omitempty"`
}

// ProjectURLs contains URLs for navigating to ekaya-central pages.
type ProjectURLs struct {
	ProjectsPage string `json:"projectsPage,omitempty"`
	ProjectPage  string `json:"projectPage,omitempty"`
	Redirect     string `json:"redirect,omitempty"`
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
