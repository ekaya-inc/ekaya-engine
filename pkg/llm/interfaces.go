// Package llm provides OpenAI-compatible LLM client functionality.
package llm

import (
	"context"
)

// LLMClient defines the interface for LLM operations.
// Combines both generative (chat completion) and embedding capabilities.
// Use this interface for dependency injection to enable mocking in tests.
type LLMClient interface {
	// GenerateResponse generates a chat completion response.
	GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64) (string, error)

	// CreateEmbedding generates an embedding vector for the input text.
	CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error)

	// CreateEmbeddings generates embeddings for multiple inputs.
	CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error)

	// GetModel returns the configured model name.
	GetModel() string

	// GetEndpoint returns the configured endpoint.
	GetEndpoint() string
}

// Ensure Client implements LLMClient at compile time.
var _ LLMClient = (*Client)(nil)
