//go:build integration

package handlers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// chatIntegrationTestContext holds all dependencies for chat integration tests.
type chatIntegrationTestContext struct {
	t                *testing.T
	engineDB         *testhelpers.EngineDB
	handler          *OntologyChatHandler
	chatRepo         repositories.OntologyChatRepository
	ontologyRepo     repositories.OntologyRepository
	knowledgeRepo    repositories.KnowledgeRepository
	projectID        uuid.UUID
	mockChatService  *mockChatService
	mockKnowledgeSvc *mockKnowledgeServiceChat
}

// mockChatService is a mock implementation of OntologyChatService for testing.
type mockChatService struct {
	initializeFunc   func(ctx context.Context, projectID uuid.UUID) (*models.ChatInitResponse, error)
	sendMessageFunc  func(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error
	getHistoryFunc   func(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error)
	clearHistoryFunc func(ctx context.Context, projectID uuid.UUID) error
	saveMessageFunc  func(ctx context.Context, message *models.ChatMessage) error
}

func (m *mockChatService) Initialize(ctx context.Context, projectID uuid.UUID) (*models.ChatInitResponse, error) {
	if m.initializeFunc != nil {
		return m.initializeFunc(ctx, projectID)
	}
	return &models.ChatInitResponse{
		OpeningMessage:       "Hello! How can I help you with your ontology?",
		PendingQuestionCount: 0,
		HasExistingHistory:   false,
	}, nil
}

func (m *mockChatService) SendMessage(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
	if m.sendMessageFunc != nil {
		return m.sendMessageFunc(ctx, projectID, message, eventChan)
	}
	// Default behavior: send a simple response
	eventChan <- models.NewTextEvent("This is a mock response to: ")
	eventChan <- models.NewTextEvent(message)
	eventChan <- models.NewDoneEvent()
	return nil
}

func (m *mockChatService) GetHistory(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error) {
	if m.getHistoryFunc != nil {
		return m.getHistoryFunc(ctx, projectID, limit)
	}
	return []*models.ChatMessage{}, nil
}

func (m *mockChatService) ClearHistory(ctx context.Context, projectID uuid.UUID) error {
	if m.clearHistoryFunc != nil {
		return m.clearHistoryFunc(ctx, projectID)
	}
	return nil
}

func (m *mockChatService) SaveMessage(ctx context.Context, message *models.ChatMessage) error {
	if m.saveMessageFunc != nil {
		return m.saveMessageFunc(ctx, message)
	}
	return nil
}

// mockKnowledgeServiceChat is a mock implementation of KnowledgeService for testing.
type mockKnowledgeServiceChat struct {
	storeFunc     func(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error)
	getAllFunc    func(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error)
	getByTypeFunc func(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error)
	deleteFunc    func(ctx context.Context, id uuid.UUID) error
}

func (m *mockKnowledgeServiceChat) Store(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error) {
	if m.storeFunc != nil {
		return m.storeFunc(ctx, projectID, factType, key, value, contextInfo)
	}
	return &models.KnowledgeFact{
		ID:        uuid.New(),
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
	}, nil
}

func (m *mockKnowledgeServiceChat) StoreWithSource(ctx context.Context, projectID uuid.UUID, factType, key, value, contextInfo, source string) (*models.KnowledgeFact, error) {
	if m.storeFunc != nil {
		return m.storeFunc(ctx, projectID, factType, key, value, contextInfo)
	}
	return &models.KnowledgeFact{
		ID:        uuid.New(),
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
		Source:    source,
	}, nil
}

func (m *mockKnowledgeServiceChat) Update(ctx context.Context, projectID, id uuid.UUID, factType, key, value, contextInfo string) (*models.KnowledgeFact, error) {
	return &models.KnowledgeFact{
		ID:        id,
		ProjectID: projectID,
		FactType:  factType,
		Key:       key,
		Value:     value,
		Context:   contextInfo,
	}, nil
}

func (m *mockKnowledgeServiceChat) GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	if m.getAllFunc != nil {
		return m.getAllFunc(ctx, projectID)
	}
	return []*models.KnowledgeFact{}, nil
}

func (m *mockKnowledgeServiceChat) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	if m.getByTypeFunc != nil {
		return m.getByTypeFunc(ctx, projectID, factType)
	}
	return []*models.KnowledgeFact{}, nil
}

func (m *mockKnowledgeServiceChat) Delete(ctx context.Context, id uuid.UUID) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

