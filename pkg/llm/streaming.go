// Package llm provides OpenAI-compatible LLM client functionality.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"go.uber.org/zap"
)

// StreamEvent represents a streaming event from the LLM.
type StreamEvent struct {
	Type    StreamEventType `json:"type"`
	Content string          `json:"content,omitempty"`
	Data    any             `json:"data,omitempty"`
}

// StreamEventType defines types of streaming events.
type StreamEventType string

const (
	StreamEventText       StreamEventType = "text"
	StreamEventToolCall   StreamEventType = "tool_call"
	StreamEventToolResult StreamEventType = "tool_result"
	StreamEventDone       StreamEventType = "done"
	StreamEventError      StreamEventType = "error"
)

// ToolCall represents a tool call from the LLM.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function ToolCallFunc `json:"function"`
}

// ToolCallFunc represents a function call within a tool call.
type ToolCallFunc struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// StreamingClient extends the basic LLM client with streaming and tool capabilities.
type StreamingClient struct {
	*Client
	maxToolIterations int
}

// NewStreamingClient creates a new streaming-capable LLM client.
func NewStreamingClient(cfg *Config, logger *zap.Logger) (*StreamingClient, error) {
	client, err := NewClient(cfg, logger)
	if err != nil {
		return nil, err
	}

	return &StreamingClient{
		Client:            client,
		maxToolIterations: 10,
	}, nil
}

// StreamingRequest represents a request for streaming chat completion.
type StreamingRequest struct {
	Messages     []Message
	Tools        []ToolDefinition
	Temperature  float64
	SystemPrompt string
}

// Message represents a chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// Message role constants.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolExecutor defines the interface for executing tools.
type ToolExecutor interface {
	ExecuteTool(ctx context.Context, name string, arguments string) (string, error)
}

