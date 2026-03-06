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
		{Name: "list_glossary"},
		{Name: "get_glossary_sql"},
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

// testDatasourceID is a fixed UUID used in tests to simulate a configured datasource.
var testDatasourceID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

// mockProjectServiceWithDatasource returns a mock project service with a datasource configured.
// This is required for tool filtering tests since tools beyond "health" require a datasource.
func mockProjectServiceWithDatasource() *mockProjectService {
	return &mockProjectService{defaultDatasourceID: testDatasourceID}
}

func TestNewToolFilter_NoAuth(t *testing.T) {
	// No DB needed - filter should return early without auth
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			Logger: zap.NewNop(),
		},
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

	// Create config with no toggles enabled
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{})

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// No toggles enabled = only health
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
}

func TestNewToolFilter_DeveloperEnabled_AllToolsAvailable(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with all toggles enabled
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{
		AddDirectDatabaseAccess:     true,
		AddOntologyMaintenanceTools: true,
		AddOntologySuggestions:      true,
		AddApprovalTools:            true,
		AddRequestTools:             true,
	})

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDOntologyForge, models.AppIDAIDataLiaison),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// All toggles enabled = ALL test tools available
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have ALL tools (12 total) when all toggles enabled
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	// All tools should be present
	for _, name := range []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema", "list_approved_queries", "execute_approved_query", "get_ontology", "list_glossary", "get_glossary_sql"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_DeveloperEnabled_VerifyAllTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with all toggles enabled
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{
		AddDirectDatabaseAccess:     true,
		AddOntologyMaintenanceTools: true,
		AddOntologySuggestions:      true,
		AddApprovalTools:            true,
		AddRequestTools:             true,
	})

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDOntologyForge, models.AppIDAIDataLiaison),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// All toggles enabled = ALL tools available
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have ALL 12 tools
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	// All tools should be present
	allTools := []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema", "list_approved_queries", "execute_approved_query", "get_ontology", "list_glossary", "get_glossary_sql"}
	for _, name := range allTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_NilConfig(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()
	// Don't create any config - should use DefaultMCPConfig which has all toggles enabled

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDOntologyForge, models.AppIDAIDataLiaison),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// No config = DefaultMCPConfig with all toggles enabled → all tools available
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// All test tools should be present (12 tools from createTestTools)
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}
	for _, name := range []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present with default config", name)
		}
	}
}

func TestFilterOutExecuteTool(t *testing.T) {
	tools := createTestTools()
	filtered := filterOutExecuteTool(tools)

	// Should filter out only execute tool, keep all others
	if len(filtered) != 11 {
		t.Errorf("expected 11 tools (all except execute), got %d: %v", len(filtered), toolNames(filtered))
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
// Uses the "tools" key with per-app toggles.
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
			"tools": config,
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

// setupTestConfigWithToolGroups creates a project and MCP config with the specified tool groups.
func setupTestConfigWithToolGroups(t *testing.T, db *database.DB, projectID uuid.UUID, toolGroups map[string]*models.ToolGroupConfig) {
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
		ToolGroups: toolGroups,
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

func TestNewToolFilter_DeveloperEnabled_ApprovedQueriesOff(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Only Direct Database Access enabled, no approval tools
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{AddDirectDatabaseAccess: true})

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Direct Database Access = health + echo + execute + query = 4 tools
	expectedTools := []string{"health", "echo", "execute", "query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}
	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Approval and ontology tools should NOT be present
	if containsTool(filtered, "list_query_suggestions") {
		t.Error("list_query_suggestions should not be present without AddApprovalTools")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("get_schema should not be present without AddOntologyMaintenanceTools")
	}
}

func TestNewToolFilter_ApprovedQueriesOnly_NoQueriesExist(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// No toggles enabled
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {},
	})

	// Mock with no queries - query count doesn't affect tool filtering
	testProjectService := &mockProjectService{defaultDatasourceID: uuid.New()}
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: nil}, testProjectService, nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: testProjectService,
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// No toggles enabled = only health
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
}

