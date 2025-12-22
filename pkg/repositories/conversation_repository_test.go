//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// conversationTestContext holds test dependencies for conversation repository tests.
type conversationTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       ConversationRepository
	projectID  uuid.UUID
	ontologyID uuid.UUID
	workflowID uuid.UUID
}

// setupConversationTest initializes the test context with shared testcontainer.
func setupConversationTest(t *testing.T) *conversationTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &conversationTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewConversationRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000050"),
		ontologyID: uuid.MustParse("00000000-0000-0000-0000-000000000052"),
		workflowID: uuid.MustParse("00000000-0000-0000-0000-000000000051"),
	}
	tc.ensureTestData()
	return tc
}

// ensureTestData creates the test project, ontology, and workflow if they don't exist.
func (tc *conversationTestContext) ensureTestData() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for data setup: %v", err)
	}
	defer scope.Close()

	// Create project
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Conversation Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Create ontology (required for workflow FK)
	now := time.Now()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, is_active, created_at, updated_at)
		VALUES ($1, $2, true, $3, $3)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID, now)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}

	// Create workflow (config contains datasource_id)
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_workflows (id, project_id, ontology_id, state, progress, task_queue, config, created_at, updated_at)
		VALUES ($1, $2, $3, 'pending', '{}', '[]', '{}', $4, $4)
		ON CONFLICT (id) DO NOTHING
	`, tc.workflowID, tc.projectID, tc.ontologyID, now)
	if err != nil {
		tc.t.Fatalf("failed to ensure test workflow: %v", err)
	}
}

// cleanup removes test conversations.
func (tc *conversationTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_llm_conversations WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *conversationTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestConversation creates a conversation for testing.
func (tc *conversationTestContext) createTestConversation(ctx context.Context, model string, status string) *models.LLMConversation {
	tc.t.Helper()
	temp := 0.7
	promptTokens := 100
	completionTokens := 50
	totalTokens := 150

	conv := &models.LLMConversation{
		ID:               uuid.New(),
		ProjectID:        tc.projectID,
		Iteration:        1,
		Endpoint:         "https://api.openai.com/v1",
		Model:            model,
		RequestMessages:  []any{map[string]string{"role": "user", "content": "Hello"}},
		Temperature:      &temp,
		ResponseContent:  "Hi there!",
		PromptTokens:     &promptTokens,
		CompletionTokens: &completionTokens,
		TotalTokens:      &totalTokens,
		DurationMs:       250,
		Status:           status,
	}

	err := tc.repo.Save(ctx, conv)
	if err != nil {
		tc.t.Fatalf("failed to create test conversation: %v", err)
	}
	return conv
}

// ============================================================================
// Save Tests
// ============================================================================

func TestConversationRepository_Save_Success(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	temp := 0.5
	promptTokens := 10
	completionTokens := 5
	totalTokens := 15

	conv := &models.LLMConversation{
		ProjectID:       tc.projectID,
		Context:         map[string]any{"workflow_id": tc.workflowID.String()},
		Iteration:       1,
		Endpoint:        "https://api.openai.com/v1",
		Model:           "gpt-4",
		RequestMessages: []any{map[string]string{"role": "system", "content": "Be helpful"}},
		RequestTools: []any{map[string]any{
			"type": "function",
			"function": map[string]string{
				"name":        "get_weather",
				"description": "Get weather info",
			},
		}},
		Temperature:       &temp,
		ResponseContent:   "Hello!",
		ResponseToolCalls: []any{map[string]string{"id": "call_1", "function": "get_weather"}},
		PromptTokens:      &promptTokens,
		CompletionTokens:  &completionTokens,
		TotalTokens:       &totalTokens,
		DurationMs:        150,
		Status:            models.LLMConversationStatusSuccess,
	}

	err := tc.repo.Save(ctx, conv)
	if err != nil {
		t.Fatalf("failed to save conversation: %v", err)
	}

	// Verify ID was generated
	if conv.ID == uuid.Nil {
		t.Error("expected ID to be generated")
	}

	// Verify CreatedAt was set
	if conv.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
}

func TestConversationRepository_Save_WithError(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	conv := &models.LLMConversation{
		ProjectID:       tc.projectID,
		Iteration:       1,
		Endpoint:        "https://api.openai.com/v1",
		Model:           "gpt-4",
		RequestMessages: []any{map[string]string{"role": "user", "content": "Hello"}},
		DurationMs:      100,
		Status:          models.LLMConversationStatusError,
		ErrorMessage:    "Rate limit exceeded",
		ResponseContent: "",
	}

	err := tc.repo.Save(ctx, conv)
	if err != nil {
		t.Fatalf("failed to save error conversation: %v", err)
	}
}

func TestConversationRepository_Save_NullableFieldsAsNil(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Save with all nullable fields as nil
	conv := &models.LLMConversation{
		ProjectID:        tc.projectID,
		Context:          nil, // nullable
		ConversationID:   nil, // nullable
		Iteration:        1,
		Endpoint:         "https://api.openai.com/v1",
		Model:            "gpt-4",
		RequestMessages:  []any{map[string]string{"role": "user", "content": "Test"}},
		RequestTools:     nil, // nullable
		Temperature:      nil, // nullable
		ResponseContent:  "Response",
		PromptTokens:     nil, // nullable
		CompletionTokens: nil,
		TotalTokens:      nil,
		DurationMs:       50,
		Status:           models.LLMConversationStatusSuccess,
		ErrorMessage:     "", // Should be stored as NULL
	}

	err := tc.repo.Save(ctx, conv)
	if err != nil {
		t.Fatalf("failed to save conversation with nil fields: %v", err)
	}
}

// ============================================================================
// GetByProject Tests
// ============================================================================

func TestConversationRepository_GetByProject_ReturnsConversations(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test conversations
	conv1 := tc.createTestConversation(ctx, "gpt-4", models.LLMConversationStatusSuccess)
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	conv2 := tc.createTestConversation(ctx, "gpt-3.5-turbo", models.LLMConversationStatusSuccess)

	// Fetch
	conversations, err := tc.repo.GetByProject(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("failed to get conversations: %v", err)
	}

	if len(conversations) != 2 {
		t.Fatalf("expected 2 conversations, got %d", len(conversations))
	}

	// Should be ordered by created_at DESC (newest first)
	if conversations[0].ID != conv2.ID {
		t.Error("expected newest conversation first")
	}
	if conversations[1].ID != conv1.ID {
		t.Error("expected older conversation second")
	}
}

func TestConversationRepository_GetByProject_RespectsLimit(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create 5 conversations
	for i := 0; i < 5; i++ {
		tc.createTestConversation(ctx, "gpt-4", models.LLMConversationStatusSuccess)
	}

	// Fetch with limit 3
	conversations, err := tc.repo.GetByProject(ctx, tc.projectID, 3)
	if err != nil {
		t.Fatalf("failed to get conversations: %v", err)
	}

	if len(conversations) != 3 {
		t.Errorf("expected 3 conversations (limit), got %d", len(conversations))
	}
}

func TestConversationRepository_GetByProject_EmptyResult(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Don't create any conversations
	conversations, err := tc.repo.GetByProject(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("failed to get conversations: %v", err)
	}

	if len(conversations) != 0 {
		t.Errorf("expected 0 conversations, got %d", len(conversations))
	}
}

// ============================================================================
// GetByContext Tests
// ============================================================================

func TestConversationRepository_GetByContext_ReturnsConversations(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create conversations with context
	for i := 0; i < 3; i++ {
		conv := &models.LLMConversation{
			ProjectID:       tc.projectID,
			Context:         map[string]any{"workflow_id": tc.workflowID.String()},
			Iteration:       i + 1,
			Endpoint:        "https://api.openai.com/v1",
			Model:           "gpt-4",
			RequestMessages: []any{map[string]string{"role": "user", "content": "Test"}},
			DurationMs:      100,
			Status:          models.LLMConversationStatusSuccess,
		}
		if err := tc.repo.Save(ctx, conv); err != nil {
			t.Fatalf("failed to save conversation: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Fetch by context key-value
	conversations, err := tc.repo.GetByContext(ctx, tc.projectID, "workflow_id", tc.workflowID.String())
	if err != nil {
		t.Fatalf("failed to get conversations: %v", err)
	}

	if len(conversations) != 3 {
		t.Fatalf("expected 3 conversations, got %d", len(conversations))
	}

	// Should be ordered by created_at ASC
	for i, conv := range conversations {
		if conv.Iteration != i+1 {
			t.Errorf("expected iteration %d, got %d", i+1, conv.Iteration)
		}
	}
}

// ============================================================================
// GetByConversationID Tests
// ============================================================================

func TestConversationRepository_GetByConversationID_ReturnsConversations(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	conversationID := uuid.New()

	// Create conversations with same conversation ID (multi-turn)
	for i := 0; i < 3; i++ {
		conv := &models.LLMConversation{
			ProjectID:       tc.projectID,
			ConversationID:  &conversationID,
			Iteration:       i + 1,
			Endpoint:        "https://api.openai.com/v1",
			Model:           "gpt-4",
			RequestMessages: []any{map[string]string{"role": "user", "content": "Test"}},
			DurationMs:      100,
			Status:          models.LLMConversationStatusSuccess,
		}
		if err := tc.repo.Save(ctx, conv); err != nil {
			t.Fatalf("failed to save conversation: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Fetch by conversation ID
	conversations, err := tc.repo.GetByConversationID(ctx, conversationID)
	if err != nil {
		t.Fatalf("failed to get conversations: %v", err)
	}

	if len(conversations) != 3 {
		t.Fatalf("expected 3 conversations, got %d", len(conversations))
	}

	// Should be ordered by iteration ASC
	for i, conv := range conversations {
		if conv.Iteration != i+1 {
			t.Errorf("expected iteration %d, got %d", i+1, conv.Iteration)
		}
	}
}

// ============================================================================
// JSONB Field Tests
// ============================================================================

func TestConversationRepository_JSONBFieldsRoundTrip(t *testing.T) {
	tc := setupConversationTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Save with complex JSONB data
	requestMessages := []any{
		map[string]string{"role": "system", "content": "You are helpful"},
		map[string]string{"role": "user", "content": "Hello"},
	}
	requestTools := []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "get_weather",
				"description": "Get the weather",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]string{"type": "string"},
					},
				},
			},
		},
	}
	responseToolCalls := []any{
		map[string]any{
			"id":   "call_123",
			"type": "function",
			"function": map[string]any{
				"name":      "get_weather",
				"arguments": `{"location": "NYC"}`,
			},
		},
	}

	conv := &models.LLMConversation{
		ProjectID:         tc.projectID,
		Iteration:         1,
		Endpoint:          "https://api.openai.com/v1",
		Model:             "gpt-4",
		RequestMessages:   requestMessages,
		RequestTools:      requestTools,
		ResponseContent:   "Tool call response",
		ResponseToolCalls: responseToolCalls,
		DurationMs:        200,
		Status:            models.LLMConversationStatusSuccess,
	}

	err := tc.repo.Save(ctx, conv)
	if err != nil {
		t.Fatalf("failed to save: %v", err)
	}

	// Fetch and verify
	conversations, err := tc.repo.GetByProject(ctx, tc.projectID, 1)
	if err != nil {
		t.Fatalf("failed to get: %v", err)
	}

	if len(conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(conversations))
	}

	fetched := conversations[0]

	// Verify request messages
	if len(fetched.RequestMessages) != 2 {
		t.Errorf("expected 2 request messages, got %d", len(fetched.RequestMessages))
	}

	// Verify request tools
	if len(fetched.RequestTools) != 1 {
		t.Errorf("expected 1 request tool, got %d", len(fetched.RequestTools))
	}

	// Verify response tool calls
	if len(fetched.ResponseToolCalls) != 1 {
		t.Errorf("expected 1 response tool call, got %d", len(fetched.ResponseToolCalls))
	}
}
