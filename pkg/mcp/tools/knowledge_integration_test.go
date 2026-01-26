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

// knowledgeToolTestContext holds test dependencies for knowledge tool integration tests.
type knowledgeToolTestContext struct {
	t                   *testing.T
	engineDB            *testhelpers.EngineDB
	projectID           uuid.UUID
	mcpServer           *server.MCPServer
	knowledgeRepository repositories.KnowledgeRepository
}

// setupKnowledgeToolIntegrationTest initializes the test context with shared testcontainer.
func setupKnowledgeToolIntegrationTest(t *testing.T) *knowledgeToolTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000044")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Knowledge Tool Integration Test Project")
	require.NoError(t, err)

	// Create MCP server with knowledge tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	knowledgeRepo := repositories.NewKnowledgeRepository()

	// Configure mock to enable knowledge tools (developer with ontology maintenance)
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:                true,
			AddOntologyMaintenance: true, // Enables ontology maintenance tools including knowledge tools
		},
	}

	deps := &KnowledgeToolDeps{
		DB:                  engineDB.DB,
		MCPConfigService:    mockMCPConfig,
		KnowledgeRepository: knowledgeRepo,
		Logger:              zap.NewNop(),
	}

	RegisterKnowledgeTools(mcpServer, deps)

	return &knowledgeToolTestContext{
		t:                   t,
		engineDB:            engineDB,
		projectID:           projectID,
		mcpServer:           mcpServer,
		knowledgeRepository: knowledgeRepo,
	}
}

// cleanup removes test knowledge facts.
func (tc *knowledgeToolTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_project_knowledge WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *knowledgeToolTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	// Include admin role to access developer tools (knowledge update/delete require ontology maintenance)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{
		ProjectID: tc.projectID.String(),
		Roles:     []string{models.RoleAdmin},
	})

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *knowledgeToolTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
	tc.t.Helper()

	// Build MCP request
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

	// Call the tool through MCP server
	result := tc.mcpServer.HandleMessage(ctx, reqBytes)

	// Parse response
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

// mcpError represents an MCP JSON-RPC error.
type mcpError struct {
	Code    int
	Message string
}

func (e *mcpError) Error() string {
	return e.Message
}

// ============================================================================
// Integration Tests: update_project_knowledge
// ============================================================================

func TestUpdateProjectKnowledgeTool_Integration_CreateNewFact(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call update_project_knowledge tool
	result, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact":     "A tik represents 6 seconds of engagement",
		"category": "terminology",
		"context":  "Found in billing_engagements table",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response updateProjectKnowledgeResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify response fields
	assert.NotEmpty(t, response.FactID, "fact_id should be set")
	assert.Equal(t, "A tik represents 6 seconds of engagement", response.Fact)
	assert.Equal(t, "terminology", response.Category)
	assert.Equal(t, "Found in billing_engagements table", response.Context)

	// Verify fact was persisted in database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_project_knowledge
		WHERE project_id = $1 AND fact_type = 'terminology' AND key = $2
	`, tc.projectID, "A tik represents 6 seconds of engagement").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "fact should exist in database")
}

func TestUpdateProjectKnowledgeTool_Integration_UpdateExistingFact(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create initial fact
	initialFact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  "business_rule",
		Key:       "Platform fees calculation",
		Value:     "Platform fees calculation",
		Context:   "Initial observation",
	}
	err := tc.knowledgeRepository.Upsert(ctx, initialFact)
	require.NoError(t, err)
	factID := initialFact.ID

	// Update the fact with new context using fact_id
	result, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact":     "Platform fees calculation",
		"category": "business_rule",
		"context":  "Verified: tikr_share/total_amount ≈ 0.33",
		"fact_id":  factID.String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response updateProjectKnowledgeResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify fact_id is preserved
	assert.Equal(t, factID.String(), response.FactID)
	assert.Equal(t, "Verified: tikr_share/total_amount ≈ 0.33", response.Context)

	// Verify database has only one fact (update, not insert)
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_project_knowledge
		WHERE project_id = $1 AND fact_type = 'business_rule'
	`, tc.projectID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should have exactly one fact after update")
}

func TestUpdateProjectKnowledgeTool_Integration_DefaultCategory(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call without category parameter (should default to terminology)
	result, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact": "GMV means Gross Merchandise Value",
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response updateProjectKnowledgeResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify default category
	assert.Equal(t, "terminology", response.Category, "category should default to terminology")
}

func TestUpdateProjectKnowledgeTool_Integration_EmptyFactError(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with empty fact (whitespace only)
	result, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact": "   ",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error result")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "fact")
	assert.Contains(t, errorResp.Message, "empty")
}

func TestUpdateProjectKnowledgeTool_Integration_InvalidCategory(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with invalid category
	result, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact":     "Test fact",
		"category": "invalid_category",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error result")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "category")
}

func TestUpdateProjectKnowledgeTool_Integration_InvalidFactID(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with invalid fact_id (not a UUID)
	result, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact":    "Test fact",
		"fact_id": "not-a-uuid",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error result")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "fact_id")
	assert.Contains(t, errorResp.Message, "UUID")
}

// ============================================================================
// Integration Tests: delete_project_knowledge
// ============================================================================

