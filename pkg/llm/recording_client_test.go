package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockRecorder captures recorded conversations for testing.
type mockRecorder struct {
	pending     []*models.LLMConversation
	completions []*models.LLMConversation
	recordings  []*models.LLMConversation // Legacy Record calls
	saveErr     error
}

func (m *mockRecorder) Record(conv *models.LLMConversation) {
	m.recordings = append(m.recordings, conv)
}

func (m *mockRecorder) SavePending(ctx context.Context, conv *models.LLMConversation) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	// Make a copy since the caller mutates the conv after this call
	convCopy := *conv
	m.pending = append(m.pending, &convCopy)
	return nil
}

func (m *mockRecorder) RecordCompletion(conv *models.LLMConversation) {
	// Make a copy for consistency
	convCopy := *conv
	m.completions = append(m.completions, &convCopy)
}

func TestRecordingClient_GenerateResponse_RecordsSuccess(t *testing.T) {
	mockClient := NewMockLLMClient()
	mockClient.Model = "gpt-4"
	mockClient.Endpoint = "https://api.openai.com"
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*GenerateResponseResult, error) {
		return &GenerateResponseResult{
			Content:          "Hello, world!",
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		}, nil
	}

	recorder := &mockRecorder{}
	projectID := uuid.New()
	client := NewRecordingClient(mockClient, recorder, projectID)

	ctx := context.Background()
	result, err := client.GenerateResponse(ctx, "Say hello", "You are helpful", 0.7, false)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got '%s'", result.Content)
	}

	// Verify pending was saved first
	if len(recorder.pending) != 1 {
		t.Fatalf("expected 1 pending record, got %d", len(recorder.pending))
	}

	pendingConv := recorder.pending[0]
	if pendingConv.Status != models.LLMConversationStatusPending {
		t.Errorf("expected pending status 'pending', got '%s'", pendingConv.Status)
	}
	if pendingConv.ProjectID != projectID {
		t.Errorf("expected project ID %s, got %s", projectID, pendingConv.ProjectID)
	}
	if pendingConv.Model != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got '%s'", pendingConv.Model)
	}

	// Verify completion was recorded
	if len(recorder.completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(recorder.completions))
	}

	conv := recorder.completions[0]
	if conv.Status != models.LLMConversationStatusSuccess {
		t.Errorf("expected status 'success', got '%s'", conv.Status)
	}
	if conv.ResponseContent != "Hello, world!" {
		t.Errorf("expected response 'Hello, world!', got '%s'", conv.ResponseContent)
	}
	if conv.ErrorMessage != "" {
		t.Errorf("expected empty error message, got '%s'", conv.ErrorMessage)
	}

	// Verify token counts
	if conv.PromptTokens == nil || *conv.PromptTokens != 10 {
		t.Errorf("expected prompt tokens 10, got %v", conv.PromptTokens)
	}
	if conv.CompletionTokens == nil || *conv.CompletionTokens != 5 {
		t.Errorf("expected completion tokens 5, got %v", conv.CompletionTokens)
	}
	if conv.TotalTokens == nil || *conv.TotalTokens != 15 {
		t.Errorf("expected total tokens 15, got %v", conv.TotalTokens)
	}

	// Verify request messages are on the pending record
	if len(pendingConv.RequestMessages) != 2 {
		t.Fatalf("expected 2 request messages, got %d", len(pendingConv.RequestMessages))
	}

	// Verify duration is recorded on completion (may be 0 for fast mocks)
	if conv.DurationMs < 0 {
		t.Error("expected non-negative duration")
	}

	// Same ID should be used for pending and completion
	if pendingConv.ID != conv.ID {
		t.Errorf("expected same ID for pending and completion, got %s and %s", pendingConv.ID, conv.ID)
	}
}

func TestRecordingClient_GenerateResponse_RecordsError(t *testing.T) {
	mockClient := NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*GenerateResponseResult, error) {
		return nil, errors.New("API rate limit exceeded")
	}

	recorder := &mockRecorder{}
	projectID := uuid.New()
	client := NewRecordingClient(mockClient, recorder, projectID)

	ctx := context.Background()
	_, err := client.GenerateResponse(ctx, "Say hello", "You are helpful", 0.7, false)

	if err == nil {
		t.Fatal("expected error")
	}

	// Verify pending was saved
	if len(recorder.pending) != 1 {
		t.Fatalf("expected 1 pending record, got %d", len(recorder.pending))
	}

	// Verify completion was recorded with error status
	if len(recorder.completions) != 1 {
		t.Fatalf("expected 1 completion, got %d", len(recorder.completions))
	}

	conv := recorder.completions[0]
	if conv.Status != models.LLMConversationStatusError {
		t.Errorf("expected status 'error', got '%s'", conv.Status)
	}
	if conv.ErrorMessage != "API rate limit exceeded" {
		t.Errorf("expected error message 'API rate limit exceeded', got '%s'", conv.ErrorMessage)
	}
	if conv.ResponseContent != "" {
		t.Errorf("expected empty response on error, got '%s'", conv.ResponseContent)
	}

	// Token counts should be nil on error
	if conv.PromptTokens != nil {
		t.Error("expected nil prompt tokens on error")
	}
}

