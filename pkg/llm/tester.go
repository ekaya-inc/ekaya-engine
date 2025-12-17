package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// ErrorType indicates which configuration field caused the error.
type ErrorType string

const (
	ErrorTypeNone     ErrorType = ""
	ErrorTypeEndpoint ErrorType = "endpoint"
	ErrorTypeAuth     ErrorType = "auth"
	ErrorTypeModel    ErrorType = "model"
	ErrorTypeUnknown  ErrorType = "unknown"
)

// TestResult contains connection test results.
type TestResult struct {
	Success            bool      `json:"success"`
	Message            string    `json:"message"`
	LLMSuccess         bool      `json:"llm_success"`
	LLMMessage         string    `json:"llm_message,omitempty"`
	LLMErrorType       ErrorType `json:"llm_error_type,omitempty"`
	LLMResponseTimeMs  int64     `json:"llm_response_time_ms,omitempty"`
	EmbeddingSuccess   bool      `json:"embedding_success"`
	EmbeddingMessage   string    `json:"embedding_message,omitempty"`
	EmbeddingErrorType ErrorType `json:"embedding_error_type,omitempty"`
}

// TestConfig contains credentials to test.
type TestConfig struct {
	LLMBaseURL       string
	LLMAPIKey        string
	LLMModel         string
	EmbeddingBaseURL string
	EmbeddingAPIKey  string
	EmbeddingModel   string
}

// EffectiveEmbeddingBaseURL returns embedding URL, falling back to LLM URL.
func (c *TestConfig) EffectiveEmbeddingBaseURL() string {
	if c.EmbeddingBaseURL != "" {
		return c.EmbeddingBaseURL
	}
	return c.LLMBaseURL
}

// EffectiveEmbeddingAPIKey returns embedding key, falling back to LLM key.
func (c *TestConfig) EffectiveEmbeddingAPIKey() string {
	if c.EmbeddingAPIKey != "" {
		return c.EmbeddingAPIKey
	}
	return c.LLMAPIKey
}

// ConnectionTester tests AI provider connections.
// This interface enables mocking in tests.
type ConnectionTester interface {
	// Test tests both LLM and embedding connections.
	Test(ctx context.Context, cfg *TestConfig) *TestResult
}

// connectionTester implements ConnectionTester with real API calls.
type connectionTester struct {
	timeout time.Duration
}

// NewConnectionTester creates a new tester.
func NewConnectionTester() ConnectionTester {
	return &connectionTester{timeout: 30 * time.Second}
}

// Test tests both LLM and embedding connections.
func (t *connectionTester) Test(ctx context.Context, cfg *TestConfig) *TestResult {
	result := &TestResult{}

	// Test LLM
	if cfg.LLMBaseURL != "" && cfg.LLMModel != "" {
		llmResult := t.testLLM(ctx, cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel)
		result.LLMSuccess = llmResult.Success
		result.LLMMessage = llmResult.Message
		result.LLMErrorType = llmResult.ErrorType
		result.LLMResponseTimeMs = llmResult.ResponseTimeMs
	}

	// Test embedding
	embURL := cfg.EffectiveEmbeddingBaseURL()
	embKey := cfg.EffectiveEmbeddingAPIKey()

	if embURL != "" && cfg.EmbeddingModel != "" {
		embResult := t.testEmbedding(ctx, embURL, embKey, cfg.EmbeddingModel)
		result.EmbeddingSuccess = embResult.Success
		result.EmbeddingMessage = embResult.Message
		result.EmbeddingErrorType = embResult.ErrorType
	}

	// Overall success
	if result.LLMSuccess {
		result.Success = true
		if result.EmbeddingSuccess {
			result.Message = "LLM and embedding connections successful"
		} else if cfg.EmbeddingModel == "" {
			result.Message = "LLM connection successful (embedding not configured)"
		} else {
			result.Message = "LLM connection successful, embedding failed"
		}
	} else {
		result.Message = result.LLMMessage
	}

	return result
}

type singleResult struct {
	Success        bool
	Message        string
	ErrorType      ErrorType
	ResponseTimeMs int64
}

func (t *connectionTester) testLLM(ctx context.Context, baseURL, apiKey, model string) singleResult {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = strings.TrimSuffix(baseURL, "/")
	client := openai.NewClientWithConfig(config)

	start := time.Now()

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "Say 'ok' and nothing else."},
		},
		MaxCompletionTokens: 10,
	})

	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		msg, errType := categorizeError("LLM", err)
		return singleResult{Message: msg, ErrorType: errType, ResponseTimeMs: elapsed}
	}

	if len(resp.Choices) == 0 {
		return singleResult{Message: "LLM returned no response", ErrorType: ErrorTypeUnknown}
	}

	return singleResult{
		Success:        true,
		Message:        fmt.Sprintf("LLM connection successful (model: %s, %dms)", model, elapsed),
		ResponseTimeMs: elapsed,
	}
}

func (t *connectionTester) testEmbedding(ctx context.Context, baseURL, apiKey, model string) singleResult {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	config := openai.DefaultConfig(apiKey)
	config.BaseURL = strings.TrimSuffix(baseURL, "/")
	client := openai.NewClientWithConfig(config)

	start := time.Now()

	resp, err := client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(model),
		Input: []string{"test"},
	})

	elapsed := time.Since(start).Milliseconds()

	if err != nil {
		msg, errType := categorizeError("Embedding", err)
		return singleResult{Message: msg, ErrorType: errType, ResponseTimeMs: elapsed}
	}

	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return singleResult{Message: "Embedding returned no vectors", ErrorType: ErrorTypeUnknown}
	}

	return singleResult{
		Success:        true,
		Message:        fmt.Sprintf("Embedding successful (model: %s, %dms, %d dims)", model, elapsed, len(resp.Data[0].Embedding)),
		ResponseTimeMs: elapsed,
	}
}

func categorizeError(prefix string, err error) (string, ErrorType) {
	errStr := err.Error()
	lower := strings.ToLower(errStr)

	if strings.Contains(errStr, "401") || strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "invalid api key") {
		return fmt.Sprintf("%s: Invalid API key", prefix), ErrorTypeAuth
	}

	if strings.Contains(lower, "model") && (strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist")) {
		return fmt.Sprintf("%s: Model not found", prefix), ErrorTypeModel
	}

	if strings.Contains(errStr, "404") {
		return fmt.Sprintf("%s: Endpoint not found - check base URL", prefix), ErrorTypeEndpoint
	}

	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") {
		return fmt.Sprintf("%s: Connection failed - check base URL", prefix), ErrorTypeEndpoint
	}

	if strings.Contains(lower, "timeout") {
		return fmt.Sprintf("%s: Connection timed out", prefix), ErrorTypeEndpoint
	}

	return fmt.Sprintf("%s: %s", prefix, errStr), ErrorTypeUnknown
}

// Ensure connectionTester implements ConnectionTester at compile time.
var _ ConnectionTester = (*connectionTester)(nil)
