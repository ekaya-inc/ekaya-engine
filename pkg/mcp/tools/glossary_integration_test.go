//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// glossaryToolTestContext holds test dependencies for glossary tool integration tests.
type glossaryToolTestContext struct {
	t                  *testing.T
	engineDB           *testhelpers.EngineDB
	projectID          uuid.UUID
	mcpServer          *server.MCPServer
	glossaryRepository repositories.GlossaryRepository
}

// setupGlossaryToolIntegrationTest initializes the test context with shared testcontainer.
func setupGlossaryToolIntegrationTest(t *testing.T) *glossaryToolTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000045")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Glossary Tool Integration Test Project")
	require.NoError(t, err)

	// Create MCP server with glossary tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	glossaryRepo := repositories.NewGlossaryRepository()

	// Configure mock to enable glossary tools (developer tools enabled with ontology maintenance)
	// update_glossary_term and delete_glossary_term are in LoadoutOntologyMaintenance,
	// which requires AddOntologyMaintenance=true for developer tools.
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:                true, // Enables developer tools
			AddOntologyMaintenance: true, // Enables ontology maintenance tools including glossary write tools
		},
	}

	// Create a mock glossary service for testing
	mockGlossarySvc := &testGlossaryService{
		repo:      glossaryRepo,
		projectID: projectID,
		db:        engineDB.DB,
	}

	deps := &GlossaryToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			DB:               engineDB.DB,
			MCPConfigService: mockMCPConfig,
			Logger:           zap.NewNop(),
		},
		GlossaryService: mockGlossarySvc,
	}

	RegisterGlossaryTools(mcpServer, deps)

	return &glossaryToolTestContext{
		t:                  t,
		engineDB:           engineDB,
		projectID:          projectID,
		mcpServer:          mcpServer,
		glossaryRepository: glossaryRepo,
	}
}

// testGlossaryService is a minimal implementation for integration testing.
// It bypasses SQL validation (which requires a datasource) but uses real repository.
type testGlossaryService struct {
	repo      repositories.GlossaryRepository
	projectID uuid.UUID
	db        *database.DB
}

func (s *testGlossaryService) CreateTerm(ctx context.Context, projectID uuid.UUID, term *models.BusinessGlossaryTerm) error {
	// Validate test term (same check as production)
	if services.IsTestTerm(term.Term) {
		return services.ErrTestTermRejected
	}

	// Check if term already exists (explicit create should fail on duplicate)
	existing, err := s.repo.GetByTerm(ctx, projectID, term.Term)
	if err != nil {
		return fmt.Errorf("failed to check for existing term: %w", err)
	}
	if existing != nil {
		return fmt.Errorf("term %q already exists", term.Term)
	}

	term.ProjectID = projectID
	if term.Source == "" {
		term.Source = models.GlossarySourceMCP
	}
	return s.repo.Create(ctx, term)
}

func (s *testGlossaryService) UpdateTerm(ctx context.Context, term *models.BusinessGlossaryTerm) error {
	// Validate test term (same check as production)
	if services.IsTestTerm(term.Term) {
		return services.ErrTestTermRejected
	}
	return s.repo.Update(ctx, term)
}

func (s *testGlossaryService) DeleteTerm(ctx context.Context, termID uuid.UUID) error {
	return s.repo.Delete(ctx, termID)
}

func (s *testGlossaryService) GetTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return s.repo.GetByProject(ctx, projectID)
}

func (s *testGlossaryService) GetTerm(ctx context.Context, termID uuid.UUID) (*models.BusinessGlossaryTerm, error) {
	return s.repo.GetByID(ctx, termID)
}

func (s *testGlossaryService) GetTermByName(ctx context.Context, projectID uuid.UUID, termName string) (*models.BusinessGlossaryTerm, error) {
	term, err := s.repo.GetByTerm(ctx, projectID, termName)
	if err != nil {
		return nil, err
	}
	if term == nil {
		return s.repo.GetByAlias(ctx, projectID, termName)
	}
	return term, nil
}

func (s *testGlossaryService) TestSQL(ctx context.Context, projectID uuid.UUID, sql string) (*services.SQLTestResult, error) {
	// Skip SQL validation in tests - return valid result
	return &services.SQLTestResult{Valid: true}, nil
}

func (s *testGlossaryService) CreateAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return s.repo.CreateAlias(ctx, termID, alias)
}

func (s *testGlossaryService) DeleteAlias(ctx context.Context, termID uuid.UUID, alias string) error {
	return s.repo.DeleteAlias(ctx, termID, alias)
}

func (s *testGlossaryService) SuggestTerms(ctx context.Context, projectID uuid.UUID) ([]*models.BusinessGlossaryTerm, error) {
	return nil, nil
}

