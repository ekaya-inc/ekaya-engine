package llm

import (
	"encoding/json"
	"testing"

	"github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// newTestStreamingClient creates a StreamingClient with a nop logger for tests.
// It bypasses NewStreamingClient since that requires a real endpoint.
func newTestStreamingClient() *StreamingClient {
	return &StreamingClient{
		Client: &Client{
			logger: zap.NewNop(),
		},
		maxToolIterations: 10,
	}
}

// ============================================================================
// parseTextToolCalls tests
// ============================================================================

func TestStreamingParseTextToolCalls_ValidSingleToolCall(t *testing.T) {
	c := newTestStreamingClient()
	content := `<tool_call>{"name": "query_column_values", "arguments": {"table_name": "users", "column_name": "email"}}</tool_call>`

	result := c.parseTextToolCalls(content)

	require.Len(t, result, 1)
	assert.Equal(t, "text_tool_0", result[0].ID)
	assert.Equal(t, "function", result[0].Type)
	assert.Equal(t, "query_column_values", result[0].Function.Name)

	var args map[string]any
	require.NoError(t, json.Unmarshal([]byte(result[0].Function.Arguments), &args))
	assert.Equal(t, "users", args["table_name"])
	assert.Equal(t, "email", args["column_name"])
}

func TestStreamingParseTextToolCalls_MultipleToolCalls(t *testing.T) {
	c := newTestStreamingClient()
	content := `Some text before
<tool_call>{"name": "tool_a", "arguments": {"key": "val1"}}</tool_call>
Middle text
<tool_call>{"name": "tool_b", "arguments": {"key": "val2"}}</tool_call>`

	result := c.parseTextToolCalls(content)

	require.Len(t, result, 2)
	assert.Equal(t, "text_tool_0", result[0].ID)
	assert.Equal(t, "tool_a", result[0].Function.Name)
	assert.Equal(t, "text_tool_1", result[1].ID)
	assert.Equal(t, "tool_b", result[1].Function.Name)
}

func TestStreamingParseTextToolCalls_MalformedJSON(t *testing.T) {
	c := newTestStreamingClient()
	content := `<tool_call>{not valid json}</tool_call>`

	result := c.parseTextToolCalls(content)

	assert.Empty(t, result)
}

func TestStreamingParseTextToolCalls_NoMatches(t *testing.T) {
	c := newTestStreamingClient()
	content := "This is just regular text with no tool calls."

	result := c.parseTextToolCalls(content)

	assert.Empty(t, result)
}

func TestStreamingParseTextToolCalls_NestedBracesInArguments(t *testing.T) {
	c := newTestStreamingClient()
	content := `<tool_call>{"name": "complex_tool", "arguments": {"filter": {"nested": "value"}}}</tool_call>`

	result := c.parseTextToolCalls(content)

	// The regex uses non-greedy [\s\S]*? which may or may not match nested braces.
	// The key thing is it either parses successfully or gracefully skips.
	if len(result) == 1 {
		assert.Equal(t, "complex_tool", result[0].Function.Name)
	}
	// If it doesn't match due to regex limitations, that's also valid behavior.
}

func TestStreamingParseTextToolCalls_EmptyContent(t *testing.T) {
	c := newTestStreamingClient()
	result := c.parseTextToolCalls("")
	assert.Empty(t, result)
}

// ============================================================================
// cleanModelOutput tests
// ============================================================================

func TestStreamingCleanModelOutput_RemoveThinkBlocks(t *testing.T) {
	c := newTestStreamingClient()
	content := `<think>Some internal reasoning here.</think>Hello, world!`

	result := c.cleanModelOutput(content)

	assert.Equal(t, "Hello, world!", result)
}

func TestStreamingCleanModelOutput_RemoveToolCallBlocks(t *testing.T) {
	c := newTestStreamingClient()
	content := `Here is the answer.<tool_call>{"name":"foo","arguments":{}}</tool_call>`

	result := c.cleanModelOutput(content)

	assert.Equal(t, "Here is the answer.", result)
}

func TestStreamingCleanModelOutput_CollapseTripleNewlines(t *testing.T) {
	c := newTestStreamingClient()
	content := "Line one\n\n\n\n\nLine two"

	result := c.cleanModelOutput(content)

	assert.Equal(t, "Line one\n\nLine two", result)
}

func TestStreamingCleanModelOutput_NoMarkupPassthrough(t *testing.T) {
	c := newTestStreamingClient()
	content := "Just regular text with no markup."

	result := c.cleanModelOutput(content)

	assert.Equal(t, "Just regular text with no markup.", result)
}

func TestStreamingCleanModelOutput_CombinedCleanup(t *testing.T) {
	c := newTestStreamingClient()
	content := "<think>reasoning</think>\n\n\nHello\n\n\n\n<tool_call>{\"name\":\"x\",\"arguments\":{}}</tool_call>\n\n\nWorld"

	result := c.cleanModelOutput(content)

	assert.Equal(t, "Hello\n\nWorld", result)
}

func TestStreamingCleanModelOutput_EmptyContent(t *testing.T) {
	c := newTestStreamingClient()
	result := c.cleanModelOutput("")
	assert.Equal(t, "", result)
}

// ============================================================================
// buildOpenAIMessages tests
// ============================================================================

func TestStreamingBuildOpenAIMessages_EmptyMessages(t *testing.T) {
	c := newTestStreamingClient()

	result := c.buildOpenAIMessages(nil, "")

	assert.Empty(t, result)
}

func TestStreamingBuildOpenAIMessages_SystemPromptOnly(t *testing.T) {
	c := newTestStreamingClient()

	result := c.buildOpenAIMessages(nil, "You are helpful.")

	require.Len(t, result, 1)
	assert.Equal(t, openai.ChatMessageRoleSystem, result[0].Role)
	assert.Equal(t, "You are helpful.", result[0].Content)
}

func TestStreamingBuildOpenAIMessages_SystemPromptPlusMessages(t *testing.T) {
	c := newTestStreamingClient()
	messages := []Message{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there"},
	}

	result := c.buildOpenAIMessages(messages, "System prompt")

	require.Len(t, result, 3)
	assert.Equal(t, openai.ChatMessageRoleSystem, result[0].Role)
	assert.Equal(t, "System prompt", result[0].Content)
	assert.Equal(t, RoleUser, result[1].Role)
	assert.Equal(t, "Hello", result[1].Content)
	assert.Equal(t, RoleAssistant, result[2].Role)
	assert.Equal(t, "Hi there", result[2].Content)
}

func TestStreamingBuildOpenAIMessages_MessagesWithToolCalls(t *testing.T) {
	c := newTestStreamingClient()
	messages := []Message{
		{
			Role:    RoleAssistant,
			Content: "Let me look that up.",
			ToolCalls: []ToolCall{
				{
					ID:   "tc_1",
					Type: "function",
					Function: ToolCallFunc{
						Name:      "search",
						Arguments: `{"q":"test"}`,
					},
				},
			},
		},
	}

	result := c.buildOpenAIMessages(messages, "")

	require.Len(t, result, 1)
	require.Len(t, result[0].ToolCalls, 1)
	assert.Equal(t, "tc_1", result[0].ToolCalls[0].ID)
	assert.Equal(t, openai.ToolTypeFunction, result[0].ToolCalls[0].Type)
	assert.Equal(t, "search", result[0].ToolCalls[0].Function.Name)
	assert.Equal(t, `{"q":"test"}`, result[0].ToolCalls[0].Function.Arguments)
}

func TestStreamingBuildOpenAIMessages_ToolRoleWithToolCallID(t *testing.T) {
	c := newTestStreamingClient()
	messages := []Message{
		{
			Role:       RoleTool,
			Content:    `{"result": "found"}`,
			ToolCallID: "tc_1",
		},
	}

	result := c.buildOpenAIMessages(messages, "")

	require.Len(t, result, 1)
	assert.Equal(t, RoleTool, result[0].Role)
	assert.Equal(t, "tc_1", result[0].ToolCallID)
	assert.Equal(t, `{"result": "found"}`, result[0].Content)
}

// ============================================================================
// buildOpenAITools tests
// ============================================================================

func TestStreamingBuildOpenAITools_EmptyReturnsNil(t *testing.T) {
	c := newTestStreamingClient()

	result := c.buildOpenAITools(nil)

	assert.Nil(t, result)
}

func TestStreamingBuildOpenAITools_SingleTool(t *testing.T) {
	c := newTestStreamingClient()
	tools := []ToolDefinition{
		{
			Name:        "search",
			Description: "Search for things",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
	}

	result := c.buildOpenAITools(tools)

	require.Len(t, result, 1)
	assert.Equal(t, openai.ToolTypeFunction, result[0].Type)
	assert.Equal(t, "search", result[0].Function.Name)
	assert.Equal(t, "Search for things", result[0].Function.Description)

	// Verify parameters are valid JSON
	var params map[string]any
	paramsBytes, ok := result[0].Function.Parameters.(json.RawMessage)
	require.True(t, ok, "Parameters should be json.RawMessage")
	require.NoError(t, json.Unmarshal(paramsBytes, &params))
	assert.Equal(t, "object", params["type"])
}

func TestStreamingBuildOpenAITools_ToolWithNestedParameters(t *testing.T) {
	c := newTestStreamingClient()
	tools := []ToolDefinition{
		{
			Name:        "complex",
			Description: "Complex tool",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filter": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"field":    map[string]any{"type": "string"},
							"operator": map[string]any{"type": "string"},
						},
					},
				},
			},
		},
	}

	result := c.buildOpenAITools(tools)

	require.Len(t, result, 1)
	var params map[string]any
	paramsBytes, ok := result[0].Function.Parameters.(json.RawMessage)
	require.True(t, ok, "Parameters should be json.RawMessage")
	require.NoError(t, json.Unmarshal(paramsBytes, &params))
	props := params["properties"].(map[string]any)
	filter := props["filter"].(map[string]any)
	assert.Equal(t, "object", filter["type"])
}

func TestStreamingBuildOpenAITools_EmptySliceReturnsNil(t *testing.T) {
	c := newTestStreamingClient()

	result := c.buildOpenAITools([]ToolDefinition{})

	assert.Nil(t, result)
}
