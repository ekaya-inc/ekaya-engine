package tools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// withClaims adds auth claims to the context.
func withClaims(ctx context.Context, projectID string) context.Context {
	claims := &auth.Claims{ProjectID: projectID}
	return context.WithValue(ctx, auth.ClaimsKey, claims)
}

func TestRegisterHealthTool(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	// Pass nil deps for basic registration test
	RegisterHealthTool(mcpServer, "test-version", nil)

	// Verify tool is registered by calling tools/list
	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	// Marshal the result back to JSON for parsing
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Parse the response to verify the health tool is present
	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resultBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	found := false
	for _, tool := range response.Result.Tools {
		if tool.Name == "health" {
			found = true
			if tool.Description != "Returns server health status, version, and datasource connectivity" {
				t.Errorf("unexpected description: %s", tool.Description)
			}
			break
		}
	}
	if !found {
		t.Error("health tool not found in tools/list response")
	}
}

func TestHealthTool_Execute(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	// Pass nil deps - datasource health check will be skipped
	RegisterHealthTool(mcpServer, "1.2.3", nil)

	ctx := context.Background()
	request := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"health"},"id":1}`
	result := mcpServer.HandleMessage(ctx, []byte(request))

	// Marshal the result back to JSON for parsing
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Parse the response
	var response struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resultBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(response.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	content := response.Result.Content[0]
	if content.Type != "text" {
		t.Errorf("expected content type 'text', got '%s'", content.Type)
	}

	// Parse the health result
	var health struct {
		Engine  string `json:"engine"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(content.Text), &health); err != nil {
		t.Fatalf("failed to unmarshal health result: %v", err)
	}

	if health.Engine != "healthy" {
		t.Errorf("expected engine 'healthy', got '%s'", health.Engine)
	}
	if health.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got '%s'", health.Version)
	}
}

func TestHealthTool_VersionWithSpecialChars(t *testing.T) {
	// Test that version with special characters is properly JSON-escaped
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	versionWithQuotes := `1.0.0-beta"test`
	RegisterHealthTool(mcpServer, versionWithQuotes, nil)

	ctx := context.Background()
	request := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"health"},"id":1}`
	result := mcpServer.HandleMessage(ctx, []byte(request))

	// Marshal the result back to JSON for parsing
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Parse the response
	var response struct {
		Result struct {
			Content []mcp.TextContent `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resultBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(response.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	// Parse the health result - this should work because we use json.Marshal
	var health healthResult
	if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &health); err != nil {
		t.Fatalf("failed to unmarshal health result with special chars: %v", err)
	}

	if health.Version != versionWithQuotes {
		t.Errorf("expected version %q, got %q", versionWithQuotes, health.Version)
	}
}

func TestHealthTool_DatasourceConnected(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	deps := &HealthToolDeps{
		ProjectService: &mockProjectService{
			defaultDatasourceID: datasourceID,
		},
		DatasourceService: &mockDatasourceService{
			datasource: &models.Datasource{
				ID:             datasourceID,
				Name:           "test-db",
				DatasourceType: "postgres",
				Config:         map[string]any{"host": "localhost"},
			},
			connectionError: nil, // Connection succeeds
		},
		Logger: zap.NewNop(),
	}

	ctx := withClaims(context.Background(), projectID.String())
	result := checkDatasourceHealth(ctx, deps)

	if result.Status != "connected" {
		t.Errorf("expected status 'connected', got '%s'", result.Status)
	}
	if result.Name != "test-db" {
		t.Errorf("expected name 'test-db', got '%s'", result.Name)
	}
	if result.Type != "postgres" {
		t.Errorf("expected type 'postgres', got '%s'", result.Type)
	}
	if result.Error != "" {
		t.Errorf("expected no error, got '%s'", result.Error)
	}
}

func TestHealthTool_DatasourceConnectionError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	deps := &HealthToolDeps{
		ProjectService: &mockProjectService{
			defaultDatasourceID: datasourceID,
		},
		DatasourceService: &mockDatasourceService{
			datasource: &models.Datasource{
				ID:             datasourceID,
				Name:           "test-db",
				DatasourceType: "postgres",
				Config:         map[string]any{"host": "localhost"},
			},
			connectionError: errors.New("connection refused"),
		},
		Logger: zap.NewNop(),
	}

	ctx := withClaims(context.Background(), projectID.String())
	result := checkDatasourceHealth(ctx, deps)

	if result.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", result.Status)
	}
	if result.Name != "test-db" {
		t.Errorf("expected name 'test-db', got '%s'", result.Name)
	}
	if result.Type != "postgres" {
		t.Errorf("expected type 'postgres', got '%s'", result.Type)
	}
	if result.Error != "connection refused" {
		t.Errorf("expected error 'connection refused', got '%s'", result.Error)
	}
}

func TestHealthTool_NoDatasourceConfigured(t *testing.T) {
	projectID := uuid.New()

	deps := &HealthToolDeps{
		ProjectService: &mockProjectService{
			defaultDatasourceID: uuid.Nil, // No datasource configured
		},
		DatasourceService: &mockDatasourceService{},
		Logger:            zap.NewNop(),
	}

	ctx := withClaims(context.Background(), projectID.String())
	result := checkDatasourceHealth(ctx, deps)

	if result.Status != "not_configured" {
		t.Errorf("expected status 'not_configured', got '%s'", result.Status)
	}
	if result.Error != "no default datasource configured for project" {
		t.Errorf("expected specific error message, got '%s'", result.Error)
	}
}

func TestHealthTool_GetDefaultDatasourceError(t *testing.T) {
	projectID := uuid.New()

	deps := &HealthToolDeps{
		ProjectService: &mockProjectService{
			defaultDatasourceError: errors.New("database connection failed"),
		},
		DatasourceService: &mockDatasourceService{},
		Logger:            zap.NewNop(),
	}

	ctx := withClaims(context.Background(), projectID.String())
	result := checkDatasourceHealth(ctx, deps)

	if result.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestHealthTool_GetDatasourceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	deps := &HealthToolDeps{
		ProjectService: &mockProjectService{
			defaultDatasourceID: datasourceID,
		},
		DatasourceService: &mockDatasourceService{
			getError: errors.New("datasource not found"),
		},
		Logger: zap.NewNop(),
	}

	ctx := withClaims(context.Background(), projectID.String())
	result := checkDatasourceHealth(ctx, deps)

	if result.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestHealthTool_NoAuthClaims(t *testing.T) {
	deps := &HealthToolDeps{
		ProjectService:    &mockProjectService{},
		DatasourceService: &mockDatasourceService{},
		Logger:            zap.NewNop(),
	}

	// Context without claims
	ctx := context.Background()
	result := checkDatasourceHealth(ctx, deps)

	if result.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", result.Status)
	}
	if result.Error != "authentication required" {
		t.Errorf("expected error 'authentication required', got '%s'", result.Error)
	}
}

func TestHealthTool_InvalidProjectID(t *testing.T) {
	deps := &HealthToolDeps{
		ProjectService:    &mockProjectService{},
		DatasourceService: &mockDatasourceService{},
		Logger:            zap.NewNop(),
	}

	// Context with invalid project ID
	ctx := withClaims(context.Background(), "not-a-uuid")
	result := checkDatasourceHealth(ctx, deps)

	if result.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", result.Status)
	}
	if result.Error == "" {
		t.Error("expected error message for invalid project ID")
	}
}
