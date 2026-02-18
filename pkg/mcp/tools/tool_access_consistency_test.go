package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// TestToolAccessConsistency verifies that if a tool is listed via NewToolFilter,
// it can also be called via AcquireToolAccess. This ensures no mismatch
// between what tools are visible and what tools are executable.

// consistencyTestDatasourceID is a fixed UUID used in tests to simulate a configured datasource.
var consistencyTestDatasourceID = uuid.MustParse("22222222-2222-2222-2222-222222222222")

// consistencyMockProjectService returns a mock project service with a datasource configured.
func consistencyMockProjectService() *mockProjectService {
	return &mockProjectService{defaultDatasourceID: consistencyTestDatasourceID}
}

// setupTestProject creates a project and MCP config for testing.
func setupTestProject(t *testing.T, db *database.DB, projectID uuid.UUID, toolGroups map[string]*models.ToolGroupConfig) {
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

	if toolGroups != nil {
		mcpConfig := &models.MCPConfig{
			ProjectID:  projectID,
			ToolGroups: toolGroups,
		}

		repo := repositories.NewMCPConfigRepository()
		if err := repo.Upsert(tenantCtx, mcpConfig); err != nil {
			t.Fatalf("failed to create test config: %v", err)
		}
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

// TestAgentToolsEnabled_ListAndCallConsistency tests that when agent_tools is enabled,
// the approved queries tools are both listed AND callable for agent authentication.
// This is a regression test for the bug where tools were listed but calls failed with
// "approved queries tools are not enabled for this project".
func TestAgentToolsEnabled_ListAndCallConsistency(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: agent_tools enabled, approved_queries NOT enabled
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools": {Enabled: true},
		// approved_queries is NOT in this config - this is the key test case
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil, // installedAppService - not needed for this test
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING shows approved queries tools for agent auth
	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// Create agent auth context
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent" // Agent authentication
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	// Verify list_approved_queries is in the filtered list
	if !containsTool(filteredTools, "list_approved_queries") {
		t.Fatal("LISTING: list_approved_queries should be visible when agent_tools is enabled")
	}
	if !containsTool(filteredTools, "execute_approved_query") {
		t.Fatal("LISTING: execute_approved_query should be visible when agent_tools is enabled")
	}

	t.Log("LISTING: approved queries tools are correctly visible for agent auth with agent_tools enabled")

	// Part 2: Verify tool CALLING works (this is where the bug was)
	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:   &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
	}

	// AcquireToolAccess is called when executing list_approved_queries or execute_approved_query
	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, queryDeps, "list_approved_queries")
	if err != nil {
		t.Fatalf("CALLING: AcquireToolAccess failed: %v\n"+
			"This means tools are LISTED but cannot be CALLED - list/call inconsistency!", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: approved queries tools are correctly callable for agent auth with agent_tools enabled")
}

// TestApprovedQueriesEnabled_ListAndCallConsistency tests that when AddQueryTools
// is enabled for regular user auth, Query loadout tools are both listed and callable.
func TestApprovedQueriesEnabled_ListAndCallConsistency(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: AddQueryTools enabled to get Query loadout
	// Note: approved_queries.Enabled is now ignored for user auth
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"developer": {AddQueryTools: true}, // Need AddQueryTools for Query loadout
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil, // installedAppService - not needed for this test
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING shows approved queries tools
	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// Create user auth context (not agent)
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123" // Regular user
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	if !containsTool(filteredTools, "list_approved_queries") {
		t.Fatal("LISTING: list_approved_queries should be visible when approved_queries is enabled")
	}

	t.Log("LISTING: approved queries tools are correctly visible for user auth with AddQueryTools enabled")

	// Part 2: Verify tool CALLING works
	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:   &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
	}

	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, queryDeps, "list_approved_queries")
	if err != nil {
		t.Fatalf("CALLING: AcquireToolAccess failed: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: approved queries tools are correctly callable for user auth with AddQueryTools enabled")
}

// TestNeitherEnabled_QueryToolsNotListed tests that when AddQueryTools is not enabled,
// Query loadout tools are not listed (but Developer Core tools are always included for users).
func TestNeitherEnabled_QueryToolsNotListed(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: no AddQueryTools enabled
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		// Empty - no sub-options enabled
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{},
		&mockProjectService{},
		nil, // installedAppService - not needed for this test
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING does NOT show Query loadout tools
	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// Admin auth (use admin role to test config-based filtering, not role-based filtering)
	claims := &auth.Claims{ProjectID: projectID.String(), Roles: []string{models.RoleAdmin}}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	// Query loadout tools should NOT be present without AddQueryTools
	if containsTool(filteredTools, "list_approved_queries") {
		t.Error("LISTING: list_approved_queries should NOT be visible without AddQueryTools")
	}
	if containsTool(filteredTools, "execute_approved_query") {
		t.Error("LISTING: execute_approved_query should NOT be visible without AddQueryTools")
	}
	if containsTool(filteredTools, "query") {
		t.Error("LISTING: query should NOT be visible without AddQueryTools")
	}

	// Developer Core tools should still be present (for admin auth)
	if !containsTool(filteredTools, "health") {
		t.Error("LISTING: health should be visible")
	}
	if !containsTool(filteredTools, "echo") {
		t.Error("LISTING: echo should be visible (Developer Core)")
	}
	if !containsTool(filteredTools, "execute") {
		t.Error("LISTING: execute should be visible (Developer Core)")
	}

	t.Log("LISTING: Query tools are correctly hidden when AddQueryTools is not enabled")

	// Part 2: Verify Query tool CALLING fails (since not in loadout)
	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: &mockProjectService{},
		QueryService:   &mockQueryService{},
	}

	_, _, cleanup, err := AcquireToolAccess(ctx, queryDeps, "list_approved_queries")
	if cleanup != nil {
		defer cleanup()
	}

	if err == nil {
		t.Fatal("CALLING: AcquireToolAccess should fail when AddQueryTools is not enabled")
	}

	t.Logf("CALLING: correctly rejected with error: %v", err)
}

