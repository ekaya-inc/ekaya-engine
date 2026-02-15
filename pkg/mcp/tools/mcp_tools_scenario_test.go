//go:build integration

// Package tools provides scenario-based integration tests for MCP tool filtering.
//
// These tests follow the actual user journey:
// 1. Project is provisioned (no datasource yet)
// 2. Datasource is added
// 3. Schema is imported
// 4. Tools become available based on auth type
//
// See plans/PLAN-mcp-tools-default-on.md for context.
package tools

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// ============================================================================
// Configurable Mock Project Service
// ============================================================================

// scenarioProjectService is a mock that can be configured during the test.
// It tracks datasource state so we can test the tool filter's behavior
// as the project progresses through setup stages.
type scenarioProjectService struct {
	mu                  sync.RWMutex
	defaultDatasourceID uuid.UUID
}

func newScenarioProjectService() *scenarioProjectService {
	return &scenarioProjectService{}
}

func (m *scenarioProjectService) setDefaultDatasourceID(id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultDatasourceID = id
}

func (m *scenarioProjectService) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultDatasourceID, nil
}

// Implement remaining ProjectService interface methods (unused in these tests)
func (m *scenarioProjectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*services.ProvisionResult, error) {
	return nil, nil
}
func (m *scenarioProjectService) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*services.ProvisionResult, error) {
	return nil, nil
}
func (m *scenarioProjectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}
func (m *scenarioProjectService) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}
func (m *scenarioProjectService) Delete(ctx context.Context, id uuid.UUID) (*services.DeleteResult, error) {
	return &services.DeleteResult{}, nil
}
func (m *scenarioProjectService) CompleteDeleteCallback(ctx context.Context, projectID uuid.UUID, action, status, nonce string) (*services.DeleteCallbackResult, error) {
	return &services.DeleteCallbackResult{}, nil
}
func (m *scenarioProjectService) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	return nil
}
func (m *scenarioProjectService) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {}
func (m *scenarioProjectService) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", nil
}
func (m *scenarioProjectService) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return nil
}
func (m *scenarioProjectService) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*services.AutoApproveSettings, error) {
	return nil, nil
}
func (m *scenarioProjectService) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *services.AutoApproveSettings) error {
	return nil
}
func (m *scenarioProjectService) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*services.OntologySettings, error) {
	return &services.OntologySettings{UseLegacyPatternMatching: true}, nil
}
func (m *scenarioProjectService) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *services.OntologySettings) error {
	return nil
}
func (m *scenarioProjectService) SyncServerURL(ctx context.Context, projectID uuid.UUID, papiURL, token string) error {
	return nil
}

// ============================================================================
// Scenario Test Context
// ============================================================================

// mcpScenarioTest provides a reusable test context for MCP tool scenario tests.
// It tracks the project through its lifecycle: creation -> datasource -> schema -> tools.
type mcpScenarioTest struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	projectID    uuid.UUID
	datasourceID uuid.UUID

	// Configurable mock for project service
	projectService *scenarioProjectService

	// Services used for setup and verification
	mcpConfigService services.MCPConfigService
	toolFilter       func(context.Context, []mcp.Tool) []mcp.Tool
}

// newMCPScenarioTest creates a new scenario test with a freshly provisioned project.
// The project has NO datasource, NO MCP config - just like a real new project.
func newMCPScenarioTest(t *testing.T) *mcpScenarioTest {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()

	// Create project directly in DB - simulates what provisioning does
	// This includes creating MCP config with defaults (as of the fix)
	ctx := context.Background()
	scope, err := engineDB.DB.WithTenant(ctx, projectID)
	require.NoError(t, err)

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, created_at, updated_at)
		VALUES ($1, 'MCP Scenario Test Project', NOW(), NOW())`,
		projectID)
	require.NoError(t, err, "Failed to create project")

	// Create MCP config with defaults - this is what provisioning does now
	defaultToolGroups := `{
		"user": {"allowOntologyMaintenance": true},
		"developer": {"addQueryTools": true, "addOntologyMaintenance": true},
		"agent_tools": {"enabled": true}
	}`
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_mcp_config (project_id, tool_groups, created_at, updated_at)
		VALUES ($1, $2::jsonb, NOW(), NOW())`,
		projectID, defaultToolGroups)
	require.NoError(t, err, "Failed to create MCP config")

	scope.Close()

	// Create configurable mock project service
	projectService := newScenarioProjectService()

	// Create MCP config service (uses real repository, not mock)
	mcpConfigService := services.NewMCPConfigService(
		repositories.NewMCPConfigRepository(),
		&mockQueryService{},
		projectService,
		nil,
		"http://localhost",
		zap.NewNop(),
	)

	// Create tool filter with real dependencies and configurable project service
	deps := &MCPToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mcpConfigService,
			Logger:           zap.NewNop(),
		},
		ProjectService: projectService,
	}

	st := &mcpScenarioTest{
		t:                t,
		engineDB:         engineDB,
		projectID:        projectID,
		projectService:   projectService,
		mcpConfigService: mcpConfigService,
		toolFilter:       NewToolFilter(deps),
	}

	t.Cleanup(func() {
		st.cleanup()
	})

	return st
}

