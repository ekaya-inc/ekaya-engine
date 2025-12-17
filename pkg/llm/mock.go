package llm

import (
	"context"
)

// MockLLMClient is a configurable mock for testing LLM functionality.
// Set the function fields to control behavior in tests.
type MockLLMClient struct {
	// GenerateResponseFunc is called when GenerateResponse is invoked.
	// If nil, returns empty string and nil error.
	GenerateResponseFunc func(ctx context.Context, prompt string, systemMessage string, temperature float64) (string, error)

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
func (m *MockLLMClient) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64) (string, error) {
	m.GenerateResponseCalls++
	if m.GenerateResponseFunc != nil {
		return m.GenerateResponseFunc(ctx, prompt, systemMessage, temperature)
	}
	return "", nil
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
