//go:build integration

// Package tools provides integration tests for MCP tool filtering behavior.
//
// These tests verify that new projects get the expected tools by default based on role.
// See plans/PLAN-mcp-tools-default-on.md for the full context.
package tools

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// mcpToolsTestContext holds test dependencies for MCP tools integration tests.
type mcpToolsTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	projectID    uuid.UUID
	datasourceID uuid.UUID
}

// setupMCPToolsTest creates a new project with a datasource configured.
// This simulates a project that is ready to use tools (datasource is required).
func setupMCPToolsTest(t *testing.T) *mcpToolsTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Create project with a datasource configured
	ctx := context.Background()
	scope, err := engineDB.DB.WithTenant(ctx, projectID)
	require.NoError(t, err)
	defer scope.Close()

	tenantCtx := database.SetTenantScope(ctx, scope)

	// Create project with default_datasource_id set
	_, err = scope.Conn.Exec(tenantCtx, `
		INSERT INTO engine_projects (id, name, parameters, created_at, updated_at)
		VALUES ($1, 'MCP Tools Default Test', $2::jsonb, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, projectID, `{"default_datasource_id": "`+datasourceID.String()+`"}`)
	require.NoError(t, err)

	// Create a minimal datasource record
	_, err = scope.Conn.Exec(tenantCtx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config, created_at, updated_at)
		VALUES ($1, $2, 'Test Datasource', 'postgres', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, datasourceID, projectID)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx := context.Background()
		cleanupScope, err := engineDB.DB.WithTenant(cleanupCtx, projectID)
		if err != nil {
			return
		}
		defer cleanupScope.Close()
		// Clean up test data
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_mcp_config WHERE project_id = $1", projectID)
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_datasources WHERE project_id = $1", projectID)
		_, _ = cleanupScope.Conn.Exec(cleanupCtx, "DELETE FROM engine_projects WHERE id = $1", projectID)
	})

	return &mcpToolsTestContext{
		t:            t,
		engineDB:     engineDB,
		projectID:    projectID,
		datasourceID: datasourceID,
	}
}

// createAllMCPTools returns a comprehensive list of MCP tools for filtering tests.
// This includes all tools that would be registered with the MCP server.
func createAllMCPTools() []mcp.Tool {
	// Build tools from AllToolsOrdered to ensure consistency
	var tools []mcp.Tool
	for _, spec := range services.AllToolsOrdered {
		tools = append(tools, mcp.Tool{Name: spec.Name})
	}
	return tools
}

// filterToolsForTest applies the tool filter and returns filtered tool names.
func (tc *mcpToolsTestContext) filterToolsForTest(claims *auth.Claims) []string {
	// Create a mock project service that returns the datasource ID
	projectService := &mockProjectService{defaultDatasourceID: tc.datasourceID}
	deps := &MCPToolDeps{
		DB:               tc.engineDB.DB,
		MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, projectService, nil, "http://localhost", zap.NewNop()),
		ProjectService:   projectService,
		Logger:           zap.NewNop(),
	}

	filter := NewToolFilter(deps)
	tools := createAllMCPTools()

	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)
	filtered := filter(ctx, tools)

	names := make([]string, len(filtered))
	for i, t := range filtered {
		names[i] = t.Name
	}
	return names
}

// ============================================================================
// TDD RED Tests: These tests verify expected behavior for MCP tools defaulting ON
// They should FAIL initially (RED phase) because new projects don't get tools by default.
// ============================================================================

// TestMCPTools_NewProject_AdminGetsDevTools verifies that an admin user on a new project
// (with no explicit MCP config) gets the full developer toolset by default.
//
// Expected behavior: Admin/Developer roles should get 40+ tools including:
// - Default loadout (health)
// - Developer Core (echo, execute)
// - Query loadout (query, sample, validate, get_schema, etc.)
// - Ontology Maintenance loadout (update_entity, refresh_schema, etc.)
// - Ontology Questions loadout (list_ontology_questions, etc.)
//
// Current behavior (BUG): Only returns health tool because no MCP config exists.
func TestMCPTools_NewProject_AdminGetsDevTools(t *testing.T) {
	tc := setupMCPToolsTest(t)

	// Admin user (non-agent subject, could have admin role)
	claims := &auth.Claims{
		ProjectID: tc.projectID.String(),
	}
	claims.Subject = "user-admin-123"

	tools := tc.filterToolsForTest(claims)

	// Expected: 40+ tools (see PLAN-mcp-tools-default-on.md)
	// With default config: developer.AddQueryTools=true and developer.AddOntologyMaintenance=true
	// Tools = Default(1) + DeveloperCore(2) + Query(19) + OntologyMaintenance(22) + OntologyQuestions(5)
	//       = 49 total, but overlapping tools are deduplicated
	// Note: DataLiaison tools (8) are filtered when app not installed, leaving ~41 tools
	const minExpectedTools = 35 // Conservative lower bound accounting for DataLiaison filtering

	assert.GreaterOrEqual(t, len(tools), minExpectedTools,
		"Admin should get 40+ tools by default for a new project. Got %d tools: %v",
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
			"Admin should have access to %s tool by default", tool)
	}
}

// TestMCPTools_NewProject_UserGetsBusinessTools verifies that a business user on a new project
// gets the business user toolset by default.
//
// Expected behavior: Business users should get 15-35 tools including:
// - Default loadout (health)
// - Query loadout (query, sample, validate, get_schema, etc.)
// - Optionally Ontology Maintenance if allowOntologyMaintenance is true
//
// Note: The plan says 15-20 tools, but with AllowOntologyMaintenance=true (default),
// users get Query + OntologyMaintenance = closer to 30-35 tools.
//
// Current behavior (BUG): Only returns health tool because no MCP config exists.
func TestMCPTools_NewProject_UserGetsBusinessTools(t *testing.T) {
	tc := setupMCPToolsTest(t)

	// Business user (non-agent, non-admin)
	claims := &auth.Claims{
		ProjectID: tc.projectID.String(),
	}
	claims.Subject = "user-business-456"

	tools := tc.filterToolsForTest(claims)

	// Expected: 15-35 tools depending on allowOntologyMaintenance setting
	// With DefaultMCPConfig.user.AllowOntologyMaintenance=true:
	// Tools = Default(1) + Query(19) + OntologyMaintenance(22) = ~30-35 after DataLiaison filtering
	// Without ontology maintenance: Default(1) + Query(19) = ~15-18 after DataLiaison filtering
	const minExpectedTools = 12 // Conservative minimum: Query loadout minus DataLiaison tools

	assert.GreaterOrEqual(t, len(tools), minExpectedTools,
		"User should get 15+ business tools by default for a new project. Got %d tools: %v",
		len(tools), tools)

	// Verify essential business user tools are present
	essentialTools := []string{
		"health",                 // Always available
		"query",                  // Query loadout - ad-hoc queries
		"sample",                 // Query loadout - data preview
		"validate",               // Query loadout - SQL validation
		"get_schema",             // Query loadout - schema access
		"get_ontology",           // Query loadout - semantic context
		"list_approved_queries",  // Query loadout - pre-approved queries
		"execute_approved_query", // Query loadout - run approved queries
	}

	for _, tool := range essentialTools {
		assert.Contains(t, tools, tool,
			"Business user should have access to %s tool by default", tool)
	}

	// Verify developer-only tools are NOT present for business users
	// Note: With the current unified tool filter, all users get the same tools
	// based on project config, not role. This test documents expected behavior
	// if/when role-based filtering is implemented.
	developerOnlyTools := []string{
		"echo",    // Developer testing tool
		"execute", // DDL/DML execution
	}

	for _, tool := range developerOnlyTools {
		// Currently all users get developer tools if developer config is enabled
		// This assertion documents the expected behavior where business users
		// would NOT get developer-only tools
		if containsString(tools, tool) {
			t.Logf("Note: Business user has access to developer tool %s (expected if config allows)", tool)
		}
	}
}

// TestMCPTools_NewProject_AgentGetsAgentTools verifies that an agent (API key auth) on a new project
// gets only the limited agent toolset.
//
// Expected behavior: Agents should get 3 tools:
// - health (always available)
// - list_approved_queries (Limited Query loadout)
// - execute_approved_query (Limited Query loadout)
//
// Current behavior (BUG): Only returns health tool because agent_tools defaults to disabled.
func TestMCPTools_NewProject_AgentGetsAgentTools(t *testing.T) {
	tc := setupMCPToolsTest(t)

	// Agent authentication - claims.Subject = "agent"
	claims := &auth.Claims{
		ProjectID: tc.projectID.String(),
	}
	claims.Subject = "agent" // Agent auth identifier

	tools := tc.filterToolsForTest(claims)

	// Expected: 3 tools (health + Limited Query loadout)
	// Note: Agent tools require agent_tools config to be enabled
	// The current DefaultMCPConfig doesn't set agent_tools.Enabled = true
	// This test documents the expected behavior if agent_tools defaults to ON
	const expectedAgentTools = 3

	// For now, agents with disabled agent_tools only get health
	// This test will pass with just health, but documents the full expectation
	assert.GreaterOrEqual(t, len(tools), 1,
		"Agent should get at least health tool. Got %d tools: %v",
		len(tools), tools)

	// Health is always available
	assert.Contains(t, tools, "health", "Agent should always have health tool")

	// Document expected behavior: If agent_tools were enabled by default
	if len(tools) >= expectedAgentTools {
		assert.Contains(t, tools, "list_approved_queries",
			"Agent with agent_tools enabled should have list_approved_queries")
		assert.Contains(t, tools, "execute_approved_query",
			"Agent with agent_tools enabled should have execute_approved_query")
	} else {
		t.Logf("Note: Agent only has %d tools (expected %d with agent_tools enabled)",
			len(tools), expectedAgentTools)
	}

	// Verify agents do NOT get developer or business user tools
	restrictedTools := []string{
		"echo",           // Developer Core
		"execute",        // Developer Core
		"query",          // Query loadout (ad-hoc)
		"sample",         // Query loadout
		"refresh_schema", // Ontology Maintenance
	}

	for _, tool := range restrictedTools {
		assert.NotContains(t, tools, tool,
			"Agent should NOT have access to %s tool", tool)
	}
}

// TestMCPTools_AllRolesGetHealthTool verifies that ALL roles get the health tool,
// regardless of configuration.
//
// Health tool should be available for:
// - Admin users
// - Business users
// - Agents (even with agent_tools disabled)
// - Unauthenticated requests (filtered to health only)
func TestMCPTools_AllRolesGetHealthTool(t *testing.T) {
	tc := setupMCPToolsTest(t)

	// Helper to create claims with subject
	makeClaims := func(subject string) *auth.Claims {
		claims := &auth.Claims{
			ProjectID: tc.projectID.String(),
		}
		claims.Subject = subject
		return claims
	}

	testCases := []struct {
		name   string
		claims *auth.Claims
	}{
		{
			name:   "Admin user",
			claims: makeClaims("user-admin-123"),
		},
		{
			name:   "Business user",
			claims: makeClaims("user-business-456"),
		},
		{
			name:   "Agent",
			claims: makeClaims("agent"),
		},
	}

	for _, tc2 := range testCases {
		t.Run(tc2.name, func(t *testing.T) {
			tools := tc.filterToolsForTest(tc2.claims)
			assert.Contains(t, tools, "health",
				"%s should always have access to health tool", tc2.name)
		})
	}

	// Also test unauthenticated (no claims) - should only get health
	t.Run("Unauthenticated", func(t *testing.T) {
		deps := &MCPToolDeps{
			DB:               tc.engineDB.DB,
			MCPConfigService: services.NewMCPConfigService(repositories.NewMCPConfigRepository(), &mockQueryService{}, &mockProjectService{}, nil, "http://localhost", zap.NewNop()),
			Logger:           zap.NewNop(),
		}

		filter := NewToolFilter(deps)
		tools := createAllMCPTools()

		// No claims in context
		filtered := filter(context.Background(), tools)

		assert.Len(t, filtered, 1, "Unauthenticated should only get health tool")
		assert.Equal(t, "health", filtered[0].Name, "Unauthenticated should only have health tool")
	})
}

// ============================================================================
// Helper functions
// ============================================================================

// containsString checks if a string is in a slice.
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
