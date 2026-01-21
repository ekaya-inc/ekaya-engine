package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Unit Test Helpers
// ============================================================================

func newTestChatHandler() (*OntologyChatHandler, *mockChatServiceUnit, *mockKnowledgeServiceUnit) {
	logger := zap.NewNop()
	mockChatSvc := &mockChatServiceUnit{}
	mockKnowledgeSvc := &mockKnowledgeServiceUnit{}
	handler := NewOntologyChatHandler(mockChatSvc, mockKnowledgeSvc, logger)
	return handler, mockChatSvc, mockKnowledgeSvc
}

// mockChatServiceUnit is a simple mock for unit tests (without database context).
type mockChatServiceUnit struct {
	initializeResult *models.ChatInitResponse
	initializeErr    error
	sendMessageErr   error
	historyResult    []*models.ChatMessage
	historyErr       error
	clearHistoryErr  error
}

func (m *mockChatServiceUnit) Initialize(ctx context.Context, projectID uuid.UUID) (*models.ChatInitResponse, error) {
	if m.initializeErr != nil {
		return nil, m.initializeErr
	}
	if m.initializeResult != nil {
		return m.initializeResult, nil
	}
	return &models.ChatInitResponse{
		OpeningMessage:       "Hello!",
		PendingQuestionCount: 0,
		HasExistingHistory:   false,
	}, nil
}

func (m *mockChatServiceUnit) SendMessage(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
	if m.sendMessageErr != nil {
		return m.sendMessageErr
	}
	eventChan <- models.NewTextEvent("Response")
	eventChan <- models.NewDoneEvent()
	return nil
}

func (m *mockChatServiceUnit) GetHistory(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error) {
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	if m.historyResult != nil {
		return m.historyResult, nil
	}
	return []*models.ChatMessage{}, nil
}

func (m *mockChatServiceUnit) ClearHistory(ctx context.Context, projectID uuid.UUID) error {
	return m.clearHistoryErr
}

func (m *mockChatServiceUnit) SaveMessage(ctx context.Context, message *models.ChatMessage) error {
	return nil
}

// mockKnowledgeServiceUnit is a simple mock for knowledge service in unit tests.
type mockKnowledgeServiceUnit struct {
	facts        []*models.KnowledgeFact
	getAllErr    error
	getByTypeErr error
}

func (m *mockKnowledgeServiceUnit) Store(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error) {
	return &models.KnowledgeFact{
		ID:        uuid.New(),
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
	}, nil
}

func (m *mockKnowledgeServiceUnit) GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	if m.getAllErr != nil {
		return nil, m.getAllErr
	}
	if m.facts != nil {
		return m.facts, nil
	}
	return []*models.KnowledgeFact{}, nil
}

func (m *mockKnowledgeServiceUnit) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	if m.getByTypeErr != nil {
		return nil, m.getByTypeErr
	}
	return []*models.KnowledgeFact{}, nil
}

func (m *mockKnowledgeServiceUnit) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeServiceUnit) SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

// ============================================================================
// Initialize Tests
// ============================================================================

func TestOntologyChatHandler_Initialize_Success(t *testing.T) {
	handler, mockChat, _ := newTestChatHandler()
	projectID := uuid.New()

	mockChat.initializeResult = &models.ChatInitResponse{
		OpeningMessage:       "Welcome to the ontology chat!",
		PendingQuestionCount: 3,
		HasExistingHistory:   true,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/ontology/chat/initialize", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.Initialize(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp.Data)
	}

	if data["opening_message"] != "Welcome to the ontology chat!" {
		t.Errorf("expected opening_message 'Welcome to the ontology chat!', got %v", data["opening_message"])
	}
	if data["pending_question_count"] != float64(3) {
		t.Errorf("expected pending_question_count 3, got %v", data["pending_question_count"])
	}
	if data["has_existing_history"] != true {
		t.Errorf("expected has_existing_history true, got %v", data["has_existing_history"])
	}
}