// cleanup removes all test data for this project.
func (st *mcpScenarioTest) cleanup() {
	ctx := context.Background()
	scope, err := st.engineDB.DB.WithTenant(ctx, st.projectID)
	if err != nil {
		return
	}
	defer scope.Close()

	// Clean up in reverse dependency order
	if st.datasourceID != uuid.Nil {
		_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_datasources WHERE id = $1", st.datasourceID)
	}
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_mcp_config WHERE project_id = $1", st.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_projects WHERE id = $1", st.projectID)
}

// addDatasource creates a datasource for the project and sets it as the default.
// This simulates the user adding a datasource in the UI.
func (st *mcpScenarioTest) addDatasource() {
	st.datasourceID = uuid.New()

	// Insert datasource into database
	ctx := context.Background()
	scope, err := st.engineDB.DB.WithTenant(ctx, st.projectID)
	require.NoError(st.t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, provider, datasource_config)
		VALUES ($1, $2, 'Test Datasource', 'postgres', 'postgresql', '{}')`,
		st.datasourceID, st.projectID)
	require.NoError(st.t, err, "Failed to create datasource")

	// Update the mock project service to return this datasource as the default
	st.projectService.setDefaultDatasourceID(st.datasourceID)
}

// getToolsForUser returns the tools available to a user with the given subject.
// Subject determines auth type:
// - "agent" = agent auth (API key)
// - anything else = user auth (OAuth)
func (st *mcpScenarioTest) getToolsForUser(subject string) []string {
	claims := &auth.Claims{
		ProjectID: st.projectID.String(),
	}
	claims.Subject = subject

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	allTools := createAllMCPTools()
	filtered := st.toolFilter(ctx, allTools)

	names := make([]string, len(filtered))
	for i, t := range filtered {
		names[i] = t.Name
	}
	return names
}

// getToolsForAdmin returns tools available to an admin user.
func (st *mcpScenarioTest) getToolsForAdmin() []string {
	return st.getToolsForUser("user-admin-123")
}

// getToolsForBusinessUser returns tools available to a business user.
func (st *mcpScenarioTest) getToolsForBusinessUser() []string {
	return st.getToolsForUser("user-business-456")
}

// getToolsForAgent returns tools available to an agent (API key auth).
func (st *mcpScenarioTest) getToolsForAgent() []string {
	return st.getToolsForUser("agent")
}

// ============================================================================
// Scenario: New Project Without Datasource
// ============================================================================

// TestScenario_NewProject_NoDatasource_AllAuthTypesGetOnlyHealth verifies that
// when a project is first provisioned (no datasource configured), ALL auth types
// should only get the "health" tool.
//
// Rationale: Without a datasource, tools like query, execute, sample, etc. are
// useless and potentially confusing. The health tool is always available for
// connectivity checks.
//
// This test covers:
// - Admin user auth
// - Business user auth
// - Agent auth (API key)
//
// Expected: Each auth type gets exactly 1 tool: "health"
func TestScenario_NewProject_NoDatasource_AllAuthTypesGetOnlyHealth(t *testing.T) {
	st := newMCPScenarioTest(t)

	t.Run("Admin gets only health tool", func(t *testing.T) {
		tools := st.getToolsForAdmin()

		assert.Len(t, tools, 1,
			"Admin should get exactly 1 tool (health) when no datasource exists. Got %d tools: %v",
			len(tools), tools)
		assert.Contains(t, tools, "health",
			"Admin should have access to health tool")
	})

	t.Run("Business user gets only health tool", func(t *testing.T) {
		tools := st.getToolsForBusinessUser()

		assert.Len(t, tools, 1,
			"Business user should get exactly 1 tool (health) when no datasource exists. Got %d tools: %v",
			len(tools), tools)
		assert.Contains(t, tools, "health",
			"Business user should have access to health tool")
	})

	t.Run("Agent gets only health tool", func(t *testing.T) {
		tools := st.getToolsForAgent()

		assert.Len(t, tools, 1,
			"Agent should get exactly 1 tool (health) when no datasource exists. Got %d tools: %v",
			len(tools), tools)
		assert.Contains(t, tools, "health",
			"Agent should have access to health tool")
	})
}

// TestScenario_NewProject_NoDatasource_MCPConfigCreatedWithDefaults verifies that
// provisioning a project creates an MCP config record with correct defaults.
//
// Rationale: MCP config should be created eagerly during provisioning to ensure
// consistent state. This prevents bugs where other code paths (like SetAgentAPIKey)
// might create config with incorrect defaults.
func TestScenario_NewProject_NoDatasource_MCPConfigCreatedWithDefaults(t *testing.T) {
	st := newMCPScenarioTest(t)

	// Check that MCP config WAS created during provisioning
	ctx := context.Background()
	scope, err := st.engineDB.DB.WithTenant(ctx, st.projectID)
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx,
		"SELECT COUNT(*) FROM engine_mcp_config WHERE project_id = $1",
		st.projectID).Scan(&count)
	require.NoError(t, err)

	assert.Equal(t, 1, count,
		"Provisioning a project should create an MCP config record with defaults.")

	// Verify the config has correct defaults
	tenantCtx := database.SetTenantScope(ctx, scope)
	config, err := st.mcpConfigService.Get(tenantCtx, st.projectID)
	require.NoError(t, err)
	require.NotNil(t, config)

	// Developer tools should have sub-options enabled
	devConfig := config.ToolGroups["developer"]
	require.NotNil(t, devConfig, "Developer config should exist")
	assert.True(t, devConfig.AddQueryTools,
		"Developer AddQueryTools should default to true")
	assert.True(t, devConfig.AddOntologyMaintenance,
		"Developer AddOntologyMaintenance should default to true")

	// Agent tools should be enabled
	agentConfig := config.ToolGroups["agent_tools"]
	require.NotNil(t, agentConfig, "Agent config should exist")
	assert.True(t, agentConfig.Enabled,
		"Agent tools Enabled should default to true")
}

// ============================================================================
// Scenario: Project With Datasource
// ============================================================================

// TestScenario_ProjectWithDatasource_AllAuthTypesGetExpectedTools verifies that
// after a datasource is added, each auth type gets the appropriate tools.
//
// Expected tool counts based on auth type:
// - Admin: 40+ tools (Developer Tools with Query + Ontology Maintenance)
// - Business User: 40+ tools (same as admin in current implementation)
// - Agent: 3 tools (health + list_approved_queries + execute_approved_query)
func TestScenario_ProjectWithDatasource_AllAuthTypesGetExpectedTools(t *testing.T) {
	st := newMCPScenarioTest(t)

	// Add datasource - this is the key transition
	st.addDatasource()

	t.Run("Admin gets full developer tools", func(t *testing.T) {
		tools := st.getToolsForAdmin()

		// Admin should get Developer Tools with all sub-options enabled (40+ tools)
		const minExpectedTools = 35 // Conservative lower bound accounting for DataLiaison filtering
		assert.GreaterOrEqual(t, len(tools), minExpectedTools,
			"Admin should get 40+ tools with datasource configured. Got %d tools: %v",
			len(tools), tools)

		// Verify essential developer tools are present
		essentialTools := []string{
			"health",                  // Always available
			"echo",                    // Developer Core
			"execute",                 // Developer Core
			"query",                   // Query loadout
			"sample",                  // Query loadout
			"validate",                // Query loadout
			"get_schema",              // Query loadout
			"get_context",             // Query loadout
			"refresh_schema",          // Ontology Maintenance
			"list_ontology_questions", // Ontology Questions
		}
		for _, tool := range essentialTools {
			assert.Contains(t, tools, tool,
				"Admin should have access to %s tool", tool)
		}
	})

	t.Run("Business user gets query and ontology tools", func(t *testing.T) {
		tools := st.getToolsForBusinessUser()

		// Business user should get Query + Ontology tools
		// Note: In current implementation, user auth gets same tools as admin
		// because the tool filter uses GetEnabledTools which is config-based, not role-based
		const minExpectedTools = 12 // Query loadout minus DataLiaison tools
		assert.GreaterOrEqual(t, len(tools), minExpectedTools,
			"Business user should get 15+ tools with datasource configured. Got %d tools: %v",
			len(tools), tools)

		// Verify essential business user tools are present
		essentialTools := []string{
			"health",                 // Always available
			"query",                  // Query loadout
			"sample",                 // Query loadout
			"validate",               // Query loadout
			"get_schema",             // Query loadout
			"get_ontology",           // Query loadout
			"list_approved_queries",  // Query loadout
			"execute_approved_query", // Query loadout
		}
		for _, tool := range essentialTools {
			assert.Contains(t, tools, tool,
				"Business user should have access to %s tool", tool)
		}
	})

	t.Run("Agent gets limited query tools", func(t *testing.T) {
		tools := st.getToolsForAgent()

		// Agent should get exactly 3 tools: health + approved query tools
		assert.Len(t, tools, 3,
			"Agent should get exactly 3 tools (health + approved queries). Got %d tools: %v",
			len(tools), tools)

		// Verify agent tools
		expectedTools := []string{
			"health",
			"list_approved_queries",
			"execute_approved_query",
		}
		for _, tool := range expectedTools {
			assert.Contains(t, tools, tool,
				"Agent should have access to %s tool", tool)
		}

		// Verify agent does NOT have developer tools
		restrictedTools := []string{
			"echo",
			"execute",
			"query",
			"sample",
		}
		for _, tool := range restrictedTools {
			assert.NotContains(t, tools, tool,
				"Agent should NOT have access to %s tool", tool)
		}
	})
}
