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
// it can also be called via checkApprovedQueriesEnabled. This ensures no mismatch
// between what tools are visible and what tools are executable.

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
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING shows approved queries tools for agent auth
	filterDeps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		Logger:           zap.NewNop(),
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
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		ProjectService:   &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:     &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		Logger:           zap.NewNop(),
	}

	// checkApprovedQueriesEnabled is called when executing list_approved_queries or execute_approved_query
	_, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, queryDeps)
	if err != nil {
		t.Fatalf("CALLING: checkApprovedQueriesEnabled failed: %v\n"+
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

// TestApprovedQueriesEnabled_ListAndCallConsistency tests that when approved_queries
// is enabled for regular user auth, tools are both listed and callable.
func TestApprovedQueriesEnabled_ListAndCallConsistency(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: approved_queries enabled (normal user flow)
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"approved_queries": {Enabled: true},
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING shows approved queries tools
	filterDeps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		Logger:           zap.NewNop(),
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

	t.Log("LISTING: approved queries tools are correctly visible for user auth with approved_queries enabled")

	// Part 2: Verify tool CALLING works
	queryDeps := &QueryToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		ProjectService:   &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:     &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		Logger:           zap.NewNop(),
	}

	_, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, queryDeps)
	if err != nil {
		t.Fatalf("CALLING: checkApprovedQueriesEnabled failed: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: approved queries tools are correctly callable for user auth with approved_queries enabled")
}

// TestNeitherEnabled_NeitherListedNorCallable tests that when neither agent_tools
// nor approved_queries is enabled, the tools are neither listed nor callable.
func TestNeitherEnabled_NeitherListedNorCallable(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: nothing enabled
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		// Empty - nothing enabled
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{},
		&mockProjectService{},
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING does NOT show approved queries tools
	filterDeps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// Regular user auth
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "user-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	if containsTool(filteredTools, "list_approved_queries") {
		t.Error("LISTING: list_approved_queries should NOT be visible when nothing is enabled")
	}
	if containsTool(filteredTools, "execute_approved_query") {
		t.Error("LISTING: execute_approved_query should NOT be visible when nothing is enabled")
	}

	t.Log("LISTING: approved queries tools are correctly hidden when nothing is enabled")

	// Part 2: Verify tool CALLING fails
	queryDeps := &QueryToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		ProjectService:   &mockProjectService{},
		QueryService:     &mockQueryService{},
		Logger:           zap.NewNop(),
	}

	_, _, cleanup, err := checkApprovedQueriesEnabled(ctx, queryDeps)
	if cleanup != nil {
		defer cleanup()
	}

	if err == nil {
		t.Fatal("CALLING: checkApprovedQueriesEnabled should fail when nothing is enabled")
	}

	t.Logf("CALLING: correctly rejected with error: %v", err)
}

// TestAgentAuth_AgentToolsDisabled tests that when agent_tools is disabled,
// agent auth should not see or be able to call approved queries tools.
func TestAgentAuth_AgentToolsDisabled(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: agent_tools disabled, approved_queries enabled
	// Agent should NOT be able to use tools even though approved_queries is enabled
	// (approved_queries is for users, agent_tools is for agents)
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools":      {Enabled: false},
		"approved_queries": {Enabled: true},
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING does NOT show tools for agent
	filterDeps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		Logger:           zap.NewNop(),
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
// approved_queries are enabled, a regular user sees approved queries via the
// approved_queries path (not agent_tools).
func TestBothEnabled_UserSeesApprovedQueries(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: both enabled
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools":      {Enabled: true},
		"approved_queries": {Enabled: true},
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify user sees approved queries tools
	filterDeps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		Logger:           zap.NewNop(),
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
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		ProjectService:   &mockProjectService{defaultDatasourceID: uuid.New()},
		QueryService:     &mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		Logger:           zap.NewNop(),
	}

	_, tenantCtx, cleanup, err := checkApprovedQueriesEnabled(ctx, queryDeps)
	if err != nil {
		t.Fatalf("CALLING: checkApprovedQueriesEnabled failed for user: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: approved queries tools callable for user")
}

// TestAgentToolsEnabled_EchoListAndCallConsistency tests that when agent_tools is enabled,
// the echo tool is both listed AND callable for agent authentication.
// This is a regression test for the bug where echo was listed but calls failed with
// "developer tools are not enabled for this project".
func TestAgentToolsEnabled_EchoListAndCallConsistency(t *testing.T) {
	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Setup: agent_tools enabled, developer tools NOT enabled
	setupTestProject(t, engineDB.DB, projectID, map[string]*models.ToolGroupConfig{
		"agent_tools": {Enabled: true},
		// developer tools is NOT in this config - this is the key test case
	})

	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{enabledQueries: []*models.Query{{ID: uuid.New()}}},
		&mockProjectService{defaultDatasourceID: uuid.New()},
		"http://localhost",
		zap.NewNop(),
	)

	// Part 1: Verify tool LISTING shows echo for agent auth
	filterDeps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(filterDeps)
	allTools := createTestTools()

	// Create agent auth context
	claims := &auth.Claims{ProjectID: projectID.String()}
	claims.Subject = "agent" // Agent authentication
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	filteredTools := filter(ctx, allTools)

	// Verify echo is in the filtered list
	if !containsTool(filteredTools, "echo") {
		t.Fatal("LISTING: echo should be visible when agent_tools is enabled")
	}

	t.Log("LISTING: echo tool is correctly visible for agent auth with agent_tools enabled")

	// Part 2: Verify tool CALLING works (this is where the bug was)
	devDeps := &DeveloperToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mcpConfigService,
		ProjectService:   &mockProjectService{defaultDatasourceID: uuid.New()},
		Logger:           zap.NewNop(),
	}

	// checkEchoEnabled is called when executing echo tool
	_, tenantCtx, cleanup, err := checkEchoEnabled(ctx, devDeps)
	if err != nil {
		t.Fatalf("CALLING: checkEchoEnabled failed: %v\n"+
			"This means echo is LISTED but cannot be CALLED - list/call inconsistency!", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if tenantCtx == nil {
		t.Fatal("CALLING: expected tenant context to be set")
	}

	t.Log("CALLING: echo tool is correctly callable for agent auth with agent_tools enabled")
}