func (s *testGlossaryService) DiscoverGlossaryTerms(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

func (s *testGlossaryService) EnrichGlossaryTerms(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (s *testGlossaryService) GetGenerationStatus(projectID uuid.UUID) *models.GlossaryGenerationStatus {
	return &models.GlossaryGenerationStatus{Status: "idle"}
}

func (s *testGlossaryService) RunAutoGenerate(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// cleanup removes test glossary terms.
func (tc *glossaryToolTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_business_glossary WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *glossaryToolTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	// Include admin role to access developer tools (glossary update/delete require ontology maintenance)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{
		ProjectID: tc.projectID.String(),
		Roles:     []string{models.RoleAdmin},
	})

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *glossaryToolTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
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
		return nil, &glossaryMCPError{Code: response.Error.Code, Message: response.Error.Message}
	}

	return response.Result, nil
}

// glossaryMCPError represents an MCP JSON-RPC error.
type glossaryMCPError struct {
	Code    int
	Message string
}

func (e *glossaryMCPError) Error() string {
	return e.Message
}

// ============================================================================
// Integration Tests: update_glossary_term - Test Term Validation
// ============================================================================

func TestUpdateGlossaryTermTool_Integration_RejectsTestTermOnCreate(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Test terms that should be rejected (patterns from services.IsTestTerm)
	testTerms := []string{
		"TestRevenue",    // Starts with "test"
		"RevenueTest",    // Ends with "test"
		"UITestMetric",   // UI test prefix
		"DebugCounter",   // Debug prefix
		"TodoMetric",     // Todo prefix
		"FixmeValue",     // Fixme prefix
		"DummyRevenue",   // Dummy prefix
		"SampleMetric",   // Sample prefix
		"ExampleRevenue", // Example prefix
		"Revenue2026",    // Ends with 4 digits (year pattern)
	}

	for _, testTerm := range testTerms {
		t.Run(testTerm, func(t *testing.T) {
			result, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
				"term":       testTerm,
				"definition": "Some definition for testing",
				"sql":        "SELECT SUM(amount) FROM transactions",
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError, "should return error for test term %q", testTerm)

			// Parse error response
			var errorResp ErrorResponse
			require.Len(t, result.Content, 1)
			err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "test data", "error message should mention test data")
		})
	}
}

func TestUpdateGlossaryTermTool_Integration_AcceptsValidBusinessTerms(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Valid business terms that should be accepted
	validTerms := []struct {
		term       string
		definition string
		sql        string
	}{
		{
			term:       "Revenue",
			definition: "Total earned amount from completed transactions",
			sql:        "SELECT SUM(amount) FROM transactions WHERE status = 'completed'",
		},
		{
			term:       "Active Users",
			definition: "Users who logged in within the last 30 days",
			sql:        "SELECT COUNT(DISTINCT user_id) FROM sessions WHERE created_at > NOW() - INTERVAL '30 days'",
		},
		{
			term:       "Monthly Recurring Revenue",
			definition: "Sum of recurring subscription revenue for the month",
			sql:        "SELECT SUM(monthly_amount) FROM subscriptions WHERE status = 'active'",
		},
		{
			term:       "Gross Merchandise Value",
			definition: "Total value of merchandise sold through the platform",
			sql:        "SELECT SUM(order_total) FROM orders WHERE status = 'delivered'",
		},
	}

	for _, vt := range validTerms {
		t.Run(vt.term, func(t *testing.T) {
			result, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
				"term":       vt.term,
				"definition": vt.definition,
				"sql":        vt.sql,
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.False(t, result.IsError, "should accept valid business term %q", vt.term)

			// Parse success response
			var response struct {
				Term       string `json:"term"`
				Definition string `json:"definition"`
				Created    bool   `json:"created"`
			}
			require.Len(t, result.Content, 1)
			err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
			require.NoError(t, err)

			assert.Equal(t, vt.term, response.Term)
			assert.Equal(t, vt.definition, response.Definition)
			assert.True(t, response.Created, "should be created (new term)")
		})
	}
}

func TestUpdateGlossaryTermTool_Integration_RejectsTestTermOnUpdate(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// First create a valid term
	createResult, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
		"term":       "Revenue",
		"definition": "Original definition",
		"sql":        "SELECT SUM(amount) FROM transactions",
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	assert.False(t, createResult.IsError, "should create valid term")

	// Now try to "update" with a test-like term name (this would technically be a new term
	// since update_glossary_term uses upsert semantics based on term name)
	// The validation should still reject it regardless of whether it's create or update path
	testResult, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
		"term":       "TestRevenue",
		"definition": "Test definition",
		"sql":        "SELECT 1",
	})
	require.NoError(t, err)
	require.NotNil(t, testResult)
	assert.True(t, testResult.IsError, "should reject test term even when creating new via update")

	var errorResp ErrorResponse
	require.Len(t, testResult.Content, 1)
	err = json.Unmarshal([]byte(testResult.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "test data")
}

