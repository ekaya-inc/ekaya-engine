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

	// Create config with developer tools "disabled" via Enabled=false
	// Note: For user auth, the Enabled flag is now ignored - tools are determined by sub-options
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: false})

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

	// For user auth, tools are always enabled - only sub-options control loadouts
	// With no sub-options (AddQueryTools=false, AddOntologyMaintenance=false), user gets Developer Core only
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Developer Core = health + echo + execute = 3 tools
	if len(filtered) != 3 {
		t.Errorf("expected 3 tools (Developer Core), got %d: %v", len(filtered), toolNames(filtered))
	}
	for _, name := range []string{"health", "echo", "execute"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_DeveloperEnabled_AllToolsAvailable(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with developer tools enabled AND AddQueryTools=true for all tools
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, AddQueryTools: true})

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

	// Developer mode with AddQueryTools = ALL tools available (full access for developers)
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have ALL tools (12 total) when developer is enabled with AddQueryTools
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	// Execute should be included (part of Developer Core)
	if !containsTool(filtered, "execute") {
		t.Error("expected execute tool to be present in developer mode")
	}

	// All business user tools should also be present in developer mode
	if !containsTool(filtered, "query") {
		t.Error("expected query tool to be present in developer mode")
	}
	if !containsTool(filtered, "sample") {
		t.Error("expected sample tool to be present in developer mode")
	}
	if !containsTool(filtered, "validate") {
		t.Error("expected validate tool to be present in developer mode")
	}

	// All developer-specific tools should be present
	for _, name := range []string{"health", "echo", "get_schema"} {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_DeveloperEnabled_VerifyAllTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with developer tools enabled AND AddQueryTools=true for all tools
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, AddQueryTools: true})

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

	// Developer mode with AddQueryTools = ALL tools available (full access)
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, &auth.Claims{ProjectID: projectID.String()})
	filtered := filter(ctx, tools)

	// Should have ALL 12 tools when developer is enabled with AddQueryTools
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	// All tools should be present
	allTools := []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema", "list_approved_queries", "execute_approved_query", "get_ontology", "list_glossary", "get_glossary_sql"}
	for _, name := range allTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present in developer mode", name)
		}
	}
}

func TestNewToolFilter_NilConfig(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()
	// Don't create any config - should use defaults (all tools ON)

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

	// No config - uses DefaultMCPConfig which has AddQueryTools=true and AddOntologyMaintenance=true
	// This gives: Developer Core + Query + OntologyMaintenance + OntologyQuestions loadouts
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

func TestNewToolFilter_DeveloperEnabled_ApprovedQueriesOff(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Developer enabled with AddQueryTools=true (only developer is ON, approved_queries is OFF)
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, AddQueryTools: true})

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

	// Developer mode with AddQueryTools = ALL tools available (12 total)
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	// All tools should be present in developer mode with AddQueryTools
	allTools := []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema", "list_approved_queries", "execute_approved_query", "get_ontology", "list_glossary", "get_glossary_sql"}
	for _, name := range allTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present in developer mode", name)
		}
	}
}

func TestNewToolFilter_ApprovedQueriesOnly_NoQueriesExist(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Legacy test: approved_queries enabled, developer "disabled"
	// Note: For user auth, the Enabled flag is now ignored. Developer Core is always included.
	// Sub-options (AddQueryTools, AddOntologyMaintenance) control additional loadouts.
	// Since no developer sub-options are set, user gets Developer Core only.
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		nil,                                    // no developer config (uses defaults: AddQueryTools=false, AddOntologyMaintenance=false)
		&models.ToolGroupConfig{Enabled: true}, // approved_queries Enabled is also now ignored
	)

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

	// For user auth, tools are always enabled. With no explicit developer sub-options,
	// user gets Developer Core loadout (health + echo + execute).
	// The approved_queries.Enabled flag is no longer checked for user auth.
	expectedTools := []string{"health", "echo", "execute"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools (Developer Core), got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Query tools should NOT be present since AddQueryTools=false
	if containsTool(filtered, "query") {
		t.Error("query tool should not be present without AddQueryTools=true")
	}
}

