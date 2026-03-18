package llm

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

// ErrorType indicates which configuration field caused the error.
type ErrorType string

const (
	ErrorTypeNone        ErrorType = ""
	ErrorTypeEndpoint    ErrorType = "endpoint"
	ErrorTypeAuth        ErrorType = "auth"
	ErrorTypeModel       ErrorType = "model"
	ErrorTypeRateLimited ErrorType = "rate_limited"
	ErrorTypeUnknown     ErrorType = "unknown"
)

// TestResult contains connection test results.
type TestResult struct {
	Success                  bool      `json:"success"`
	Message                  string    `json:"message"`
	LLMSuccess               bool      `json:"llm_success"`
	LLMMessage               string    `json:"llm_message,omitempty"`
	LLMErrorType             ErrorType `json:"llm_error_type,omitempty"`
	LLMResponseTimeMs        int64     `json:"llm_response_time_ms,omitempty"`
	ResolvedLLMBaseURL       string    `json:"resolved_llm_base_url,omitempty"`
	EmbeddingSuccess         bool      `json:"embedding_success"`
	EmbeddingMessage         string    `json:"embedding_message,omitempty"`
	EmbeddingErrorType       ErrorType `json:"embedding_error_type,omitempty"`
	ResolvedEmbeddingBaseURL string    `json:"resolved_embedding_base_url,omitempty"`
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

var commonOpenAIBasePathSuffixes = []string{
	"/v1",
	"/api/v1",
	"/openai/v1",
	"/api/openai/v1",
	"/v1beta/openai",
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
		result.ResolvedLLMBaseURL = llmResult.ResolvedBaseURL
	}

	// Test embedding
	embURL := cfg.EffectiveEmbeddingBaseURL()
	embKey := cfg.EffectiveEmbeddingAPIKey()
	if cfg.EmbeddingBaseURL == "" && result.ResolvedLLMBaseURL != "" {
		embURL = result.ResolvedLLMBaseURL
	}

	if embURL != "" && cfg.EmbeddingModel != "" {
		embResult := t.testEmbedding(ctx, embURL, embKey, cfg.EmbeddingModel)
		result.EmbeddingSuccess = embResult.Success
		result.EmbeddingMessage = embResult.Message
		result.EmbeddingErrorType = embResult.ErrorType
		result.ResolvedEmbeddingBaseURL = embResult.ResolvedBaseURL
	}

	// Overall success
	if result.LLMSuccess {
		result.Success = true
		result.Message = "LLM connection successful"
	} else {
		result.Message = result.LLMMessage
	}

	return result
}

type singleResult struct {
	Success         bool
	Message         string
	ErrorType       ErrorType
	ResponseTimeMs  int64
	ResolvedBaseURL string
	StatusCode      int
}

func (t *connectionTester) testLLM(ctx context.Context, baseURL, apiKey, model string) singleResult {
	return t.testWithFallback(ctx, baseURL, func(ctx context.Context, candidate string) singleResult {
		return t.testLLMOnce(ctx, candidate, apiKey, model)
	})
}

func (t *connectionTester) testLLMOnce(ctx context.Context, baseURL, apiKey, model string) singleResult {
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
		msg, errType, statusCode := categorizeError("LLM", err)
		return singleResult{Message: msg, ErrorType: errType, ResponseTimeMs: elapsed, StatusCode: statusCode}
	}

	if len(resp.Choices) == 0 {
		return singleResult{Message: "LLM returned no response", ErrorType: ErrorTypeUnknown}
	}

	return singleResult{
		Success:         true,
		Message:         fmt.Sprintf("LLM connection successful (model: %s, %dms)", model, elapsed),
		ResponseTimeMs:  elapsed,
		ResolvedBaseURL: strings.TrimSuffix(baseURL, "/"),
	}
}

