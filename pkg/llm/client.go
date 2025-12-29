// Package llm provides OpenAI-compatible LLM client functionality.
package llm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

// Client provides access to OpenAI-compatible LLM endpoints.
type Client struct {
	client    *openai.Client
	endpoint  string
	model     string
	projectID string
	logger    *zap.Logger
}

// Config holds configuration for creating an LLM client.
type Config struct {
	Endpoint  string // Base URL, e.g., "https://api.openai.com/v1"
	Model     string // Model name, e.g., "gpt-4o"
	APIKey    string // Optional for local endpoints
	ProjectID string // For logging context
}

// NewClient creates a new OpenAI-compatible LLM client.
func NewClient(cfg *Config, logger *zap.Logger) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if cfg.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	clientConfig := openai.DefaultConfig(cfg.APIKey)
	clientConfig.BaseURL = strings.TrimSuffix(cfg.Endpoint, "/")

	return &Client{
		client:    openai.NewClientWithConfig(clientConfig),
		endpoint:  cfg.Endpoint,
		model:     cfg.Model,
		projectID: cfg.ProjectID,
		logger:    logger.Named("llm"),
	}, nil
}

// GenerateResponse generates a chat completion response with usage stats.
// Set thinking=true to enable chain-of-thought reasoning, false to disable it.
// Uses chat_template_kwargs for vLLM/Nemotron/Qwen models that support it.
func (c *Client) GenerateResponse(
	ctx context.Context,
	prompt string,
	systemMessage string,
	temperature float64,
	thinking bool,
) (*GenerateResponseResult, error) {
	messages := []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: systemMessage},
		{Role: openai.ChatMessageRoleUser, Content: prompt},
	}

	c.logger.Debug("LLM request",
		zap.String("model", c.model),
		zap.Int("prompt_len", len(prompt)),
		zap.Float64("temperature", temperature),
		zap.Bool("thinking", thinking))

	start := time.Now()

	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: float32(temperature),
		// Control thinking/reasoning mode via chat_template_kwargs
		// Works with vLLM, Nemotron, Qwen3 and other models that support it
		ChatTemplateKwargs: map[string]any{
			"enable_thinking": thinking,
		},
	})
	if err != nil {
		c.logger.Error("LLM request failed",
			zap.Duration("elapsed", time.Since(start)),
			zap.Error(err))
		return nil, c.parseError(err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	content := resp.Choices[0].Message.Content
	elapsed := time.Since(start)

	c.logger.Info("LLM request completed",
		zap.Int("prompt_tokens", resp.Usage.PromptTokens),
		zap.Int("completion_tokens", resp.Usage.CompletionTokens),
		zap.Duration("elapsed", elapsed))

	return &GenerateResponseResult{
		Content:          content,
		PromptTokens:     resp.Usage.PromptTokens,
		CompletionTokens: resp.Usage.CompletionTokens,
		TotalTokens:      resp.Usage.TotalTokens,
	}, nil
}

// CreateEmbedding generates an embedding vector for the input text.
func (c *Client) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	if model == "" {
		model = "text-embedding-3-small" // Default
	}

	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(model),
		Input: []string{input},
	})
	if err != nil {
		return nil, fmt.Errorf("create embedding: %w", err)
	}

	if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("no embedding in response")
	}

	return resp.Data[0].Embedding, nil
}

// CreateEmbeddings generates embeddings for multiple inputs.
func (c *Client) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	if model == "" {
		model = "text-embedding-3-small"
	}

	resp, err := c.client.CreateEmbeddings(ctx, openai.EmbeddingRequest{
		Model: openai.EmbeddingModel(model),
		Input: inputs,
	})
	if err != nil {
		return nil, fmt.Errorf("create embeddings: %w", err)
	}

	embeddings := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		embeddings[i] = d.Embedding
	}

	return embeddings, nil
}

// GetModel returns the configured model name.
func (c *Client) GetModel() string {
	return c.model
}

// GetEndpoint returns the configured endpoint.
func (c *Client) GetEndpoint() string {
	return c.endpoint
}

// parseError categorizes OpenAI API errors using the structured Error type.
func (c *Client) parseError(err error) error {
	return ClassifyError(err)
}