// StreamWithTools performs streaming chat completion with tool support.
// The eventChan receives events as they occur. The caller should consume
// events until the channel is closed or a done/error event is received.
func (c *StreamingClient) StreamWithTools(
	ctx context.Context,
	req *StreamingRequest,
	executor ToolExecutor,
	eventChan chan<- StreamEvent,
) error {
	messages := c.buildOpenAIMessages(req.Messages, req.SystemPrompt)
	tools := c.buildOpenAITools(req.Tools)

	temperature := float32(req.Temperature)
	if temperature == 0 {
		temperature = 0.7
	}

	for iteration := 0; iteration < c.maxToolIterations; iteration++ {
		content, toolCalls, err := c.streamIteration(ctx, messages, tools, temperature, eventChan)
		if err != nil {
			eventChan <- StreamEvent{Type: StreamEventError, Content: err.Error()}
			return err
		}

		// No tool calls means we're done
		if len(toolCalls) == 0 {
			eventChan <- StreamEvent{Type: StreamEventDone}
			return nil
		}

		// Add assistant message with tool calls
		assistantMsg := openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		}
		for _, tc := range toolCalls {
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
		messages = append(messages, assistantMsg)

		// Execute tools and add results
		for _, tc := range toolCalls {
			eventChan <- StreamEvent{
				Type: StreamEventToolCall,
				Data: tc,
			}

			result, execErr := executor.ExecuteTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf("Error executing tool: %s", execErr.Error())
			}

			eventChan <- StreamEvent{
				Type:    StreamEventToolResult,
				Content: result,
				Data:    map[string]string{"tool_call_id": tc.ID, "name": tc.Function.Name},
			}

			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return fmt.Errorf("exceeded maximum tool iterations (%d)", c.maxToolIterations)
}

// streamIteration performs a single streaming request and returns content and tool calls.
func (c *StreamingClient) streamIteration(
	ctx context.Context,
	messages []openai.ChatCompletionMessage,
	tools []openai.Tool,
	temperature float32,
	eventChan chan<- StreamEvent,
) (string, []ToolCall, error) {
	start := time.Now()

	c.logger.Debug("Starting stream iteration",
		zap.Int("message_count", len(messages)),
		zap.Int("tool_count", len(tools)))

	stream, err := c.client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model:       c.model,
		Messages:    messages,
		Tools:       tools,
		Temperature: temperature,
		Stream:      true,
	})
	if err != nil {
		c.logger.Error("Failed to create stream", zap.Error(err))
		return "", nil, c.parseError(err)
	}
	defer stream.Close()

	var contentBuilder strings.Builder
	toolCallsMap := make(map[int]*ToolCall)

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			c.logger.Error("Stream receive error", zap.Error(err))
			return "", nil, c.parseError(err)
		}

		if len(response.Choices) == 0 {
			continue
		}

		delta := response.Choices[0].Delta

		// Handle text content
		if delta.Content != "" {
			contentBuilder.WriteString(delta.Content)
			eventChan <- StreamEvent{
				Type:    StreamEventText,
				Content: delta.Content,
			}
		}

		// Handle tool calls (accumulated across chunks)
		for _, tc := range delta.ToolCalls {
			idx := 0
			if tc.Index != nil {
				idx = *tc.Index
			}

			if existing, exists := toolCallsMap[idx]; !exists {
				toolCallsMap[idx] = &ToolCall{
					ID:   tc.ID,
					Type: string(tc.Type),
					Function: ToolCallFunc{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
			} else {
				// Accumulate arguments across chunks
				existing.Function.Arguments += tc.Function.Arguments
			}
		}
	}

	// Convert content to string
	content := contentBuilder.String()

	// If no native tool calls, try parsing text-based tool calls
	if len(toolCallsMap) == 0 && content != "" {
		parsedToolCalls := c.parseTextToolCalls(content)
		if len(parsedToolCalls) > 0 {
			// Clean the content to remove tool call markup
			content = c.cleanModelOutput(content)
			for i, tc := range parsedToolCalls {
				toolCallsMap[i] = &tc
			}
		}
	}

	// Convert map to slice
	var toolCalls []ToolCall
	for i := 0; i < len(toolCallsMap); i++ {
		if tc, ok := toolCallsMap[i]; ok {
			toolCalls = append(toolCalls, *tc)
		}
	}

	c.logger.Info("Stream iteration completed",
		zap.Duration("elapsed", time.Since(start)),
		zap.Int("content_length", len(content)),
		zap.Int("tool_calls", len(toolCalls)))

	return content, toolCalls, nil
}

// parseTextToolCalls parses tool calls from text output (for non-native tool calling models).
func (c *StreamingClient) parseTextToolCalls(content string) []ToolCall {
	var toolCalls []ToolCall

	// XML format: <tool_call>{"name": "...", "arguments": {...}}</tool_call>
	toolCallRegex := regexp.MustCompile(`<tool_call>\s*(\{[\s\S]*?\})\s*</tool_call>`)
	matches := toolCallRegex.FindAllStringSubmatch(content, -1)

	for i, match := range matches {
		if len(match) < 2 {
			continue
		}

		var toolCallJSON struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}

		if err := json.Unmarshal([]byte(match[1]), &toolCallJSON); err != nil {
			c.logger.Debug("Failed to parse text tool call", zap.Error(err))
			continue
		}

		argsJSON, err := json.Marshal(toolCallJSON.Arguments)
		if err != nil {
			continue
		}

		toolCalls = append(toolCalls, ToolCall{
			ID:   fmt.Sprintf("text_tool_%d", i),
			Type: "function",
			Function: ToolCallFunc{
				Name:      toolCallJSON.Name,
				Arguments: string(argsJSON),
			},
		})
	}

	return toolCalls
}

// cleanModelOutput removes tool call markup and thinking blocks from model output.
func (c *StreamingClient) cleanModelOutput(content string) string {
	// Remove <think>...</think> blocks
	thinkRegex := regexp.MustCompile(`<think>[\s\S]*?</think>`)
	content = thinkRegex.ReplaceAllString(content, "")

	// Remove tool call blocks
	toolCallRegex := regexp.MustCompile(`<tool_call>[\s\S]*?</tool_call>`)
	content = toolCallRegex.ReplaceAllString(content, "")

	// Collapse multiple newlines
	multiNewline := regexp.MustCompile(`\n{3,}`)
	content = multiNewline.ReplaceAllString(content, "\n\n")

	return strings.TrimSpace(content)
}