func (t *connectionTester) testEmbedding(ctx context.Context, baseURL, apiKey, model string) singleResult {
	return t.testWithFallback(ctx, baseURL, func(ctx context.Context, candidate string) singleResult {
		return t.testEmbeddingOnce(ctx, candidate, apiKey, model)
	})
}

func (t *connectionTester) testEmbeddingOnce(ctx context.Context, baseURL, apiKey, model string) singleResult {
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
		msg, errType, statusCode := categorizeError("Embedding", err)
		return singleResult{Message: msg, ErrorType: errType, ResponseTimeMs: elapsed, StatusCode: statusCode}
	}

	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return singleResult{Message: "Embedding returned no vectors", ErrorType: ErrorTypeUnknown}
	}

	return singleResult{
		Success:         true,
		Message:         fmt.Sprintf("Embedding successful (model: %s, %dms, %d dims)", model, elapsed, len(resp.Data[0].Embedding)),
		ResponseTimeMs:  elapsed,
		ResolvedBaseURL: strings.TrimSuffix(baseURL, "/"),
	}
}

func (t *connectionTester) testWithFallback(
	ctx context.Context,
	baseURL string,
	testFn func(context.Context, string) singleResult,
) singleResult {
	candidates := candidateBaseURLs(baseURL)
	var last singleResult

	for idx, candidate := range candidates {
		attempt := testFn(ctx, candidate)
		if attempt.Success {
			return attempt
		}

		last = attempt
		if idx == len(candidates)-1 || !shouldRetryWithAlternatePath(attempt) {
			return attempt
		}
	}

	return last
}

func candidateBaseURLs(raw string) []string {
	normalized := strings.TrimSpace(strings.TrimSuffix(raw, "/"))
	if normalized == "" {
		return nil
	}

	candidates := []string{normalized}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return candidates
	}

	if strings.Trim(parsed.Path, "/") != "" {
		return candidates
	}

	root := parsed.Scheme + "://" + parsed.Host
	for _, suffix := range commonOpenAIBasePathSuffixes {
		candidates = appendUniqueString(candidates, root+suffix)
	}

	return candidates
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func shouldRetryWithAlternatePath(result singleResult) bool {
	if result.ErrorType != ErrorTypeEndpoint {
		return false
	}
	if result.StatusCode == 404 || result.StatusCode == 405 {
		return true
	}

	lower := strings.ToLower(result.Message)
	return strings.Contains(lower, "endpoint not found")
}

func categorizeError(prefix string, err error) (string, ErrorType, int) {
	errStr := err.Error()
	lower := strings.ToLower(errStr)
	statusCode := 0
	if classified := ClassifyError(err); classified != nil {
		statusCode = classified.StatusCode
	}

	if statusCode == 401 || strings.Contains(errStr, "401") || strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "invalid api key") {
		return fmt.Sprintf("%s: Invalid API key", prefix), ErrorTypeAuth, statusCode
	}

	if strings.Contains(lower, "model") && (strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist")) {
		return fmt.Sprintf("%s: Model not found", prefix), ErrorTypeModel, statusCode
	}

	if statusCode == 404 || strings.Contains(errStr, "404") {
		return fmt.Sprintf("%s: Endpoint not found - check base URL", prefix), ErrorTypeEndpoint, statusCode
	}

	if statusCode == 429 || strings.Contains(lower, "rate limit") || strings.Contains(lower, "too many requests") {
		return fmt.Sprintf("%s: Rate limited", prefix), ErrorTypeRateLimited, statusCode
	}

	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") {
		return fmt.Sprintf("%s: Connection failed - check base URL", prefix), ErrorTypeEndpoint, statusCode
	}

	if strings.Contains(lower, "timeout") {
		return fmt.Sprintf("%s: Connection timed out", prefix), ErrorTypeEndpoint, statusCode
	}

	return fmt.Sprintf("%s: %s", prefix, errStr), ErrorTypeUnknown, statusCode
}

// Ensure connectionTester implements ConnectionTester at compile time.
var _ ConnectionTester = (*connectionTester)(nil)