func TestNewToolFilter_ApprovedQueriesEnabledWithQueries(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Both developer and approved_queries "enabled" - but Enabled flag is now ignored for user auth
	// Developer has AddQueryTools=false by default, so only Developer Core is enabled
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: true}, // Enabled is ignored, but no AddQueryTools
		&models.ToolGroupConfig{Enabled: true}, // Enabled is ignored for user auth
	)

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

	// Developer Core = health + echo + execute = 3 tools
	// (no AddQueryTools, so Query loadout not included)
	expectedTools := []string{"health", "echo", "execute"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools (Developer Core), got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}

	// Query tools should NOT be present since AddQueryTools=false
	if containsTool(filtered, "query") {
		t.Error("query tool should not be present without AddQueryTools=true")
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

	// ForceMode is a legacy feature. For user auth, Developer Core is always included.
	// ForceMode on approved_queries has no effect on user auth tool filtering.
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		nil, // no developer config - uses defaults (AddQueryTools=false)
		&models.ToolGroupConfig{Enabled: true, ForceMode: true}, // ForceMode is ignored for user auth
	)

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

	// For user auth, Developer Core is always included (3 tools)
	// No AddQueryTools, so no Query loadout
	expectedTools := []string{"health", "echo", "execute"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools (Developer Core), got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_ForceModeOffAllowsDeveloperTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// ForceMode is a legacy feature. For user auth, sub-options control loadouts.
	// Need AddQueryTools=true to get all tools.
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: true, AddQueryTools: true}, // Need AddQueryTools for all tools
		&models.ToolGroupConfig{Enabled: true, ForceMode: false},    // ForceMode is ignored
	)

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

	// With AddQueryTools=true, all test tools (12) should be present
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all) with AddQueryTools, got %d: %v", len(filtered), toolNames(filtered))
	}

	// Query and developer tools should be present
	if !containsTool(filtered, "query") {
		t.Error("query tool should be present with AddQueryTools")
	}
	if !containsTool(filtered, "execute") {
		t.Error("execute tool should be present")
	}
}

// Test legacy Mode 2: Pre-Approved ON, Developer "OFF"
// Note: For user auth, the Enabled flag is now ignored. Developer Core is always included.
func TestNewToolFilter_ApprovedQueriesOn_DeveloperOff(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Pre-Approved ON, Developer "OFF" - but Enabled flag is ignored for user auth
	setupTestConfigWithApprovedQueries(t, engineDB.DB, projectID,
		&models.ToolGroupConfig{Enabled: false},                  // Enabled is ignored, AddQueryTools=false by default
		&models.ToolGroupConfig{Enabled: true, ForceMode: false}, // Enabled is ignored for user auth
	)

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

	// For user auth, Developer Core is always included (3 tools)
	// No AddQueryTools=true in config, so no Query loadout
	expectedTools := []string{"health", "echo", "execute"}
	if len(filtered) != len(expectedTools) {
		t.Errorf("expected %d tools (Developer Core), got %d: %v", len(expectedTools), len(filtered), toolNames(filtered))
	}

	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

// Test that execute tool is always available when Developer Tools is enabled
// Execute is part of Developer Core loadout and doesn't require a separate flag
func TestNewToolFilter_DeveloperEnabled_ExecuteAlwaysAvailable(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Developer ON with AddQueryTools=true - execute should always be visible
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true, AddQueryTools: true})

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

	// Developer mode with AddQueryTools = 12 tools (all)
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all), got %d: %v", len(filtered), toolNames(filtered))
	}

	// Execute should always be present when developer is enabled
	if !containsTool(filtered, "execute") {
		t.Error("execute tool should be present when developer is enabled")
	}

	// All tools should be present
	allTools := []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema", "list_approved_queries", "execute_approved_query", "get_ontology", "list_glossary", "get_glossary_sql"}
	for _, name := range allTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present in developer mode", name)
		}
	}
}