// TestAgentAuth_AgentToolsDisabled tests that when agent_tools is disabled,
// agent auth should not see or be able to call approved queries tools.
func TestAgentAuth_AgentToolsDisabled(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: agent_tools disabled (agent auth still checks agent_tools.Enabled)
	// AddQueryTools is for users, agent_tools.Enabled is for agents
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools": {Enabled: false},
		"developer":   {AddQueryTools: true}, // User has Query loadout, but agent doesn't
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil, // installedAppService - not needed for this test
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING does NOT show tools for agent
	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// Agent auth
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	if containsTool(filteredTools, "list_approved_queries") {
		t.Error("LISTING: list_approved_queries should NOT be visible for agent when agent_tools is disabled")
	}

	// Only health should be visible
	if len(filteredTools) != 1 || !containsTool(filteredTools, "health") {
		t.Errorf("LISTING: only health should be visible, got: %v", toolNames(filteredTools))
	}

	t.Log("LISTING: correctly hidden for agent when agent_tools is disabled")
}

// TestBothEnabled_UserSeesApprovedQueries tests that when both agent_tools and
// AddQueryTools are enabled, a regular user sees Query loadout tools.
func TestBothEnabled_UserSeesApprovedQueries(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: both enabled
	// Note: approved_queries.Enabled is now ignored for user auth
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools": {Enabled: true},
		"developer":   {AddQueryTools: true}, // Need AddQueryTools for Query loadout
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil, // installedAppService - not needed for this test
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify user sees approved queries tools
	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// User auth (not agent)
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	if !containsTool(filteredTools, "list_approved_queries") {
		t.Fatal("LISTING: list_approved_queries should be visible for user")
	}

	t.Log("LISTING: approved queries tools visible for user when both configs enabled")

	// Part 2: Verify calling works for user
	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:   &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
	}

	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, queryDeps, "list_approved_queries")
	if err != nil {
		t.Fatalf("CALLING: AcquireToolAccess failed for user: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: approved queries tools callable for user")
}

// TestAgentToolsEnabled_LimitedQueryToolsConsistency tests that when agent_tools is enabled,
// agents only get Limited Query loadout tools (health, list_approved_queries, execute_approved_query).
// Echo is a developer tool and should NOT be available to agents.
func TestAgentToolsEnabled_LimitedQueryToolsConsistency(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: agent_tools enabled, developer tools NOT enabled
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools": {Enabled: true},
		// developer tools is NOT in this config
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil, // installedAppService - not needed for this test
		"http://localhost",
		zap.NewNop(),
	)

	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// Create agent auth context
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent" // Agent authentication
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	// Verify Limited Query loadout tools are present
	if !containsTool(filteredTools, "health") {
		t.Error("LISTING: health should be visible for agents")
	}
	if !containsTool(filteredTools, "list_approved_queries") {
		t.Error("LISTING: list_approved_queries should be visible for agents")
	}
	if !containsTool(filteredTools, "execute_approved_query") {
		t.Error("LISTING: execute_approved_query should be visible for agents")
	}

	// Verify echo is NOT available for agents (it's a developer tool)
	if containsTool(filteredTools, "echo") {
		t.Error("LISTING: echo should NOT be visible for agents (it's a developer tool)")
	}

	t.Log("LISTING: agent tools correctly limited to Limited Query loadout")

	// Verify tool CALLING consistency for list_approved_queries
	devDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
	}

	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, devDeps, "list_approved_queries")
	if err != nil {
		t.Fatalf("CALLING: AcquireToolAccess for list_approved_queries failed: %v\n"+
			"This means it is LISTED but cannot be CALLED - list/call inconsistency!", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: list_approved_queries is correctly callable for agents")
}

// TestDataLiaison_Uninstalled_ExecutionBlocked tests that data liaison tools
// cannot be called when the AI Data Liaison app is uninstalled, even if the
// tool is in the config-enabled set.
func TestDataLiaison_Uninstalled_ExecutionBlocked(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: developer with AddQueryTools (data liaison tools are in this loadout)
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"developer": {AddQueryTools: true, AddOntologyMaintenance: true},
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil,
		"http://localhost",
		zap.NewNop(),
	)

	// AI Data Liaison NOT installed
	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(), // No AI Data Liaison
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:   &mockQueryService{},
	}

	// User auth context
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	// Try to call suggest_approved_query (a data liaison tool)
	_, _, cleanup, err := AcquireToolAccess(ctx, queryDeps, "suggest_approved_query")
	if cleanup != nil {
		defer cleanup()
	}

	if err == nil {
		t.Fatal("CALLING: AcquireToolAccess should fail for data liaison tool when app is not installed")
	}

	// Verify it's a ToolAccessError (not a system error)
	if result := AsToolAccessResult(err); result == nil {
		t.Fatalf("CALLING: expected ToolAccessError, got system error: %v", err)
	}

	t.Logf("CALLING: correctly blocked with error: %v", err)
}