// buildOpenAIMessages converts our message format to OpenAI format.
func (c *StreamingClient) buildOpenAIMessages(messages []Message, systemPrompt string) []openai.ChatCompletionMessage {
	var result []openai.ChatCompletionMessage

	// Add system message if provided
	if systemPrompt != "" {
		result = append(result, openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemPrompt,
		})
	}

	for _, msg := range messages {
		oaiMsg := openai.ChatCompletionMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}

		for _, tc := range msg.ToolCalls {
			oaiMsg.ToolCalls = append(oaiMsg.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}

		result = append(result, oaiMsg)
	}

	return result
}

// buildOpenAITools converts our tool definitions to OpenAI format.
func (c *StreamingClient) buildOpenAITools(tools []ToolDefinition) []openai.Tool {
	if len(tools) == 0 {
		return nil
	}

	result := make([]openai.Tool, len(tools))
	for i, def := range tools {
		paramsJSON, _ := json.Marshal(def.Parameters)
		result[i] = openai.Tool{
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        def.Name,
				Description: def.Description,
				Parameters:  json.RawMessage(paramsJSON),
			},
		}
	}

	return result
}

// GenerateWithTools performs a non-streaming chat completion with tool support.
// Useful for question answering where streaming is not needed.
func (c *StreamingClient) GenerateWithTools(
	ctx context.Context,
	req *StreamingRequest,
	executor ToolExecutor,
) (string, error) {
	messages := c.buildOpenAIMessages(req.Messages, req.SystemPrompt)
	tools := c.buildOpenAITools(req.Tools)

	temperature := float32(req.Temperature)
	if temperature == 0 {
		temperature = 0.3 // Lower temp for deterministic tool use
	}

	for iteration := 0; iteration < c.maxToolIterations; iteration++ {
		c.logger.Debug("Non-streaming iteration",
			zap.Int("iteration", iteration),
			zap.Int("message_count", len(messages)))

		resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Tools:       tools,
			Temperature: temperature,
		})
		if err != nil {
			return "", c.parseError(err)
		}

		if len(resp.Choices) == 0 {
			return "", fmt.Errorf("no choices in response")
		}

		choice := resp.Choices[0]
		content := choice.Message.Content

		// Check for text-based tool calls if no native ones
		var toolCalls []ToolCall
		if len(choice.Message.ToolCalls) == 0 && content != "" {
			toolCalls = c.parseTextToolCalls(content)
			if len(toolCalls) > 0 {
				content = c.cleanModelOutput(content)
			}
		} else {
			for _, tc := range choice.Message.ToolCalls {
				toolCalls = append(toolCalls, ToolCall{
					ID:   tc.ID,
					Type: string(tc.Type),
					Function: ToolCallFunc{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}

		// No tool calls means we're done
		if len(toolCalls) == 0 {
			return content, nil
		}

		// Add assistant message with tool calls
		assistantMsg := openai.ChatCompletionMessage{
			Role:    openai.ChatMessageRoleAssistant,
			Content: content,
		}
		for _, tc := range toolCalls {
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls, openai.ToolCall{
				ID:   tc.ID,
				Type: openai.ToolTypeFunction,
				Function: openai.FunctionCall{
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				},
			})
		}
		messages = append(messages, assistantMsg)

		// Execute tools and add results
		for _, tc := range toolCalls {
			result, execErr := executor.ExecuteTool(ctx, tc.Function.Name, tc.Function.Arguments)
			if execErr != nil {
				result = fmt.Sprintf("Error executing tool: %s", execErr.Error())
			}

			messages = append(messages, openai.ChatCompletionMessage{
				Role:       openai.ChatMessageRoleTool,
				Content:    result,
				ToolCallID: tc.ID,
			})
		}
	}

	return "", fmt.Errorf("exceeded maximum tool iterations (%d)", c.maxToolIterations)
}