func TestUpdateGlossaryTermTool_Integration_RejectsEmptyTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with empty term (whitespace only)
	result, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
		"term":       "   ",
		"definition": "Some definition",
		"sql":        "SELECT 1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error for empty term")

	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "term")
	assert.Contains(t, errorResp.Message, "empty")
}

// ============================================================================
// Integration Tests: update_glossary_term - Upsert Behavior
// ============================================================================

func TestUpdateGlossaryTermTool_Integration_CreatesNewTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	result, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
		"term":       "Customer Lifetime Value",
		"definition": "Total revenue expected from a customer over their relationship with the business",
		"sql":        "SELECT SUM(amount) FROM orders GROUP BY customer_id",
		"aliases":    []string{"CLV", "CLTV", "LTV"},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError)

	var response struct {
		Term       string   `json:"term"`
		Definition string   `json:"definition"`
		SQL        string   `json:"sql"`
		Aliases    []string `json:"aliases"`
		Created    bool     `json:"created"`
	}
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	assert.Equal(t, "Customer Lifetime Value", response.Term)
	assert.True(t, response.Created, "should indicate term was created")
	assert.Contains(t, response.Aliases, "CLV")

	// Verify term was persisted in database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_business_glossary
		WHERE project_id = $1 AND term = $2
	`, tc.projectID, "Customer Lifetime Value").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "term should exist in database")
}

func TestUpdateGlossaryTermTool_Integration_UpdatesExistingTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create initial term
	createResult, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
		"term":       "Churn Rate",
		"definition": "Original definition",
		"sql":        "SELECT COUNT(*) FROM users WHERE status = 'churned'",
	})
	require.NoError(t, err)
	assert.False(t, createResult.IsError)

	var createResp struct {
		Created bool `json:"created"`
	}
	err = json.Unmarshal([]byte(createResult.Content[0].(mcp.TextContent).Text), &createResp)
	require.NoError(t, err)
	assert.True(t, createResp.Created, "first call should create")

	// Update the term with same name (upsert)
	updateResult, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
		"term":       "Churn Rate",
		"definition": "Updated definition: percentage of customers who stopped subscribing",
		"sql":        "SELECT (churned_count::float / total_count) * 100 FROM user_stats",
	})
	require.NoError(t, err)
	assert.False(t, updateResult.IsError)

	var updateResp struct {
		Term       string `json:"term"`
		Definition string `json:"definition"`
		Created    bool   `json:"created"`
	}
	err = json.Unmarshal([]byte(updateResult.Content[0].(mcp.TextContent).Text), &updateResp)
	require.NoError(t, err)

	assert.Equal(t, "Churn Rate", updateResp.Term)
	assert.False(t, updateResp.Created, "second call should update, not create")
	assert.Contains(t, updateResp.Definition, "percentage")

	// Verify only one term in database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_business_glossary
		WHERE project_id = $1 AND term = $2
	`, tc.projectID, "Churn Rate").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should have exactly one term after update")
}

// ============================================================================
// Integration Tests: delete_glossary_term
// ============================================================================

