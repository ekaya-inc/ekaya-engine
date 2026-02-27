package central

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestProjectInfo_UnmarshalWithApplications(t *testing.T) {
	body := `{
		"project": {
			"id": "test-id",
			"name": "Test Project",
			"applications": [
				{
					"name": "mcp-server",
					"billing": {
						"status": "dormant",
						"freeSeatsLimit": 2
					}
				}
			],
			"urls": {
				"projectsPage": "https://example.com/projects",
				"projectPage": "https://example.com/projects/test-id"
			}
		}
	}`

	var response struct {
		Project ProjectInfo `json:"project"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	p := response.Project
	if p.ID != "test-id" {
		t.Errorf("expected ID %q, got %q", "test-id", p.ID)
	}
	if p.Name != "Test Project" {
		t.Errorf("expected Name %q, got %q", "Test Project", p.Name)
	}
	if len(p.Applications) != 1 {
		t.Fatalf("expected 1 application, got %d", len(p.Applications))
	}

	app := p.Applications[0]
	if app.Name != "mcp-server" {
		t.Errorf("expected application name %q, got %q", "mcp-server", app.Name)
	}
	if app.Billing == nil {
		t.Fatal("expected billing info, got nil")
	}
	if app.Billing.Status != "dormant" {
		t.Errorf("expected billing status %q, got %q", "dormant", app.Billing.Status)
	}
	if app.Billing.FreeSeatsLimit != 2 {
		t.Errorf("expected freeSeatsLimit %d, got %d", 2, app.Billing.FreeSeatsLimit)
	}
}

func TestProjectInfo_UnmarshalWithoutApplications(t *testing.T) {
	body := `{
		"project": {
			"id": "test-id",
			"name": "Test Project",
			"urls": {
				"projectsPage": "https://example.com/projects"
			}
		}
	}`

	var response struct {
		Project ProjectInfo `json:"project"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if response.Project.Applications != nil {
		t.Errorf("expected nil applications, got %v", response.Project.Applications)
	}
}

func TestProjectInfo_UnmarshalMultipleApplications(t *testing.T) {
	body := `{
		"project": {
			"id": "test-id",
			"name": "Test Project",
			"applications": [
				{"name": "mcp-server"},
				{"name": "ai-data-liaison", "billing": {"status": "active", "freeSeatsLimit": 5}}
			]
		}
	}`

	var response struct {
		Project ProjectInfo `json:"project"`
	}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(response.Project.Applications) != 2 {
		t.Fatalf("expected 2 applications, got %d", len(response.Project.Applications))
	}

	if response.Project.Applications[0].Name != "mcp-server" {
		t.Errorf("expected first app %q, got %q", "mcp-server", response.Project.Applications[0].Name)
	}
	if response.Project.Applications[0].Billing != nil {
		t.Error("expected nil billing for mcp-server (not provided)")
	}

	if response.Project.Applications[1].Name != "ai-data-liaison" {
		t.Errorf("expected second app %q, got %q", "ai-data-liaison", response.Project.Applications[1].Name)
	}
	if response.Project.Applications[1].Billing == nil {
		t.Fatal("expected billing for ai-data-liaison")
	}
	if response.Project.Applications[1].Billing.Status != "active" {
		t.Errorf("expected billing status %q, got %q", "active", response.Project.Applications[1].Billing.Status)
	}
}

// newTestClient creates a Client with a nop logger for testing.
func newTestClient() *Client {
	logger := zap.NewNop()
	c := NewClient(logger)
	c.httpClient = &http.Client{}
	return c
}

// --- doProjectRequest error paths (via GetProject) ---

func TestGetProject_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"project":{"id":"proj-1","name":"My Project","urls":{"projectsPage":"https://example.com/projects"}}}`))
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.GetProject(context.Background(), server.URL, "proj-1", "test-token")
	require.NoError(t, err)
	assert.Equal(t, "proj-1", info.ID)
	assert.Equal(t, "My Project", info.Name)
	assert.Equal(t, "https://example.com/projects", info.URLs.ProjectsPage)
}

func TestGetProject_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.GetProject(context.Background(), server.URL, "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestGetProject_HTTP404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.GetProject(context.Background(), server.URL, "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 404")
}

func TestGetProject_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.GetProject(context.Background(), server.URL, "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestGetProject_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// empty body
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.GetProject(context.Background(), server.URL, "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestGetProject_InvalidBaseURL(t *testing.T) {
	c := newTestClient()
	info, err := c.GetProject(context.Background(), "://bad-url", "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build URL")
}

// --- ProvisionProject error paths ---

func TestProvisionProject_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "provision")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"project":{"id":"proj-1","name":"Provisioned"}}`))
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.ProvisionProject(context.Background(), server.URL, "proj-1", "test-token")
	require.NoError(t, err)
	assert.Equal(t, "proj-1", info.ID)
	assert.Equal(t, "Provisioned", info.Name)
}

