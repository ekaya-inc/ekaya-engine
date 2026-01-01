package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
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
	// No DB needed - filter should return early without auth
	deps := &DeveloperToolDeps{
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
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with developer tools disabled
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: false, EnableExecute: false})

	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Developer tools disabled - should filter out all developer tools
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
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with developer tools enabled but execute disabled
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, EnableExecute: false})

	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Developer tools enabled, execute disabled - should filter out execute only
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have health + all developer tools except execute
	expectedTools := []string{"health", "echo", "query", "sample", "validate"}
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
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with all tools enabled
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, EnableExecute: true})

	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// All tools enabled
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
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()
	// Don't create any config - should use defaults (disabled)

	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// No config - should filter out all developer tools
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

	if len(filtered) != 5 {
		t.Errorf("expected 5 tools, got %d", len(filtered))
	}

	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered out")
	}

	// All other tools should be present
	for _, name := range []string{"health", "echo", "query", "sample", "validate"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

// setupTestConfig creates a project and MCP config for testing.
func setupTestConfig(t *testing.T, db *database.DB, projectID uuid.UUID, config *models.ToolGroupConfig) {
	t.Helper()

	ctx := context.Background()
	scope, err := db.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to get tenant scope: %v", err)
	}
	defer scope.Close()

	tenantCtx := database.SetTenantScope(ctx, scope)

	// Create project first (MCP config has FK to projects)
	_, err = scope.Conn.Exec(tenantCtx, `
		INSERT INTO engine_projects (id, name, created_at, updated_at)
		VALUES ($1, 'Test Project', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, projectID)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	mcpConfig := &models.MCPConfig{
		ProjectID: projectID,
		ToolGroups: map[string]*models.ToolGroupConfig{
			"developer": config,
		},
	}

	repo := repositories.NewMCPConfigRepository()
	if err := repo.Upsert(tenantCtx, mcpConfig); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	t.Cleanup(func() {
		// Clean up after test
		cleanupCtx := context.Background()
		cleanupScope, err := db.WithTenant(cleanupCtx, projectID)
		if err != nil {
			return
		}
		defer cleanupScope.Close()
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_mcp_config WHERE project_id = $1", projectID)
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_projects WHERE id = $1", projectID)
	})
}