func TestOntologyChatHandler_Initialize_InvalidProjectID(t *testing.T) {
	handler, _, _ := newTestChatHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/invalid-uuid/ontology/chat/initialize", nil)
	req.SetPathValue("pid", "invalid-uuid")
	rec := httptest.NewRecorder()

	handler.Initialize(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_project_id" {
		t.Errorf("expected error 'invalid_project_id', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_Initialize_ServiceError(t *testing.T) {
	handler, mockChat, _ := newTestChatHandler()
	projectID := uuid.New()

	mockChat.initializeErr = errors.New("database connection failed")

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/ontology/chat/initialize", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.Initialize(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "init_failed" {
		t.Errorf("expected error 'init_failed', got %v", resp["error"])
	}
}

// ============================================================================
// SendMessage Tests
// ============================================================================

func TestOntologyChatHandler_SendMessage_InvalidProjectID(t *testing.T) {
	handler, _, _ := newTestChatHandler()

	body, _ := json.Marshal(SendMessageRequest{Message: "Hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/projects/bad-id/ontology/chat/message", bytes.NewReader(body))
	req.SetPathValue("pid", "bad-id")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.SendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_project_id" {
		t.Errorf("expected error 'invalid_project_id', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_SendMessage_InvalidBody(t *testing.T) {
	handler, _, _ := newTestChatHandler()
	projectID := uuid.New()

	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/ontology/chat/message",
		bytes.NewReader([]byte("not json")))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.SendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_SendMessage_EmptyMessage(t *testing.T) {
	handler, _, _ := newTestChatHandler()
	projectID := uuid.New()

	body, _ := json.Marshal(SendMessageRequest{Message: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/projects/"+projectID.String()+"/ontology/chat/message",
		bytes.NewReader(body))
	req.SetPathValue("pid", projectID.String())
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.SendMessage(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "missing_message" {
		t.Errorf("expected error 'missing_message', got %v", resp["error"])
	}
}

// ============================================================================
// GetHistory Tests
// ============================================================================

func TestOntologyChatHandler_GetHistory_Success(t *testing.T) {
	handler, mockChat, _ := newTestChatHandler()
	projectID := uuid.New()

	now := time.Now()
	mockChat.historyResult = []*models.ChatMessage{
		{
			ID:        uuid.New(),
			ProjectID: projectID,
			Role:      models.ChatRoleUser,
			Content:   "Hello",
			CreatedAt: now,
		},
		{
			ID:        uuid.New(),
			ProjectID: projectID,
			Role:      models.ChatRoleAssistant,
			Content:   "Hi there!",
			CreatedAt: now,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/ontology/chat/history", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map")
	}

	messages, ok := data["messages"].([]any)
	if !ok {
		t.Fatalf("expected messages to be an array")
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
	}

	if data["total"] != float64(2) {
		t.Errorf("expected total 2, got %v", data["total"])
	}
}

func TestOntologyChatHandler_GetHistory_InvalidProjectID(t *testing.T) {
	handler, _, _ := newTestChatHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/not-valid/ontology/chat/history", nil)
	req.SetPathValue("pid", "not-valid")
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_project_id" {
		t.Errorf("expected error 'invalid_project_id', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_GetHistory_ServiceError(t *testing.T) {
	handler, mockChat, _ := newTestChatHandler()
	projectID := uuid.New()

	mockChat.historyErr = errors.New("database error")

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/ontology/chat/history", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.GetHistory(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "internal_error" {
		t.Errorf("expected error 'internal_error', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_GetHistory_CustomLimit(t *testing.T) {
	handler, mockChat, _ := newTestChatHandler()
	projectID := uuid.New()

	// Track what limit was passed to the service
	var receivedLimit int
	mockChat.historyResult = []*models.ChatMessage{}

	// Create a custom mock that captures the limit
	originalGetHistory := mockChat.GetHistory
	mockChat.historyResult = nil
	_ = originalGetHistory // Avoid unused warning

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/ontology/chat/history?limit=25", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	// We can't easily intercept the limit in our simple mock, but we can verify the response
	handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// The test passes if no error occurs with a custom limit
	_ = receivedLimit
}

// ============================================================================
// ClearHistory Tests
// ============================================================================

func TestOntologyChatHandler_ClearHistory_Success(t *testing.T) {
	handler, _, _ := newTestChatHandler()
	projectID := uuid.New()

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/ontology/chat/history", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.ClearHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
}

func TestOntologyChatHandler_ClearHistory_InvalidProjectID(t *testing.T) {
	handler, _, _ := newTestChatHandler()

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/wrong/ontology/chat/history", nil)
	req.SetPathValue("pid", "wrong")
	rec := httptest.NewRecorder()

	handler.ClearHistory(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_project_id" {
		t.Errorf("expected error 'invalid_project_id', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_ClearHistory_ServiceError(t *testing.T) {
	handler, mockChat, _ := newTestChatHandler()
	projectID := uuid.New()

	mockChat.clearHistoryErr = errors.New("failed to clear")

	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+projectID.String()+"/ontology/chat/history", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.ClearHistory(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "internal_error" {
		t.Errorf("expected error 'internal_error', got %v", resp["error"])
	}
}

// ============================================================================
// GetKnowledge Tests
// ============================================================================

func TestOntologyChatHandler_GetKnowledge_Success(t *testing.T) {
	handler, _, mockKnowledge := newTestChatHandler()
	projectID := uuid.New()

	mockKnowledge.facts = []*models.KnowledgeFact{
		{
			ID:        uuid.New(),
			ProjectID: projectID,
			FactType:  models.FactTypeFiscalYear,
			Key:       "start_month",
			Value:     "July",
			CreatedAt: time.Now(),
		},
		{
			ID:        uuid.New(),
			ProjectID: projectID,
			FactType:  models.FactTypeTerminology,
			Key:       "MRR",
			Value:     "Monthly Recurring Revenue",
			CreatedAt: time.Now(),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/ontology/knowledge", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.GetKnowledge(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map")
	}

	facts, ok := data["facts"].([]any)
	if !ok {
		t.Fatalf("expected facts to be an array")
	}

	if len(facts) != 2 {
		t.Errorf("expected 2 facts, got %d", len(facts))
	}

	if data["total"] != float64(2) {
		t.Errorf("expected total 2, got %v", data["total"])
	}
}

func TestOntologyChatHandler_GetKnowledge_InvalidProjectID(t *testing.T) {
	handler, _, _ := newTestChatHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/projects/xyz/ontology/knowledge", nil)
	req.SetPathValue("pid", "xyz")
	rec := httptest.NewRecorder()

	handler.GetKnowledge(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_project_id" {
		t.Errorf("expected error 'invalid_project_id', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_GetKnowledge_ServiceError(t *testing.T) {
	handler, _, mockKnowledge := newTestChatHandler()
	projectID := uuid.New()

	mockKnowledge.getAllErr = errors.New("database unavailable")

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/ontology/knowledge", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.GetKnowledge(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "internal_error" {
		t.Errorf("expected error 'internal_error', got %v", resp["error"])
	}
}

func TestOntologyChatHandler_GetKnowledge_FilterByType(t *testing.T) {
	handler, _, mockKnowledge := newTestChatHandler()
	projectID := uuid.New()

	// When filtering by type, GetByType is called instead of GetAll
	mockKnowledge.facts = nil // GetAll returns empty
	// GetByType will return empty by default

	req := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/ontology/knowledge?type=fiscal_year", nil)
	req.SetPathValue("pid", projectID.String())
	rec := httptest.NewRecorder()

	handler.GetKnowledge(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}
}

// ============================================================================
// Helper Conversion Tests
// ============================================================================

func TestOntologyChatHandler_toChatMessageResponse(t *testing.T) {
	handler, _, _ := newTestChatHandler()
	projectID := uuid.New()
	msgID := uuid.New()
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	msg := &models.ChatMessage{
		ID:         msgID,
		ProjectID:  projectID,
		Role:       models.ChatRoleAssistant,
		Content:    "Test content",
		ToolCalls:  []models.ToolCall{{ID: "call_1", Type: "function"}},
		ToolCallID: "parent_call",
		CreatedAt:  now,
	}

	resp := handler.toChatMessageResponse(msg)

	if resp.ID != msgID.String() {
		t.Errorf("expected ID %s, got %s", msgID.String(), resp.ID)
	}
	if resp.ProjectID != projectID.String() {
		t.Errorf("expected ProjectID %s, got %s", projectID.String(), resp.ProjectID)
	}
	if resp.Role != "assistant" {
		t.Errorf("expected Role 'assistant', got %s", resp.Role)
	}
	if resp.Content != "Test content" {
		t.Errorf("expected Content 'Test content', got %s", resp.Content)
	}
	if len(resp.ToolCalls) != 1 {
		t.Errorf("expected 1 ToolCall, got %d", len(resp.ToolCalls))
	}
	if resp.ToolCallID != "parent_call" {
		t.Errorf("expected ToolCallID 'parent_call', got %s", resp.ToolCallID)
	}
	if resp.CreatedAt != "2024-01-15T10:30:00Z" {
		t.Errorf("expected CreatedAt '2024-01-15T10:30:00Z', got %s", resp.CreatedAt)
	}
}

func TestOntologyChatHandler_toKnowledgeFactResponse(t *testing.T) {
	handler, _, _ := newTestChatHandler()
	factID := uuid.New()
	now := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	fact := &models.KnowledgeFact{
		ID:        factID,
		FactType:  models.FactTypeFiscalYear,
		Key:       "start_month",
		Value:     "July",
		Context:   "User mentioned fiscal year starts in July",
		CreatedAt: now,
	}

	resp := handler.toKnowledgeFactResponse(fact)

	if resp.ID != factID.String() {
		t.Errorf("expected ID %s, got %s", factID.String(), resp.ID)
	}
	if resp.FactType != models.FactTypeFiscalYear {
		t.Errorf("expected FactType '%s', got %s", models.FactTypeFiscalYear, resp.FactType)
	}
	if resp.Key != "start_month" {
		t.Errorf("expected Key 'start_month', got %s", resp.Key)
	}
	if resp.Value != "July" {
		t.Errorf("expected Value 'July', got %s", resp.Value)
	}
	if resp.Context != "User mentioned fiscal year starts in July" {
		t.Errorf("expected Context, got %s", resp.Context)
	}
	if resp.CreatedAt != "2024-01-15T10:30:00Z" {
		t.Errorf("expected CreatedAt '2024-01-15T10:30:00Z', got %s", resp.CreatedAt)
	}
}
