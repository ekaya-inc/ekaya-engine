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

// createTestTools creates a list of tools for testing.
func createTestTools() []mcp.Tool {
	return []mcp.Tool{
		{Name: "health"},
		{Name: "echo"},
		{Name: "query"},
		{Name: "sample"},
		{Name: "execute"},
		{Name: "validate"},
		{Name: "get_schema"},
		{Name: "list_approved_queries"},
		{Name: "execute_approved_query"},
		{Name: "get_ontology"},
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
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Developer tools disabled - should filter out all developer tools and approved_queries (no queries exist)
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
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Developer tools enabled, execute disabled - should filter out execute only
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have health + all developer tools except execute + get_schema
	expectedTools := []string{"health", "echo", "query", "sample", "validate", "get_schema"}
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
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// All developer tools enabled, but no approved_queries (no queries exist)
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have all developer tools + get_schema but not approved_queries tools (no queries exist)
	expectedCount := 7 // health + 5 developer tools + get_schema, no approved_queries
	if len(filtered) != expectedCount {
		t.Errorf("expected %d tools, got %d: %v", expectedCount, len(filtered), toolNames(filtered))
	}

	if !containsTool(filtered, "execute") {
		t.Error("expected execute tool to be present when enabled")
	}

	// Approved queries tools should be filtered since no queries exist
	if containsTool(filtered, "list_approved_queries") {
		t.Error("expected list_approved_queries to be filtered (no enabled queries)")
	}
}

func TestNewToolFilter_NilConfig(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()
	// Don't create any config - should use defaults (disabled)

	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
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

	// Should filter out all developer tools, keep health, get_schema, approved_queries tools, and ontology tools
	if len(filtered) != 5 {
		t.Errorf("expected 5 tools (health + get_schema + 2 approved_queries + 1 ontology), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
	if !containsTool(filtered, "get_schema") {
		t.Error("expected get_schema tool to be present")
	}
	if !containsTool(filtered, "list_approved_queries") {
		t.Error("expected list_approved_queries tool to be present")
	}
	if !containsTool(filtered, "get_ontology") {
		t.Error("expected get_ontology tool to be present")
	}
}

func TestFilterOutExecuteTool(t *testing.T) {
	tools := createTestTools()
	filtered := filterOutExecuteTool(tools)

	// Should filter out only execute tool, keep all others
	if len(filtered) != 9 {
		t.Errorf("expected 9 tools (all except execute), got %d: %v", len(filtered), toolNames(filtered))
	}

	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered out")
	}

	// Developer tools except execute should be present
	for _, name := range []string{"health", "echo", "query", "sample", "validate"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
	// Approved queries tools should also be present
	for _, name := range []string{"list_approved_queries", "execute_approved_query"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
	// Ontology tools should also be present
	if !containsTool(filtered, "get_ontology") {
		t.Error("expected get_ontology tool to be present")
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

// setupTestConfigWithApprovedQueries creates a project and MCP config with approved_queries enabled.
func setupTestConfigWithApprovedQueries(t *testing.T, db *database.DB, projectID uuid.UUID, devConfig, approvedQueriesConfig *models.ToolGroupConfig) {
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
		ProjectID:  projectID,
		ToolGroups: make(map[string]*models.ToolGroupConfig),
	}
	if devConfig != nil {
		mcpConfig.ToolGroups["developer"] = devConfig
	}
	if approvedQueriesConfig != nil {
		mcpConfig.ToolGroups["approved_queries"] = approvedQueriesConfig
	}

	repo := repositories.NewMCPConfigRepository()
	if err := repo.Upsert(tenantCtx, mcpConfig); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	t.Cleanup(func() {
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

func TestNewToolFilter_ApprovedQueriesToggleOff(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// approved_queries toggle OFF (not in config = defaults to disabled)
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, EnableExecute: true})

	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// approved_queries tools should be filtered out since toggle is OFF
	if containsTool(filtered, "list_approved_queries") {
		t.Error("expected list_approved_queries to be filtered when toggle is OFF")
	}
	if containsTool(filtered, "execute_approved_query") {
		t.Error("expected execute_approved_query to be filtered when toggle is OFF")
	}

	// Developer tools should still be present
	if !containsTool(filtered, "query") {
		t.Error("expected developer tools to be present")
	}
}

func TestNewToolFilter_ApprovedQueriesEnabledNoQueries(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// approved_queries toggle ON but no queries exist
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: true, EnableExecute: true},
		&models.ToolGroupConfig{Enabled: true},
	)

	// Mock with no queries
	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: nil}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// approved_queries tools should be filtered since no queries exist
	if containsTool(filtered, "list_approved_queries") {
		t.Error("expected list_approved_queries to be filtered when no queries exist")
	}

	// Developer tools should still be present
	if !containsTool(filtered, "query") {
		t.Error("expected developer tools to be present")
	}
}

func TestNewToolFilter_ApprovedQueriesEnabledWithQueries(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// approved_queries toggle ON with queries
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: true, EnableExecute: true},
		&models.ToolGroupConfig{Enabled: true},
	)

	// Mock with queries
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// approved_queries tools should be present since queries exist
	if !containsTool(filtered, "list_approved_queries") {
		t.Error("expected list_approved_queries to be present when queries exist")
	}
	if !containsTool(filtered, "execute_approved_query") {
		t.Error("expected execute_approved_query to be present when queries exist")
	}
	if !containsTool(filtered, "get_ontology") {
		t.Error("expected get_ontology to be present when approved queries enabled")
	}

	// All tools should be present
	if len(filtered) != 10 {
		t.Errorf("expected 10 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}
}

func TestFilterTools_SchemaToolsFilteredWhenDeveloperDisabled(t *testing.T) {
	tools := createTestTools()

	// Test all disabled - get_schema should be filtered
	filtered := filterTools(tools, false, false, false)
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if containsTool(filtered, "get_schema") {
		t.Error("get_schema should be filtered when developer tools disabled")
	}

	// Test developer enabled - get_schema should be present
	filtered = filterTools(tools, true, false, false)
	if !containsTool(filtered, "get_schema") {
		t.Error("get_schema should be present when developer tools enabled")
	}
}

func TestFilterTools(t *testing.T) {
	tools := createTestTools()

	// Test all disabled
	filtered := filterTools(tools, false, false, false)
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}

	// Test developer enabled, execute disabled, approved_queries disabled
	filtered = filterTools(tools, true, false, false)
	if len(filtered) != 6 {
		t.Errorf("expected 6 tools (health + 4 dev tools + get_schema), got %d: %v", len(filtered), toolNames(filtered))
	}
	if containsTool(filtered, "execute") {
		t.Error("execute should be filtered when execute disabled")
	}

	// Test developer enabled, execute enabled, approved_queries disabled
	filtered = filterTools(tools, true, true, false)
	if len(filtered) != 7 {
		t.Errorf("expected 7 tools (health + 5 dev tools + get_schema), got %d: %v", len(filtered), toolNames(filtered))
	}

	// Test all enabled
	filtered = filterTools(tools, true, true, true)
	if len(filtered) != 10 {
		t.Errorf("expected 10 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}
}

func TestNewToolFilter_ForceModeDisablesDeveloperTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with ForceMode enabled on approved_queries
	// Even with developer tools enabled, they should be forcibly disabled
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: true, EnableExecute: true},
		&models.ToolGroupConfig{Enabled: true, ForceMode: true},
	)

	// Mock with queries
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// With ForceMode ON, only approved_queries tools, ontology tools and health should be present
	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query", "get_ontology"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools with ForceMode, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	// Developer tools should be filtered out despite being enabled in config
	if containsTool(filtered, "query") {
		t.Error("developer tools should be filtered when ForceMode is enabled")
	}
	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered when ForceMode is enabled")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("schema tools should be filtered when ForceMode is enabled")
	}

	// Approved queries tools and ontology tools should be present
	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present with ForceMode", name)
		}
	}
}