func TestNewToolFilter_ApprovedQueriesEnabledWithQueries(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Only AddRequestTools enabled (user-facing request tools)
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {AddRequestTools: true},
	})

	// Mock with queries - query count doesn't affect tool filtering
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	testProjectService := &mockProjectService{defaultDatasourceID: uuid.New()}
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, testProjectService, nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDAIDataLiaison),
		},
		ProjectService: testProjectService,
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// AddRequestTools enables user-facing request tools: query, sample, validate,
	// list_approved_queries, execute_approved_query, etc.
	// Health always included.
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
	if !containsTool(filtered, "query") {
		t.Error("expected query tool to be present with AddRequestTools")
	}
	if !containsTool(filtered, "list_approved_queries") {
		t.Error("expected list_approved_queries to be present with AddRequestTools")
	}

	// Developer-only tools should NOT be present
	if containsTool(filtered, "echo") {
		t.Error("echo should not be present without AddDirectDatabaseAccess")
	}
	if containsTool(filtered, "execute") {
		t.Error("execute should not be present without AddDirectDatabaseAccess")
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
	// Should have: health + query + sample + validate + get_ontology + list_glossary + get_glossary_sql + list_approved_queries + execute_approved_query = 9
	if len(filtered) != 9 {
		t.Errorf("expected 9 tools (health + business user tools + approved_queries tools), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "query") {
		t.Error("query should be present when approved_queries enabled")
	}
	if containsTool(filtered, "echo") {
		t.Error("echo should be filtered when developer disabled")
	}

	// Test all enabled
	filtered = filterTools(tools, true, true, true)
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}
}

func TestNewToolFilter_ForceModeOnlyShowsApprovedQueriesTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// No toggles enabled — ForceMode is a legacy feature with no effect
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {},
	})

	// Mock with queries - query count doesn't affect tool filtering
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	testProjectService := &mockProjectService{defaultDatasourceID: uuid.New()}
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, testProjectService, nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: testProjectService,
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// No toggles enabled = only health
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
}

func TestNewToolFilter_ForceModeOffAllowsDeveloperTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// All toggles enabled
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddOntologySuggestions:      true,
			AddApprovalTools:            true,
			AddRequestTools:             true,
		},
	})

	// Mock with queries - query count doesn't affect tool filtering
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	testProjectService := &mockProjectService{defaultDatasourceID: uuid.New()}
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, testProjectService, nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDOntologyForge, models.AppIDAIDataLiaison),
		},
		ProjectService: testProjectService,
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// All toggles = all 12 test tools
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	if !containsTool(filtered, "query") {
		t.Error("query tool should be present")
	}
	if !containsTool(filtered, "execute") {
		t.Error("execute tool should be present")
	}
}

// Test with only user request tools enabled, no developer tools
func TestNewToolFilter_ApprovedQueriesOn_DeveloperOff(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Only AddRequestTools enabled
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {AddRequestTools: true},
	})

	// Mock with queries - query count doesn't affect tool filtering
	mockQueries := []*models.Query{{ID: uuid.New(), NaturalLanguagePrompt: "Test query"}}
	testProjectService := &mockProjectService{defaultDatasourceID: uuid.New()}
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{enabledQueries: mockQueries}, testProjectService, nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: testProjectService,
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// AddRequestTools enables user request tools: query, sample, validate,
	// list_approved_queries, execute_approved_query, etc.
	if !containsTool(filtered, "health") {
		t.Error("expected health tool to be present")
	}
	if !containsTool(filtered, "query") {
		t.Error("expected query tool to be present with AddRequestTools")
	}

	// Developer-only tools should NOT be present
	if containsTool(filtered, "echo") {
		t.Error("echo should not be present without AddDirectDatabaseAccess")
	}
	if containsTool(filtered, "execute") {
		t.Error("execute should not be present without AddDirectDatabaseAccess")
	}
}

// Test that execute tool is always available when Developer Tools is enabled
// Execute is part of Developer Core loadout and doesn't require a separate flag
func TestNewToolFilter_DeveloperEnabled_ExecuteAlwaysAvailable(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// AddDirectDatabaseAccess includes echo, execute, query
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{AddDirectDatabaseAccess: true})

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Execute should be present with AddDirectDatabaseAccess
	if !containsTool(filtered, "execute") {
		t.Error("execute tool should be present with AddDirectDatabaseAccess")
	}
	if !containsTool(filtered, "echo") {
		t.Error("echo tool should be present with AddDirectDatabaseAccess")
	}
	if !containsTool(filtered, "query") {
		t.Error("query tool should be present with AddDirectDatabaseAccess")
	}
}

