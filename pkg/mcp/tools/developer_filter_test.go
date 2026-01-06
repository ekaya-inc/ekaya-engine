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
		{Name: "get_glossary"},
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
	deps := &MCPToolDeps{
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

	deps := &MCPToolDeps{
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

	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Developer tools enabled, execute disabled - should filter out execute
	// Note: query, sample, validate are now in approved_queries group, not developer
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have health + echo + get_schema (developer tools minus execute)
	// query, sample, validate are now part of approved_queries and should be filtered (no queries exist)
	expectedTools := []string{"health", "echo", "get_schema"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	if containsTool(filtered, "execute") {
		t.Error("expected execute tool to be filtered out")
	}

	// Business user tools (query, sample, validate) should be filtered out since approved_queries is not enabled
	if containsTool(filtered, "query") {
		t.Error("expected query tool to be filtered (approved_queries not enabled)")
	}
	if containsTool(filtered, "sample") {
		t.Error("expected sample tool to be filtered (approved_queries not enabled)")
	}
	if containsTool(filtered, "validate") {
		t.Error("expected validate tool to be filtered (approved_queries not enabled)")
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

	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// All developer tools enabled, but no approved_queries (no queries exist)
	// Note: query, sample, validate are now in approved_queries group
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have developer tools (echo, execute, get_schema) + health
	// query, sample, validate are now part of approved_queries and should be filtered (no queries exist)
	expectedCount := 4 // health + echo + execute + get_schema
	if len(filtered) != expectedCount {
		t.Errorf("expected %d tools, got %d: %v", expectedCount, len(filtered), toolNames(filtered))
	}

	if !containsTool(filtered, "execute") {
		t.Error("expected execute tool to be present when enabled")
	}

	// Business user tools (query, sample, validate) should be filtered since approved_queries not enabled
	if containsTool(filtered, "query") {
		t.Error("expected query to be filtered (approved_queries not enabled)")
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

	deps := &MCPToolDeps{
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

func TestFilterOutExecuteTool(t *testing.T) {
	tools := createTestTools()
	filtered := filterOutExecuteTool(tools)

	// Should filter out only execute tool, keep all others
	if len(filtered) != 10 {
		t.Errorf("expected 10 tools (all except execute), got %d: %v", len(filtered), toolNames(filtered))
	}

	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered out")
	}

	// Developer tools except execute should be present
	for _, name := range []string{"health", "echo"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
	// Business user tools (now in approved_queries group) should be present
	for _, name := range []string{"query", "sample", "validate"} {
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

	deps := &MCPToolDeps{
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

	// Business user tools (query, sample, validate) are now in approved_queries group, should be filtered
	if containsTool(filtered, "query") {
		t.Error("expected query to be filtered when approved_queries toggle is OFF")
	}
	if containsTool(filtered, "sample") {
		t.Error("expected sample to be filtered when approved_queries toggle is OFF")
	}
	if containsTool(filtered, "validate") {
		t.Error("expected validate to be filtered when approved_queries toggle is OFF")
	}

	// Developer tools should still be present
	if !containsTool(filtered, "echo") {
		t.Error("expected developer tools (echo) to be present")
	}
	if !containsTool(filtered, "execute") {
		t.Error("expected developer tools (execute) to be present")
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
	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: nil}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// approved_queries tools should be filtered since no queries exist
	// This includes list_approved_queries, execute_approved_query, get_ontology, query, sample, validate
	if containsTool(filtered, "list_approved_queries") {
		t.Error("expected list_approved_queries to be filtered when no queries exist")
	}

	// Business user tools (query, sample, validate) are now in approved_queries group
	// They should also be filtered when no queries exist
	if containsTool(filtered, "query") {
		t.Error("expected query to be filtered when no queries exist")
	}

	// Developer tools should still be present
	if !containsTool(filtered, "echo") {
		t.Error("expected developer tools (echo) to be present")
	}
	if !containsTool(filtered, "execute") {
		t.Error("expected developer tools (execute) to be present")
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
	deps := &MCPToolDeps{
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

	// Business user tools (query, sample, validate) should be present when approved queries enabled
	if !containsTool(filtered, "query") {
		t.Error("expected query to be present when approved queries enabled")
	}
	if !containsTool(filtered, "sample") {
		t.Error("expected sample to be present when approved queries enabled")
	}
	if !containsTool(filtered, "validate") {
		t.Error("expected validate to be present when approved queries enabled")
	}

	// All tools should be present
	if len(filtered) != 11 {
		t.Errorf("expected 11 tools (all), got %d: %v", len(filtered), toolNames(filtered))
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
	// Developer tools now: echo, execute (but execute disabled), get_schema
	// Business user tools (query, sample, validate) are now in approved_queries
	filtered = filterTools(tools, true, false, false)
	if len(filtered) != 3 {
		t.Errorf("expected 3 tools (health + echo + get_schema), got %d: %v", len(filtered), toolNames(filtered))
	}
	if containsTool(filtered, "execute") {
		t.Error("execute should be filtered when execute disabled")
	}
	if containsTool(filtered, "query") {
		t.Error("query should be filtered when approved_queries disabled")
	}

	// Test developer enabled, execute enabled, approved_queries disabled
	filtered = filterTools(tools, true, true, false)
	if len(filtered) != 4 {
		t.Errorf("expected 4 tools (health + echo + execute + get_schema), got %d: %v", len(filtered), toolNames(filtered))
	}

	// Test approved_queries enabled, developer disabled
	filtered = filterTools(tools, false, false, true)
	// Should have: health + query + sample + validate + get_ontology + get_glossary + list_approved_queries + execute_approved_query = 8
	if len(filtered) != 8 {
		t.Errorf("expected 8 tools (health + business user tools + approved_queries tools), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "query") {
		t.Error("query should be present when approved_queries enabled")
	}
	if containsTool(filtered, "echo") {
		t.Error("echo should be filtered when developer disabled")
	}

	// Test all enabled
	filtered = filterTools(tools, true, true, true)
	if len(filtered) != 11 {
		t.Errorf("expected 11 tools (all), got %d: %v", len(filtered), toolNames(filtered))
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
	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// With ForceMode ON, only approved_queries tools (including business user tools), ontology tools and health should be present
	// Business user tools (query, sample, validate) are now in approved_queries group
	expectedTools := []string{"health", "query", "sample", "validate", "list_approved_queries", "execute_approved_query", "get_ontology", "get_glossary"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools with ForceMode, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	// Developer tools (echo, execute, get_schema) should be filtered out despite being enabled in config
	if containsTool(filtered, "echo") {
		t.Error("echo should be filtered when ForceMode is enabled")
	}
	if containsTool(filtered, "execute") {
		t.Error("execute tool should be filtered when ForceMode is enabled")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("schema tools should be filtered when ForceMode is enabled")
	}

	// Approved queries tools (including business user tools) and ontology tools should be present
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
	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// With ForceMode OFF, all tools should be present
	if len(filtered) != 11 {
		t.Errorf("expected 11 tools (all) with ForceMode OFF, got %d: %v", len(filtered), toolNames(filtered))
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
// Expected: health, business user tools, approved_queries tools, ontology tools
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
	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Expected: health, query, sample, validate (business user tools), list_approved_queries, execute_approved_query, get_ontology, get_glossary
	// Business user tools are now in approved_queries group
	expectedTools := []string{"health", "query", "sample", "validate", "list_approved_queries", "execute_approved_query", "get_ontology", "get_glossary"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Developer tools (echo, execute, get_schema) should be filtered out
	if containsTool(filtered, "echo") {
		t.Error("developer tool 'echo' should be filtered when developer disabled")
	}
	if containsTool(filtered, "execute") {
		t.Error("developer tool 'execute' should be filtered when developer disabled")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("schema tool 'get_schema' should be filtered when developer disabled")
	}
}

// Test Mode 3 from Appendix: Pre-Approved ON, Developer ON (Execute OFF)
// Expected: health, list_approved_queries, execute_approved_query, get_schema, query, sample, validate, echo, get_ontology, get_glossary
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
	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, &mockProjectService{defaultDatasourceID: uuid.New()}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Expected: health, list_approved_queries, execute_approved_query, get_ontology, get_glossary, get_schema, query, sample, validate, echo (10 tools)
	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query", "get_ontology", "get_glossary", "get_schema", "query", "sample", "validate", "echo"}
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
// Expected: health, get_schema, echo (developer tools only)
func TestNewToolFilter_ApprovedQueriesOff_DeveloperOn(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Pre-Approved OFF (not in config), Developer ON, Execute OFF
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, EnableExecute: false})

	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Expected: health, get_schema, echo (3 tools)
	// query, sample, validate are now in approved_queries group, so they should be filtered out
	expectedTools := []string{"health", "get_schema", "echo"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Business user tools (query, sample, validate) are now in approved_queries group, should be filtered
	if containsTool(filtered, "query") {
		t.Error("query should be filtered when approved_queries disabled")
	}
	if containsTool(filtered, "sample") {
		t.Error("sample should be filtered when approved_queries disabled")
	}
	if containsTool(filtered, "validate") {
		t.Error("validate should be filtered when approved_queries disabled")
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

	// Test ontology and business user tools present with approved_queries even when developer tools disabled
	filtered = filterTools(tools, false, false, true)
	// query, sample, validate are now in approved_queries group, so they should be present
	if !containsTool(filtered, "query") {
		t.Error("query should be present when approved_queries enabled (now in approved_queries group)")
	}
	if !containsTool(filtered, "get_ontology") {
		t.Error("get_ontology should be present when approved_queries enabled, regardless of developer tools")
	}
	// echo is a developer tool and should be filtered
	if containsTool(filtered, "echo") {
		t.Error("echo should be filtered when developer disabled")
	}
}

// Test filterAgentTools function directly
func TestFilterAgentTools_Disabled(t *testing.T) {
	tools := createTestTools()

	// When agent_tools is disabled, only health should be available
	filtered := filterAgentTools(tools, false)

	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("health tool should always be present")
	}
}

func TestFilterAgentTools_Enabled(t *testing.T) {
	tools := createTestTools()

	// When agent_tools is enabled, health + echo + approved_queries tools should be available
	// Note: query, sample, validate are in approved_queries group now, but NOT available to agents
	// (agentToolNames only includes echo, list_approved_queries, execute_approved_query)
	filtered := filterAgentTools(tools, true)

	expectedTools := []string{"health", "echo", "list_approved_queries", "execute_approved_query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Business user tools (query, sample, validate) should NOT be present for agents
	// Even though they're in approved_queries group, they're not in agentToolNames
	if containsTool(filtered, "query") {
		t.Error("business user tool 'query' should not be available to agents")
	}
	if containsTool(filtered, "sample") {
		t.Error("business user tool 'sample' should not be available to agents")
	}
	if containsTool(filtered, "validate") {
		t.Error("business user tool 'validate' should not be available to agents")
	}
	if containsTool(filtered, "execute") {
		t.Error("developer tool 'execute' should not be available to agents")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("schema tool 'get_schema' should not be available to agents")
	}
	if containsTool(filtered, "get_ontology") {
		t.Error("ontology tool 'get_ontology' should not be available to agents")
	}
}

// setupTestConfigWithAgentTools creates a project and MCP config with agent_tools enabled.
func setupTestConfigWithAgentTools(t *testing.T, db *database.DB, projectID uuid.UUID, agentToolsEnabled bool) {
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
			"agent_tools": {Enabled: agentToolsEnabled},
		},
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

func TestNewToolFilter_AgentAuth_AgentToolsEnabled(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with agent_tools enabled
	setupTestConfigWithAgentTools(t, engineDB.DB, projectID, true)

	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Agent authentication - claims.Subject = "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Agent should only see health + echo + approved_queries tools
	expectedTools := []string{"health", "echo", "list_approved_queries", "execute_approved_query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools for agent, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present for agent", name)
		}
	}

	// Business user tools and schema tools should NOT be present for agents
	if containsTool(filtered, "query") {
		t.Error("business user tool 'query' should not be available to agents")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("schema tool 'get_schema' should not be available to agents")
	}
}

func TestNewToolFilter_AgentAuth_AgentToolsDisabled(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with agent_tools disabled
	setupTestConfigWithAgentTools(t, engineDB.DB, projectID, false)

	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Agent authentication - claims.Subject = "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Agent should only see health when agent_tools is disabled
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("health tool should always be present")
	}
}

func TestNewToolFilter_AgentAuth_NoConfig(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create project but no MCP config - should default to disabled
	ctx := context.Background()
	scope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to get tenant scope: %v", err)
	}
	defer scope.Close()

	tenantCtx := database.SetTenantScope(ctx, scope)
	_, err = scope.Conn.Exec(tenantCtx, `
		INSERT INTO engine_projects (id, name, created_at, updated_at)
		VALUES ($1, 'Test Project', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, projectID)
	if err != nil {
		t.Fatalf("failed to create test project: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		cleanupScope, err := engineDB.DB.WithTenant(cleanupCtx, projectID)
		if err != nil {
			return
		}
		defer cleanupScope.Close()
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_projects WHERE id = $1", projectID)
	})

	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Agent authentication - claims.Subject = "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Agent should only see health when no config (defaults to disabled)
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("health tool should always be present")
	}
}

func TestNewToolFilter_UserAuth_AgentToolsEnabledDoesNotAffectUsers(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with agent_tools enabled but developer tools disabled
	ctx := context.Background()
	scope, err := engineDB.DB.WithTenant(ctx, projectID)
	if err != nil {
		t.Fatalf("failed to get tenant scope: %v", err)
	}
	defer scope.Close()

	tenantCtx := database.SetTenantScope(ctx, scope)
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
			"agent_tools": {Enabled: true},
			"developer":   {Enabled: true, EnableExecute: true},
		},
	}

	repo := repositories.NewMCPConfigRepository()
	if err := repo.Upsert(tenantCtx, mcpConfig); err != nil {
		t.Fatalf("failed to create test config: %v", err)
	}

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		cleanupScope, err := engineDB.DB.WithTenant(cleanupCtx, projectID)
		if err != nil {
			return
		}
		defer cleanupScope.Close()
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_mcp_config WHERE project_id = $1", projectID)
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_projects WHERE id = $1", projectID)
	})

	deps := &MCPToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, "http://localhost", zap.NewNop()),
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// User authentication - claims.Subject is NOT "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123" // Normal user
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// User should see all developer tools (agent_tools config doesn't affect users)
	// Note: query, sample, validate are now in approved_queries group, not developer
	// They won't be present since approved_queries is not enabled
	expectedDeveloperTools := []string{"health", "echo", "execute", "get_schema"}
	for _, name := range expectedDeveloperTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected developer tool %s to be present for user", name)
		}
	}

	// Business user tools (query, sample, validate) should NOT be present since approved_queries not enabled
	if containsTool(filtered, "query") {
		t.Error("query should not be present (approved_queries not enabled)")
	}
	if containsTool(filtered, "sample") {
		t.Error("sample should not be present (approved_queries not enabled)")
	}
	if containsTool(filtered, "validate") {
		t.Error("validate should not be present (approved_queries not enabled)")
	}
}