func TestNewToolFilter_ForceModeOffAllowsDeveloperTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with ForceMode disabled (default)
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: true, EnableExecute: true},
		&models.ToolGroupConfig{Enabled: true, ForceMode: false},
	)

	// Mock with queries
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// With ForceMode OFF, all tools should be present
	if len(filtered) != 10 {
		t.Errorf("expected 10 tools (all) with ForceMode OFF, got %d: %v", len(filtered), toolNames(filtered))
	}

	// Developer tools should be present
	if !containsTool(filtered, "query") {
		t.Error("developer tools should be present when ForceMode is disabled")
	}
	if !containsTool(filtered, "execute") {
		t.Error("execute tool should be present when ForceMode is disabled")
	}
}

// Test Mode 2 from Appendix: Pre-Approved ON, Developer OFF
// Expected: health, list_approved_queries, execute_approved_query
func TestNewToolFilter_ApprovedQueriesOn_DeveloperOff(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Pre-Approved ON, Developer OFF
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: false, EnableExecute: false},
		&models.ToolGroupConfig{Enabled: true, ForceMode: false},
	)

	// Mock with queries
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Expected: health, list_approved_queries, execute_approved_query, get_ontology
	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query", "get_ontology"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Developer tools should be filtered out
	if containsTool(filtered, "query") {
		t.Error("developer tool 'query' should be filtered when developer disabled")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("schema tool 'get_schema' should be filtered when developer disabled")
	}
}

