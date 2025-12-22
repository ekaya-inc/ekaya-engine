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

// chatTestContext holds test dependencies for chat repository tests.
type chatTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       OntologyChatRepository
	projectID  uuid.UUID
	ontologyID uuid.UUID
}

// setupChatTest initializes the test context with shared testcontainer.
func setupChatTest(t *testing.T) *chatTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &chatTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewOntologyChatRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000042"),
		ontologyID: uuid.MustParse("00000000-0000-0000-0000-000000000043"),
	}
	tc.ensureTestProject()
	return tc
}

// ensureTestProject creates the test project and ontology if they don't exist.
func (tc *chatTestContext) ensureTestProject() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Chat Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Create ontology (required for chat messages FK)
	now := time.Now()
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, is_active, created_at, updated_at)
		VALUES ($1, $2, true, $3, $3)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID, now)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}
}

// cleanup removes test chat messages.
func (tc *chatTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_chat_messages WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *chatTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestMessage creates a chat message for testing.
func (tc *chatTestContext) createTestMessage(ctx context.Context, role models.ChatRole, content string) *models.ChatMessage {
	tc.t.Helper()
	message := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       role,
		Content:    content,
	}
	err := tc.repo.SaveMessage(ctx, message)
	if err != nil {
		tc.t.Fatalf("failed to create test message: %v", err)
	}
	// Small delay to ensure different timestamps
	time.Sleep(10 * time.Millisecond)
	return message
}

// ============================================================================
// SaveMessage Tests
// ============================================================================

func TestChatRepository_SaveMessage_UserMessage(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	message := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       models.ChatRoleUser,
		Content:    "What does the accounts table represent?",
		Metadata:   map[string]any{"source": "web"},
	}

	err := tc.repo.SaveMessage(ctx, message)
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	if message.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if message.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching history
	history, err := tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 message, got %d", len(history))
	}
	if history[0].Content != "What does the accounts table represent?" {
		t.Errorf("expected content mismatch")
	}
	if history[0].Role != models.ChatRoleUser {
		t.Errorf("expected role user, got %q", history[0].Role)
	}
}

func TestChatRepository_SaveMessage_AssistantMessage(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	message := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       models.ChatRoleAssistant,
		Content:    "The accounts table represents customer accounts.",
	}

	err := tc.repo.SaveMessage(ctx, message)
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	history, err := tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if history[0].Role != models.ChatRoleAssistant {
		t.Errorf("expected role assistant, got %q", history[0].Role)
	}
}

func TestChatRepository_SaveMessage_WithToolCalls(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	message := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       models.ChatRoleAssistant,
		Content:    "",
		ToolCalls: []models.ToolCall{
			{
				ID:   "call_123",
				Type: "function",
				Function: models.ToolCallFunction{
					Name:      "query_column_values",
					Arguments: `{"table_name": "accounts", "column_name": "status"}`,
				},
			},
			{
				ID:   "call_456",
				Type: "function",
				Function: models.ToolCallFunction{
					Name:      "update_entity",
					Arguments: `{"table_name": "accounts", "description": "Updated"}`,
				},
			},
		},
	}

	err := tc.repo.SaveMessage(ctx, message)
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	history, err := tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(history[0].ToolCalls) != 2 {
		t.Errorf("expected 2 tool calls, got %d", len(history[0].ToolCalls))
	}
	if history[0].ToolCalls[0].ID != "call_123" {
		t.Errorf("expected tool call ID 'call_123', got %q", history[0].ToolCalls[0].ID)
	}
	if history[0].ToolCalls[0].Function.Name != "query_column_values" {
		t.Errorf("expected function name 'query_column_values', got %q", history[0].ToolCalls[0].Function.Name)
	}
}

func TestChatRepository_SaveMessage_ToolResult(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	message := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       models.ChatRoleTool,
		Content:    `["active", "inactive", "pending"]`,
		ToolCallID: "call_123",
	}

	err := tc.repo.SaveMessage(ctx, message)
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}

	history, err := tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if history[0].Role != models.ChatRoleTool {
		t.Errorf("expected role tool, got %q", history[0].Role)
	}
	if history[0].ToolCallID != "call_123" {
		t.Errorf("expected tool_call_id 'call_123', got %q", history[0].ToolCallID)
	}
}

// ============================================================================
// GetHistory Tests
// ============================================================================

func TestChatRepository_GetHistory_Ordering(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create messages in order
	tc.createTestMessage(ctx, models.ChatRoleUser, "First message")
	tc.createTestMessage(ctx, models.ChatRoleAssistant, "Second message")
	tc.createTestMessage(ctx, models.ChatRoleUser, "Third message")

	history, err := tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(history))
	}

	// Should be in chronological order
	if history[0].Content != "First message" {
		t.Errorf("expected first message, got %q", history[0].Content)
	}
	if history[1].Content != "Second message" {
		t.Errorf("expected second message, got %q", history[1].Content)
	}
	if history[2].Content != "Third message" {
		t.Errorf("expected third message, got %q", history[2].Content)
	}
}