// Test with only AddDirectDatabaseAccess toggle (no other toggles)
func TestNewToolFilter_DeveloperOnly_CoreToolsOnly(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Only AddDirectDatabaseAccess enabled
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{AddDirectDatabaseAccess: true})

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// AddDirectDatabaseAccess = health + echo + execute + query = 4 tools
	expectedTools := []string{"health", "echo", "execute", "query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Ontology tools should NOT be present
	if containsTool(filtered, "get_schema") {
		t.Error("get_schema should NOT be present without AddOntologyMaintenanceTools")
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

	// When agent_tools is enabled, agents get Limited Query loadout only:
	// health + list_approved_queries + execute_approved_query
	// Echo is a developer testing tool and is NOT available to agents
	filtered := filterAgentTools(tools, true)

	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Developer tools should NOT be present for agents
	if containsTool(filtered, "echo") {
		t.Error("developer tool 'echo' should not be available to agents")
	}
	if containsTool(filtered, "execute") {
		t.Error("developer tool 'execute' should not be available to agents")
	}

	// Business user tools should NOT be present for agents
	if containsTool(filtered, "query") {
		t.Error("business user tool 'query' should not be available to agents")
	}
	if containsTool(filtered, "sample") {
		t.Error("business user tool 'sample' should not be available to agents")
	}
	if containsTool(filtered, "validate") {
		t.Error("business user tool 'validate' should not be available to agents")
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
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Agent authentication - claims.Subject = "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Agent should only see Limited Query loadout tools (health + approved_queries tools)
	// Echo is a developer tool and NOT available to agents
	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools for agent, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present for agent", name)
		}
	}

	// Developer tools should NOT be present for agents
	if containsTool(filtered, "echo") {
		t.Error("developer tool 'echo' should not be available to agents")
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
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
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

	// Create project but no MCP config - should use DefaultMCPConfig which has agent_tools.Enabled=true
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
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Agent authentication - claims.Subject = "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// With DefaultMCPConfig, agent_tools.Enabled=true, so agent gets Limited Query loadout
	// health + list_approved_queries + execute_approved_query = 3 tools
	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools (Limited Query), got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}
	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present with default config", name)
		}
	}
}

func TestNewToolFilter_UserAuth_DeveloperMode_AllTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with all toggles enabled
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddOntologySuggestions:      true,
			AddApprovalTools:            true,
			AddRequestTools:             true,
		},
	})

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDOntologyForge, models.AppIDAIDataLiaison),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// User authentication - claims.Subject is NOT "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123" // Normal user
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// All toggles enabled = ALL tools (12 total)
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	// All tools should be present
	allTools := []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema", "list_approved_queries", "execute_approved_query", "get_ontology", "list_glossary", "get_glossary_sql"}
	for _, name := range allTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

// createTestToolsWithDataLiaison creates a list of tools including data liaison tools.
func createTestToolsWithDataLiaison() []mcp.Tool {
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
		{Name: "list_glossary"},
		{Name: "get_glossary_sql"},
		// Data Liaison tools - Business User
		{Name: "suggest_approved_query"},
		{Name: "suggest_query_update"},
		// Data Liaison tools - Developer
		{Name: "list_query_suggestions"},
		{Name: "approve_query_suggestion"},
		{Name: "reject_query_suggestion"},
		{Name: "create_approved_query"},
		{Name: "update_approved_query"},
		{Name: "delete_approved_query"},
	}
}

func TestNewToolFilter_DataLiaisonNotInstalled_BusinessTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// All toggles enabled — but AI Data Liaison not installed
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddOntologySuggestions:      true,
			AddApprovalTools:            true,
			AddRequestTools:             true,
		},
	})

	// AI Data Liaison NOT installed (only Ontology Forge)
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDOntologyForge), // Only Ontology Forge, no AI Data Liaison
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestToolsWithDataLiaison()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Data Liaison tools should NOT be present (app not installed)
	dataLiaisonTools := []string{"suggest_approved_query", "suggest_query_update"}
	for _, name := range dataLiaisonTools {
		if containsTool(filtered, name) {
			t.Errorf("expected tool %s to be HIDDEN when AI Data Liaison is not installed", name)
		}
	}

	// MCP Server tools should be present (always installed)
	for _, name := range []string{"health", "echo", "execute", "query"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Ontology Forge tools should be present (app installed)
	if !containsTool(filtered, "get_schema") {
		t.Error("expected get_schema to be present (Ontology Forge installed)")
	}
}