// Test Mode 3 from Appendix: Pre-Approved ON, Developer ON (Execute OFF)
// Expected: health, list_approved_queries, execute_approved_query, get_schema, query, sample, validate, echo
func TestNewToolFilter_ApprovedQueriesOn_DeveloperOn_ExecuteOff(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Pre-Approved ON, Developer ON, Execute OFF
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: true, EnableExecute: false},
		&models.ToolGroupConfig{Enabled: true, ForceMode: false},
	)

	// Mock with queries
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Expected: health, list_approved_queries, execute_approved_query, get_ontology, get_schema, query, sample, validate, echo (9 tools)
	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query", "get_ontology", "get_schema", "query", "sample", "validate", "echo"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Execute should be filtered out
	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered when execute disabled")
	}
}

// Test Mode 5 from Appendix: Pre-Approved OFF, Developer ON
// Expected: health, get_schema, query, sample, validate, echo
func TestNewToolFilter_ApprovedQueriesOff_DeveloperOn(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Pre-Approved OFF (not in config), Developer ON, Execute OFF
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, EnableExecute: false})

	deps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Expected: health, get_schema, query, sample, validate, echo (6 tools)
	expectedTools := []string{"health", "get_schema", "query", "sample", "validate", "echo"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Approved queries tools should be filtered out
	if containsTool(filtered, "list_approved_queries") {
		t.Error("list_approved_queries should be filtered when approved_queries disabled")
	}
	if containsTool(filtered, "execute_approved_query") {
		t.Error("execute_approved_query should be filtered when approved_queries disabled")
	}
	// Ontology tools should be filtered out (tied to approved_queries visibility)
	if containsTool(filtered, "get_ontology") {
		t.Error("get_ontology should be filtered when approved_queries disabled")
	}
	// Execute should be filtered out
	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered when execute disabled")
	}
}

// Test ontology tools filtering behavior
func TestFilterTools_OntologyToolsFilteredWithApprovedQueries(t *testing.T) {
	tools := createTestTools()

	// Test ontology tools filtered when approved_queries disabled
	filtered := filterTools(tools, false, false, false)
	if containsTool(filtered, "get_ontology") {
		t.Error("get_ontology should be filtered when approved_queries disabled")
	}

	// Test ontology tools present when approved_queries enabled
	filtered = filterTools(tools, false, false, true)
	if !containsTool(filtered, "get_ontology") {
		t.Error("get_ontology should be present when approved_queries enabled")
	}

	// Test ontology tools present with approved_queries even when developer tools disabled
	filtered = filterTools(tools, false, false, true)
	if containsTool(filtered, "query") {
		t.Error("developer tools should be filtered when developer disabled")
	}
	if !containsTool(filtered, "get_ontology") {
		t.Error("get_ontology should be present when approved_queries enabled, regardless of developer tools")
	}
}
