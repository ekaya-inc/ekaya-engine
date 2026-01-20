//go:build integration

package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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

// questionToolTestContext holds test dependencies for question tool integration tests.
type questionToolTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	projectID    uuid.UUID
	ontologyID   uuid.UUID
	mcpServer    *server.MCPServer
	questionRepo repositories.OntologyQuestionRepository
}

// setupQuestionToolIntegrationTest initializes the test context with shared testcontainer.
func setupQuestionToolIntegrationTest(t *testing.T) *questionToolTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000055")
	ontologyID := uuid.MustParse("00000000-0000-0000-0000-000000000056")

	// Ensure test project exists
	ctx := context.Background()
	scope, err := engineDB.DB.WithoutTenant(ctx)
	require.NoError(t, err)
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, projectID, "Question Tool Integration Test Project")
	require.NoError(t, err)

	// Ensure test ontology exists
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, is_active, domain_summary)
		VALUES ($1, $2, true, '{"summary": "Test ontology for question tools"}')
		ON CONFLICT (id) DO NOTHING
	`, ontologyID, projectID)
	require.NoError(t, err)

	// Create MCP server with question tools
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	questionRepo := repositories.NewOntologyQuestionRepository()

	// Configure mock to enable question tools (developer with ontology questions)
	mockMCPConfig := &mockMCPConfigService{
		config: &models.ToolGroupConfig{
			Enabled:              true,
			AddOntologyQuestions: true, // Enables ontology question tools
		},
	}

	deps := &QuestionToolDeps{
		DB:               engineDB.DB,
		MCPConfigService: mockMCPConfig,
		QuestionRepo:     questionRepo,
		Logger:           zap.NewNop(),
	}

	RegisterQuestionTools(mcpServer, deps)

	return &questionToolTestContext{
		t:            t,
		engineDB:     engineDB,
		projectID:    projectID,
		ontologyID:   ontologyID,
		mcpServer:    mcpServer,
		questionRepo: questionRepo,
	}
}

// cleanup removes test questions for the project.
func (tc *questionToolTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_questions WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope and project ID.
func (tc *questionToolTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	require.NoError(tc.t, err)

	ctx = database.SetTenantScope(ctx, scope)
	ctx = context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{ProjectID: tc.projectID.String()})

	return ctx, func() { scope.Close() }
}

// callTool executes an MCP tool via the server's HandleMessage method.
func (tc *questionToolTestContext) callTool(ctx context.Context, toolName string, arguments map[string]any) (*mcp.CallToolResult, error) {
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

// createTestQuestion creates a question in the database for testing.
func (tc *questionToolTestContext) createTestQuestion(ctx context.Context, text string, category string, priority int, status models.QuestionStatus) *models.OntologyQuestion {
	tc.t.Helper()

	question := &models.OntologyQuestion{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Text:       text,
		Category:   category,
		Priority:   priority,
		IsRequired: priority <= 2, // High priority = required
		Status:     status,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := tc.questionRepo.Create(ctx, question)
	require.NoError(tc.t, err)

	return question
}

// ============================================================================
// Integration Tests: resolve_ontology_question
// ============================================================================

func TestResolveOntologyQuestionTool_Integration_ResolveQuestion(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a pending question
	question := tc.createTestQuestion(ctx, "What does status='ACTIVE' mean?", models.QuestionCategoryEnumeration, 1, models.QuestionStatusPending)

	// Resolve the question
	result, err := tc.callTool(ctx, "resolve_ontology_question", map[string]any{
		"question_id":      question.ID.String(),
		"resolution_notes": "Found in user_status.go:45 - ACTIVE means user can log in",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify response fields
	assert.Equal(t, question.ID.String(), response["question_id"])
	assert.Equal(t, "answered", response["status"])
	assert.NotEmpty(t, response["resolved_at"])
	assert.Equal(t, "Found in user_status.go:45 - ACTIVE means user can log in", response["resolution_notes"])

	// Verify database state
	updatedQuestion, err := tc.questionRepo.GetByID(ctx, question.ID)
	require.NoError(t, err)
	assert.Equal(t, models.QuestionStatusAnswered, updatedQuestion.Status)
	assert.NotNil(t, updatedQuestion.AnsweredAt)
	assert.Contains(t, updatedQuestion.Answer, "Found in user_status.go:45")
}

func TestResolveOntologyQuestionTool_Integration_QuestionNotFound(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Try to resolve a non-existent question
	nonExistentID := uuid.New()
	result, err := tc.callTool(ctx, "resolve_ontology_question", map[string]any{
		"question_id": nonExistentID.String(),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
	assert.Contains(t, errorResp.Message, nonExistentID.String())
}

func TestResolveOntologyQuestionTool_Integration_InvalidQuestionID(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with invalid UUID format
	result, err := tc.callTool(ctx, "resolve_ontology_question", map[string]any{
		"question_id": "not-a-valid-uuid",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "question_id")
}

// ============================================================================
// Integration Tests: skip_ontology_question
// ============================================================================

func TestSkipOntologyQuestionTool_Integration_SkipQuestion(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a pending question
	question := tc.createTestQuestion(ctx, "How is order total calculated?", models.QuestionCategoryBusinessRules, 2, models.QuestionStatusPending)

	// Skip the question
	result, err := tc.callTool(ctx, "skip_ontology_question", map[string]any{
		"question_id": question.ID.String(),
		"reason":      "Need access to billing service code",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify response fields
	assert.Equal(t, question.ID.String(), response["question_id"])
	assert.Equal(t, "skipped", response["status"])
	assert.Equal(t, "Need access to billing service code", response["reason"])
	assert.NotEmpty(t, response["skipped_at"])

	// Verify database state
	updatedQuestion, err := tc.questionRepo.GetByID(ctx, question.ID)
	require.NoError(t, err)
	assert.Equal(t, models.QuestionStatusSkipped, updatedQuestion.Status)
	assert.Equal(t, "Need access to billing service code", updatedQuestion.StatusReason)
}

func TestSkipOntologyQuestionTool_Integration_QuestionNotFound(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Try to skip a non-existent question
	nonExistentID := uuid.New()
	result, err := tc.callTool(ctx, "skip_ontology_question", map[string]any{
		"question_id": nonExistentID.String(),
		"reason":      "Some reason",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
}

func TestSkipOntologyQuestionTool_Integration_EmptyReason(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a pending question
	question := tc.createTestQuestion(ctx, "What does field X mean?", models.QuestionCategoryTerminology, 3, models.QuestionStatusPending)

	// Try to skip with empty reason
	result, err := tc.callTool(ctx, "skip_ontology_question", map[string]any{
		"question_id": question.ID.String(),
		"reason":      "   ", // Whitespace only
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "reason")
}

// ============================================================================
// Integration Tests: escalate_ontology_question
// ============================================================================

func TestEscalateOntologyQuestionTool_Integration_EscalateQuestion(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a pending question
	question := tc.createTestQuestion(ctx, "What business rules apply to refunds?", models.QuestionCategoryBusinessRules, 1, models.QuestionStatusPending)

	// Escalate the question
	result, err := tc.callTool(ctx, "escalate_ontology_question", map[string]any{
		"question_id": question.ID.String(),
		"reason":      "Business rules not documented in code - need product team input",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify response fields
	assert.Equal(t, question.ID.String(), response["question_id"])
	assert.Equal(t, "escalated", response["status"])
	assert.Equal(t, "Business rules not documented in code - need product team input", response["reason"])
	assert.NotEmpty(t, response["escalated_at"])

	// Verify database state
	updatedQuestion, err := tc.questionRepo.GetByID(ctx, question.ID)
	require.NoError(t, err)
	assert.Equal(t, models.QuestionStatusEscalated, updatedQuestion.Status)
	assert.Equal(t, "Business rules not documented in code - need product team input", updatedQuestion.StatusReason)
}

func TestEscalateOntologyQuestionTool_Integration_QuestionNotFound(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Try to escalate a non-existent question
	nonExistentID := uuid.New()
	result, err := tc.callTool(ctx, "escalate_ontology_question", map[string]any{
		"question_id": nonExistentID.String(),
		"reason":      "Some reason",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
}

// ============================================================================
// Integration Tests: dismiss_ontology_question
// ============================================================================

func TestDismissOntologyQuestionTool_Integration_DismissQuestion(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a pending question
	question := tc.createTestQuestion(ctx, "What does legacy_field mean?", models.QuestionCategoryTerminology, 5, models.QuestionStatusPending)

	// Dismiss the question
	result, err := tc.callTool(ctx, "dismiss_ontology_question", map[string]any{
		"question_id": question.ID.String(),
		"reason":      "Column appears unused - legacy feature from 2019",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify response fields
	assert.Equal(t, question.ID.String(), response["question_id"])
	assert.Equal(t, "dismissed", response["status"])
	assert.Equal(t, "Column appears unused - legacy feature from 2019", response["reason"])
	assert.NotEmpty(t, response["dismissed_at"])

	// Verify database state
	updatedQuestion, err := tc.questionRepo.GetByID(ctx, question.ID)
	require.NoError(t, err)
	assert.Equal(t, models.QuestionStatusDismissed, updatedQuestion.Status)
	assert.Equal(t, "Column appears unused - legacy feature from 2019", updatedQuestion.StatusReason)
}

func TestDismissOntologyQuestionTool_Integration_QuestionNotFound(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Try to dismiss a non-existent question
	nonExistentID := uuid.New()
	result, err := tc.callTool(ctx, "dismiss_ontology_question", map[string]any{
		"question_id": nonExistentID.String(),
		"reason":      "Some reason",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "QUESTION_NOT_FOUND", errorResp.Code)
}

// ============================================================================
// Integration Tests: list_ontology_questions
// ============================================================================

func TestListOntologyQuestionsTool_Integration_ListAllQuestions(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test questions with different statuses
	tc.createTestQuestion(ctx, "Question 1", models.QuestionCategoryEnumeration, 1, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Question 2", models.QuestionCategoryBusinessRules, 2, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Question 3", models.QuestionCategoryRelationship, 3, models.QuestionStatusSkipped)
	tc.createTestQuestion(ctx, "Question 4", models.QuestionCategoryTerminology, 4, models.QuestionStatusAnswered)

	// List all questions (no filters)
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify response fields
	questions := response["questions"].([]any)
	assert.Len(t, questions, 4)
	assert.Equal(t, float64(4), response["total_count"])

	// Verify counts_by_status
	countsByStatus := response["counts_by_status"].(map[string]any)
	assert.Equal(t, float64(2), countsByStatus["pending"])
	assert.Equal(t, float64(1), countsByStatus["skipped"])
	assert.Equal(t, float64(1), countsByStatus["answered"])
}

func TestListOntologyQuestionsTool_Integration_FilterByStatus(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test questions
	tc.createTestQuestion(ctx, "Pending 1", models.QuestionCategoryEnumeration, 1, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Pending 2", models.QuestionCategoryBusinessRules, 2, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Skipped 1", models.QuestionCategoryRelationship, 3, models.QuestionStatusSkipped)
	tc.createTestQuestion(ctx, "Answered 1", models.QuestionCategoryTerminology, 4, models.QuestionStatusAnswered)

	// List only pending questions
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"status": "pending",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify only pending questions returned
	questions := response["questions"].([]any)
	assert.Len(t, questions, 2)
	assert.Equal(t, float64(2), response["total_count"])

	for _, q := range questions {
		qMap := q.(map[string]any)
		assert.Equal(t, "pending", qMap["status"])
	}
}

func TestListOntologyQuestionsTool_Integration_FilterByCategory(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test questions with different categories
	tc.createTestQuestion(ctx, "Business rule 1", models.QuestionCategoryBusinessRules, 1, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Business rule 2", models.QuestionCategoryBusinessRules, 2, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Relationship 1", models.QuestionCategoryRelationship, 3, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Terminology 1", models.QuestionCategoryTerminology, 4, models.QuestionStatusPending)

	// List only business_rules questions
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"category": "business_rules",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify only business_rules questions returned
	questions := response["questions"].([]any)
	assert.Len(t, questions, 2)
	assert.Equal(t, float64(2), response["total_count"])

	for _, q := range questions {
		qMap := q.(map[string]any)
		assert.Equal(t, "business_rules", qMap["category"])
	}
}

func TestListOntologyQuestionsTool_Integration_FilterByPriority(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test questions with different priorities
	tc.createTestQuestion(ctx, "High priority 1", models.QuestionCategoryBusinessRules, 1, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "High priority 2", models.QuestionCategoryEnumeration, 1, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Medium priority", models.QuestionCategoryRelationship, 3, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Low priority", models.QuestionCategoryTerminology, 5, models.QuestionStatusPending)

	// List only priority 1 questions
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"priority": float64(1),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Verify only priority 1 questions returned
	questions := response["questions"].([]any)
	assert.Len(t, questions, 2)
	assert.Equal(t, float64(2), response["total_count"])

	for _, q := range questions {
		qMap := q.(map[string]any)
		assert.Equal(t, float64(1), qMap["priority"])
	}
}

func TestListOntologyQuestionsTool_Integration_Pagination(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create 5 test questions
	for i := 1; i <= 5; i++ {
		tc.createTestQuestion(ctx, "Question "+string(rune('A'+i-1)), models.QuestionCategoryBusinessRules, i, models.QuestionStatusPending)
	}

	// List with limit 2
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"limit": float64(2),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	questions := response["questions"].([]any)
	assert.Len(t, questions, 2)
	assert.Equal(t, float64(5), response["total_count"]) // Total is still 5

	// List with offset 2
	result2, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"limit":  float64(2),
		"offset": float64(2),
	})
	require.NoError(t, err)
	require.NotNil(t, result2)
	require.False(t, result2.IsError)

	var response2 map[string]any
	require.Len(t, result2.Content, 1)
	err = json.Unmarshal([]byte(result2.Content[0].(mcp.TextContent).Text), &response2)
	require.NoError(t, err)

	questions2 := response2["questions"].([]any)
	assert.Len(t, questions2, 2) // 2 more questions
}

func TestListOntologyQuestionsTool_Integration_InvalidStatusFilter(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with invalid status
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"status": "invalid_status",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "status")
}

func TestListOntologyQuestionsTool_Integration_InvalidCategoryFilter(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with invalid category
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"category": "invalid_category",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "category")
}

func TestListOntologyQuestionsTool_Integration_InvalidPriorityFilter(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Call with invalid priority (out of range)
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"priority": float64(6), // Max is 5
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsError)

	// Parse error response
	var errorResp ErrorResponse
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &errorResp)
	require.NoError(t, err)

	assert.True(t, errorResp.Error)
	assert.Equal(t, "invalid_parameters", errorResp.Code)
	assert.Contains(t, errorResp.Message, "priority")
}

func TestListOntologyQuestionsTool_Integration_CombinedFilters(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test questions with various combinations
	tc.createTestQuestion(ctx, "Pending business rule P1", models.QuestionCategoryBusinessRules, 1, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Pending business rule P2", models.QuestionCategoryBusinessRules, 2, models.QuestionStatusPending)
	tc.createTestQuestion(ctx, "Answered business rule P1", models.QuestionCategoryBusinessRules, 1, models.QuestionStatusAnswered)
	tc.createTestQuestion(ctx, "Pending relationship P1", models.QuestionCategoryRelationship, 1, models.QuestionStatusPending)

	// Filter by status=pending, category=business_rules, priority=1
	result, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"status":   "pending",
		"category": "business_rules",
		"priority": float64(1),
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.IsError)

	// Parse response
	var response map[string]any
	require.Len(t, result.Content, 1)
	err = json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &response)
	require.NoError(t, err)

	// Should only match 1 question
	questions := response["questions"].([]any)
	assert.Len(t, questions, 1)
	assert.Equal(t, float64(1), response["total_count"])

	q := questions[0].(map[string]any)
	assert.Equal(t, "pending", q["status"])
	assert.Equal(t, "business_rules", q["category"])
	assert.Equal(t, float64(1), q["priority"])
}

// ============================================================================
// Integration Tests: Complete Workflow
// ============================================================================

func TestQuestionTools_Integration_CompleteWorkflow(t *testing.T) {
	tc := setupQuestionToolIntegrationTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Step 1: Create questions with different priorities
	q1 := tc.createTestQuestion(ctx, "What does status ACTIVE mean?", models.QuestionCategoryEnumeration, 1, models.QuestionStatusPending)
	q2 := tc.createTestQuestion(ctx, "Business rule for refunds?", models.QuestionCategoryBusinessRules, 1, models.QuestionStatusPending)
	q3 := tc.createTestQuestion(ctx, "What does legacy_field mean?", models.QuestionCategoryTerminology, 5, models.QuestionStatusPending)
	q4 := tc.createTestQuestion(ctx, "How are users related to accounts?", models.QuestionCategoryRelationship, 2, models.QuestionStatusPending)

	// Step 2: List pending questions - should be 4
	listResult, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{
		"status": "pending",
	})
	require.NoError(t, err)
	require.False(t, listResult.IsError)

	var listResponse map[string]any
	err = json.Unmarshal([]byte(listResult.Content[0].(mcp.TextContent).Text), &listResponse)
	require.NoError(t, err)
	assert.Equal(t, float64(4), listResponse["total_count"])

	// Step 3: Resolve the first question (found the answer)
	_, err = tc.callTool(ctx, "resolve_ontology_question", map[string]any{
		"question_id":      q1.ID.String(),
		"resolution_notes": "Found in user_status.go:45",
	})
	require.NoError(t, err)

	// Step 4: Escalate the second question (needs human input)
	_, err = tc.callTool(ctx, "escalate_ontology_question", map[string]any{
		"question_id": q2.ID.String(),
		"reason":      "Business rule not documented - need product team",
	})
	require.NoError(t, err)

	// Step 5: Dismiss the third question (legacy/unused)
	_, err = tc.callTool(ctx, "dismiss_ontology_question", map[string]any{
		"question_id": q3.ID.String(),
		"reason":      "Column unused since 2019",
	})
	require.NoError(t, err)

	// Step 6: Skip the fourth question (need more context)
	_, err = tc.callTool(ctx, "skip_ontology_question", map[string]any{
		"question_id": q4.ID.String(),
		"reason":      "Need to analyze auth service first",
	})
	require.NoError(t, err)

	// Step 7: Verify final state - list all and check counts
	finalResult, err := tc.callTool(ctx, "list_ontology_questions", map[string]any{})
	require.NoError(t, err)
	require.False(t, finalResult.IsError)

	var finalResponse map[string]any
	err = json.Unmarshal([]byte(finalResult.Content[0].(mcp.TextContent).Text), &finalResponse)
	require.NoError(t, err)

	countsByStatus := finalResponse["counts_by_status"].(map[string]any)
	// Note: status counts may be nil if count is 0 (not included in map)
	pendingCount, _ := countsByStatus["pending"].(float64)
	answeredCount, _ := countsByStatus["answered"].(float64)
	escalatedCount, _ := countsByStatus["escalated"].(float64)
	dismissedCount, _ := countsByStatus["dismissed"].(float64)
	skippedCount, _ := countsByStatus["skipped"].(float64)

	assert.Equal(t, float64(0), pendingCount) // All processed
	assert.Equal(t, float64(1), answeredCount)
	assert.Equal(t, float64(1), escalatedCount)
	assert.Equal(t, float64(1), dismissedCount)
	assert.Equal(t, float64(1), skippedCount)

	// Step 8: Verify individual database states
	updatedQ1, _ := tc.questionRepo.GetByID(ctx, q1.ID)
	assert.Equal(t, models.QuestionStatusAnswered, updatedQ1.Status)

	updatedQ2, _ := tc.questionRepo.GetByID(ctx, q2.ID)
	assert.Equal(t, models.QuestionStatusEscalated, updatedQ2.Status)

	updatedQ3, _ := tc.questionRepo.GetByID(ctx, q3.ID)
	assert.Equal(t, models.QuestionStatusDismissed, updatedQ3.Status)

	updatedQ4, _ := tc.questionRepo.GetByID(ctx, q4.ID)
	assert.Equal(t, models.QuestionStatusSkipped, updatedQ4.Status)
}