func TestChatRepository_GetHistory_Limit(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create 5 messages
	for i := 1; i <= 5; i++ {
		tc.createTestMessage(ctx, models.ChatRoleUser, "Message")
	}

	// Request only 3
	history, err := tc.repo.GetHistory(ctx, tc.projectID, 3)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("expected 3 messages with limit, got %d", len(history))
	}
}

func TestChatRepository_GetHistory_DefaultLimit(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestMessage(ctx, models.ChatRoleUser, "Test message")

	// Limit 0 should use default
	history, err := tc.repo.GetHistory(ctx, tc.projectID, 0)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("expected 1 message, got %d", len(history))
	}
}

func TestChatRepository_GetHistory_Empty(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	history, err := tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected 0 messages, got %d", len(history))
	}
}

// ============================================================================
// GetHistoryCount Tests
// ============================================================================

func TestChatRepository_GetHistoryCount_Success(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestMessage(ctx, models.ChatRoleUser, "Message 1")
	tc.createTestMessage(ctx, models.ChatRoleAssistant, "Message 2")
	tc.createTestMessage(ctx, models.ChatRoleUser, "Message 3")

	count, err := tc.repo.GetHistoryCount(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetHistoryCount failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestChatRepository_GetHistoryCount_Empty(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	count, err := tc.repo.GetHistoryCount(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetHistoryCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected count 0, got %d", count)
	}
}

// ============================================================================
// ClearHistory Tests
// ============================================================================

func TestChatRepository_ClearHistory_Success(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestMessage(ctx, models.ChatRoleUser, "Message 1")
	tc.createTestMessage(ctx, models.ChatRoleAssistant, "Message 2")

	err := tc.repo.ClearHistory(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("ClearHistory failed: %v", err)
	}

	count, err := tc.repo.GetHistoryCount(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetHistoryCount failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 messages after clear, got %d", count)
	}
}

func TestChatRepository_ClearHistory_Empty(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Clear when already empty should not error
	err := tc.repo.ClearHistory(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("ClearHistory on empty should not error: %v", err)
	}
}

// ============================================================================
// Conversation Flow Tests
// ============================================================================

func TestChatRepository_ConversationFlow(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Simulate a realistic conversation flow

	// 1. User asks question
	tc.createTestMessage(ctx, models.ChatRoleUser, "What does the status column in accounts mean?")

	// 2. Assistant responds with tool call
	assistantMsg := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       models.ChatRoleAssistant,
		Content:    "",
		ToolCalls: []models.ToolCall{
			{
				ID:   "call_abc",
				Type: "function",
				Function: models.ToolCallFunction{
					Name:      "query_column_values",
					Arguments: `{"table_name": "accounts", "column_name": "status"}`,
				},
			},
		},
	}
	err := tc.repo.SaveMessage(ctx, assistantMsg)
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// 3. Tool result
	toolMsg := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       models.ChatRoleTool,
		Content:    `["active", "suspended", "closed"]`,
		ToolCallID: "call_abc",
	}
	err = tc.repo.SaveMessage(ctx, toolMsg)
	if err != nil {
		t.Fatalf("SaveMessage failed: %v", err)
	}
	time.Sleep(10 * time.Millisecond)

	// 4. Assistant final response
	tc.createTestMessage(ctx, models.ChatRoleAssistant, "The status column has three possible values: active, suspended, and closed.")

	// Verify full conversation
	history, err := tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}

	if len(history) != 4 {
		t.Fatalf("expected 4 messages in conversation, got %d", len(history))
	}

	// Verify order and roles
	expectedRoles := []models.ChatRole{
		models.ChatRoleUser,
		models.ChatRoleAssistant,
		models.ChatRoleTool,
		models.ChatRoleAssistant,
	}
	for i, expected := range expectedRoles {
		if history[i].Role != expected {
			t.Errorf("message %d: expected role %q, got %q", i, expected, history[i].Role)
		}
	}

	// Verify tool call is preserved
	if len(history[1].ToolCalls) != 1 {
		t.Errorf("expected tool call in assistant message")
	}

	// Verify tool result has tool_call_id
	if history[2].ToolCallID != "call_abc" {
		t.Errorf("expected tool_call_id 'call_abc', got %q", history[2].ToolCallID)
	}
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestChatRepository_NoTenantScope(t *testing.T) {
	tc := setupChatTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	message := &models.ChatMessage{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		Role:       models.ChatRoleUser,
		Content:    "Test",
	}

	// SaveMessage should fail
	err := tc.repo.SaveMessage(ctx, message)
	if err == nil {
		t.Error("expected error for SaveMessage without tenant scope")
	}

	// GetHistory should fail
	_, err = tc.repo.GetHistory(ctx, tc.projectID, 10)
	if err == nil {
		t.Error("expected error for GetHistory without tenant scope")
	}

	// GetHistoryCount should fail
	_, err = tc.repo.GetHistoryCount(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for GetHistoryCount without tenant scope")
	}

	// ClearHistory should fail
	err = tc.repo.ClearHistory(ctx, tc.projectID)
	if err == nil {
		t.Error("expected error for ClearHistory without tenant scope")
	}
}