func TestNewToolFilter_DataLiaisonInstalled_BusinessTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// All toggles enabled
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddOntologySuggestions:      true,
			AddApprovalTools:            true,
			AddRequestTools:             true,
		},
	})

	// AI Data Liaison IS installed
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDAIDataLiaison), // AI Data Liaison installed
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestToolsWithDataLiaison()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Data Liaison business tools SHOULD be present when app is installed
	dataLiaisonTools := []string{"suggest_approved_query", "suggest_query_update"}
	for _, name := range dataLiaisonTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present when AI Data Liaison is installed", name)
		}
	}
}

func TestNewToolFilter_DataLiaisonNotInstalled_DeveloperTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// All toggles enabled — but AI Data Liaison not installed
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddOntologySuggestions:      true,
			AddApprovalTools:            true,
			AddRequestTools:             true,
		},
	})

	// AI Data Liaison NOT installed (only Ontology Forge)
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDOntologyForge), // Only Ontology Forge
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestToolsWithDataLiaison()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Developer Data Liaison tools should NOT be present
	devDataLiaisonTools := []string{
		"list_query_suggestions",
		"approve_query_suggestion",
		"reject_query_suggestion",
		"create_approved_query",
		"update_approved_query",
		"delete_approved_query",
	}
	for _, name := range devDataLiaisonTools {
		if containsTool(filtered, name) {
			t.Errorf("expected tool %s to be HIDDEN when AI Data Liaison is not installed", name)
		}
	}

	// Core developer tools should still be present (MCP Server + Ontology Forge)
	coreDevTools := []string{"health", "echo", "execute", "get_schema"}
	for _, name := range coreDevTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_DataLiaisonInstalled_DeveloperTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// All toggles enabled
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddOntologySuggestions:      true,
			AddApprovalTools:            true,
			AddRequestTools:             true,
		},
	})

	// AI Data Liaison IS installed
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDAIDataLiaison), // AI Data Liaison installed
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestToolsWithDataLiaison()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Developer Data Liaison tools SHOULD be present when app is installed
	devDataLiaisonTools := []string{
		"list_query_suggestions",
		"approve_query_suggestion",
		"reject_query_suggestion",
		"create_approved_query",
		"update_approved_query",
		"delete_approved_query",
	}
	for _, name := range devDataLiaisonTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present when AI Data Liaison is installed", name)
		}
	}
}

func TestNewToolFilter_DataLiaisonNotInstalled_NilService_FallbackToHidden(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// All toggles enabled
	setupTestConfigWithToolGroups(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddOntologySuggestions:      true,
			AddApprovalTools:            true,
			AddRequestTools:             true,
		},
	})

	// InstalledAppService is nil - should default to hiding non-MCP-Server app tools
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: nil, // Nil service - fallback to not installed
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestToolsWithDataLiaison()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// All Data Liaison tools should be hidden when service is nil
	allDataLiaisonTools := []string{
		"suggest_approved_query",
		"suggest_query_update",
		"list_query_suggestions",
		"approve_query_suggestion",
		"reject_query_suggestion",
		"create_approved_query",
		"update_approved_query",
		"delete_approved_query",
	}
	for _, name := range allDataLiaisonTools {
		if containsTool(filtered, name) {
			t.Errorf("expected tool %s to be HIDDEN when InstalledAppService is nil", name)
		}
	}
}

func TestNewToolFilter_AIAgentsNotInstalled_AgentToolsHidden(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with agent_tools enabled
	setupTestConfigWithAgentTools(t, engineDB.DB, projectID, true)

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(), // ai-agents NOT installed
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Agent authentication
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Agent tools should be hidden when ai-agents app is NOT installed,
	// even though agent_tools config is enabled
	if len(filtered) != 1 {
		t.Errorf("expected 1 tool (health only), got %d: %v", len(filtered), toolNames(filtered))
	}
	if !containsTool(filtered, "health") {
		t.Error("health tool should always be present")
	}
}

func TestNewToolFilter_AIAgentsInstalled_AgentToolsShown(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with agent_tools enabled
	setupTestConfigWithAgentTools(t, engineDB.DB, projectID, true)

	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDAIAgents), // ai-agents IS installed
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// Agent authentication
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Agent tools should be visible when BOTH config enabled AND app installed
	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools, got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}
	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}