// TestDataLiaison_Installed_ExecutionAllowed tests that data liaison tools
// can be called when the AI Data Liaison app is installed.
func TestDataLiaison_Installed_ExecutionAllowed(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: developer with AddQueryTools (data liaison tools are in this loadout)
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"developer": {AddQueryTools: true, AddOntologyMaintenance: true},
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil,
		"http://localhost",
		zap.NewNop(),
	)

	// AI Data Liaison IS installed
	queryDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: newMockInstalledAppService(models.AppIDAIDataLiaison),
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:   &mockQueryService{},
	}

	// Data role auth context (data liaison tools require at least data role with RBAC)
	claims := &auth.Claims{ProjectID: projectID.String(), Roles: []string{models.RoleData}}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	// Try to call suggest_approved_query (a data liaison tool)
	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, queryDeps, "suggest_approved_query")
	if err != nil {
		t.Fatalf("CALLING: AcquireToolAccess should succeed when app is installed: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: data liaison tool correctly allowed when app is installed")
}

// TestAIAgents_Uninstalled_ListAndCallBlocked tests that agent tools are both
// hidden from listing AND blocked from execution when ai-agents app is uninstalled.
func TestAIAgents_Uninstalled_ListAndCallBlocked(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: agent_tools enabled in config
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools": {Enabled: true},
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil,
		"http://localhost",
		zap.NewNop(),
	)

	// ai-agents NOT installed
	appService := newMockInstalledAppService() // No ai-agents

	// Part 1: Verify LISTING hides agent tools
	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: appService,
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	if containsTool(filteredTools, "list_approved_queries") {
		t.Error("LISTING: list_approved_queries should be hidden when ai-agents app not installed")
	}
	if len(filteredTools) != 1 || !containsTool(filteredTools, "health") {
		t.Errorf("LISTING: expected only health, got: %v", toolNames(filteredTools))
	}

	t.Log("LISTING: agent tools correctly hidden when ai-agents not installed")

	// Part 2: Verify CALLING is also blocked
	callDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: appService,
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:   &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
	}

	_, _, cleanup, err := AcquireToolAccess(ctx, callDeps, "list_approved_queries")
	if cleanup != nil {
		defer cleanup()
	}

	if err == nil {
		t.Fatal("CALLING: AcquireToolAccess should fail for agent when ai-agents app not installed")
	}

	if result := AsToolAccessResult(err); result == nil {
		t.Fatalf("CALLING: expected ToolAccessError, got system error: %v", err)
	}

	t.Logf("CALLING: correctly blocked with error: %v", err)
}

// TestAIAgents_Installed_ListAndCallAllowed tests that agent tools are both
// visible in listing AND callable when ai-agents app is installed.
func TestAIAgents_Installed_ListAndCallAllowed(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: agent_tools enabled in config
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools": {Enabled: true},
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		nil,
		"http://localhost",
		zap.NewNop(),
	)

	// ai-agents IS installed
	appService := newMockInstalledAppService(models.AppIDAIAgents)

	// Part 1: Verify LISTING shows agent tools
	filterDeps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: appService,
		},
		ProjectService: consistencyMockProjectService(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	expectedTools := []string{"health", "list_approved_queries", "execute_approved_query"}
	if len(filteredTools) != len(expectedTools) {
		t.Errorf("LISTING: expected %d tools, got %d: %v", len(expectedTools), len(filteredTools), toolNames(filteredTools))
	}
	for _, name := range expectedTools {
		if !containsTool(filteredTools, name) {
			t.Errorf("LISTING: expected tool %s to be present", name)
		}
	}

	t.Log("LISTING: agent tools correctly visible when ai-agents installed")

	// Part 2: Verify CALLING works
	callDeps := &QueryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:                  engineDB.DB,
			MCPConfigService:    mcpConfigService,
			Logger:              zap.NewNop(),
			InstalledAppService: appService,
		},
		ProjectService: &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:   &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
	}

	_, tenantCtx, cleanup, err := AcquireToolAccess(ctx, callDeps, "list_approved_queries")
	if err != nil {
		t.Fatalf("CALLING: AcquireToolAccess failed: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: agent tools correctly callable when ai-agents installed")
}