// setupChatIntegrationTest creates a test context with mock services.
func setupChatIntegrationTest(t *testing.T) *chatIntegrationTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)

	mockChatSvc := &mockChatService{}
	mockKnowledgeSvc := &mockKnowledgeServiceChat{}

	handler := NewOntologyChatHandler(mockChatSvc, mockKnowledgeSvc, zap.NewNop())

	// Use a unique project ID for chat tests
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000044")

	tc := &chatIntegrationTestContext{
		t:                t,
		engineDB:         engineDB,
		handler:          handler,
		chatRepo:         repositories.NewOntologyChatRepository(),
		ontologyRepo:     repositories.NewOntologyRepository(),
		knowledgeRepo:    repositories.NewKnowledgeRepository(),
		projectID:        projectID,
		mockChatService:  mockChatSvc,
		mockKnowledgeSvc: mockKnowledgeSvc,
	}

	tc.ensureTestProject()
	return tc
}

// setupChatIntegrationTestWithRealServices creates test context with real services (for end-to-end).
func setupChatIntegrationTestWithRealServices(t *testing.T) *chatIntegrationTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)

	// Create real repositories
	chatRepo := repositories.NewOntologyChatRepository()
	ontologyRepo := repositories.NewOntologyRepository()
	knowledgeRepo := repositories.NewKnowledgeRepository()
	projectRepo := repositories.NewProjectRepository()

	// Create real services
	knowledgeSvc := services.NewKnowledgeService(knowledgeRepo, projectRepo, ontologyRepo, zap.NewNop())

	// Use a unique project ID for chat tests
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000045")

	tc := &chatIntegrationTestContext{
		t:             t,
		engineDB:      engineDB,
		chatRepo:      chatRepo,
		ontologyRepo:  ontologyRepo,
		knowledgeRepo: knowledgeRepo,
		projectID:     projectID,
	}

	tc.ensureTestProject()

	// Create mock chat service that writes to real repositories
	mockChatSvc := &mockChatService{
		getHistoryFunc: func(ctx context.Context, pid uuid.UUID, limit int) ([]*models.ChatMessage, error) {
			return chatRepo.GetHistory(ctx, pid, limit)
		},
		clearHistoryFunc: func(ctx context.Context, pid uuid.UUID) error {
			return chatRepo.ClearHistory(ctx, pid)
		},
	}

	tc.mockChatService = mockChatSvc
	tc.mockKnowledgeSvc = &mockKnowledgeServiceChat{
		getAllFunc: func(ctx context.Context, pid uuid.UUID) ([]*models.KnowledgeFact, error) {
			return knowledgeRepo.GetByProject(ctx, pid)
		},
		getByTypeFunc: func(ctx context.Context, pid uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
			return knowledgeRepo.GetByType(ctx, pid, factType)
		},
	}

	tc.handler = NewOntologyChatHandler(mockChatSvc, knowledgeSvc, zap.NewNop())

	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *chatIntegrationTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Chat Integration Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// cleanup removes test data.
func (tc *chatIntegrationTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_chat_messages WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_project_knowledge WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_questions WHERE project_id = $1", tc.projectID)
	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontologies WHERE project_id = $1", tc.projectID)
}

// makeRequest creates an HTTP request with proper context (tenant scope + auth claims).
func (tc *chatIntegrationTestContext) makeRequest(method, path string, body any) *http.Request {
	tc.t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			tc.t.Fatalf("Failed to marshal request body: %v", err)
		}
		reqBody = bytes.NewReader(bodyBytes)
	} else {
		reqBody = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")

	// Set up tenant scope
	ctx := req.Context()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)

	// Set up auth claims
	claims := &auth.Claims{ProjectID: tc.projectID.String()}
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)

	req = req.WithContext(ctx)

	// Clean up tenant scope after test
	tc.t.Cleanup(func() {
		scope.Close()
	})

	return req
}

// parseSSEEvents parses SSE data from response body.
func parseSSEEvents(body string) []models.ChatEvent {
	events := []models.ChatEvent{}
	scanner := bufio.NewScanner(strings.NewReader(body))

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			var event models.ChatEvent
			jsonData := line[6:] // Remove "data: " prefix
			if err := json.Unmarshal([]byte(jsonData), &event); err == nil {
				events = append(events, event)
			}
		}
	}

	return events
}

// ============================================================================
// Initialize Tests
// ============================================================================

func TestChatHandlerIntegration_Initialize_Success(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockChatService.initializeFunc = func(ctx context.Context, projectID uuid.UUID) (*models.ChatInitResponse, error) {
		return &models.ChatInitResponse{
			OpeningMessage:       "Welcome! Ready to help with your ontology.",
			PendingQuestionCount: 5,
			HasExistingHistory:   true,
		}, nil
	}

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/initialize", nil)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.Initialize(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp ApiResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Errorf("expected success to be true")
	}

	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected data to be a map, got %T", resp.Data)
	}

	if data["opening_message"] != "Welcome! Ready to help with your ontology." {
		t.Errorf("expected opening message, got %v", data["opening_message"])
	}
	if data["pending_question_count"] != float64(5) {
		t.Errorf("expected 5 pending questions, got %v", data["pending_question_count"])
	}
	if data["has_existing_history"] != true {
		t.Errorf("expected has_existing_history true, got %v", data["has_existing_history"])
	}
}