func TestDeleteGlossaryTermTool_Integration_DeletesExistingTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a term to delete
	createResult, err := tc.callTool(ctx, "update_glossary_term", map[string]any{
		"term":       "Temporary Metric",
		"definition": "A metric to be deleted",
		"sql":        "SELECT 1",
	})
	require.NoError(t, err)
	assert.False(t, createResult.IsError)

	// Delete the term
	deleteResult, err := tc.callTool(ctx, "delete_glossary_term", map[string]any{
		"term": "Temporary Metric",
	})
	require.NoError(t, err)
	require.NotNil(t, deleteResult)
	assert.False(t, deleteResult.IsError)

	var deleteResp struct {
		Term    string `json:"term"`
		Deleted bool   `json:"deleted"`
	}
	err = json.Unmarshal([]byte(deleteResult.Content[0].(mcp.TextContent).Text), &deleteResp)
	require.NoError(t, err)

	assert.Equal(t, "Temporary Metric", deleteResp.Term)
	assert.True(t, deleteResp.Deleted)

	// Verify term was deleted from database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_business_glossary
		WHERE project_id = $1 AND term = $2
	`, tc.projectID, "Temporary Metric").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "term should be deleted from database")
}

func TestDeleteGlossaryTermTool_Integration_IdempotentForNonExistentTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Delete a non-existent term (should be idempotent - success with deleted=false)
	result, err := tc.callTool(ctx, "delete_glossary_term", map[string]any{
		"term": "Non Existent Term",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "delete should be idempotent for non-existent term")

	var response struct {
		Term    string `json:"term"`
		Deleted bool   `json:"deleted"`
	}
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	assert.Equal(t, "Non Existent Term", response.Term)
	assert.False(t, response.Deleted, "deleted should be false for non-existent term")
}

// ============================================================================
// Integration Tests: create_glossary_term
// ============================================================================

func TestCreateGlossaryTermTool_Integration_CreatesNewTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	result, err := tc.callTool(ctx, "create_glossary_term", map[string]any{
		"term":         "Net Promoter Score",
		"definition":   "Measure of customer loyalty based on likelihood to recommend",
		"defining_sql": "SELECT AVG(CASE WHEN score >= 9 THEN 1 WHEN score <= 6 THEN -1 ELSE 0 END) * 100 FROM surveys",
		"base_table":   "surveys",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.False(t, result.IsError, "should create term successfully")

	var response struct {
		Success bool `json:"success"`
		Term    struct {
			ID         string `json:"id"`
			Term       string `json:"term"`
			Definition string `json:"definition"`
		} `json:"term"`
	}
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	assert.True(t, response.Success)
	assert.Equal(t, "Net Promoter Score", response.Term.Term)
	assert.Contains(t, response.Term.Definition, "customer loyalty")
	assert.NotEmpty(t, response.Term.ID, "should return term ID")

	// Verify term was persisted in database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_business_glossary
		WHERE project_id = $1 AND term = $2
	`, tc.projectID, "Net Promoter Score").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "term should exist in database")
}

func TestCreateGlossaryTermTool_Integration_RejectsTestTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Test terms that should be rejected (patterns from services.IsTestTerm)
	testTerms := []string{
		"TestMetric",    // Starts with "test"
		"MetricTest",    // Ends with "test"
		"DebugRevenue",  // Debug prefix
		"DummyUsers",    // Dummy prefix
		"ExampleMetric", // Example prefix
		"Metric2025",    // Ends with 4 digits
	}

	for _, testTerm := range testTerms {
		t.Run(testTerm, func(t *testing.T) {
			result, err := tc.callTool(ctx, "create_glossary_term", map[string]any{
				"term":         testTerm,
				"definition":   "Some definition",
				"defining_sql": "SELECT 1",
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.IsError, "should return error for test term %q", testTerm)

			var errorResp ErrorResponse
			require.Len(t, result.Content, 1)
			err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "test data")
		})
	}
}

func TestCreateGlossaryTermTool_Integration_RejectsEmptyTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	result, err := tc.callTool(ctx, "create_glossary_term", map[string]any{
		"term":         "   ",
		"definition":   "Some definition",
		"defining_sql": "SELECT 1",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError, "should return error for empty term")

	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "term")
	assert.Contains(t, errorResp.Message, "empty")
}

func TestCreateGlossaryTermTool_Integration_FailsOnDuplicateTerm(t *testing.T) {
	tc := setupGlossaryToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	termName := "Average Order Value"

	// Create the term first
	createResult, err := tc.callTool(ctx, "create_glossary_term", map[string]any{
		"term":         termName,
		"definition":   "Average value of all orders",
		"defining_sql": "SELECT AVG(total) FROM orders",
	})
	require.NoError(t, err)
	require.NotNil(t, createResult)
	assert.False(t, createResult.IsError, "first create should succeed")

	// Try to create the same term again - should fail
	duplicateResult, err := tc.callTool(ctx, "create_glossary_term", map[string]any{
		"term":         termName,
		"definition":   "Different definition",
		"defining_sql": "SELECT AVG(amount) FROM transactions",
	})
	require.NoError(t, err)
	require.NotNil(t, duplicateResult)
	assert.True(t, duplicateResult.IsError, "duplicate create should fail")

	var errorResp ErrorResponse
	require.Len(t, duplicateResult.Content, 1)
	err = json.Unmarshal([]byte(duplicateResult.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "create_failed", errorResp.Code)

	// Verify still only one term in database
	scope, err := tc.engineDB.DB.WithoutTenant(context.Background())
	require.NoError(t, err)
	defer scope.Close()

	var count int
	err = scope.Conn.QueryRow(ctx, `
		SELECT COUNT(*) FROM engine_business_glossary
		WHERE project_id = $1 AND term = $2
	`, tc.projectID, termName).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "should still have exactly one term after failed duplicate create")
}