func TestProvisionProject_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.ProvisionProject(context.Background(), server.URL, "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

// --- UpdateServerUrl error paths ---

func TestUpdateServerUrl_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"project":{"id":"proj-1","name":"Updated"}}`))
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.UpdateServerUrl(context.Background(), server.URL, "proj-1", "https://new.example.com", "test-token")
	require.NoError(t, err)
	assert.Equal(t, "Updated", info.Name)
}

func TestUpdateServerUrl_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient()
	info, err := c.UpdateServerUrl(context.Background(), server.URL, "proj-1", "https://new.example.com", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

// --- doAppAction error paths (via InstallApp) ---

func TestInstallApp_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "install")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"installed","redirectUrl":"https://example.com/redirect"}`))
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.InstallApp(context.Background(), server.URL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	require.NoError(t, err)
	assert.Equal(t, "installed", resp.Status)
	assert.Equal(t, "https://example.com/redirect", resp.RedirectUrl)
}

func TestInstallApp_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.InstallApp(context.Background(), server.URL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestInstallApp_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{broken json`))
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.InstallApp(context.Background(), server.URL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestInstallApp_EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// empty body
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.InstallApp(context.Background(), server.URL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestActivateApp_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "activate")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.ActivateApp(context.Background(), server.URL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestUninstallApp_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "uninstall")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.UninstallApp(context.Background(), server.URL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

// --- DeleteProject error paths ---

func TestDeleteProject_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.URL.Path, "delete")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"pending_deletion","redirectUrl":"https://example.com/confirm"}`))
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.DeleteProject(context.Background(), server.URL, "proj-1", "test-token", "https://callback.example.com")
	require.NoError(t, err)
	assert.Equal(t, "pending_deletion", resp.Status)
	assert.Equal(t, "https://example.com/confirm", resp.RedirectUrl)
}

func TestDeleteProject_HTTP500(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`server error`))
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.DeleteProject(context.Background(), server.URL, "proj-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestDeleteProject_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	c := newTestClient()
	resp, err := c.DeleteProject(context.Background(), server.URL, "proj-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse response")
}

func TestDeleteProject_InvalidBaseURL(t *testing.T) {
	c := newTestClient()
	resp, err := c.DeleteProject(context.Background(), "://bad-url", "proj-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to build URL")
}

// --- buildURL tests ---

func TestBuildURL_ValidBaseAndSegments(t *testing.T) {
	result, err := buildURL("https://central.example.com", "api", "v1", "projects", "abc")
	require.NoError(t, err)
	assert.Equal(t, "https://central.example.com/api/v1/projects/abc", result)
}

func TestBuildURL_BaseWithExistingPath(t *testing.T) {
	result, err := buildURL("https://central.example.com/base", "api", "v1")
	require.NoError(t, err)
	assert.Equal(t, "https://central.example.com/base/api/v1", result)
}

func TestBuildURL_InvalidBaseURL(t *testing.T) {
	_, err := buildURL("://invalid", "api")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base URL")
}

func TestBuildURL_EmptySegments(t *testing.T) {
	result, err := buildURL("https://central.example.com")
	require.NoError(t, err)
	assert.Equal(t, "https://central.example.com", result)
}

// --- Connection error (server not reachable) ---

func TestGetProject_ConnectionError(t *testing.T) {
	// Start and immediately close a server to get an unreachable URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	c := newTestClient()
	info, err := c.GetProject(context.Background(), serverURL, "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to call ekaya-central")
}

func TestInstallApp_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	c := newTestClient()
	resp, err := c.InstallApp(context.Background(), serverURL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to call ekaya-central")
}

func TestDeleteProject_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	serverURL := server.URL
	server.Close()

	c := newTestClient()
	resp, err := c.DeleteProject(context.Background(), serverURL, "proj-1", "test-token", "https://callback.example.com")
	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to call ekaya-central")
}

// --- Context cancellation ---

func TestGetProject_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"project":{"id":"proj-1","name":"Test"}}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	c := newTestClient()
	info, err := c.GetProject(ctx, server.URL, "proj-1", "test-token")
	assert.Nil(t, info)
	require.Error(t, err)
}

// --- Verify request path construction ---

func TestGetProject_RequestPath(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"project":{"id":"proj-1","name":"Test"}}`))
	}))
	defer server.Close()

	c := newTestClient()
	_, _ = c.GetProject(context.Background(), server.URL, "my-project-id", "test-token")
	assert.Equal(t, "/api/v1/projects/my-project-id", receivedPath)
}

func TestProvisionProject_RequestPath(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"project":{"id":"proj-1","name":"Test"}}`))
	}))
	defer server.Close()

	c := newTestClient()
	_, _ = c.ProvisionProject(context.Background(), server.URL, "my-project-id", "test-token")
	assert.Equal(t, "/api/v1/projects/my-project-id/provision", receivedPath)
}

func TestInstallApp_RequestPath(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"installed"}`))
	}))
	defer server.Close()

	c := newTestClient()
	_, _ = c.InstallApp(context.Background(), server.URL, "proj-1", "app-1", "test-token", "https://callback.example.com")
	assert.Equal(t, "/api/v1/projects/proj-1/apps/app-1/install", receivedPath)
}

func TestDeleteProject_RequestPath(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"deleted"}`))
	}))
	defer server.Close()

	c := newTestClient()
	_, _ = c.DeleteProject(context.Background(), server.URL, "proj-1", "test-token", "https://callback.example.com")
	assert.Equal(t, "/api/v1/projects/proj-1/delete", receivedPath)
}