// ============================================================================
// SendMessage SSE Tests
// ============================================================================

func TestChatHandlerIntegration_SendMessage_SSE_Headers(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/message",
		SendMessageRequest{Message: "Hello"})
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.SendMessage(rec, req)

	// Verify SSE headers
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("expected Content-Type 'text/event-stream', got %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("expected Cache-Control 'no-cache', got %q", cc)
	}
	if conn := rec.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("expected Connection 'keep-alive', got %q", conn)
	}
	if xab := rec.Header().Get("X-Accel-Buffering"); xab != "no" {
		t.Errorf("expected X-Accel-Buffering 'no', got %q", xab)
	}
}

func TestChatHandlerIntegration_SendMessage_SSE_Events(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockChatService.sendMessageFunc = func(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
		eventChan <- models.NewTextEvent("Hello, ")
		eventChan <- models.NewTextEvent("how can I help?")
		eventChan <- models.NewDoneEvent()
		return nil
	}

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/message",
		SendMessageRequest{Message: "Hi"})
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.SendMessage(rec, req)

	events := parseSSEEvents(rec.Body.String())

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// First text event
	if events[0].Type != models.ChatEventText {
		t.Errorf("expected first event type 'text', got %q", events[0].Type)
	}
	if events[0].Content != "Hello, " {
		t.Errorf("expected first event content 'Hello, ', got %q", events[0].Content)
	}

	// Second text event
	if events[1].Type != models.ChatEventText {
		t.Errorf("expected second event type 'text', got %q", events[1].Type)
	}

	// Done event
	if events[2].Type != models.ChatEventDone {
		t.Errorf("expected last event type 'done', got %q", events[2].Type)
	}
}

func TestChatHandlerIntegration_SendMessage_WithToolCalls(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockChatService.sendMessageFunc = func(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
		// Simulate tool call flow
		toolCall := models.ToolCall{
			ID:   "call_123",
			Type: "function",
			Function: models.ToolCallFunction{
				Name:      "query_column_values",
				Arguments: `{"table_name": "accounts", "column_name": "status"}`,
			},
		}
		eventChan <- models.NewToolCallEvent(toolCall)
		eventChan <- models.NewToolResultEvent("call_123", []string{"active", "inactive", "pending"})
		eventChan <- models.NewTextEvent("The status column has 3 values.")
		eventChan <- models.NewDoneEvent()
		return nil
	}

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/message",
		SendMessageRequest{Message: "What are the status values?"})
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.SendMessage(rec, req)

	events := parseSSEEvents(rec.Body.String())

	if len(events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(events))
	}

	// Tool call event
	if events[0].Type != models.ChatEventToolCall {
		t.Errorf("expected tool_call event, got %q", events[0].Type)
	}

	// Tool result event
	if events[1].Type != models.ChatEventToolResult {
		t.Errorf("expected tool_result event, got %q", events[1].Type)
	}
	if events[1].Content != "call_123" {
		t.Errorf("expected tool result content 'call_123', got %q", events[1].Content)
	}

	// Text event
	if events[2].Type != models.ChatEventText {
		t.Errorf("expected text event, got %q", events[2].Type)
	}

	// Done event
	if events[3].Type != models.ChatEventDone {
		t.Errorf("expected done event, got %q", events[3].Type)
	}
}

func TestChatHandlerIntegration_SendMessage_OntologyUpdate(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockChatService.sendMessageFunc = func(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
		eventChan <- models.NewTextEvent("I'll update the accounts table.")
		eventChan <- models.NewOntologyUpdateEvent("accounts", map[string]any{
			"business_name": "Customer Accounts",
			"description":   "Contains all customer account information",
		})
		eventChan <- models.NewDoneEvent()
		return nil
	}

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/message",
		SendMessageRequest{Message: "Update accounts description"})
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.SendMessage(rec, req)

	events := parseSSEEvents(rec.Body.String())

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Ontology update event
	if events[1].Type != models.ChatEventOntologyUpdate {
		t.Errorf("expected ontology_update event, got %q", events[1].Type)
	}
	if events[1].Content != "accounts" {
		t.Errorf("expected table name 'accounts', got %q", events[1].Content)
	}
}

func TestChatHandlerIntegration_SendMessage_KnowledgeStored(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockChatService.sendMessageFunc = func(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
		fact := &models.KnowledgeFact{
			ID:       uuid.New(),
			FactType: models.FactTypeFiscalYear,
			Key:      "start_month",
			Value:    "July",
		}
		eventChan <- models.NewTextEvent("I've stored that information.")
		eventChan <- models.NewKnowledgeStoredEvent(fact)
		eventChan <- models.NewDoneEvent()
		return nil
	}

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/message",
		SendMessageRequest{Message: "Our fiscal year starts in July"})
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.SendMessage(rec, req)

	events := parseSSEEvents(rec.Body.String())

	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	if events[1].Type != models.ChatEventKnowledgeStored {
		t.Errorf("expected knowledge_stored event, got %q", events[1].Type)
	}
}