// Test developer mode with only Developer Core (no AddQueryTools)
// Verifies that developer mode gives Developer Core tools only by default
func TestNewToolFilter_DeveloperOnly_CoreToolsOnly(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Developer tools enabled without AddQueryTools - only Developer Core tools
	setupTestConfig(t, engineDB.DB, projectID, &models.ToolGroupConfig{Enabled: true})

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

	// Developer mode without AddQueryTools = 3 tools (health + echo + execute)
	if len(filtered) != 3 {
		t.Errorf("expected 3 tools (Developer Core), got %d: %v", len(filtered), toolNames(filtered))
	}

	// Developer Core tools should be present
	coreTools := []string{"health", "echo", "execute"}
	for _, name := range coreTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected Developer Core tool %s to be present", name)
		}
	}

	// Query tools should NOT be present without AddQueryTools
	if containsTool(filtered, "query") {
		t.Error("query should NOT be present without AddQueryTools")
	}
	if containsTool(filtered, "get_schema") {
		t.Error("get_schema should NOT be present without AddQueryTools")
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

	// Create config with developer tools enabled AND AddQueryTools=true for all tools
	// (agent_tools config doesn't affect user authentication)
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
			"developer": {Enabled: true, AddQueryTools: true},
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
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestTools()

	// User authentication - claims.Subject is NOT "agent"
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123" // Normal user
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// User with developer mode + AddQueryTools = ALL tools (12 total)
	if len(filtered) != 12 {
		t.Errorf("expected 12 tools (all) for user in developer mode, got %d: %v", len(filtered), toolNames(filtered))
	}

	// All tools should be present in developer mode with AddQueryTools
	allTools := []string{"health", "echo", "execute", "query", "sample", "validate", "get_schema", "list_approved_queries", "execute_approved_query", "get_ontology", "list_glossary", "get_glossary_sql"}
	for _, name := range allTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present in developer mode", name)
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

	// Create config with AddQueryTools enabled (to get Query loadout)
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
			"developer": {AddQueryTools: true}, // Need AddQueryTools for Query loadout
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

	// AI Data Liaison NOT installed
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(), // No AI Data Liaison
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestToolsWithDataLiaison()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	// Data Liaison tools should NOT be present
	dataLiaisonTools := []string{"suggest_approved_query", "suggest_query_update"}
	for _, name := range dataLiaisonTools {
		if containsTool(filtered, name) {
			t.Errorf("expected tool %s to be HIDDEN when AI Data Liaison is not installed", name)
		}
	}

	// Query loadout tools should be present
	expectedTools := []string{"health", "query", "sample", "validate", "list_approved_queries", "execute_approved_query"}
	for _, name := range expectedTools {
		if !containsTool(filtered, name) {
			t.Errorf("expected tool %s to be present", name)
		}
	}
}

func TestNewToolFilter_DataLiaisonInstalled_BusinessTools(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create config with AddQueryTools enabled (to get Query loadout)
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
			"developer": {AddQueryTools: true}, // Need AddQueryTools for Query loadout
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
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
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

	// Create config with developer tools enabled including ontology maintenance
	// (data liaison dev tools are in the ontology maintenance loadout)
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
			"developer": {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
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

	// AI Data Liaison NOT installed
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, mockProjectServiceWithDatasource(), nil, "http://localhost", zap.NewNop()),
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(), // No AI Data Liaison
		},
		ProjectService: mockProjectServiceWithDatasource(),
	}

	filter := NewToolFilter(deps)
	tools := createTestToolsWithDataLiaison()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
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

	// Core developer tools should still be present
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

	// Create config with developer tools enabled including ontology maintenance
	// (data liaison dev tools are in the ontology maintenance loadout)
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
			"developer": {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
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
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
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

	// Create config with developer tools enabled including ontology maintenance
	// (data liaison dev tools are in the ontology maintenance loadout)
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
			"developer": {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
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

	// InstalledAppService is nil - should default to hiding data liaison tools
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
	ctx = context.WithValue(context.Background(), auth.ClaimsKey, claims)
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
