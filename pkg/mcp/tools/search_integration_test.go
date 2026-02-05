//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// searchToolTestContext holds test dependencies for search tool integration tests.
type searchToolTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	projectID  uuid.UUID
	mcpServer  *server.MCPServer
	schemaRepo repositories.SchemaRepository
}

// setupSearchToolIntegrationTest initializes the test context with shared testcontainer.
func setupSearchToolIntegrationTest(t *testing.T) *searchToolTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000077")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Search Tool Integration Test Project")
	require.NoError(t, err)

	// Create MCP server with search tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	schemaRepo := repositories.NewSchemaRepository()

	// Configure mock to enable developer tools
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:       true,
			AddQueryTools: true,
		},
	}

	deps := &SearchToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mockMCPConfig,
		SchemaRepo:       schemaRepo,
		Logger:           zap.NewNop(),
	}

	RegisterSearchTools(mcpServer, deps)

	return &searchToolTestContext{
		t:          t,
		engineDB:   engineDB,
		projectID:  projectID,
		mcpServer:  mcpServer,
		schemaRepo: schemaRepo,
	}
}

// cleanup removes test data.
func (tc *searchToolTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	// Clean up schema tables if any
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_columns WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_schema_tables WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *searchToolTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{
		ProjectID: tc.projectID.String(),
		Roles:     []string{models.RoleAdmin},
	})
	ctx = models.WithMCPProvenance(ctx, uuid.Nil)

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *searchToolTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	tc.t.Helper()

	reqID := 1
	callReq := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"id":      reqID,
		"params": map[string]any{
			"name":      toolName,
			"arguments": arguments,
		},
	}

	reqBytes, err := json.Marshal(callReq)
	require.NoError(tc.t, err)

	result := tc.mcpServer.HandleMessage(ctx, reqBytes)

	resultBytes, err := json.Marshal(result)
	require.NoError(tc.t, err)

	var response struct {
		Result *mcp.CallToolResult `json:"result,omitempty"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	err = json.Unmarshal(resultBytes, &response)
	require.NoError(tc.t, err)

	if response.Error != nil {
		return nil, &mcpError{Code: response.Error.Code, Message: response.Error.Message}
	}

	return response.Result, nil
}

// TestSearchSchemaTool_Integration_EmptySearch verifies that search_schema
// gracefully handles an empty query result.
func TestSearchSchemaTool_Integration_EmptySearch(t *testing.T) {
	tc := setupSearchToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Search without any data created
	result, err := tc.callTool(ctx, "search_schema", map[string]any{
		"query": "nonexistent_xyz_12345",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should not return error when no matches")

	// Parse response
	var searchResp searchResult
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &searchResp)
	require.NoError(t, err)

	// Verify empty results (no error, just no matches)
	assert.Len(t, searchResp.Tables, 0, "should return empty tables")
	assert.Len(t, searchResp.Columns, 0, "should return empty columns")
	assert.Equal(t, 0, searchResp.TotalCount, "total count should be 0")
}
