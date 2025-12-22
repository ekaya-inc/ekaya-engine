package llm

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// MockLLMClient is a configurable mock for testing LLM functionality.
// Set the function fields to control behavior in tests.
type MockLLMClient struct {
	// GenerateResponseFunc is called when GenerateResponse is invoked.
	// If nil, returns empty result and nil error.
	GenerateResponseFunc func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*GenerateResponseResult, error)

	// CreateEmbeddingFunc is called when CreateEmbedding is invoked.
	// If nil, returns nil slice and nil error.
	CreateEmbeddingFunc func(ctx context.Context, input string, model string) ([]float32, error)

	// CreateEmbeddingsFunc is called when CreateEmbeddings is invoked.
	// If nil, returns nil slice and nil error.
	CreateEmbeddingsFunc func(ctx context.Context, inputs []string, model string) ([][]float32, error)

	// Model is returned by GetModel. Defaults to "mock-model".
	Model string

	// Endpoint is returned by GetEndpoint. Defaults to "http://mock-endpoint".
	Endpoint string

	// Call tracking for verification
	GenerateResponseCalls int
	CreateEmbeddingCalls  int
	CreateEmbeddingsCalls int
}

// NewMockLLMClient creates a new mock with sensible defaults.
func NewMockLLMClient() *MockLLMClient {
	return &MockLLMClient{
		Model:    "mock-model",
		Endpoint: "http://mock-endpoint",
	}
}

// GenerateResponse implements LLMClient.
func (m *MockLLMClient) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*GenerateResponseResult, error) {
	m.GenerateResponseCalls++
	if m.GenerateResponseFunc != nil {
		return m.GenerateResponseFunc(ctx, prompt, systemMessage, temperature, thinking)
	}
	return &GenerateResponseResult{}, nil
}

// CreateEmbedding implements LLMClient.
func (m *MockLLMClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	m.CreateEmbeddingCalls++
	if m.CreateEmbeddingFunc != nil {
		return m.CreateEmbeddingFunc(ctx, input, model)
	}
	return nil, nil
}

// CreateEmbeddings implements LLMClient.
func (m *MockLLMClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	m.CreateEmbeddingsCalls++
	if m.CreateEmbeddingsFunc != nil {
		return m.CreateEmbeddingsFunc(ctx, inputs, model)
	}
	return nil, nil
}

// GetModel implements LLMClient.
func (m *MockLLMClient) GetModel() string {
	if m.Model == "" {
		return "mock-model"
	}
	return m.Model
}

// GetEndpoint implements LLMClient.
func (m *MockLLMClient) GetEndpoint() string {
	if m.Endpoint == "" {
		return "http://mock-endpoint"
	}
	return m.Endpoint
}

// Reset clears call tracking counters.
func (m *MockLLMClient) Reset() {
	m.GenerateResponseCalls = 0
	m.CreateEmbeddingCalls = 0
	m.CreateEmbeddingsCalls = 0
}

// Ensure MockLLMClient implements LLMClient at compile time.
var _ LLMClient = (*MockLLMClient)(nil)

// MockToolExecutor is a configurable mock for testing tool execution.
type MockToolExecutor struct {
	// ExecuteToolFunc is called when ExecuteTool is invoked.
	// If nil, returns empty string and nil error.
	ExecuteToolFunc func(ctx context.Context, name string, arguments string) (string, error)

	// Call tracking
	ExecuteToolCalls []MockToolCall
}

// MockToolCall records a call to ExecuteTool.
type MockToolCall struct {
	Name      string
	Arguments string
}

// NewMockToolExecutor creates a new mock tool executor.
func NewMockToolExecutor() *MockToolExecutor {
	return &MockToolExecutor{
		ExecuteToolCalls: []MockToolCall{},
	}
}

// ExecuteTool implements ToolExecutor.
func (m *MockToolExecutor) ExecuteTool(ctx context.Context, name string, arguments string) (string, error) {
	m.ExecuteToolCalls = append(m.ExecuteToolCalls, MockToolCall{Name: name, Arguments: arguments})
	if m.ExecuteToolFunc != nil {
		return m.ExecuteToolFunc(ctx, name, arguments)
	}
	return `{"success": true}`, nil
}

// Reset clears call tracking.
func (m *MockToolExecutor) Reset() {
	m.ExecuteToolCalls = []MockToolCall{}
}

// Ensure MockToolExecutor implements ToolExecutor at compile time.
var _ ToolExecutor = (*MockToolExecutor)(nil)

// MockClientFactory is a configurable mock for testing LLM client creation.
type MockClientFactory struct {
	// CreateForProjectFunc is called when CreateForProject is invoked.
	// If nil, returns a new MockLLMClient.
	CreateForProjectFunc func(ctx context.Context, projectID uuid.UUID) (LLMClient, error)

	// CreateEmbeddingClientFunc is called when CreateEmbeddingClient is invoked.
	// If nil, returns a new MockLLMClient.
	CreateEmbeddingClientFunc func(ctx context.Context, projectID uuid.UUID) (LLMClient, error)

	// CreateStreamingClientFunc is called when CreateStreamingClient is invoked.
	// If nil, returns nil and an error.
	CreateStreamingClientFunc func(ctx context.Context, projectID uuid.UUID) (*StreamingClient, error)

	// MockClient is the default client returned if functions are not set.
	MockClient *MockLLMClient
}

// NewMockClientFactory creates a new mock client factory.
func NewMockClientFactory() *MockClientFactory {
	return &MockClientFactory{
		MockClient: NewMockLLMClient(),
	}
}

// CreateForProject implements LLMClientFactory.
func (f *MockClientFactory) CreateForProject(ctx context.Context, projectID uuid.UUID) (LLMClient, error) {
	if f.CreateForProjectFunc != nil {
		return f.CreateForProjectFunc(ctx, projectID)
	}
	return f.MockClient, nil
}

// CreateEmbeddingClient implements LLMClientFactory.
func (f *MockClientFactory) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (LLMClient, error) {
	if f.CreateEmbeddingClientFunc != nil {
		return f.CreateEmbeddingClientFunc(ctx, projectID)
	}
	return f.MockClient, nil
}

// CreateStreamingClient implements LLMClientFactory.
func (f *MockClientFactory) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*StreamingClient, error) {
	if f.CreateStreamingClientFunc != nil {
		return f.CreateStreamingClientFunc(ctx, projectID)
	}
	return nil, fmt.Errorf("streaming client not configured in mock")
}

// Ensure MockClientFactory implements LLMClientFactory at compile time.
var _ LLMClientFactory = (*MockClientFactory)(nil)