func TestChatHandlerIntegration_SendMessage_Error(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockChatService.sendMessageFunc = func(ctx context.Context, projectID uuid.UUID, message string, eventChan chan<- models.ChatEvent) error {
		eventChan <- models.NewErrorEvent("Something went wrong")
		return nil
	}

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/message",
		SendMessageRequest{Message: "test"})
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.SendMessage(rec, req)

	events := parseSSEEvents(rec.Body.String())

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Type != models.ChatEventError {
		t.Errorf("expected error event, got %q", events[0].Type)
	}
	if events[0].Content != "Something went wrong" {
		t.Errorf("expected error message, got %q", events[0].Content)
	}
}

func TestChatHandlerIntegration_SendMessage_MissingMessage(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	req := tc.makeRequest(http.MethodPost, "/api/projects/"+tc.projectID.String()+"/ontology/chat/message",
		SendMessageRequest{Message: ""})
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.SendMessage(rec, req)

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

func TestChatHandlerIntegration_GetHistory_Success(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockChatService.getHistoryFunc = func(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error) {
		return []*models.ChatMessage{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				Role:      models.ChatRoleUser,
				Content:   "Hello",
			},
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				Role:      models.ChatRoleAssistant,
				Content:   "Hi there!",
			},
		}, nil
	}

	req := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/ontology/chat/history", nil)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
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

func TestChatHandlerIntegration_GetHistory_WithLimit(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	receivedLimit := 0
	tc.mockChatService.getHistoryFunc = func(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.ChatMessage, error) {
		receivedLimit = limit
		return []*models.ChatMessage{}, nil
	}

	req := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/ontology/chat/history?limit=25", nil)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.GetHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if receivedLimit != 25 {
		t.Errorf("expected limit 25, got %d", receivedLimit)
	}
}

// ============================================================================
// ClearHistory Tests
// ============================================================================

func TestChatHandlerIntegration_ClearHistory_Success(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	cleared := false
	tc.mockChatService.clearHistoryFunc = func(ctx context.Context, projectID uuid.UUID) error {
		cleared = true
		return nil
	}

	req := tc.makeRequest(http.MethodDelete, "/api/projects/"+tc.projectID.String()+"/ontology/chat/history", nil)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.ClearHistory(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if !cleared {
		t.Error("expected clear history to be called")
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
// GetKnowledge Tests
// ============================================================================

func TestChatHandlerIntegration_GetKnowledge_Success(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	tc.mockKnowledgeSvc.getAllFunc = func(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
		return []*models.KnowledgeFact{
			{
				ID:       uuid.New(),
				FactType: models.FactTypeFiscalYear,
				Key:      "start_month",
				Value:    "July",
			},
			{
				ID:       uuid.New(),
				FactType: models.FactTypeTerminology,
				Key:      "MRR",
				Value:    "Monthly Recurring Revenue",
			},
		}, nil
	}

	req := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/ontology/knowledge", nil)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.GetKnowledge(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
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
}

func TestChatHandlerIntegration_GetKnowledge_FilterByType(t *testing.T) {
	tc := setupChatIntegrationTest(t)
	tc.cleanup()

	receivedType := ""
	tc.mockKnowledgeSvc.getByTypeFunc = func(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
		receivedType = factType
		return []*models.KnowledgeFact{
			{
				ID:       uuid.New(),
				FactType: factType,
				Key:      "test",
				Value:    "value",
			},
		}, nil
	}

	req := tc.makeRequest(http.MethodGet, "/api/projects/"+tc.projectID.String()+"/ontology/knowledge?type=fiscal_year", nil)
	req.SetPathValue("pid", tc.projectID.String())

	rec := httptest.NewRecorder()
	tc.handler.GetKnowledge(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	if receivedType != "fiscal_year" {
		t.Errorf("expected type filter 'fiscal_year', got %q", receivedType)
	}
}

// ============================================================================
// Invalid Project ID Tests
// ============================================================================

func TestChatHandlerIntegration_InvalidProjectID(t *testing.T) {
	tc := setupChatIntegrationTest(t)

	// Create request with invalid project ID
	req := httptest.NewRequest(http.MethodPost, "/api/projects/invalid-uuid/ontology/chat/initialize", nil)
	req.SetPathValue("pid", "invalid-uuid")

	rec := httptest.NewRecorder()
	tc.handler.Initialize(rec, req)

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