func TestRecordingClient_GenerateResponse_CapturesContext(t *testing.T) {
	mockClient := NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*GenerateResponseResult, error) {
		return &GenerateResponseResult{Content: "OK"}, nil
	}

	recorder := &mockRecorder{}
	projectID := uuid.New()
	workflowID := uuid.New()
	client := NewRecordingClient(mockClient, recorder, projectID)

	ctx := WithTaskContext(context.Background(), workflowID, "task-123", "Analyze users", "users")
	_, err := client.GenerateResponse(ctx, "Test", "System", 0.5, false)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Context is captured on the pending record
	conv := recorder.pending[0]
	if conv.Context == nil {
		t.Fatal("expected context to be captured")
	}
	if conv.Context["workflow_id"] != workflowID.String() {
		t.Errorf("expected workflow_id %s, got %v", workflowID, conv.Context["workflow_id"])
	}
	if conv.Context["task_id"] != "task-123" {
		t.Errorf("expected task_id 'task-123', got %v", conv.Context["task_id"])
	}
	if conv.Context["task_name"] != "Analyze users" {
		t.Errorf("expected task_name 'Analyze users', got %v", conv.Context["task_name"])
	}
	if conv.Context["entity_name"] != "users" {
		t.Errorf("expected entity_name 'users', got %v", conv.Context["entity_name"])
	}
}

func TestRecordingClient_GenerateResponse_NilContextWhenNotProvided(t *testing.T) {
	mockClient := NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*GenerateResponseResult, error) {
		return &GenerateResponseResult{Content: "OK"}, nil
	}

	recorder := &mockRecorder{}
	client := NewRecordingClient(mockClient, recorder, uuid.New())

	ctx := context.Background() // No context
	_, _ = client.GenerateResponse(ctx, "Test", "System", 0.5, false)

	conv := recorder.pending[0]
	if conv.Context != nil {
		t.Error("expected nil context when not provided")
	}
}

func TestRecordingClient_GenerateResponse_CapturesTemperature(t *testing.T) {
	mockClient := NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMessage string, temperature float64, thinking bool) (*GenerateResponseResult, error) {
		return &GenerateResponseResult{Content: "OK"}, nil
	}

	recorder := &mockRecorder{}
	client := NewRecordingClient(mockClient, recorder, uuid.New())

	_, _ = client.GenerateResponse(context.Background(), "Test", "System", 0.42, false)

	conv := recorder.pending[0]
	if conv.Temperature == nil {
		t.Fatal("expected temperature to be captured")
	}
	if *conv.Temperature != 0.42 {
		t.Errorf("expected temperature 0.42, got %f", *conv.Temperature)
	}
}

func TestRecordingClient_CreateEmbedding_DelegatesToInner(t *testing.T) {
	mockClient := NewMockLLMClient()
	expectedEmbedding := []float32{0.1, 0.2, 0.3}
	mockClient.CreateEmbeddingFunc = func(ctx context.Context, input, model string) ([]float32, error) {
		return expectedEmbedding, nil
	}

	recorder := &mockRecorder{}
	client := NewRecordingClient(mockClient, recorder, uuid.New())

	embedding, err := client.CreateEmbedding(context.Background(), "test input", "text-embedding-3-small")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embedding) != 3 {
		t.Errorf("expected 3 elements, got %d", len(embedding))
	}

	// Embeddings are not recorded
	if len(recorder.recordings) != 0 {
		t.Error("expected no recordings for embedding calls")
	}
}

func TestRecordingClient_CreateEmbeddings_DelegatesToInner(t *testing.T) {
	mockClient := NewMockLLMClient()
	expectedEmbeddings := [][]float32{{0.1, 0.2}, {0.3, 0.4}}
	mockClient.CreateEmbeddingsFunc = func(ctx context.Context, inputs []string, model string) ([][]float32, error) {
		return expectedEmbeddings, nil
	}

	recorder := &mockRecorder{}
	client := NewRecordingClient(mockClient, recorder, uuid.New())

	embeddings, err := client.CreateEmbeddings(context.Background(), []string{"a", "b"}, "text-embedding-3-small")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(embeddings) != 2 {
		t.Errorf("expected 2 embeddings, got %d", len(embeddings))
	}

	// Embeddings are not recorded
	if len(recorder.recordings) != 0 {
		t.Error("expected no recordings for embedding calls")
	}
}

func TestRecordingClient_GetModel_DelegatesToInner(t *testing.T) {
	mockClient := NewMockLLMClient()
	mockClient.Model = "gpt-4-turbo"

	recorder := &mockRecorder{}
	client := NewRecordingClient(mockClient, recorder, uuid.New())

	model := client.GetModel()

	if model != "gpt-4-turbo" {
		t.Errorf("expected 'gpt-4-turbo', got '%s'", model)
	}
}

func TestRecordingClient_GetEndpoint_DelegatesToInner(t *testing.T) {
	mockClient := NewMockLLMClient()
	mockClient.Endpoint = "https://custom.endpoint.com"

	recorder := &mockRecorder{}
	client := NewRecordingClient(mockClient, recorder, uuid.New())

	endpoint := client.GetEndpoint()

	if endpoint != "https://custom.endpoint.com" {
		t.Errorf("expected 'https://custom.endpoint.com', got '%s'", endpoint)
	}
}
