package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockMCPConfigService implements services.MCPConfigService for testing.
type mockMCPConfigService struct {
	config *models.ToolGroupConfig
	err    error
}

func (m *mockMCPConfigService) Get(ctx context.Context, projectID uuid.UUID) (*services.MCPConfigResponse, error) {
	return nil, nil
}

func (m *mockMCPConfigService) Update(ctx context.Context, projectID uuid.UUID, req *services.UpdateMCPConfigRequest) (*services.MCPConfigResponse, error) {
	return nil, nil
}

func (m *mockMCPConfigService) IsToolGroupEnabled(ctx context.Context, projectID uuid.UUID, toolGroup string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.config == nil {
		return false, nil
	}
	return m.config.Enabled, nil
}

func (m *mockMCPConfigService) GetToolGroupConfig(ctx context.Context, projectID uuid.UUID, toolGroup string) (*models.ToolGroupConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

// createTestTools creates a list of tools for testing.
func createTestTools() []mcp.Tool {
	return []mcp.Tool{
		{Name: "health"},
		{Name: "echo"},
		{Name: "schema"},
		{Name: "query"},
		{Name: "sample"},
		{Name: "execute"},
		{Name: "validate"},
	}
}

// toolNames extracts tool names from a list of tools.
func toolNames(tools []mcp.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// containsTool checks if a tool name is in the list.
func containsTool(tools []mcp.Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

func TestNewToolFilter_NoAuth(t *testing.T) {
	deps := &DeveloperToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true, EnableExecute: true},
		},
		Logger: zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// No auth context - should filter out all developer tools
	ctx := context.Background()
	filtered := filter(ctx, tools)

	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
}

func TestNewToolFilter_DeveloperDisabled(t *testing.T) {
	deps := &DeveloperToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: false, EnableExecute: false},
		},
		Logger: zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Developer tools disabled - should filter out all developer tools
	projectID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
}

func TestNewToolFilter_DeveloperEnabledExecuteDisabled(t *testing.T) {
	deps := &DeveloperToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true, EnableExecute: false},
		},
		Logger: zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Developer tools enabled, execute disabled - should filter out execute only
	projectID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have health + all developer tools except execute
	expectedTools := []string{"health", "echo", "schema", "query", "sample", "validate"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	if containsTool(filtered, "execute") {
		t.Error("expected execute tool to be filtered out")
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_AllEnabled(t *testing.T) {
	deps := &DeveloperToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true, EnableExecute: true},
		},
		Logger: zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// All tools enabled
	projectID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have all tools
	if len(filtered) != len(tools) {
		t.Errorf("expected %d tools, got %d: %v", len(tools), len(filtered), toolNames(filtered))
	}

	if !containsTool(filtered, "execute") {
		t.Error("expected execute tool to be present when enabled")
	}
}

func TestNewToolFilter_NilConfig(t *testing.T) {
	deps := &DeveloperToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: nil, // No config = defaults (disabled)
		},
		Logger: zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// No config - should filter out all developer tools
	projectID := uuid.New()
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
}

func TestFilterOutDeveloperTools(t *testing.T) {
	tools := createTestTools()
	filtered := filterOutDeveloperTools(tools, true)

	if len(filtered) != 1 {
		t.Errorf("expected 1 tool, got %d", len(filtered))
	}
	if filtered[0].Name != "health" {
		t.Errorf("expected health tool, got %s", filtered[0].Name)
	}
}

func TestFilterOutExecuteTool(t *testing.T) {
	tools := createTestTools()
	filtered := filterOutExecuteTool(tools)

	if len(filtered) != 6 {
		t.Errorf("expected 6 tools, got %d", len(filtered))
	}

	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered out")
	}

	// All other tools should be present
	for _, name := range []string{"health", "echo", "schema", "query", "sample", "validate"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}