func TestDeleteProjectKnowledgeTool_Integration_DeleteExistingFact(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a fact to delete
	fact := &models.KnowledgeFact{
		ProjectID: tc.projectID,
		FactType:  "convention",
		Key:       "Fact to delete",
		Value:     "Fact to delete",
		Context:   "Test context",
	}
	err := tc.knowledgeRepository.Upsert(ctx, fact)
	require.NoError(t, err)
	factID := fact.ID

	// Call delete_project_knowledge tool
	result, err := tc.callTool(ctx, "delete_project_knowledge", map[string]any{
		"fact_id": factID.String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Parse response
	var response deleteProjectKnowledgeResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify response
	assert.Equal(t, factID.String(), response.FactID)
	assert.True(t, response.Deleted)

	// Verify fact was deleted from database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_project_knowledge WHERE id = $1
	`, factID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "fact should be deleted from database")
}

func TestDeleteProjectKnowledgeTool_Integration_FactNotFound(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call delete with non-existent fact_id
	nonExistentID := uuid.New()
	result, err := tc.callTool(ctx, "delete_project_knowledge", map[string]any{
		"fact_id": nonExistentID.String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error result for not found")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "FACT_NOT_FOUND", errorResp.Code)
	assert.Contains(t, errorResp.Message, "not found")
	assert.Contains(t, errorResp.Message, nonExistentID.String())
}

func TestDeleteProjectKnowledgeTool_Integration_EmptyFactID(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with empty fact_id (whitespace only)
	result, err := tc.callTool(ctx, "delete_project_knowledge", map[string]any{
		"fact_id": "   ",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error result")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "fact_id")
	assert.Contains(t, errorResp.Message, "empty")
}

func TestDeleteProjectKnowledgeTool_Integration_InvalidUUID(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with invalid UUID format
	result, err := tc.callTool(ctx, "delete_project_knowledge", map[string]any{
		"fact_id": "not-a-valid-uuid",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error result")

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "fact_id")
	assert.Contains(t, errorResp.Message, "UUID")
}

// ============================================================================
// Integration Tests: Complete Workflows
// ============================================================================

func TestKnowledgeTools_Integration_CreateUpdateDeleteWorkflow(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create a new fact
	createResult, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact":     "User accounts expire after 90 days of inactivity",
		"category": "business_rule",
		"context":  "Found in user cleanup job",
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)

	var createResponse updateProjectKnowledgeResponse
	require.Len(t, createResult.Content, 1)
	err = json.Unmarshal([]byte(createResult.Content[0].(mcp.TextContent).Text), &createResponse)
	require.NoError(t, err)
	factID := createResponse.FactID

	// Step 2: Update the fact with additional context
	updateResult, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
		"fact":     "User accounts expire after 90 days of inactivity",
		"category": "business_rule",
		"context":  "Found in user cleanup job. Confirmed in accounts_service.go:123",
		"fact_id":  factID,
	})
	require.NoError(t, err)
	require.NotNil(t, updateResult)

	var updateResponse updateProjectKnowledgeResponse
	require.Len(t, updateResult.Content, 1)
	err = json.Unmarshal([]byte(updateResult.Content[0].(mcp.TextContent).Text), &updateResponse)
	require.NoError(t, err)

	assert.Equal(t, factID, updateResponse.FactID, "fact_id should be preserved")
	assert.Contains(t, updateResponse.Context, "accounts_service.go:123")

	// Step 3: Delete the fact
	deleteResult, err := tc.callTool(ctx, "delete_project_knowledge", map[string]any{
		"fact_id": factID,
	})
	require.NoError(t, err)
	require.NotNil(t, deleteResult)

	var deleteResponse deleteProjectKnowledgeResponse
	require.Len(t, deleteResult.Content, 1)
	err = json.Unmarshal([]byte(deleteResult.Content[0].(mcp.TextContent).Text), &deleteResponse)
	require.NoError(t, err)

	assert.Equal(t, factID, deleteResponse.FactID)
	assert.True(t, deleteResponse.Deleted)

	// Step 4: Verify deletion - try to delete again
	deleteAgainResult, err := tc.callTool(ctx, "delete_project_knowledge", map[string]any{
		"fact_id": factID,
	})
	require.NoError(t, err)
	require.NotNil(t, deleteAgainResult)
	assert.True(t, deleteAgainResult.IsError, "should return error for already deleted fact")
}

func TestKnowledgeTools_Integration_MultipleFactsWithDifferentCategories(t *testing.T) {
	tc := setupKnowledgeToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create facts with different categories
	categories := []struct {
		fact     string
		category string
	}{
		{"MRR stands for Monthly Recurring Revenue", "terminology"},
		{"Discount cannot exceed 50% of total price", "business_rule"},
		{"Order status can be: PENDING, PROCESSING, SHIPPED, DELIVERED", "enumeration"},
		{"Soft delete pattern: is_deleted column with timestamp", "convention"},
	}

	factIDs := make([]string, len(categories))
	for i, cat := range categories {
		result, err := tc.callTool(ctx, "update_project_knowledge", map[string]any{
			"fact":     cat.fact,
			"category": cat.category,
		})
		require.NoError(t, err)
		require.NotNil(t, result)

		var response updateProjectKnowledgeResponse
		require.Len(t, result.Content, 1)
		err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
		require.NoError(t, err)

		factIDs[i] = response.FactID
		assert.Equal(t, cat.category, response.Category)
	}

	// Verify all facts exist in database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_project_knowledge WHERE project_id = $1
	`, tc.projectID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, len(categories), count, "all facts should exist in database")
}
